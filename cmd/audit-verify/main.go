package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"nocturn.example/aegis-operations/internal/platform"
	"os"
	"strings"
	"time"
)

type event struct {
	id, actor, kind, resource, correlation, outcome, prev, hash string
	detail                                                      []byte
	at                                                          time.Time
}

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL required")
	}
	ctx := context.Background()
	s, err := platform.Open(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer s.DB.Close()
	rows, err := s.DB.Query(ctx, "SELECT id,actor,event_type,resource,correlation_id,outcome,detail,occurred_at,previous_hash,event_hash FROM audit_events")
	if err != nil {
		log.Fatal(err)
	}
	byPrev := map[string]event{}
	count := 0
	for rows.Next() {
		var e event
		if err = rows.Scan(&e.id, &e.actor, &e.kind, &e.resource, &e.correlation, &e.outcome, &e.detail, &e.at, &e.prev, &e.hash); err != nil {
			log.Fatal(err)
		}
		if _, exists := byPrev[e.prev]; exists {
			log.Fatal("audit chain fork")
		}
		byPrev[e.prev] = e
		count++
	}
	rows.Close()
	var head string
	if err = s.DB.QueryRow(ctx, "SELECT event_hash FROM audit_chain_head WHERE id=true").Scan(&head); err != nil {
		log.Fatal(err)
	}
	current := strings.Repeat("0", 64)
	visited := 0
	for {
		e, ok := byPrev[current]
		if !ok {
			break
		}
		var value any
		if json.Unmarshal(e.detail, &value) != nil {
			log.Fatal("invalid audit detail")
		}
		canonical, _ := json.Marshal(value)
		material := strings.Join([]string{current, e.id, e.actor, e.kind, e.resource, e.correlation, e.outcome, e.at.UTC().Format(time.RFC3339Nano), string(canonical)}, "|")
		sum := sha256.Sum256([]byte(material))
		if hex.EncodeToString(sum[:]) != e.hash {
			log.Fatalf("audit hash mismatch at %s", e.id)
		}
		current = e.hash
		visited++
	}
	if visited != count || current != head {
		log.Fatalf("audit chain incomplete: visited %d of %d", visited, count)
	}
	fmt.Printf("verified %d audit events; head %s\n", count, head)
}
