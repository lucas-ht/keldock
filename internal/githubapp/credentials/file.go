// Package credentials keeps installation-owned GitHub App secrets outside product state.
package credentials

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
)

// App is a private installation document, never an API response or application input.
type App struct {
	ID            int64  `json:"appId"`
	ClientID      string `json:"clientId"`
	Slug          string `json:"slug"`
	PrivateKey    string `json:"privateKey"`
	WebhookSecret string `json:"webhookSecret"`
}

var slugPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,99}$`)

// Key validates the bounded credential document and parses GitHub's RSA key formats.
func (app App) Key() (*rsa.PrivateKey, error) {
	if app.ID <= 0 || !slugPattern.MatchString(app.Slug) || len(app.ClientID) > 100 || len(app.WebhookSecret) < 32 || len(app.WebhookSecret) > 256 || len(app.PrivateKey) > 16384 {
		return nil, fmt.Errorf("invalid GitHub App credential document")
	}
	block, rest := pem.Decode([]byte(app.PrivateKey))
	if block == nil || len(rest) != 0 {
		return nil, fmt.Errorf("one RSA private key is required")
	}
	var key *rsa.PrivateKey
	var err error
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		key, _ = parsed.(*rsa.PrivateKey)
	}
	if err != nil || key == nil || key.N.BitLen() < 2048 || key.N.BitLen() > 8192 || key.Validate() != nil {
		return nil, fmt.Errorf("valid RSA private key between 2048 and 8192 bits required")
	}
	return key, nil
}

// File is an operator-selected, owner-only credential path. A missing file means unconfigured.
type File struct{ Path string }

// Read rejects symlinks, public permissions and oversized input before decoding.
func (file File) Read() (App, error) {
	var app App
	info, err := os.Lstat(file.Path)
	if err != nil {
		return app, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 32768 {
		return app, fmt.Errorf("GitHub App file requires owner-only regular-file permissions")
	}
	data, err := os.ReadFile(file.Path)
	if err != nil {
		return app, err
	}
	if err := v1.DecodeJSON(data, &app); err != nil {
		return app, fmt.Errorf("invalid GitHub App file")
	}
	_, err = app.Key()
	return app, err
}

// Write replaces credentials atomically inside a pre-existing private installation directory.
// Rotation keeps the same App identity; changing Apps requires explicit removal first.
func (file File) Write(app App) error {
	if _, err := app.Key(); err != nil {
		return err
	}
	previous, err := file.Read()
	if err == nil && previous.ID != app.ID {
		return fmt.Errorf("remove the configured GitHub App before replacing its identity")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return writePrivate(file.Path, app)
}

// Remove removes only this installation credential file, leaving GitHub's App untouched.
func (file File) Remove() error {
	if _, err := file.Read(); err != nil {
		return err
	}
	if err := os.Remove(file.Path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(file.Path))
}
