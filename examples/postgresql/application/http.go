package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func web(ctx context.Context) error {
	target, _ := url.Parse("http://api:8080")
	proxy := httputil.NewSingleHostReverseProxy(target)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.Handle("/", proxy)
	return serve(ctx, mux)
}

func api(ctx context.Context, pool *pgxpool.Pool) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		check, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if pool.Ping(check) != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /markers/{marker}", func(w http.ResponseWriter, r *http.Request) {
		marker := r.PathValue("marker")
		if len(marker) < 1 || len(marker) > 80 {
			http.Error(w, "invalid marker", 400)
			return
		}
		if _, err := pool.Exec(r.Context(), `INSERT INTO events(kind,payload)VALUES('marker',$1)`, marker); err != nil {
			http.Error(w, "database unavailable", 503)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.Query(r.Context(), `SELECT kind,payload FROM events ORDER BY id DESC LIMIT 30`)
		if err != nil {
			http.Error(w, "database unavailable", 503)
			return
		}
		defer rows.Close()
		events := []map[string]string{}
		for rows.Next() {
			var kind, payload string
			if rows.Scan(&kind, &payload) != nil {
				http.Error(w, "database unavailable", 503)
				return
			}
			events = append(events, map[string]string{"kind": kind, "payload": payload})
		}
		if rows.Err() != nil {
			http.Error(w, "database unavailable", 503)
			return
		}
		var super, createDB, createRole bool
		if err := pool.QueryRow(r.Context(), `SELECT rolsuper,rolcreatedb,rolcreaterole FROM pg_roles WHERE rolname=current_user`).Scan(&super, &createDB, &createRole); err != nil {
			http.Error(w, "database unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"events": events, "superuser": super, "createDatabase": createDB, "createRole": createRole}); err != nil {
			return
		}
	})
	return serve(ctx, mux)
}

func serve(ctx context.Context, handler http.Handler) error {
	server := &http.Server{Addr: ":8080", Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		return err
	}
	err := <-done
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
