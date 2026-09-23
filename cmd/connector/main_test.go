package main

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestProbeRequiresApprovedCIDR(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	probe := Probe{Name: "local", Kind: "test", Host: "127.0.0.1", Port: port, AllowedCIDRs: []string{"192.0.2.0/24"}}
	if err = check(ctx, probe); err == nil {
		t.Fatal("probe escaped allowed CIDR")
	}
	probe.AllowedCIDRs = []string{"127.0.0.0/8"}
	if err = check(ctx, probe); err != nil {
		t.Fatalf("approved probe failed: %v", err)
	}
}
