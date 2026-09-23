package platform

import (
	"math"
	"testing"
	"time"
)

func TestTelemetryValidationBindsCapabilityAndNumericLimits(t *testing.T) {
	now := time.Now().UTC()
	base := incomingHeartbeat{BatchID: "0123456789abcdef0123456789abcdef", Version: "1", Components: []incomingComponent{{Name: "database", Kind: "postgres", Status: "healthy", ObservedAt: now, Metrics: []incomingMetric{{Name: "postgres_connections", Value: 3, Unit: "connections"}}}}}
	if !base.validate([]string{"postgres_health"}, now) {
		t.Fatal("valid scoped metric rejected")
	}
	if base.validate([]string{"tcp_health"}, now) {
		t.Fatal("connector sent signal outside recorded capability")
	}
	base.Components[0].Metrics[0].Value = math.NaN()
	if base.validate([]string{"postgres_health"}, now) {
		t.Fatal("non-finite metric accepted")
	}
	base.Components[0].Metrics[0].Value = 3
	base.Components[0].ObservedAt = now.Add(-9 * 24 * time.Hour)
	if base.validate([]string{"postgres_health"}, now) {
		t.Fatal("sample outside replay window accepted")
	}
}
