package v1alpha1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

// DecodeJSON rejects unknown fields, duplicate keys and trailing values.
func DecodeJSON(data []byte, target any) error {
	check := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueKeys(check); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err := check.Token(); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode contract: %w", err)
	}
	return nil
}

func uniqueKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	keys := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return fmt.Errorf("duplicate or invalid object key %q", key)
			}
			keys[name] = true
		}
		if err := uniqueKeys(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

// DecodeApplication reads one strict YAML definition, including duplicate-key checks.
func DecodeApplication(data []byte) (Application, error) {
	var app Application
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&app); err != nil {
		return app, fmt.Errorf("decode application: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return app, fmt.Errorf("expected exactly one YAML document")
	}
	return app, app.Validate()
}
