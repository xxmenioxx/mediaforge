package handlers

import (
	"errors"
	"log"
	"sync"
	"time"

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
		return
	}
	if !limits.AutoWorkerEnabled {
		return
	}

	for {
		job, err := w.handler.claimNextJob(limits.DefaultWorkerName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, errWorkerLimitReached) || errors.Is(err, errWorkerDelayActive) {
				return
			}
			log.Printf("auto worker claim error: %v", err)
			return
		}

		if _, _, err := w.handler.executeQueueJob(job, true); err != nil {
			log.Printf("auto worker execute job %d error: %v", job.ID, err)
			return
		}
	}
}
