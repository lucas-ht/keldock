// The acceptance application uses only its declared PostgreSQL binding.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("mode required")
	}
	if args[0] == "web" {
		return web(ctx)
	}
	if args[0] == "health" {
		info, err := os.Stat("/tmp/healthy")
		if err != nil || time.Since(info.ModTime()) > 30*time.Second {
			return fmt.Errorf("worker is unhealthy")
		}
		return nil
	}
	if args[0] == "probe" {
		if len(args) != 2 {
			return fmt.Errorf("probe host required")
		}
		connection, err := net.DialTimeout("tcp", net.JoinHostPort(args[1], "5432"), 3*time.Second)
		if err != nil {
			return fmt.Errorf("database network denied")
		}
		return connection.Close()
	}
	config, err := pgxpool.ParseConfig("")
	if err != nil {
		return fmt.Errorf("invalid database binding")
	}
	config.MaxConns = 2
	config.ConnConfig.ConnectTimeout = 5 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return err
	}
	defer pool.Close()
	// Service endpoints and NetworkPolicy rules converge asynchronously. Bound the
	// initial connection wait instead of making a one-shot migration race them.
	usable := false
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline) && ctx.Err() == nil; {
		check, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := pool.Ping(check)
		cancel()
		if err == nil {
			usable = true
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
	if !usable {
		return fmt.Errorf("database binding did not become usable within 30 seconds")
	}
	switch args[0] {
	case "migrate":
		if os.Getenv("FAIL_MIGRATION") == "true" {
			return fmt.Errorf("synthetic migration failed before schema changes")
		}
		_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS events(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,kind text NOT NULL,payload text NOT NULL,created_at timestamptz NOT NULL DEFAULT now())`)
		return err
	case "api":
		return api(ctx, pool)
	case "schedule":
		_, err := pool.Exec(ctx, `INSERT INTO events(kind,payload)VALUES('schedule','scheduled work completed')`)
		return err
	case "worker":
		for ctx.Err() == nil {
			if _, err := pool.Exec(ctx, `INSERT INTO events(kind,payload)VALUES('worker','background work completed')`); err != nil {
				return err
			}
			if err := os.WriteFile("/tmp/healthy", []byte("ready"), 0o600); err != nil {
				return err
			}
			select {
			case <-ctx.Done():
			case <-time.After(10 * time.Second):
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported mode")
	}
}
