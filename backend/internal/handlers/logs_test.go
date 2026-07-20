package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLogFileCategoriesCoverSystemAndPipelineSources(t *testing.T) {
	tests := map[string]string{"backend.log": "backend", "system.log": "system", "scheduler.log": "scheduler", "workers.log": "workers", "pipeline.log": "pipeline", "jobs.log": "jobs", "job-42.log": "jobs"}
	for name, expected := range tests {
		if actual := logFileCategory(name); actual != expected {
			t.Fatalf("%s category=%s expected=%s", name, actual, expected)
		}
	}
}

func TestConsolidatedLogsExposeSchedulerWorkerAndPipelineState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:consolidated-logs?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ExecutionPlan{}, &models.SchedulerReservation{}, &models.RuntimeSnapshot{}, &models.WorkerNode{}, &models.QueueJob{}); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/raw/show.mkv", Status: JobStatusRunning, Stage: JobStageConverting, WorkerName: "local", Priority: 1, StageHistory: models.JSONList{models.JSONMap{"stage": JobStageConverting}}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ExecutionPlan{JobID: job.ID, Version: 1, Status: "dispatched", SelectedEncoder: "libx265"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.WorkerNode{Name: "local", Status: "online", MaxConcurrentJobs: 1, LastSeenAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(schedulerLog(db), "libx265") || !strings.Contains(workersLog(db), "worker=local") || !strings.Contains(pipelineLog([]models.QueueJob{job}), "converting") {
		t.Fatal("consolidated subsystem state is missing")
	}
}
