package main

import (
	"log"

	"github.com/anuelvs/mediaforge/backend/internal/config"
	"github.com/anuelvs/mediaforge/backend/internal/database"
	"github.com/anuelvs/mediaforge/backend/internal/handlers"
	"github.com/anuelvs/mediaforge/backend/internal/routes"
	"github.com/anuelvs/mediaforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mediaforge/backend/internal/scheduler"
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

	if _, err := runtimeinfo.DetectAndSave(db); err != nil {
		log.Printf("detect runtime: %v", err)
	}
	if report, err := handlers.RecoverSchedulerState(db); err != nil {
		log.Printf("recover scheduler state: %v", err)
	} else {
		log.Printf("scheduler recovery: %d interrupted jobs, %d released reservations, %d orphan workspaces", report.InterruptedJobs, report.ReservationsReleased, len(report.OrphanWorkspacePaths))
	}
	router := routes.New(db)
	runtimeinfo.StartDetector(db)
	scheduler.StartReviewPlanner(db)
	handlers.StartAutoWorker(db)
	handlers.StartAutoPublisher(db)
	handlers.StartAutoHousekeeper(db)
	handlers.StartAssetInventorySyncer(db)

	if err := router.Run(cfg.Address()); err != nil {
		log.Fatalf("run api: %v", err)
	}
}
