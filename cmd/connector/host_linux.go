//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func collectHost(ctx context.Context) ComponentSample {
	sample := ComponentSample{Name: "connector-host", Kind: "host", Status: "degraded", ObservedAt: time.Now().UTC()}
	first, err := cpuCounters()
	if err != nil {
		return sample
	}
	select {
	case <-ctx.Done():
		return sample
	case <-time.After(200 * time.Millisecond):
	}
	second, err := cpuCounters()
	if err != nil || second.total <= first.total || second.idle < first.idle {
		return sample
	}
	cpuUsed := 100 * float64((second.total-first.total)-(second.idle-first.idle)) / float64(second.total-first.total)
	memUsed, err := memoryUsedPercent()
	if err != nil {
		return sample
	}
	var stat syscall.Statfs_t
	if err = syscall.Statfs("/", &stat); err != nil || stat.Blocks == 0 {
		return sample
	}
	diskUsed := 100 * float64(stat.Blocks-stat.Bavail) / float64(stat.Blocks)
	rx, tx, err := networkCounters()
	if err != nil {
		return sample
	}
	sample.Metrics = []MetricSample{
		{Name: "host_cpu_usage_pct", Value: cpuUsed, Unit: "percent"},
		{Name: "host_memory_used_pct", Value: memUsed, Unit: "percent"},
		{Name: "host_root_disk_used_pct", Value: diskUsed, Unit: "percent"},
		{Name: "host_network_rx_bytes_total", Value: float64(rx), Unit: "bytes"},
		{Name: "host_network_tx_bytes_total", Value: float64(tx), Unit: "bytes"},
	}
	sample.Status = "healthy"
	if cpuUsed >= 90 || memUsed >= 90 || diskUsed >= 90 {
		sample.Status = "degraded"
	}
	return sample
}

type cpuCount struct{ total, idle uint64 }

func cpuCounters() (cpuCount, error) {
	raw, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuCount{}, err
	}
	line := strings.SplitN(string(raw), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuCount{}, errors.New("invalid CPU counters")
	}
	var counters cpuCount
	for i, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuCount{}, err
		}
		counters.total += value
		if i == 3 || i == 4 {
			counters.idle += value
		}
	}
	return counters, nil
}

func memoryUsedPercent() (float64, error) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	var total, available uint64
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "MemTotal:" {
			total, _ = strconv.ParseUint(fields[1], 10, 64)
		}
		if fields[0] == "MemAvailable:" {
			available, _ = strconv.ParseUint(fields[1], 10, 64)
		}
	}
	if total == 0 || available > total {
		return 0, errors.New("invalid memory counters")
	}
	return 100 * float64(total-available) / float64(total), nil
}

func networkCounters() (uint64, uint64, error) {
	raw, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	var rx, tx uint64
	for _, line := range strings.Split(string(raw), "\n") {
		name, values, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(name) == "lo" {
			continue
		}
		fields := strings.Fields(values)
		if len(fields) < 16 {
			return 0, 0, errors.New("invalid network counters")
		}
		input, e1 := strconv.ParseUint(fields[0], 10, 64)
		output, e2 := strconv.ParseUint(fields[8], 10, 64)
		if e1 != nil || e2 != nil {
			return 0, 0, errors.New("invalid network counters")
		}
		rx += input
		tx += output
	}
	return rx, tx, nil
}
