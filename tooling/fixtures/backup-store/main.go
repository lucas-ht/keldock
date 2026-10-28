// This local-only fixture provisions scoped identities in its disposable MinIO server.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type configuration struct {
	URL          string `json:"url"`
	CA           string `json:"ca"`
	Bucket       string `json:"bucket"`
	Prefix       string `json:"prefix"`
	RootAccess   string `json:"rootAccess"`
	RootSecret   string `json:"rootSecret"`
	WriterAccess string `json:"writerAccess"`
	WriterSecret string `json:"writerSecret"`
	ReaderAccess string `json:"readerAccess"`
	ReaderSecret string `json:"readerSecret"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Disposable backup fixture setup failed:", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", "", "private local fixture configuration")
	flag.Parse()
	info, err := os.Lstat(*path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("private fixture configuration required")
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		return err
	}
	var config configuration
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	endpoint, err := url.Parse(config.URL)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() != "127.0.0.1" {
		return fmt.Errorf("refusing MinIO administration outside loopback HTTPS fixture")
	}
	ca, err := os.ReadFile(config.CA)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return fmt.Errorf("invalid fixture CA")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}, ResponseHeaderTimeout: 10 * time.Second}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	admin, err := madmin.NewWithOptions(endpoint.Host, &madmin.Options{Creds: credentials.NewStaticV4(config.RootAccess, config.RootSecret, ""), Secure: true})
	if err != nil {
		return err
	}
	admin.SetCustomTransport(transport)
	client, err := minio.New(endpoint.Host, &minio.Options{Creds: credentials.NewStaticV4(config.RootAccess, config.RootSecret, ""), Secure: true, Transport: transport, Region: "us-east-1"})
	if err != nil {
		return err
	}
	if exists, err := client.BucketExists(ctx, config.Bucket); err != nil {
		return err
	} else if !exists {
		if err := client.MakeBucket(ctx, config.Bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
			return err
		}
	}
	for _, identity := range []struct {
		Name, Key, Secret string
		Write             bool
	}{{"keldock-writer", config.WriterAccess, config.WriterSecret, true}, {"keldock-reader", config.ReaderAccess, config.ReaderSecret, false}} {
		identity.Name += fmt.Sprintf("-%x", sha256.Sum256([]byte(config.Prefix)))[:17]
		actions := []string{"s3:GetObject"}
		if identity.Write {
			actions = append(actions, "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts")
		}
		policy := map[string]any{"Version": "2012-10-17", "Statement": []any{
			map[string]any{"Effect": "Allow", "Action": []string{"s3:GetBucketLocation"}, "Resource": []string{"arn:aws:s3:::" + config.Bucket}},
			map[string]any{"Effect": "Allow", "Action": []string{"s3:ListBucket"}, "Resource": []string{"arn:aws:s3:::" + config.Bucket}, "Condition": map[string]any{"StringLike": map[string]any{"s3:prefix": []string{config.Prefix, config.Prefix + "/*"}}}},
			map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{"arn:aws:s3:::" + config.Bucket + "/" + config.Prefix + "/*"}},
		}}
		data, err := json.Marshal(policy)
		if err != nil {
			return err
		}
		if err := admin.AddUser(ctx, identity.Key, identity.Secret); err != nil {
			return err
		}
		if err := admin.AddCannedPolicy(ctx, identity.Name, data); err != nil {
			return err
		}
		if err := admin.SetPolicy(ctx, identity.Name, identity.Key, false); err != nil {
			return err
		}
	}
	fmt.Println("Disposable backup bucket and distinct prefix-scoped writer/restore identities configured")
	return nil
}
