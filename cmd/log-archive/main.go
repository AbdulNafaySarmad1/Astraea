package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"nocturn.example/aegis-operations/internal/platform"
)

func main() {
	config, err := platform.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := platform.Open(ctx, config.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.DB.Close()
	if err := (&platform.Worker{Store: store, Config: config}).RunArchive(ctx); err != nil {
		log.Fatal(err)
	}
}
