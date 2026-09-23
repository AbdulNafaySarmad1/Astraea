package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func collectProbe(ctx context.Context, p Probe) ComponentSample {
	start := time.Now()
	sample := ComponentSample{Name: p.Name, Kind: p.Kind, Status: "degraded", ObservedAt: start.UTC()}
	var err error
	switch p.Kind {
	case "postgres":
		sample.Metrics, err = collectPostgres(ctx, p)
	case "valkey":
		sample.Metrics, err = collectValkey(ctx, p)
	default:
		err = check(ctx, p)
	}
	latency := float64(time.Since(start).Microseconds()) / 1000
	sample.LatencyMS = &latency
	if err == nil {
		sample.Status = "healthy"
	}
	return sample
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func collectPostgres(ctx context.Context, p Probe) ([]MetricSample, error) {
	dsn := os.Getenv(p.CredentialEnv)
	if dsn == "" {
		return nil, errors.New("PostgreSQL monitoring credential unavailable")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL monitoring connection")
	}
	if !strings.EqualFold(config.Host, p.Host) || int(config.Port) != p.Port || len(config.Fallbacks) != 0 {
		return nil, errors.New("PostgreSQL connection target differs from approved probe")
	}
	if !loopbackHost(p.Host) && (config.TLSConfig == nil || config.TLSConfig.InsecureSkipVerify) {
		return nil, errors.New("PostgreSQL monitoring requires verified TLS outside loopback")
	}
	config.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil || !strings.EqualFold(host, p.Host) || port != strconv.Itoa(p.Port) {
			return nil, errors.New("PostgreSQL attempted an unapproved target")
		}
		return approvedDial(ctx, p)
	}
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	var replica bool
	var connections, rollbacks int64
	err = conn.QueryRow(ctx, "SELECT pg_is_in_recovery(),numbackends::bigint,xact_rollback FROM pg_stat_database WHERE datname=current_database()").Scan(&replica, &connections, &rollbacks)
	if err != nil {
		return nil, err
	}
	metrics := []MetricSample{{Name: "postgres_connections", Value: float64(connections), Unit: "connections"}, {Name: "postgres_rollbacks_total", Value: float64(rollbacks), Unit: "transactions"}}
	if replica {
		var backlog float64
		err = conn.QueryRow(ctx, "SELECT coalesce(pg_wal_lsn_diff(pg_last_wal_receive_lsn(),pg_last_wal_replay_lsn()),0)::double precision").Scan(&backlog)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, MetricSample{Name: "postgres_replay_backlog_bytes", Value: backlog, Unit: "bytes"})
	} else {
		var count int64
		var backlog float64
		err = conn.QueryRow(ctx, "SELECT count(*)::bigint,coalesce(max(pg_wal_lsn_diff(pg_current_wal_lsn(),replay_lsn)),0)::double precision FROM pg_stat_replication").Scan(&count, &backlog)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, MetricSample{Name: "postgres_replica_count", Value: float64(count), Unit: "replicas"}, MetricSample{Name: "postgres_replay_backlog_bytes", Value: backlog, Unit: "bytes"})
		if count < int64(p.MinReplicas) {
			return metrics, errors.New("required PostgreSQL replicas missing")
		}
	}
	return metrics, nil
}

func collectValkey(ctx context.Context, p Probe) ([]MetricSample, error) {
	password := os.Getenv(p.CredentialEnv)
	if password == "" {
		return nil, errors.New("Valkey monitoring credential unavailable")
	}
	conn, err := approvedDial(ctx, p)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if p.TLS {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: p.Host, MinVersion: tls.VersionTLS13})
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = tlsConn
	}
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	reader := bufio.NewReaderSize(conn, 128)
	if p.Username == "" {
		_, err = respCommand(conn, reader, "AUTH", password)
	} else {
		_, err = respCommand(conn, reader, "AUTH", p.Username, password)
	}
	if err != nil {
		return nil, err
	}
	if pong, err := respCommand(conn, reader, "PING"); err != nil || pong != "PONG" {
		return nil, errors.New("Valkey PING failed")
	}
	values := map[string]string{}
	for _, section := range []string{"memory", "clients", "stats", "replication"} {
		body, err := respCommand(conn, reader, "INFO", section)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(body, "\n") {
			key, value, found := strings.Cut(strings.TrimSpace(line), ":")
			if found {
				values[key] = strings.TrimSpace(value)
			}
		}
	}
	metrics := make([]MetricSample, 0, 3)
	for name, key := range map[string]string{"valkey_used_memory_bytes": "used_memory", "valkey_connected_clients": "connected_clients", "valkey_evicted_keys_total": "evicted_keys"} {
		value, err := strconv.ParseFloat(values[key], 64)
		if err != nil || value < 0 {
			return nil, errors.New("Valkey INFO missing required numeric field")
		}
		unit := "count"
		if key == "used_memory" {
			unit = "bytes"
		}
		metrics = append(metrics, MetricSample{Name: name, Value: value, Unit: unit})
	}
	if values["role"] == "slave" && values["master_link_status"] != "up" {
		return metrics, errors.New("Valkey replica link is down")
	}
	return metrics, nil
}

func respCommand(conn net.Conn, reader *bufio.Reader, parts ...string) (string, error) {
	if _, err := fmt.Fprintf(conn, "*%d\r\n", len(parts)); err != nil {
		return "", err
	}
	for _, part := range parts {
		if _, err := fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(part), part); err != nil {
			return "", err
		}
	}
	responseLine, err := reader.ReadSlice('\n')
	if err != nil || len(responseLine) < 3 || len(responseLine) > 128 {
		return "", errors.New("invalid Valkey response")
	}
	line := strings.TrimSuffix(strings.TrimSuffix(string(responseLine), "\n"), "\r")
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return "", errors.New("Valkey rejected monitoring command")
	case '$':
		length, err := strconv.Atoi(line[1:])
		if err != nil || length < 0 || length > 65536 {
			return "", errors.New("Valkey response too large")
		}
		buffer := make([]byte, length+2)
		if _, err = io.ReadFull(reader, buffer); err != nil || string(buffer[length:]) != "\r\n" {
			return "", errors.New("incomplete Valkey response")
		}
		return string(buffer[:length]), nil
	default:
		return "", errors.New("unsupported Valkey response")
	}
}
