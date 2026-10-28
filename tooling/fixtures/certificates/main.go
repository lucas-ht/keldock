// Generate a private local test CA and server certificate; never install global trust.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "", "private test certificate specification")
	flag.Parse()
	data, err := os.ReadFile(*path)
	if err != nil {
		return err
	}
	var spec struct {
		Directory string   `json:"directory"`
		DNS       []string `json:"dns"`
		IPs       []string `json:"ips"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return err
	}
	if err := os.MkdirAll(spec.Directory, 0o700); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return err
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Keldock isolated fixture CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	root, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		return err
	}
	leaf := &x509.Certificate{SerialNumber: new(big.Int).Add(serial, big.NewInt(1)), Subject: pkix.Name{CommonName: "Keldock fixture"}, DNSNames: spec.DNS, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	for _, value := range spec.IPs {
		ip := net.ParseIP(value)
		if ip == nil {
			return fmt.Errorf("invalid fixture address")
		}
		leaf.IPAddresses = append(leaf.IPAddresses, ip)
	}
	certificate, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
	if err != nil {
		return err
	}
	private, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		return err
	}
	for name, value := range map[string]*pem.Block{"ca.crt": {Type: "CERTIFICATE", Bytes: root}, "public.crt": {Type: "CERTIFICATE", Bytes: certificate}, "private.key": {Type: "PRIVATE KEY", Bytes: private}} {
		if err := os.WriteFile(filepath.Join(spec.Directory, name), pem.EncodeToMemory(value), 0o600); err != nil {
			return err
		}
	}
	fmt.Println("Private fixture certificate files prepared; system trust unchanged")
	return nil
}
