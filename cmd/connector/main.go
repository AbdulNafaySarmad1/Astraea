// The customer connector performs only locally configured TCP probes and sends outbound telemetry.
// It accepts no commands or targets from the control plane.
package main

import (
	"bytes"
	"context"
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
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	AllowedCIDRs []string `json:"allowed_cidrs"`
}
type Config struct {
	ControlPlaneURL string  `json:"control_plane_url"`
	TenantID        string  `json:"tenant_id"`
	ConnectorID     string  `json:"connector_id"`
	Version         string  `json:"version"`
	Probes          []Probe `json:"probes"`
}

func main() {
	path := flag.String("config", "connector.json", "connector configuration")
	flag.Parse()
	raw, err := os.ReadFile(*path)
	if err != nil {
		log.Fatal(err)
	}
	var c Config
	if json.Unmarshal(raw, &c) != nil {
		log.Fatal("invalid config")
	}
	credential := os.Getenv("CONNECTOR_CREDENTIAL")
	if credential == "" {
		log.Fatal("CONNECTOR_CREDENTIAL required from secrets manager")
	}
	u, err := url.Parse(c.ControlPlaneURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		log.Fatal("control plane must use HTTPS")
	}
	if c.TenantID == "" || c.ConnectorID == "" || len(c.Probes) > 100 {
		log.Fatal("connector identity or probe list invalid")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		send(ctx, client, c, credential)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func send(ctx context.Context, client *http.Client, c Config, credential string) {
	components := make([]map[string]any, 0, len(c.Probes))
	for _, p := range c.Probes {
		state := "unknown"
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		started := time.Now()
		if err := check(probeCtx, p); err == nil {
			state = "healthy"
		} else {
			state = "degraded"
		}
		latency := float64(time.Since(started).Microseconds()) / 1000
		cancel()
		components = append(components, map[string]any{"name": p.Name, "kind": p.Kind, "status": state, "observed_at": time.Now().UTC(), "latency_ms": latency})
	}
	payload, _ := json.Marshal(map[string]any{"version": c.Version, "components": components})
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.ControlPlaneURL, "/")+"/v1/connectors/heartbeat", bytes.NewReader(payload))
	if err != nil {
		log.Print(err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("heartbeat failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("heartbeat rejected: status %d", resp.StatusCode)
	}
}
func check(ctx context.Context, p Probe) error {
	if p.Name == "" || p.Kind == "" || p.Port < 1 || p.Port > 65535 || len(p.AllowedCIDRs) == 0 {
		return errors.New("invalid probe")
	}
	networks := []*net.IPNet{}
	for _, c := range p.AllowedCIDRs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return err
		}
		networks = append(networks, n)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, p.Host)
	if err != nil {
		return err
	}
	for _, a := range ips {
		allowed := false
		for _, n := range networks {
			if n.Contains(a.IP) {
				allowed = true
			}
		}
		if !allowed {
			continue
		}
		d := net.Dialer{Timeout: 2 * time.Second}
		conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(a.IP.String(), fmt.Sprint(p.Port)))
		if err == nil {
			conn.Close()
			return nil
		}
	}
	return errors.New("no approved address responded")
}
