package main

import (
	"context"
	"log"
	"nocturn.example/aegis-operations/internal/platform"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	c, err := platform.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := platform.Open(ctx, c.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.DB.Close()
	(&platform.Worker{Store: store, Config: c}).Run(ctx)
}
