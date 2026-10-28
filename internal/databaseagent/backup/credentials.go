package backup

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lucas-ht/keldock/internal/database"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (d *Service) secret(ctx context.Context, ref database.SecretReference) (map[string]string, error) {
	object, err := d.Client.Resource(resources["Secret"]).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil || string(object.GetUID()) != ref.UID {
		return nil, fmt.Errorf("supplied backup identity changed or is unavailable")
	}
	encoded, _, err := unstructured.NestedStringMap(object.Object, "data")
	if err != nil {
		return nil, fmt.Errorf("invalid backup identity document")
	}
	result := map[string]string{}
	for key, value := range encoded {
		data, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(data) > 16384 {
			return nil, fmt.Errorf("invalid backup identity field")
		}
		result[key] = string(data)
	}
	return result, nil
}

func (d *Service) s3(ctx context.Context, destination database.Destination, reader bool) (*s3.Client, map[string]string, error) {
	ref := destination.Writer
	if reader {
		ref = destination.Reader
	}
	values, err := d.secret(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	if len(values["ACCESS_KEY_ID"]) < 8 || len(values["ACCESS_SECRET_KEY"]) < 16 {
		return nil, nil, fmt.Errorf("backup identity lacks S3 credentials")
	}
	values = map[string]string{"ACCESS_KEY_ID": values["ACCESS_KEY_ID"], "ACCESS_SECRET_KEY": values["ACCESS_SECRET_KEY"]}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if destination.CA != nil {
		ca, err := d.secret(ctx, *destination.CA)
		if err != nil {
			return nil, nil, err
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM([]byte(ca["ca.crt"])) {
			return nil, nil, fmt.Errorf("invalid backup TLS authority")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	endpoint := destination.Endpoint
	if destination.ProbeEndpoint != "" {
		endpoint = destination.ProbeEndpoint
	}
	config := aws.Config{Region: destination.Region, Credentials: credentials.NewStaticCredentialsProvider(values["ACCESS_KEY_ID"], values["ACCESS_SECRET_KEY"], ""), HTTPClient: &http.Client{Transport: transport, Timeout: 10 * time.Second}, RetryMaxAttempts: 1}
	client := s3.NewFromConfig(config, func(options *s3.Options) { options.BaseEndpoint = aws.String(endpoint); options.UsePathStyle = true })
	return client, values, nil
}

func (d *Service) projectBackupSecret(ctx context.Context, a database.Allocation, namespace, name string, data map[string]string) error {
	encoded := map[string]string{}
	for key, value := range data {
		encoded[key] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	_, err := d.apply(ctx, a, map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": name, "namespace": namespace}, "type": "Opaque", "data": encoded}, "keldock-database-backup", "keldock.io/backup-generation")
	return err
}
