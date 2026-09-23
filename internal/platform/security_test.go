package platform

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHostNormalizationRejectsNetworkTargets(t *testing.T) {
	invalid := []string{"https://example.com", "127.0.0.1", "localhost", "a..example", "example.com:8080", "user@example.com", "example.com/path", "-a.example", "example.com\nother"}
	for _, host := range invalid {
		if _, err := normalizeHost(host); err == nil {
			t.Errorf("accepted %q", host)
		}
	}
	got, err := normalizeHost("  OPS.EXAMPLE.COM. ")
	if err != nil || got != "ops.example.com" {
		t.Fatalf("normalization: %q, %v", got, err)
	}
}
func TestActionHashBindsTenantTargetParametersAndPolicy(t *testing.T) {
	base := ActionInput{Target: "db-primary", ActionType: "diagnostic_snapshot", Parameters: map[string]any{"depth": 1}}
	initial := actionHash("tenant-a", base, 3)
	variants := []struct {
		tenant  string
		input   ActionInput
		version int
	}{{"tenant-b", base, 3}, {"tenant-a", ActionInput{Target: "db-replica", ActionType: base.ActionType, Parameters: base.Parameters}, 3}, {"tenant-a", ActionInput{Target: base.Target, ActionType: base.ActionType, Parameters: map[string]any{"depth": 2}}, 3}, {"tenant-a", base, 4}}
	for _, v := range variants {
		if actionHash(v.tenant, v.input, v.version) == initial {
			t.Fatal("action binding collision")
		}
	}
}
func TestProxyHeaderOnlyFromTrustedNetwork(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.test/", nil)
	r.RemoteAddr = "203.0.113.10:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.1")
	if got := requestIP(r, []string{"192.0.2.0/24"}); got != "203.0.113.10" {
		t.Fatalf("untrusted proxy changed IP: %s", got)
	}
	r.RemoteAddr = "192.0.2.10:1234"
	if got := requestIP(r, []string{"192.0.2.0/24"}); got != "10.0.0.1" {
		t.Fatalf("trusted proxy IP: %s", got)
	}
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 203.0.113.5")
	if got := requestIP(r, []string{"192.0.2.0/24"}); got != "203.0.113.5" {
		t.Fatalf("spoofed leftmost header accepted: %s", got)
	}
}
func TestOIDCVerifierChecksSignatureAudienceAndExpiry(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1})
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": issuer + "/certs"})
		case "/certs":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kid": "test", "kty": "RSA", "alg": "RS256", "n": n, "e": e}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	sign := func(audience string, expiry int64) string {
		header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test"})
		payload, _ := json.Marshal(map[string]any{"iss": issuer, "sub": "user-1", "aud": audience, "exp": expiry, "realm_access": map[string]any{"roles": []string{"ops_engineer"}}})
		body := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
		sum := sha256.Sum256([]byte(body))
		signature, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
		return body + "." + base64.RawURLEncoding.EncodeToString(signature)
	}
	verifier := NewOIDCVerifier(issuer, "nocturn-api")
	valid := sign("nocturn-api", time.Now().Add(time.Minute).Unix())
	actor, err := verifier.Verify(context.Background(), valid)
	if err != nil || actor.Subject != "user-1" || !actor.Roles["ops_engineer"] {
		t.Fatalf("valid token rejected: %v", err)
	}
	for _, token := range []string{sign("other-api", time.Now().Add(time.Minute).Unix()), sign("nocturn-api", time.Now().Add(-time.Minute).Unix()), valid + "tamper"} {
		if _, err := verifier.Verify(context.Background(), token); err == nil {
			t.Fatal("invalid token accepted")
		}
	}
}
func TestDemoModeCannotBindPublicly(t *testing.T) {
	t.Setenv("APP_MODE", "demo")
	t.Setenv("DATABASE_URL", "postgres://placeholder")
	t.Setenv("LISTEN_ADDR", ":8080")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("unsafe demo bind accepted: %v", err)
	}
	t.Setenv("LISTEN_ADDR", "127.0.0.1:8080")
	if _, err := LoadConfig(); err != nil {
		t.Fatal(fmt.Sprint(err))
	}
}
func TestTurnstileResultFailsClosed(t *testing.T) {
	now := time.Now()
	valid := turnstileResult{Success: true, Hostname: "ops.example", Action: "contact", ChallengeTS: now.Add(-time.Minute)}
	if err := validateTurnstileResult(valid, "ops.example", "contact", now); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []turnstileResult{{Success: false, Hostname: valid.Hostname, Action: valid.Action, ChallengeTS: valid.ChallengeTS}, {Success: true, Hostname: "other.example", Action: valid.Action, ChallengeTS: valid.ChallengeTS}, {Success: true, Hostname: valid.Hostname, Action: "other", ChallengeTS: valid.ChallengeTS}, {Success: true, Hostname: valid.Hostname, Action: valid.Action, ChallengeTS: now.Add(-6 * time.Minute)}} {
		if validateTurnstileResult(bad, "ops.example", "contact", now) == nil {
			t.Fatal("invalid siteverify result accepted")
		}
	}
}
func TestTenantMembershipIsolation(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	var a, b string
	err = store.DB.QueryRow(ctx, "INSERT INTO tenants(name,slug,authorization_record) VALUES('Isolation fixture A','isolation-fixture-a','test authorization') RETURNING id").Scan(&a)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Exec(ctx, "DELETE FROM tenants WHERE id=$1", a)
	err = store.DB.QueryRow(ctx, "INSERT INTO tenants(name,slug,authorization_record) VALUES('Isolation fixture B','isolation-fixture-b','test authorization') RETURNING id").Scan(&b)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Exec(ctx, "DELETE FROM tenants WHERE id=$1", b)
	_, err = store.DB.Exec(ctx, "INSERT INTO memberships(subject,tenant_id,role) VALUES('isolation-test',$1,'customer_viewer')", a)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Exec(ctx, "DELETE FROM memberships WHERE subject='isolation-test'")
	actor := Actor{Subject: "isolation-test", Roles: map[string]bool{}}
	if !store.TenantAllowed(ctx, actor, a, "view") {
		t.Fatal("own tenant denied")
	}
	if store.TenantAllowed(ctx, actor, b, "view") {
		t.Fatal("cross-tenant view allowed")
	}
	if store.TenantAllowed(ctx, actor, a, "operate") {
		t.Fatal("customer viewer could operate")
	}
	if store.TenantAllowed(ctx, actor, "", "view") {
		t.Fatal("empty tenant accepted")
	}
}
func TestAuditOutboxIntegration(t *testing.T) {
	if os.Getenv("AUDIT_INTEGRATION") != "1" {
		t.Skip("set AUDIT_INTEGRATION=1 for isolated audit database")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL required")
	}
	ctx := context.Background()
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	var tenant string
	err = store.DB.QueryRow(ctx, "INSERT INTO tenants(name,slug,authorization_record,notification_recipients) VALUES('Audit fixture','audit-fixture','test authorization',ARRAY['recipient@example.invalid']) RETURNING id").Scan(&tenant)
	if err != nil {
		t.Fatal(err)
	}
	eventID, err := store.Audit(ctx, &tenant, "test-actor", "diagnostic.viewed", "test-resource", "test-correlation", "success", "127.0.0.1", map[string]string{"category": "diagnostic"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	err = store.DB.QueryRow(ctx, "SELECT count(*) FROM notification_outbox WHERE tenant_id=$1 AND audit_event_id=$2 AND status='queued'", tenant, eventID).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("outbox missing: %d, %v", count, err)
	}
}
