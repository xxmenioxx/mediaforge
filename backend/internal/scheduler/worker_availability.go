package scheduler

import (
	"fmt"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const WorkerHeartbeatTimeout = 15 * time.Second

type WorkerAvailabilityDecision struct {
	Available bool     `json:"available"`
	Worker    string   `json:"worker"`
	Reasons   []string `json:"reasons"`
}

func EvaluateWorkerAvailability(db *gorm.DB, plan models.ExecutionPlan, now time.Time) (WorkerAvailabilityDecision, error) {
	var workers []models.WorkerNode
	if err := db.Where("status = ? AND last_seen_at >= ?", "online", now.Add(-WorkerHeartbeatTimeout)).Order("name asc").Find(&workers).Error; err != nil {
		return WorkerAvailabilityDecision{}, err
	}
	if len(workers) == 0 {
		return WorkerAvailabilityDecision{Reasons: []string{"No online worker has a recent heartbeat"}}, nil
	}
	for _, worker := range workers {
		var active int64
		if err := db.Model(&models.SchedulerReservation{}).Where("state = ? AND worker_name = ?", ReservationStateActive, worker.Name).Count(&active).Error; err != nil {
			return WorkerAvailabilityDecision{}, err
		}
		if worker.MaxConcurrentJobs > 0 && active >= int64(worker.MaxConcurrentJobs) {
			continue
		}
		if workerSupportsEncoder(worker, plan.SelectedEncoder) {
			return WorkerAvailabilityDecision{Available: true, Worker: worker.Name, Reasons: []string{fmt.Sprintf("Worker %s is online and supports %s", worker.Name, plan.SelectedEncoder)}}, nil
		}
	}
	return WorkerAvailabilityDecision{Reasons: []string{"Online workers are at capacity or do not support the selected encoder"}}, nil
}

func workerSupportsEncoder(worker models.WorkerNode, encoder string) bool {
	if encoder == "" || encoder == "copy" {
		return true
	}
	for _, raw := range worker.Encoders {
		if value, ok := raw.(string); ok && value == encoder {
			return true
		}
	}
	return false
}
