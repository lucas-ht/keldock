package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lucas-ht/keldock/internal/artifacts"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

func publish(image string) error {
	repository, digest, err := artifacts.SplitReference(image)
	if err != nil || repository != "keldock-m1-registry:5000/keldock/fixture" {
		return fmt.Errorf("refusing image copy outside the disposable fixture repositories")
	}
	data, err := os.ReadFile(".local/registry-auth/state.json")
	if err != nil {
		return err
	}
	var current state
	if err := json.Unmarshal(data, &current); err != nil {
		return err
	}
	source, err := remote.NewRepository(strings.Replace(repository, "keldock-m1-registry:5000", "localhost:5010", 1))
	if err != nil {
		return err
	}
	source.PlainHTTP = true
	target, err := remote.NewRepository("localhost:5011/keldock/fixture")
	if err != nil {
		return err
	}
	target.PlainHTTP = true
	target.Client = &auth.Client{Cache: auth.NewCache(), Credential: auth.StaticCredential("localhost:5011", auth.Credential{Username: current.Publisher.Username, Password: current.Publisher.Password})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	descriptor, err := oras.Copy(ctx, source, digest, target, "private-fixture", oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("publish private fixture image: %w", err)
	}
	fmt.Println("keldock-m1-private-registry:5000/keldock/fixture@" + descriptor.Digest.String())
	return nil
}
