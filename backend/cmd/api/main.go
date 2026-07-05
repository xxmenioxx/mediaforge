package main

import (
	"log"

	"github.com/anuelvs/mediaforge/backend/internal/config"
	"github.com/anuelvs/mediaforge/backend/internal/database"
	"github.com/anuelvs/mediaforge/backend/internal/handlers"
	"github.com/anuelvs/mediaforge/backend/internal/routes"
)

func main() {
	cfg := config.Load()

	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	if err := database.Seed(db); err != nil {
		log.Fatalf("seed database: %v", err)
	}

	router := routes.New(db)
	handlers.StartAutoWorker(db)

	if err := router.Run(cfg.Address()); err != nil {
		log.Fatalf("run api: %v", err)
	}
}
