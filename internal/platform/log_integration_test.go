package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Run only against a dedicated throwaway database with migration 006 applied.
func TestServiceLogDatabasePipeline(t *testing.T) {
	databaseURL := os.Getenv("LOG_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LOG_INTEGRATION_DATABASE_URL for dedicated database integration test")
	}
	parsed, err := pgx.ParseConfig(databaseURL)
	if err != nil || !strings.HasSuffix(parsed.Database, "_test") {
		t.Fatal("log integration database name must end in _test")
	}
	ctx := context.Background()
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	var tenant string
	fixture, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	err = store.DB.QueryRow(ctx, `INSERT INTO tenants(name,slug,monitoring_approved,authorization_record,notification_recipients)
 VALUES('Synthetic Log Test',$1,true,'test authorization',ARRAY['test@example.invalid']) RETURNING id`, "log-test-"+fixture[:12]).Scan(&tenant)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	hot, archive := filepath.Join(root, "hot"), filepath.Join(root, "archive")
	for path, marker := range map[string]string{hot: "hot-store-test", archive: "archive-store-test"} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, ".aegisops-store-id"), []byte(marker), 0600); err != nil {
			t.Fatal(err)
		}
	}
	config := Config{Mode: "demo", LogHotDir: hot, LogArchiveDir: archive, LogHotMarker: "hot-store-test", LogArchiveMarker: "archive-store-test", LogKeyB64: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{23}, 32))}
	server := NewServer(store, config).Routes()
	call := func(path string, body any, credential string, demoActor bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Content-Type", "application/json")
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		if demoActor {
			req.Header.Set("X-Demo-Actor", "console")
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, req)
		return response
	}
	approvedPath := "/var/log/aegisops-approved/postgres.log"
	pathHash := sha256.Sum256([]byte(approvedPath))
	pathSHA := hex.EncodeToString(pathHash[:])
	issued := call("/v1/tenants/"+tenant+"/enrollment-tokens", map[string]any{"capabilities": []string{"service_logs"}, "log_sources": []map[string]string{{"name": "primary_db", "kind": "postgres", "path_sha256": pathSHA}}}, "", true)
	if issued.Code != 201 {
		t.Fatalf("issue token: %d %s", issued.Code, issued.Body.String())
	}
	var token struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(issued.Body.Bytes(), &token) != nil || token.Token == "" {
		t.Fatal("missing token")
	}
	bad := call("/v1/connectors/enroll", map[string]any{"token": token.Token, "name": "log-test", "version": "test", "capabilities": []string{"service_logs", "postgres_health"}}, "", false)
	if bad.Code != 403 {
		t.Fatalf("overclaimed enrollment accepted: %d", bad.Code)
	}
	enrolled := call("/v1/connectors/enroll", map[string]any{"token": token.Token, "name": "log-test", "version": "test", "capabilities": []string{"service_logs"}}, "", false)
	if enrolled.Code != 201 {
		t.Fatalf("enrollment: %d %s", enrolled.Code, enrolled.Body.String())
	}
	var identity struct {
		ConnectorID string `json:"connector_id"`
		Credential  string `json:"credential"`
	}
	if json.Unmarshal(enrolled.Body.Bytes(), &identity) != nil || identity.Credential == "" {
		t.Fatal("missing connector identity")
	}
	batchID := "0123456789abcdef0123456789abcdef"
	batch := map[string]any{"tenant_id": tenant, "connector_id": identity.ConnectorID, "batch_id": batchID, "source_name": "primary_db", "source_kind": "postgres", "path_sha256": pathSHA, "collected_at": time.Now().UTC(), "lines": []string{"ERROR service restart", "statement: SELECT * FROM student_records", "password=hidden"}}
	response := call("/v1/connectors/logs", batch, identity.Credential, false)
	if response.Code != 200 {
		t.Fatalf("ingestion: %d %s", response.Code, response.Body.String())
	}
	response = call("/v1/connectors/logs", batch, identity.Credential, false)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "duplicate") {
		t.Fatalf("idempotency: %d %s", response.Code, response.Body.String())
	}
	listing := httptest.NewRequest(http.MethodGet, "/v1/tenants/"+tenant+"/log-batches", nil)
	listing.RemoteAddr = "127.0.0.1:12345"
	listing.Header.Set("X-Demo-Actor", "console")
	listed := httptest.NewRecorder()
	server.ServeHTTP(listed, listing)
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), batchID) || strings.Contains(listed.Body.String(), "service restart") {
		t.Fatalf("log metadata listing exposed content or failed: %d %s", listed.Code, listed.Body.String())
	}
	batch["source_name"] = "unapproved"
	if rejected := call("/v1/connectors/logs", batch, identity.Credential, false); rejected.Code != 403 {
		t.Fatalf("unapproved source accepted: %d", rejected.Code)
	}
	batch["source_name"] = "primary_db"
	batch["path_sha256"] = strings.Repeat("0", 64)
	if rejected := call("/v1/connectors/logs", batch, identity.Credential, false); rejected.Code != 403 {
		t.Fatalf("unapproved path accepted: %d", rejected.Code)
	}
	batch["path_sha256"] = pathSHA
	if _, err := store.DB.Exec(ctx, "UPDATE log_batches SET received_at=now()-interval '366 days' WHERE tenant_id=$1 AND connector_id=$2 AND batch_id=$3", tenant, identity.ConnectorID, batchID); err != nil {
		t.Fatal(err)
	}
	storage, err := openLogStorage(config)
	if err != nil {
		t.Fatal(err)
	}
	worker := Worker{Store: store, Config: config}
	if err := worker.archiveOne(ctx, storage); err != nil {
		t.Fatal(err)
	}
	if held := call("/v1/tenants/"+tenant+"/log-batches/"+identity.ConnectorID+"/"+batchID+"/legal-hold", map[string]bool{"engaged": true}, "", true); held.Code != 200 {
		t.Fatalf("legal hold: %d %s", held.Code, held.Body.String())
	}
	if err := worker.cleanupArchivedOne(ctx, storage); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(storage.hotPath(tenant, identity.ConnectorID, batchID)); err != nil {
		t.Fatal("legal hold did not retain hot copy")
	}
	if released := call("/v1/tenants/"+tenant+"/log-batches/"+identity.ConnectorID+"/"+batchID+"/legal-hold", map[string]bool{"engaged": false}, "", true); released.Code != 200 {
		t.Fatalf("release hold: %d", released.Code)
	}
	if err := worker.cleanupArchivedOne(ctx, storage); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(storage.hotPath(tenant, identity.ConnectorID, batchID)); !os.IsNotExist(err) {
		t.Fatal("verified hot copy was not deleted")
	}
	poisonID := "fedcba9876543210fedcba9876543210"
	batch["batch_id"] = poisonID
	if response := call("/v1/connectors/logs", batch, identity.Credential, false); response.Code != 200 {
		t.Fatalf("poison fixture ingestion: %d", response.Code)
	}
	if err := os.Remove(storage.hotPath(tenant, identity.ConnectorID, poisonID)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(ctx, "UPDATE log_batches SET received_at=now()-interval '367 days' WHERE tenant_id=$1 AND batch_id=$2", tenant, poisonID); err != nil {
		t.Fatal(err)
	}
	if err := worker.archiveOne(ctx, storage); err == nil {
		t.Fatal("missing hot batch marked archived")
	}
	var attempts int
	if err := store.DB.QueryRow(ctx, "SELECT archive_attempts FROM log_batches WHERE tenant_id=$1 AND batch_id=$2", tenant, poisonID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("archive retry state missing: %d %v", attempts, err)
	}
	goodID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	batch["batch_id"] = goodID
	if response := call("/v1/connectors/logs", batch, identity.Credential, false); response.Code != 200 {
		t.Fatalf("second fixture ingestion: %d", response.Code)
	}
	if _, err := store.DB.Exec(ctx, "UPDATE log_batches SET received_at=now()-interval '366 days' WHERE tenant_id=$1 AND batch_id=$2", tenant, goodID); err != nil {
		t.Fatal(err)
	}
	if err := worker.archiveOne(ctx, storage); err != nil {
		t.Fatalf("poison batch blocked another archive: %v", err)
	}
	for attempts < 8 {
		if _, err := store.DB.Exec(ctx, "UPDATE log_batches SET archive_retry_at=now()-interval '1 second' WHERE tenant_id=$1 AND batch_id=$2", tenant, poisonID); err != nil {
			t.Fatal(err)
		}
		if err := worker.archiveOne(ctx, storage); err == nil {
			t.Fatal("missing hot batch unexpectedly archived")
		}
		attempts++
	}
	var status string
	if err := store.DB.QueryRow(ctx, "SELECT status FROM log_batches WHERE tenant_id=$1 AND batch_id=$2", tenant, poisonID).Scan(&status); err != nil || status != "blocked" {
		t.Fatalf("poison batch not blocked: %q %v", status, err)
	}
	if response := call("/v1/tenants/"+tenant+"/log-batches/"+identity.ConnectorID+"/"+poisonID+"/retry-archive", map[string]any{}, "", true); response.Code != 200 {
		t.Fatalf("blocked archive retry denied: %d %s", response.Code, response.Body.String())
	}
}
