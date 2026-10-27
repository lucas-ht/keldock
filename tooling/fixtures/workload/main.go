// The synthetic acceptance workload is built by the local harness, never by Keldock.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

var buildRevision = "v1"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	mode := "serve"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	var err error
	switch mode {
	case "health":
		info, statErr := os.Stat("/tmp/heartbeat")
		if statErr != nil || time.Since(info.ModTime()) > 15*time.Second {
			err = fmt.Errorf("worker heartbeat stale")
		}
	case "worker":
		err = worker(ctx)
	case "job", "schedule":
		if os.Getenv("FAIL_JOB") == "true" {
			err = fmt.Errorf("requested prerequisite failure")
		} else {
			path := "/migrate"
			if mode == "schedule" {
				path = "/schedule"
			}
			err = waitForUpstream(ctx)
			if err == nil {
				err = call(ctx, http.MethodPost, path)
			}
		}
	case "probe":
		if len(os.Args) != 3 {
			err = fmt.Errorf("probe URL required")
		} else {
			err = request(ctx, http.MethodGet, os.Args[2])
		}
	default:
		err = serve(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func request(ctx context.Context, method, target string) error {
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return err
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("fixture request: %w", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "response close failed")
		}
	}()
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, 4096)); err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fixture response status %d", response.StatusCode)
	}
	return nil
}

func call(ctx context.Context, method, path string) error {
	return request(ctx, method, os.Getenv("UPSTREAM_URL")+path)
}

func waitForUpstream(ctx context.Context) error {
	ready, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var last error
	for {
		last = call(ready, http.MethodGet, "/ready")
		if last == nil {
			return nil
		}
		select {
		case <-ready.Done():
			return fmt.Errorf("upstream readiness: %w", last)
		case <-time.After(time.Second):
		}
	}
}

func worker(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := call(ctx, http.MethodPost, "/worker"); err == nil {
			if err := os.WriteFile("/tmp/heartbeat", []byte("healthy"), 0o600); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func serve(ctx context.Context) error {
	var migrations, workers, schedules atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", func(writer http.ResponseWriter, request *http.Request) {
		if os.Getenv("BROKEN") == "true" {
			http.Error(writer, "requested readiness failure", http.StatusServiceUnavailable)
			return
		}
		if os.Getenv("UPSTREAM_URL") != "" {
			if err := call(request.Context(), http.MethodGet, "/ready"); err != nil {
				http.Error(writer, "upstream unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		writer.WriteHeader(http.StatusOK)
	})
	for path, counter := range map[string]*atomic.Int64{"/migrate": &migrations, "/worker": &workers, "/schedule": &schedules} {
		mux.HandleFunc("POST "+path, func(writer http.ResponseWriter, _ *http.Request) { counter.Add(1); writer.WriteHeader(http.StatusOK) })
	}
	mux.HandleFunc("GET /", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		var result any = map[string]any{"version": os.Getenv("VERSION"), "artifact": buildRevision, "migrations": migrations.Load(), "workerTicks": workers.Load(), "scheduledTicks": schedules.Load()}
		if upstream := os.Getenv("UPSTREAM_URL"); upstream != "" {
			req, err := http.NewRequestWithContext(request.Context(), http.MethodGet, upstream+"/", nil)
			if err != nil {
				http.Error(writer, "upstream request invalid", http.StatusBadGateway)
				return
			}
			response, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
			if err != nil {
				http.Error(writer, "upstream unavailable", http.StatusBadGateway)
				return
			}
			defer func() {
				if err := response.Body.Close(); err != nil {
					fmt.Fprintln(os.Stderr, "response close failed")
				}
			}()
			var body json.RawMessage
			if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body); err != nil {
				http.Error(writer, "upstream response invalid", http.StatusBadGateway)
				return
			}
			result = map[string]any{"web": true, "api": body}
		}
		if err := json.NewEncoder(writer).Encode(result); err != nil {
			fmt.Fprintln(os.Stderr, "response encoding failed")
		}
	})
	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}
