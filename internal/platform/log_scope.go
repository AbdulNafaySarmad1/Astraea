package platform

import (
	"encoding/hex"
	"regexp"
	"slices"
	"strings"
)

type logSourceApproval struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	PathSHA256 string `json:"path_sha256"`
}

var logSourceName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

var enrollmentCapabilities = []string{
	"tcp_health", "host_metrics", "postgres_health", "valkey_health", "service_logs",
}

func validEnrollmentScope(capabilities []string, sources []logSourceApproval) bool {
	if len(capabilities) == 0 || len(capabilities) > len(enrollmentCapabilities) || len(sources) > 16 {
		return false
	}
	seenCapabilities := map[string]bool{}
	for _, capability := range capabilities {
		if !slices.Contains(enrollmentCapabilities, capability) || seenCapabilities[capability] {
			return false
		}
		seenCapabilities[capability] = true
	}
	if len(sources) > 0 && !seenCapabilities["service_logs"] || seenCapabilities["service_logs"] && len(sources) == 0 {
		return false
	}
	seenSources := map[string]bool{}
	for _, source := range sources {
		if !logSourceName.MatchString(source.Name) || seenSources[source.Name] {
			return false
		}
		if len(source.PathSHA256) != 64 || source.PathSHA256 != strings.ToLower(source.PathSHA256) {
			return false
		}
		if _, err := hex.DecodeString(source.PathSHA256); err != nil {
			return false
		}
		if source.Kind != "postgres" && source.Kind != "valkey" && source.Kind != "vault" && source.Kind != "aegiscore" {
			return false
		}
		seenSources[source.Name] = true
	}
	return true
}
