package platform

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Run against an isolated database after migrations and
// deploy/postgres_tenant_reader.sql. Fixtures are rolled back.
func TestTenantDatabaseBoundaries(t *testing.T) {
	if os.Getenv("TENANT_RLS_INTEGRATION") != "1" {
		t.Skip("set TENANT_RLS_INTEGRATION=1 for an isolated database")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required")
	}
	ctx := context.Background()
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	tx, err := store.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	fixture, _ := randomID()
	var tenantA, tenantB, connectorA, connectorB, componentB, auditB string
	for _, row := range []struct {
		name, slug string
		id         *string
	}{
		{"Tenant boundary A", "boundary-a-" + fixture, &tenantA},
		{"Tenant boundary B", "boundary-b-" + fixture, &tenantB},
	} {
		if err := tx.QueryRow(ctx, "INSERT INTO tenants(name,slug) VALUES($1,$2) RETURNING id", row.name, row.slug).Scan(row.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.QueryRow(ctx, "INSERT INTO connectors(tenant_id,name,version,credential_hash) VALUES($1,'fixture','1',$2) RETURNING id", tenantB, fixture).Scan(&connectorB); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, "INSERT INTO connectors(tenant_id,name,version,credential_hash) VALUES($1,'fixture','1',$2) RETURNING id", tenantA, "a-"+fixture).Scan(&connectorA); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, "INSERT INTO components(tenant_id,connector_id,name,kind) VALUES($1,$2,'fixture','test') RETURNING id", tenantB, connectorB).Scan(&componentB); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, "INSERT INTO audit_events(tenant_id,actor,event_type,resource,correlation_id,outcome,previous_hash,event_hash) VALUES($1,'fixture','test','fixture','fixture','success',$2,$2) RETURNING id", tenantB, fixture).Scan(&auditB); err != nil {
		t.Fatal(err)
	}
	foreignKeyMustReject := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, "SAVEPOINT tenant_boundary"); err != nil {
			t.Fatal(err)
		}
		_, err := tx.Exec(ctx, query, args...)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
			t.Fatalf("cross-tenant reference should violate a foreign key, got %v", err)
		}
		if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT tenant_boundary"); err != nil {
			t.Fatal(err)
		}
	}
	foreignKeyMustReject("INSERT INTO components(tenant_id,connector_id,name,kind) VALUES($1,$2,'cross-tenant','test')", tenantA, connectorB)
	foreignKeyMustReject("INSERT INTO telemetry(tenant_id,connector_id,metric,value,unit,observed_at) VALUES($1,$2,'test',1,'count',now())", tenantA, connectorB)
	foreignKeyMustReject("INSERT INTO telemetry(tenant_id,connector_id,component_id,metric,value,unit,observed_at) VALUES($1,$2,$3,'test',1,'count',now())", tenantA, connectorA, componentB)
	foreignKeyMustReject("INSERT INTO incidents(tenant_id,title,severity,component_id) VALUES($1,'test','unknown',$2)", tenantA, componentB)
	foreignKeyMustReject("INSERT INTO notification_outbox(tenant_id,audit_event_id,recipient) VALUES($1,$2,'fixture@example.invalid')", tenantA, auditB)

	// SET LOCAL ROLE demonstrates what a distinct, non-owner read connection
	// can see. A missing tenant context must reveal no customer rows.
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE nocturn_tenant_reader"); err != nil {
		t.Fatalf("apply deploy/postgres_tenant_reader.sql first: %v", err)
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM tenants WHERE id IN ($1,$2)", tenantA, tenantB).Scan(&count); err != nil || count != 0 {
		t.Fatalf("missing context exposed tenant rows: %d, %v", count, err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", tenantA); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM tenants WHERE id IN ($1,$2)", tenantA, tenantB).Scan(&count); err != nil || count != 1 {
		t.Fatalf("tenant read scope failed: %d, %v", count, err)
	}
	for _, table := range []string{"connectors", "components", "audit_events"} {
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE tenant_id=$1", tenantB).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s exposed another tenant: %d, %v", table, count, err)
		}
	}

	if err := store.OpenTenantReader(ctx, url); err == nil {
		store.Reader.Close()
		t.Fatal("privileged database owner accepted as production reader")
	} else if !strings.Contains(err.Error(), "requires a non-owner") {
		t.Fatalf("reader policy validation failed before checking owner privilege: %v", err)
	}
	// Exercise the same transaction-local role and scope used by GET handlers.
	if readURL := os.Getenv("TENANT_READ_DATABASE_URL"); readURL != "" {
		if err := store.OpenTenantReader(ctx, readURL); err != nil {
			t.Fatalf("dedicated reader rejected: %v", err)
		}
		defer store.Reader.Close()
	} else {
		// The role transition itself can still be tested with the local owner.
		store.Reader = store.DB
	}
	read, err := store.TenantRead(ctx, tenantA)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Rollback(ctx)
	var role, scope string
	if err := read.QueryRow(ctx, "SELECT current_user,current_setting('app.tenant_id')").Scan(&role, &scope); err != nil || role != "nocturn_tenant_reader" || scope != tenantA {
		t.Fatalf("tenant read transaction was not scoped: %q %q %v", role, scope, err)
	}
	_, err = read.Exec(ctx, "SELECT credential_hash FROM connectors LIMIT 1")
	var privilegeErr *pgconn.PgError
	if !errors.As(err, &privilegeErr) || privilegeErr.Code != "42501" {
		t.Fatalf("reader could query connector credentials: %v", err)
	}

	var existingTenant string
	if err := store.DB.QueryRow(ctx, "SELECT id FROM tenants WHERE id<>$1 AND id<>$2 LIMIT 1", tenantA, tenantB).Scan(&existingTenant); err != nil {
		t.Skip("no committed tenant fixture for HTTP read checks")
	}
	server := NewServer(store, Config{Mode: "demo"}).Routes()
	for _, suffix := range []string{"domains", "connectors", "policies", "actions", "investigations"} {
		req := httptest.NewRequest("GET", "/v1/tenants/"+existingTenant+"/"+suffix, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("X-Demo-Actor", "console")
		response := httptest.NewRecorder()
		server.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("scoped %s route returned %d: %s", suffix, response.Code, response.Body.String())
		}
	}
}
