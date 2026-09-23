// Package logsafe applies a conservative second-pass filter to approved
// infrastructure service logs before they leave a customer environment.
package logsafe

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const MaxLineBytes = 8192

var secretPair = regexp.MustCompile(`(?i)\b(password|passwd|token|secret|authorization|api[_-]?key|client[_-]?secret)\b\s*[:=]\s*[^\s,;]+`)
var bearer = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/-]+`)
var email = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// FilterLine returns false for SQL statements, parameters, oversized lines,
// and binary content. Redaction is defense in depth, not a PII guarantee.
func FilterLine(kind, raw string) (string, bool) {
	if len(raw) == 0 || len(raw) > MaxLineBytes || !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) {
		return "", false
	}
	lower := strings.ToLower(raw)
	for _, marker := range []string{"statement:", "execute ", "parameters:", "bind parameters", "query text:", "detail:", "context:", "x-amz-", "signature="} {
		if strings.Contains(lower, marker) {
			return "", false
		}
	}
	if secretPair.MatchString(raw) || bearer.MatchString(raw) || strings.Contains(lower, "private key") {
		return "", false
	}
	line := strings.TrimSpace(raw)
	line = email.ReplaceAllString(line, "[EMAIL REDACTED]")
	if line == "" || len(line) > MaxLineBytes {
		return "", false
	}
	return line, true
}
