// Package provenance verifies prebuilt image evidence independently of CI authentication.
package provenance

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const payloadType = "application/vnd.in-toto+json"

// Signature identifies an approved signing key and a base64 DSSE signature.
type Signature struct {
	KeyID string `json:"keyid"`
	Value string `json:"sig"`
}

// Envelope is a DSSE JSON envelope. The verified payload bytes are parsed only once.
type Envelope struct {
	PayloadType string      `json:"payloadType"`
	Payload     string      `json:"payload"`
	Signatures  []Signature `json:"signatures"`
}

func pae(body []byte) []byte {
	return fmt.Appendf(nil, "DSSEv1 %d %s %d %s", len(payloadType), payloadType, len(body), body)
}

// Sign creates an envelope in a separate CI provenance step, not in the deployment Action.
func Sign(body []byte, keyID string, key ed25519.PrivateKey) (Envelope, error) {
	if len(key) != ed25519.PrivateKeySize || keyID == "" {
		return Envelope{}, fmt.Errorf("valid signing key and key ID required")
	}
	return Envelope{PayloadType: payloadType, Payload: base64.StdEncoding.EncodeToString(body),
		Signatures: []Signature{{KeyID: keyID, Value: base64.StdEncoding.EncodeToString(ed25519.Sign(key, pae(body)))}}}, nil
}

// Verify checks approved keys and returns exactly the signed payload bytes.
// Envelope extensions are ignored as specified by DSSE; payload contracts are validated later.
func Verify(data []byte, keys map[string]ed25519.PublicKey) ([]byte, error) {
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode DSSE: %w", err)
	}
	if envelope.PayloadType != payloadType || len(envelope.Signatures) < 1 || len(envelope.Signatures) > 8 {
		return nil, fmt.Errorf("unsupported DSSE payload type or signature count")
	}
	body, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil || len(body) > 192*1024 {
		return nil, fmt.Errorf("invalid or oversized DSSE payload")
	}
	for _, signature := range envelope.Signatures {
		key := keys[signature.KeyID]
		value, err := base64.StdEncoding.DecodeString(signature.Value)
		if err == nil && len(key) == ed25519.PublicKeySize && ed25519.Verify(key, pae(body), value) {
			return body, nil
		}
	}
	return nil, fmt.Errorf("no signature from an approved provenance key")
}
