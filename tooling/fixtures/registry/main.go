// Registry provisions only synthetic credentials for the disposable local acceptance target.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
)

type credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type state struct {
	Publisher credential `json:"publisher"`
	Reader    credential `json:"reader"`
}

func run() error {
	directory := filepath.Join(".local", "registry-auth")
	if err := os.MkdirAll(filepath.Join(directory, "server"), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return err
	}
	path := filepath.Join(directory, "state.json")
	var current state
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		generate := func() (string, error) {
			value := make([]byte, 32)
			if _, err := rand.Read(value); err != nil {
				return "", err
			}
			return hex.EncodeToString(value), nil
		}
		current.Publisher.Username = "publisher"
		current.Reader.Username = "reader"
		current.Publisher.Password, err = generate()
		if err != nil {
			return err
		}
		current.Reader.Password, err = generate()
		if err != nil {
			return err
		}
		data, err = json.Marshal(current)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if err := json.Unmarshal(data, &current); err != nil {
		return err
	}
	var passwordFile []byte
	for _, user := range []credential{current.Publisher, current.Reader} {
		if user.Username == "" || len(user.Password) != 64 {
			return fmt.Errorf("invalid disposable registry credential state")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		passwordFile = append(passwordFile, []byte(user.Username+":"+string(hash)+"\n")...)
	}
	if err := os.WriteFile(filepath.Join(directory, "server", "htpasswd"), passwordFile, 0o600); err != nil {
		return err
	}
	fmt.Println("Disposable registry authentication fixture prepared; credentials remain private")
	return nil
}

func main() {
	var err error
	if len(os.Args) == 3 && os.Args[1] == "publish" {
		err = publish(os.Args[2])
	} else {
		err = run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
