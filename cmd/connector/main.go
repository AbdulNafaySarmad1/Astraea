// The tenant connector collects only locally configured infrastructure signals.
// It never accepts probe targets or commands from the control plane.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type Probe struct {
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	AllowedCIDRs  []string `json:"allowed_cidrs"`
	CredentialEnv string   `json:"credential_env,omitempty"`
	Username      string   `json:"username,omitempty"`
	TLS           bool     `json:"tls,omitempty"`
	MinReplicas   int      `json:"min_replicas,omitempty"`
}

type Config struct {
	ControlPlaneURL           string  `json:"control_plane_url"`
	TenantID                  string  `json:"tenant_id"`
	ConnectorID               string  `json:"connector_id"`
	Version                   string  `json:"version"`
	Probes                    []Probe `json:"probes"`
	HostMetrics               bool    `json:"host_metrics,omitempty"`
	SpoolDir                  string  `json:"spool_dir,omitempty"`
	CollectionIntervalSeconds int     `json:"collection_interval_seconds,omitempty"`
}

type MetricSample struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type ComponentSample struct {
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	Status     string         `json:"status"`
	ObservedAt time.Time      `json:"observed_at"`
	LatencyMS  *float64       `json:"latency_ms,omitempty"`
	Metrics    []MetricSample `json:"metrics,omitempty"`
}

type Heartbeat struct {
	TenantID    string            `json:"tenant_id"`
	ConnectorID string            `json:"connector_id"`
	BatchID     string            `json:"batch_id,omitempty"`
	Version     string            `json:"version"`
	Components  []ComponentSample `json:"components"`
}

func main() {
	path := flag.String("config", "connector.json", "connector configuration")
	flag.Parse()
	raw, err := os.ReadFile(*path)
	if err != nil {
		log.Fatal(err)
	}
	var c Config
	if err = json.Unmarshal(raw, &c); err != nil {
		log.Fatal("invalid connector configuration")
	}
	credential := os.Getenv("CONNECTOR_CREDENTIAL")
	if credential == "" {
		log.Fatal("CONNECTOR_CREDENTIAL required from secrets manager")
	}
	if err = c.validate(); err != nil {
		log.Fatal(err)
	}
	spool, err := openSpool(c.SpoolDir)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := newControlPlaneClient()
	interval := time.Duration(c.CollectionIntervalSeconds) * time.Second
	collectionTicker := time.NewTicker(interval)
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer collectionTicker.Stop()
	defer heartbeatTicker.Stop()

	collectAndQueue(ctx, c, spool)
	flushQueue(ctx, client, c, credential, spool)
	for {
		select {
		case <-ctx.Done():
			return
		case <-collectionTicker.C:
			flushQueue(ctx, client, c, credential, spool)
			collectAndQueue(ctx, c, spool)
			flushQueue(ctx, client, c, credential, spool)
		case <-heartbeatTicker.C:
			flushQueue(ctx, client, c, credential, spool)
			if err := send(ctx, client, c.ControlPlaneURL, credential, Heartbeat{TenantID: c.TenantID, ConnectorID: c.ConnectorID, Version: c.Version, Components: []ComponentSample{}}); err != nil {
				log.Printf("connector heartbeat failed: %v", err)
			}
		}
	}
}

// Connector transport requires the peer to negotiate a hybrid ML-KEM key
// exchange. Certificate validation still uses the platform trust store.
func newControlPlaneClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.X25519MLKEM768},
		VerifyConnection: func(state tls.ConnectionState) error {
			if state.CurveID != tls.X25519MLKEM768 {
				return errors.New("control plane did not negotiate hybrid ML-KEM TLS")
			}
			return nil
		},
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (c *Config) validate() error {
	u, err := url.Parse(c.ControlPlaneURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("control plane URL must be an HTTPS origin")
	}
	if c.TenantID == "" || c.ConnectorID == "" || c.Version == "" || len(c.Probes) > 100 {
		return errors.New("connector identity, version, or probe list invalid")
	}
	if c.CollectionIntervalSeconds == 0 {
		c.CollectionIntervalSeconds = 900
	}
	if c.CollectionIntervalSeconds < 60 || c.CollectionIntervalSeconds > 900 {
		return errors.New("collection interval must be 60 to 900 seconds")
	}
	if c.SpoolDir == "" {
		return errors.New("spool_dir required for durable telemetry delivery")
	}
	for _, p := range c.Probes {
		if err := validateProbe(p); err != nil {
			return fmt.Errorf("probe %q: %w", p.Name, err)
		}
	}
	return nil
}

