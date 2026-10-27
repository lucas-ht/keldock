package credentials_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucas-ht/keldock/internal/githubapp/credentials"
)

func TestPrivateFileRotationAndOwnership(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	app := credentials.App{ID: 42, Slug: "keldock", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})), WebhookSecret: strings.Repeat("a", 32)}
	file := credentials.File{Path: filepath.Join(directory, "app.json")}
	if err := file.Write(app); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file.Path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("credential file is not private")
	}
	app.WebhookSecret = strings.Repeat("b", 32)
	if err := file.Write(app); err != nil {
		t.Fatal(err)
	}
	current, err := file.Read()
	if err != nil || current.WebhookSecret != app.WebhookSecret {
		t.Fatal("atomic rotation lost the credential")
	}
	app.ID = 43
	if err := file.Write(app); err == nil {
		t.Fatal("App identity silently replaced")
	}
	if err := os.Chmod(file.Path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Read(); err == nil {
		t.Fatal("public credential permissions accepted")
	}
	link := credentials.File{Path: filepath.Join(directory, "link.json")}
	if err := os.Symlink(file.Path, link.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := link.Read(); err == nil {
		t.Fatal("symlink credential accepted")
	}
}
