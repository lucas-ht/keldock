package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyRejectsChangedMissingAndExtraMembers(t *testing.T) {
	for _, name := range []string{"valid", "changed", "missing", "extra", "symlink", "public-directory"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.Chmod(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			dump := filepath.Join(directory, "database.dump")
			if err := os.WriteFile(dump, []byte("synthetic private snapshot"), 0o600); err != nil {
				t.Fatal(err)
			}
			files, err := inventory(directory)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(Manifest{Version: 1, ID: "snapshot-fixture", Organization: "fixture", Files: files})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "manifest.json"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "changed":
				err = os.WriteFile(dump, []byte("changed"), 0o600)
			case "missing":
				err = os.Remove(dump)
			case "extra":
				err = os.WriteFile(filepath.Join(directory, "extra"), []byte("unrecorded"), 0o600)
			case "symlink":
				err = os.Symlink(dump, filepath.Join(directory, "linked"))
			case "public-directory":
				err = os.Chmod(directory, 0o755)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = Verify(directory)
			if (err == nil) != (name == "valid") {
				t.Fatalf("snapshot verification: %v", err)
			}
		})
	}
}