func collectAndQueue(ctx context.Context, c Config, spool *Spool) {
	components := make([]ComponentSample, 0, len(c.Probes)+1)
	if c.HostMetrics {
		components = append(components, collectHost(ctx))
	}
	for _, p := range c.Probes {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		components = append(components, collectProbe(probeCtx, p))
		cancel()
	}
	if len(components) == 0 {
		return
	}
	batchID, err := randomBatchID()
	if err != nil {
		log.Printf("telemetry ID unavailable: %v", err)
		return
	}
	if err = spool.Save(Heartbeat{TenantID: c.TenantID, ConnectorID: c.ConnectorID, BatchID: batchID, Version: c.Version, Components: components}); err != nil {
		log.Printf("telemetry not queued: %v", err)
	}
}

func randomBatchID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func flushQueue(ctx context.Context, client *http.Client, c Config, credential string, spool *Spool) {
	for i := 0; i < 20; i++ {
		path, batch, err := spool.Oldest()
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			log.Printf("telemetry spool unavailable: %v", err)
			return
		}
		if batch.TenantID != c.TenantID || batch.ConnectorID != c.ConnectorID {
			log.Print("telemetry spool identity does not match this connector; operator intervention required")
			return
		}
		if err = send(ctx, client, c.ControlPlaneURL, credential, batch); err != nil {
			log.Printf("telemetry delivery deferred: %v", err)
			return
		}
		if err = spool.Remove(path); err != nil {
			log.Printf("accepted telemetry remains in spool: %v", err)
			return
		}
	}
}

func send(ctx context.Context, client *http.Client, controlPlaneURL, credential string, heartbeat Heartbeat) error {
	payload, err := json.Marshal(heartbeat)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(controlPlaneURL, "/")+"/v1/connectors/heartbeat", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane rejected telemetry: HTTP %d", resp.StatusCode)
	}
	return nil
}

func validateProbe(p Probe) error {
	if p.Name == "" || len(p.Name) > 120 || strings.ContainsAny(p.Name, "\r\n") || p.Host == "" || p.Port < 1 || p.Port > 65535 || len(p.AllowedCIDRs) == 0 || len(p.AllowedCIDRs) > 8 {
		return errors.New("invalid probe target")
	}
	if p.Kind != "tcp" && p.Kind != "postgres" && p.Kind != "valkey" {
		return errors.New("unsupported probe kind")
	}
	for _, c := range p.AllowedCIDRs {
		_, network, err := net.ParseCIDR(c)
		if err != nil {
			return errors.New("invalid allowed CIDR")
		}
		ones, bits := network.Mask.Size()
		if (bits == 32 && ones < 24) || (bits == 128 && ones < 64) {
			return errors.New("allowed CIDR is too broad")
		}
	}
	if (p.Kind == "postgres" || p.Kind == "valkey") && (p.CredentialEnv == "" || !validEnvName(p.CredentialEnv)) {
		return errors.New("credential_env must name a service-specific environment variable")
	}
	if p.MinReplicas < 0 || p.MinReplicas > 100 {
		return errors.New("min_replicas out of range")
	}
	if p.Kind == "valkey" && !p.TLS && !loopbackHost(p.Host) {
		return errors.New("Valkey requires TLS unless bound to loopback")
	}
	return nil
}

func validEnvName(name string) bool {
	if len(name) < 2 || len(name) > 80 || (name[0] < 'A' || name[0] > 'Z') {
		return false
	}
	for _, c := range name {
		if c != '_' && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func approvedDial(ctx context.Context, p Probe) (net.Conn, error) {
	if err := validateProbe(p); err != nil {
		return nil, err
	}
	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, p.Host)
	if err != nil {
		return nil, err
	}
	var lastErr error = errors.New("no approved address responded")
	for _, addr := range ipAddrs {
		if loopbackHost(p.Host) && !addr.IP.IsLoopback() {
			continue
		}
		approved := false
		for _, cidr := range p.AllowedCIDRs {
			_, network, _ := net.ParseCIDR(cidr)
			if network.Contains(addr.IP) {
				approved = true
				break
			}
		}
		if !approved {
			continue
		}
		dialer := net.Dialer{Timeout: 2 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(addr.IP.String(), fmt.Sprint(p.Port)))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func check(ctx context.Context, p Probe) error {
	conn, err := approvedDial(ctx, p)
	if err == nil {
		_ = conn.Close()
	}
	return err
}
