package platform

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Actor struct {
	Subject string
	Roles   map[string]bool
	IP      string
}
type actorKey struct{}

func actorFrom(ctx context.Context) Actor { a, _ := ctx.Value(actorKey{}).(Actor); return a }
func (a Actor) has(role string) bool      { return a.Roles[role] || a.Roles["platform_admin"] }
func (a Actor) platform() bool {
	return a.Roles["platform_admin"] || a.Roles["ops_engineer"] || a.Roles["audit_reviewer"] || a.Roles["read_only_operator"]
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}
type keySet struct {
	Keys []jwk `json:"keys"`
}
type OIDCVerifier struct {
	issuer, audience string
	client           *http.Client
	mu               sync.Mutex
	keys             keySet
	until            time.Time
	jwksURL          string
}

func NewOIDCVerifier(issuer, audience string) *OIDCVerifier {
	return &OIDCVerifier{issuer: issuer, audience: audience, client: &http.Client{Timeout: httpTimeout}}
}
func (v *OIDCVerifier) refresh(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Now().Before(v.until) {
		return nil
	}
	if v.jwksURL == "" {
		req, _ := http.NewRequestWithContext(ctx, "GET", v.issuer+"/.well-known/openid-configuration", nil)
		resp, err := v.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("discovery status %d", resp.StatusCode)
		}
		var d struct {
			Issuer  string `json:"issuer"`
			JwksURI string `json:"jwks_uri"`
		}
		if err = json.NewDecoder(resp.Body).Decode(&d); err != nil {
			return err
		}
		if d.Issuer != v.issuer || !strings.HasPrefix(d.JwksURI, v.issuer+"/") {
			return errors.New("OIDC discovery mismatch")
		}
		v.jwksURL = d.JwksURI
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", v.jwksURL, nil)
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	var ks keySet
	if err = json.NewDecoder(resp.Body).Decode(&ks); err != nil {
		return err
	}
	v.keys = ks
	v.until = time.Now().Add(5 * time.Minute)
	return nil
}
func (v *OIDCVerifier) Verify(ctx context.Context, token string) (Actor, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Actor{}, errors.New("invalid JWT")
	}
	decode := func(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }
	hb, err := decode(parts[0])
	if err != nil {
		return Actor{}, err
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if json.Unmarshal(hb, &h) != nil || h.Alg != "RS256" || h.Kid == "" {
		return Actor{}, errors.New("unsupported JWT header")
	}
	if err = v.refresh(ctx); err != nil {
		return Actor{}, err
	}
	v.mu.Lock()
	keys := v.keys.Keys
	v.mu.Unlock()
	var key *rsa.PublicKey
	for _, k := range keys {
		if k.Kid == h.Kid && k.Kty == "RSA" && (k.Alg == "" || k.Alg == "RS256") {
			nb, e1 := decode(k.N)
			eb, e2 := decode(k.E)
			if e1 != nil || e2 != nil {
				return Actor{}, errors.New("invalid JWK")
			}
			key = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(new(big.Int).SetBytes(eb).Int64())}
			break
		}
	}
	if key == nil {
		return Actor{}, errors.New("unknown JWT key")
	}
	sig, err := decode(parts[2])
	if err != nil {
		return Actor{}, err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig) != nil {
		return Actor{}, errors.New("bad signature")
	}
	pb, err := decode(parts[1])
	if err != nil {
		return Actor{}, err
	}
	var p struct {
		Issuer    string          `json:"iss"`
		Subject   string          `json:"sub"`
		Audience  json.RawMessage `json:"aud"`
		Expiry    int64           `json:"exp"`
		NotBefore int64           `json:"nbf"`
		Realm     struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
	}
	if json.Unmarshal(pb, &p) != nil {
		return Actor{}, errors.New("invalid claims")
	}
	if p.Issuer != v.issuer || p.Subject == "" || p.Expiry <= time.Now().Unix() || p.NotBefore > time.Now().Unix()+30 {
		return Actor{}, errors.New("invalid JWT claims")
	}
	var aud string
	var auds []string
	_ = json.Unmarshal(p.Audience, &aud)
	_ = json.Unmarshal(p.Audience, &auds)
	found := aud == v.audience
	for _, x := range auds {
		found = found || x == v.audience
	}
	if !found {
		return Actor{}, errors.New("wrong audience")
	}
	a := Actor{Subject: p.Subject, Roles: map[string]bool{}}
	for _, r := range p.Realm.Roles {
		a.Roles[r] = true
	}
	return a, nil
}
func requestIP(r *http.Request, cidrs []string) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	networks := []*net.IPNet{}
	for _, c := range cidrs {
		_, network, e := net.ParseCIDR(strings.TrimSpace(c))
		if e == nil {
			networks = append(networks, network)
		}
	}
	trusted := func(candidate net.IP) bool {
		for _, network := range networks {
			if network.Contains(candidate) {
				return true
			}
		}
		return false
	}
	if !trusted(ip) {
		return ip.String()
	}
	chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(chain) - 1; i >= 0; i-- {
		candidate := net.ParseIP(strings.TrimSpace(chain[i]))
		if candidate == nil {
			return ip.String()
		}
		if !trusted(candidate) {
			return candidate.String()
		}
	}
	return ip.String()
}
