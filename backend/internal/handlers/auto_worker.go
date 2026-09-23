package handlers

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/gorm"
)

type AutoWorker struct {
	handler WorkerHandler
	mu      sync.Mutex
	stop    chan struct{}
}

func StartAutoWorker(db *gorm.DB) *AutoWorker {
	worker := &AutoWorker{
		handler: NewWorkerHandler(db),
		stop:    make(chan struct{}),
	}
	if limits, err := worker.handler.workerLimits(); err == nil {
		_ = worker.handler.heartbeatWorker(limits.DefaultWorkerName, limits, nil, "")
	}
	go worker.run()
	return worker
}

func (w *AutoWorker) Stop() {
	close(w.stop)
}

func (w *AutoWorker) run() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.tick()
		case <-w.stop:
			return
		}
	}
}

func (w *AutoWorker) tick() {
	if !w.mu.TryLock() {
		return
	}
	defer w.mu.Unlock()

	limits, err := w.handler.workerLimits()
	if err != nil {
		log.Printf("auto worker settings error: %v", err)
		appendSystemLog(w.handler.db, "auto_worker_settings_failed", nil, err)
		return
	}
	workerName := canonicalWorkerName(limits.DefaultWorkerName, "")
	if err := w.handler.heartbeatWorker(workerName, limits, nil, ""); err != nil {
		log.Printf("auto worker heartbeat error: %v", err)
		appendSystemLog(w.handler.db, "worker_heartbeat_failed", map[string]string{"worker": workerName}, err)
		return
	}
	if !limits.AutoWorkerEnabled {
		return
	}
	if !scheduler.AutoExecutionEnabled(w.handler.db) {
		return
	}

	for {
		job, err := w.handler.claimNextJob(workerName, nil, "")
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, errWorkerLimitReached) || errors.Is(err, errWorkerDelayActive) || errors.Is(err, errWorkerProgressPacingActive) {
				return
			}
			log.Printf("auto worker claim error: %v", err)
			appendSystemLog(w.handler.db, "auto_worker_claim_failed", map[string]string{"worker": workerName}, err)
			return
		}

		if _, _, err := w.handler.executeQueueJob(job, true); err != nil {
			w.handler.failClaimedJobExecution(job.ID, err)
			log.Printf("auto worker execute job %d error: %v", job.ID, err)
			appendSystemLog(w.handler.db, "auto_worker_execution_failed", map[string]string{"worker": workerName, "jobId": fmt.Sprint(job.ID)}, err)
			return
		}
	}
}
