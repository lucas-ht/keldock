package recovery

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func checksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func inventory(directory string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot contains a symlink")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("snapshot contains a nonregular file")
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		if relative == "manifest.json" {
			return nil
		}
		hash, err := checksum(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = hash
		if len(files) > 50000 {
			return fmt.Errorf("snapshot object limit exceeded")
		}
		return nil
	})
	return files, err
}

func privateFile(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 1024*1024 {
		return fmt.Errorf("snapshot input requires a bounded owner-only regular file")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o600)
}

// Verify checks every member and rejects missing, changed, extra or unsafe snapshot data.
func Verify(directory string) (Manifest, error) {
	var manifest Manifest
	info, err := os.Lstat(directory)
	if err != nil {
		return manifest, err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return manifest, fmt.Errorf("snapshot directory requires owner-only permissions")
	}
	data, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return manifest, err
	}
	if len(data) > 16*1024*1024 || json.Unmarshal(data, &manifest) != nil || manifest.Version != 1 || manifest.ID == "" || len(manifest.Files) < 1 {
		return manifest, fmt.Errorf("invalid snapshot manifest")
	}
	actual, err := inventory(directory)
	if err != nil {
		return manifest, err
	}
	if len(actual) != len(manifest.Files) {
		return manifest, fmt.Errorf("snapshot member inventory changed")
	}
	for name, hash := range manifest.Files {
		if !filepath.IsLocal(name) || strings.Contains(name, "\\") || len(hash) != 64 || actual[name] != hash {
			return manifest, fmt.Errorf("snapshot checksum mismatch")
		}
	}
	if manifest.Files["database.dump"] == "" {
		return manifest, fmt.Errorf("snapshot database is absent")
	}
	return manifest, nil
}
