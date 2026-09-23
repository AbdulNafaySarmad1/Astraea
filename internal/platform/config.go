package platform

import (
	"errors"
	"net"
	"os"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL        string
	Listen             string
	Mode               string
	OIDCIssuer         string
	OIDCAudience       string
	PublicURL          string
	InfluxURL          string
	InfluxToken        string
	InfluxDatabase     string
	TurnstileSecret    string
	TurnstileHostname  string
	TurnstileAction    string
	EmailMode          string
	EmailProviderURL   string
	EmailProviderToken string
	ProxyCIDRs         []string
}

func LoadConfig() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), Listen: env("LISTEN_ADDR", ":8080"), Mode: env("APP_MODE", "production"), OIDCIssuer: strings.TrimRight(os.Getenv("OIDC_ISSUER"), "/"), OIDCAudience: os.Getenv("OIDC_AUDIENCE"), PublicURL: env("PUBLIC_URL", "http://localhost:3000"), InfluxURL: os.Getenv("INFLUX_URL"), InfluxToken: os.Getenv("INFLUX_TOKEN"), InfluxDatabase: os.Getenv("INFLUX_DATABASE"), TurnstileSecret: os.Getenv("TURNSTILE_SECRET"), TurnstileHostname: os.Getenv("TURNSTILE_HOSTNAME"), TurnstileAction: env("TURNSTILE_ACTION", "contact"), EmailMode: env("EMAIL_MODE", "disabled"), EmailProviderURL: os.Getenv("EMAIL_PROVIDER_URL"), EmailProviderToken: os.Getenv("EMAIL_PROVIDER_TOKEN"), ProxyCIDRs: strings.Split(os.Getenv("TRUSTED_PROXY_CIDRS"), ",")}
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
	return c, nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

var httpTimeout = 8 * time.Second
