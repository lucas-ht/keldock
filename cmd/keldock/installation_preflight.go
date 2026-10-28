package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/lucas-ht/keldock/internal/cluster"
	"github.com/lucas-ht/keldock/internal/databaseagent"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func installationPreflight(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("preflight", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("directory", "", "private installation input directory")
	kubeconfig := flags.String("kubeconfig", "", "exact operator kubeconfig")
	contextName := flags.String("context", "", "exact target context")
	checkEndpoint := flags.Bool("check-endpoint", false, "verify installed canonical HTTPS endpoint")
	caFile := flags.String("ca-file", "", "optional private installation CA PEM")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *kubeconfig == "" || *contextName == "" {
		return fmt.Errorf("explicit kubeconfig and context required")
	}
	var settings controlSettings
	if err := readSettings(filepath.Join(*directory, "control.json"), &settings); err != nil {
		return err
	}
	var values struct {
		StorageClass string `json:"storageClass"`
		Ingress      struct {
			Enabled   bool   `json:"enabled"`
			TLSSecret string `json:"tlsSecret"`
			Host      string `json:"host"`
		} `json:"ingress"`
	}
	valuesData, err := boundedFile(filepath.Join(*directory, "values.json"), 1024*1024)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(valuesData, &values); err != nil {
		return err
	}
	var input struct {
		Metadata struct {
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	secretData, err := boundedFile(filepath.Join(*directory, "installation-secret.json"), 1024*1024)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(secretData, &input); err != nil {
		return err
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(&clientcmd.ClientConfigLoadingRules{ExplicitPath: *kubeconfig}, &clientcmd.ConfigOverrides{CurrentContext: *contextName}).ClientConfig()
	if err != nil {
		return err
	}
	cfg.Timeout = 15 * time.Second
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Read-only preflight: context=%s server=%s organization=%s cluster=%s namespace=%s\n", *contextName, cfg.Host, settings.Organization, settings.Cluster, input.Metadata.Namespace)
	version, err := kube.Discovery().ServerVersion()
	if err != nil {
		return err
	}
	if version.Major != "1" || version.Minor != "35" {
		return fmt.Errorf("alpha supports Kubernetes 1.35.x; observed %s", version.GitVersion)
	}
	nodes, err := kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	if len(nodes.Items) == 0 {
		return fmt.Errorf("cluster has no schedulable nodes")
	}
	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}
		for _, condition := range node.Status.Conditions {
			if (condition.Type == "Ready" && condition.Status != "True") || ((condition.Type == "DiskPressure" || condition.Type == "MemoryPressure" || condition.Type == "PIDPressure") && condition.Status != "False") {
				return fmt.Errorf("node %s fails %s preflight; restore capacity before installing", node.Name, condition.Type)
			}
		}
	}
	storage, err := kube.StorageV1().StorageClasses().Get(ctx, values.StorageClass, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("explicit StorageClass is unavailable")
	}
	if storage.Provisioner == "kubernetes.io/no-provisioner" {
		return fmt.Errorf("dynamic persistent storage is required")
	}
	driver, err := cluster.New(*kubeconfig, *contextName, settings.RegistryPull, settings.RegistryPlainHTTP)
	if err != nil {
		return err
	}
	dependencies, err := driver.Inspect(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Verified %d supplied workload dependencies\n", len(dependencies))
	if len(settings.Databases) > 0 {
		database, err := databaseagent.New(*kubeconfig, *contextName)
		if err != nil {
			return err
		}
		if err := database.Inspect(ctx); err != nil {
			return err
		}
	}
	if values.Ingress.Enabled {
		secret, err := kube.CoreV1().Secrets(input.Metadata.Namespace).Get(ctx, values.Ingress.TLSSecret, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("existing ingress TLS Secret is unavailable")
		}
		if _, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"]); err != nil {
			return fmt.Errorf("ingress certificate and private key do not match")
		}
		block, _ := pem.Decode(secret.Data["tls.crt"])
		if block == nil {
			return fmt.Errorf("ingress TLS certificate is invalid")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil || certificate.VerifyHostname(values.Ingress.Host) != nil || time.Now().Before(certificate.NotBefore) || time.Until(certificate.NotAfter) < 24*time.Hour {
			return fmt.Errorf("ingress certificate hostname or validity check failed")
		}
	}
	if *checkEndpoint {
		origin, err := url.Parse(settings.PublicURL)
		if err != nil || origin.Scheme != "https" {
			return fmt.Errorf("endpoint acceptance requires canonical HTTPS")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			return err
		}
		if *caFile != "" {
			data, err := os.ReadFile(*caFile)
			if err != nil {
				return err
			}
			if !roots.AppendCertsFromPEM(data) {
				return fmt.Errorf("invalid installation CA")
			}
		}
		transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
		r, err := http.NewRequestWithContext(ctx, "GET", settings.PublicURL+"/sign-in", nil)
		if err != nil {
			return err
		}
		response, err := client.Do(r)
		if err != nil {
			return fmt.Errorf("canonical HTTPS origin is unreachable or untrusted")
		}
		if err := response.Body.Close(); err != nil {
			return err
		}
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("canonical sign-in endpoint returned %d", response.StatusCode)
		}
	}
	fmt.Printf("Preflight passed: Kubernetes=%s storage=%s; optional PostgreSQL=%t. No resources changed.\n", version.GitVersion, values.StorageClass, len(settings.Databases) > 0)
	return nil
}
