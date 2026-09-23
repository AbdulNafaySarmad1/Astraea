package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestControlPlaneTransportRequiresHybridKeyExchange(t *testing.T) {
	for _, tc := range []struct {
		name    string
		curve   tls.CurveID
		wantErr bool
	}{
		{name: "hybrid", curve: tls.X25519MLKEM768},
		{name: "classical only", curve: tls.X25519, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{tc.curve}}
			server.StartTLS()
			defer server.Close()

			client := newControlPlaneClient()
			transport := client.Transport.(*http.Transport)
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			transport.TLSClientConfig.RootCAs = roots
			response, err := client.Get(server.URL)
			if tc.wantErr {
				if err == nil {
					response.Body.Close()
					t.Fatal("classical-only peer accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
		})
	}
}

func TestProbeRequiresApprovedCIDR(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	probe := Probe{Name: "local", Kind: "tcp", Host: "127.0.0.1", Port: port, AllowedCIDRs: []string{"192.0.2.0/24"}}
	if err = check(ctx, probe); err == nil {
		t.Fatal("probe escaped allowed CIDR")
	}
	probe.AllowedCIDRs = []string{"127.0.0.1/32"}
	if err = check(ctx, probe); err != nil {
		t.Fatalf("approved probe failed: %v", err)
	}
}

func TestSpoolRetainsUntilAcknowledged(t *testing.T) {
	spool, err := openSpool(filepath.Join(t.TempDir(), "spool"))
	if err != nil {
		t.Fatal(err)
	}
	batch := Heartbeat{TenantID: "tenant-a", ConnectorID: "connector-a", BatchID: "0123456789abcdef0123456789abcdef", Version: "1", Components: []ComponentSample{{Name: "db", Kind: "tcp", Status: "healthy", ObservedAt: time.Now().UTC()}}}
	if err = spool.Save(batch); err != nil {
		t.Fatal(err)
	}
	path, got, err := spool.Oldest()
	if err != nil || got.BatchID != batch.BatchID || got.TenantID != batch.TenantID {
		t.Fatalf("queued batch missing: %v", err)
	}
	if err = spool.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err = spool.Oldest(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acknowledged batch remains queued: %v", err)
	}
}

func TestProbeRejectsBroadNetworkAndPlaintextRemoteValkey(t *testing.T) {
	base := Probe{Name: "cache", Kind: "valkey", Host: "192.0.2.4", Port: 6379, AllowedCIDRs: []string{"192.0.2.4/32"}, CredentialEnv: "VALKEY_MONITOR_PASSWORD"}
	if validateProbe(base) == nil {
		t.Fatal("remote plaintext Valkey probe accepted")
	}
	base.TLS = true
	base.AllowedCIDRs = []string{"0.0.0.0/0"}
	if validateProbe(base) == nil {
		t.Fatal("unrestricted probe CIDR accepted")
	}
}
