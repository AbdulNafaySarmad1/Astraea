//go:build !linux

package main

import (
	"context"
	"time"
)

func collectHost(context.Context) ComponentSample {
	return ComponentSample{Name: "connector-host", Kind: "host", Status: "unknown", ObservedAt: time.Now().UTC()}
}
