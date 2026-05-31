package main

import (
	"context"
	"log"

	"github.com/nomenclator/houston2/internal/server"
)

func main() {
	cfg := server.LoadConfig()
	q := server.NewSupabaseQuerier(cfg.SupabaseURL, cfg.AnonKey)
	srv := server.New(cfg, q)

	if err := srv.Run(context.Background()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
