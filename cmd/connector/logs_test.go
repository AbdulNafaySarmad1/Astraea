package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceLogTailStartsAtEndAndFiltersSensitiveLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "postgres.log")
	if err := os.WriteFile(path, []byte("historical line\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spool, err := openLogSpool(filepath.Join(dir, "spool"))
	if err != nil {
		t.Fatal(err)
	}
	config := Config{TenantID: "tenant-a", ConnectorID: "connector-a"}
	source := LogSource{Name: "primary_db", Kind: "postgres", Path: path}
	if err := collectLogSource(config, spool, source); err != nil {
		t.Fatal(err)
	}
	if _, _, err := spool.oldest(); !os.IsNotExist(err) {
		t.Fatalf("historical lines were ingested: %v", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("2026-01-01 ERROR connection reset\npassword=example\nstatement: SELECT * FROM students\nuser admin@example.edu disconnected\n")
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := collectLogSource(config, spool, source); err != nil {
		t.Fatal(err)
	}
	_, batch, err := spool.oldest()
	if err != nil || len(batch.Lines) != 2 {
		t.Fatalf("expected two filtered lines: %v, %#v", err, batch.Lines)
	}
	joined := strings.Join(batch.Lines, "\n")
	if strings.Contains(joined, "example") || strings.Contains(joined, "students") || !strings.Contains(joined, "connection reset") || !strings.Contains(joined, "[EMAIL REDACTED]") {
		t.Fatalf("unsafe service log content: %q", joined)
	}
	if err := collectLogSource(config, spool, source); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(spool.dir)
	batches := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".batch") {
			batches++
		}
	}
	if batches != 1 {
		t.Fatalf("repeated tail queued %d batches", batches)
	}
}
