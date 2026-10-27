package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"

	v1 "github.com/lucas-ht/keldock/internal/contracts/v1alpha1"
	"github.com/lucas-ht/keldock/internal/provenance"
)

func provenanceCommand(args []string) error {
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	privatePath := flags.String("private-key", "", "PKCS8 Ed25519 key file")
	publicPath := flags.String("public-key", "", "public key JSON file for repository policy")
	keyID := flags.String("key-id", "build-key", "operator-approved signing key identity")
	inputPath := flags.String("input", "", "complete unsigned release JSON from user CI")
	outputPath := flags.String("output", "", "signed release JSON for submission")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *privatePath == "" || *keyID == "" {
		return fmt.Errorf("private-key and key-id required")
	}
	if args[0] == "keygen" {
		if *publicPath == "" {
			return fmt.Errorf("public-key output path required")
		}
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		encoded, err := x509.MarshalPKCS8PrivateKey(private)
		if err != nil {
			return err
		}
		if err := writeNew(*privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0o600); err != nil {
			return err
		}
		data, err := json.Marshal(map[string]string{*keyID: base64.StdEncoding.EncodeToString(public)})
		if err != nil {
			return err
		}
		if err := writeNew(*publicPath, data, 0o644); err != nil {
			return err
		}
		fmt.Println("Signing key created; copy only the public-key policy to the control plane")
		return nil
	}
	if *inputPath == "" || *outputPath == "" {
		return fmt.Errorf("input and output paths required")
	}
	data, err := os.ReadFile(*inputPath)
	if err != nil {
		return err
	}
	var input v1.SubmitRelease
	if err := v1.DecodeJSON(data, &input); err != nil {
		return err
	}
	// Structural admission is checked before signing; this temporary field is never submitted.
	input.Provenance = json.RawMessage(`{}`)
	if err := input.Validate(); err != nil {
		return err
	}
	info, err := os.Stat(*privatePath)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("signing key requires owner-only permissions")
	}
	encoded, err := os.ReadFile(*privatePath)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(encoded)
	if block == nil || block.Type != "PRIVATE KEY" {
		return fmt.Errorf("PKCS8 private key required")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("invalid signing key")
	}
	private, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return fmt.Errorf("Ed25519 signing key required")
	}
	statement, err := provenance.NewStatement(input)
	if err != nil {
		return err
	}
	body, err := json.Marshal(statement)
	if err != nil {
		return err
	}
	envelope, err := provenance.Sign(body, *keyID, private)
	if err != nil {
		return err
	}
	input.Provenance, err = json.Marshal(envelope)
	if err != nil {
		return err
	}
	output, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return err
	}
	if err := writeNew(*outputPath, append(output, '\n'), 0o600); err != nil {
		return err
	}
	fmt.Println("Signed complete prebuilt release; no build, test or image publication was performed")
	return nil
}

func writeNew(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
