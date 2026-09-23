package platform

import (
	"encoding/base64"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL           string
	TenantReadDatabaseURL string
	Listen                string
	Mode                  string
	OIDCIssuer            string
	OIDCAudience          string
	PublicURL             string
	InfluxURL             string
	InfluxToken           string
	InfluxDatabase        string
	TurnstileSecret       string
	TurnstileHostname     string
	TurnstileAction       string
	EmailMode             string
	EmailProviderURL      string
	EmailProviderToken    string
	ProxyCIDRs            []string
	LogHotDir             string
	LogArchiveDir         string
	LogKeyB64             string
	LogHotMarker          string
	LogArchiveMarker      string
}

func LoadConfig() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), TenantReadDatabaseURL: os.Getenv("TENANT_READ_DATABASE_URL"), Listen: env("LISTEN_ADDR", ":8080"), Mode: env("APP_MODE", "production"), OIDCIssuer: strings.TrimRight(os.Getenv("OIDC_ISSUER"), "/"), OIDCAudience: os.Getenv("OIDC_AUDIENCE"), PublicURL: env("PUBLIC_URL", "http://localhost:3000"), InfluxURL: os.Getenv("INFLUX_URL"), InfluxToken: os.Getenv("INFLUX_TOKEN"), InfluxDatabase: os.Getenv("INFLUX_DATABASE"), TurnstileSecret: os.Getenv("TURNSTILE_SECRET"), TurnstileHostname: os.Getenv("TURNSTILE_HOSTNAME"), TurnstileAction: env("TURNSTILE_ACTION", "contact"), EmailMode: env("EMAIL_MODE", "disabled"), EmailProviderURL: os.Getenv("EMAIL_PROVIDER_URL"), EmailProviderToken: os.Getenv("EMAIL_PROVIDER_TOKEN"), ProxyCIDRs: strings.Split(os.Getenv("TRUSTED_PROXY_CIDRS"), ","), LogHotDir: os.Getenv("LOG_HOT_DIR"), LogArchiveDir: os.Getenv("LOG_ARCHIVE_DIR"), LogKeyB64: os.Getenv("LOG_KEY_B64"), LogHotMarker: os.Getenv("LOG_HOT_MARKER"), LogArchiveMarker: os.Getenv("LOG_ARCHIVE_MARKER")}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if c.Mode == "demo" {
		host, _, e := net.SplitHostPort(c.Listen)
		if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return c, errors.New("demo API must bind to a loopback address")
		}
	}
	if c.Mode != "demo" && (c.OIDCIssuer == "" || c.OIDCAudience == "") {
		return c, errors.New("OIDC_ISSUER and OIDC_AUDIENCE are required")
	}
	if c.Mode != "demo" && (!strings.HasPrefix(c.OIDCIssuer, "https://") || !strings.HasPrefix(c.PublicURL, "https://")) {
		return c, errors.New("production OIDC issuer and PUBLIC_URL must use HTTPS")
	}
	if c.EmailMode != "disabled" && c.EmailMode != "mock" && c.EmailMode != "http" {
		return c, errors.New("unsupported email adapter; fail closed")
	}
	if c.EmailMode == "http" && (!strings.HasPrefix(c.EmailProviderURL, "https://") || c.EmailProviderToken == "") {
		return c, errors.New("HTTPS email adapter and token required")
	}
	if c.LogHotDir != "" || c.LogArchiveDir != "" || c.LogKeyB64 != "" || c.LogHotMarker != "" || c.LogArchiveMarker != "" {
		key, err := base64.StdEncoding.DecodeString(c.LogKeyB64)
		if err != nil || len(key) != 32 || !filepath.IsAbs(c.LogHotDir) || !validStoreMarker(c.LogHotMarker) {
			return c, errors.New("log storage requires an absolute hot directory, a base64 32-byte key, and a store marker")
		}
		if c.LogArchiveDir == "" {
			if c.LogArchiveMarker != "" {
				return c, errors.New("archive marker requires an archive directory")
			}
		} else if !filepath.IsAbs(c.LogArchiveDir) || pathsOverlap(c.LogHotDir, c.LogArchiveDir) || !validStoreMarker(c.LogArchiveMarker) || c.LogArchiveMarker == c.LogHotMarker {
			return c, errors.New("log archive requires a separate absolute directory and distinct marker")
		}
	}
	return c, nil
}

func pathsOverlap(a, b string) bool {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, err := filepath.Rel(filepath.Clean(pair[0]), filepath.Clean(pair[1]))
		if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

var httpTimeout = 8 * time.Second
