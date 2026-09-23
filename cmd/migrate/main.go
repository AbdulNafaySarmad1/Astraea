package main

import (
	"context"
	"log"
	"nocturn.example/aegis-operations/internal/platform"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL required")
	}
	ctx := context.Background()
	s, err := platform.Open(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer s.DB.Close()
	_, err = s.DB.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())")
	if err != nil {
		log.Fatal(err)
	}
	files, err := filepath.Glob("db/[0-9][0-9][0-9]_*.sql")
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(files)
	for _, f := range files {
		name := filepath.Base(f)
		var exists bool
		if err = s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)", name).Scan(&exists); err != nil {
			log.Fatal(err)
		}
		if exists {
			continue
		}
		raw, e := os.ReadFile(f)
		if e != nil {
			log.Fatal(e)
		}
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			log.Fatal(e)
		}
		if _, e = tx.Exec(ctx, string(raw)); e != nil {
			tx.Rollback(ctx)
			log.Fatal(e)
		}
		if _, e = tx.Exec(ctx, "INSERT INTO schema_migrations(name) VALUES($1)", name); e != nil {
			tx.Rollback(ctx)
			log.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			log.Fatal(e)
		}
		log.Printf("applied %s", strings.TrimSuffix(name, ".sql"))
	}
}
