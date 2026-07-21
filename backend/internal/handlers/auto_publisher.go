package handlers

import (
	"sync"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/gorm"
)

type AutoPublisher struct {
	db   *gorm.DB
	mu   sync.Mutex
	stop chan struct{}
}

func StartAutoPublisher(db *gorm.DB) *AutoPublisher {
	publisher := &AutoPublisher{db: db, stop: make(chan struct{})}
	go publisher.run()
	return publisher
}

func (p *AutoPublisher) Stop() { close(p.stop) }

func (p *AutoPublisher) run() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.tick()
		case <-p.stop:
			return
		}
	}
}

func (p *AutoPublisher) tick() {
	if !p.mu.TryLock() {
		return
	}
	defer p.mu.Unlock()
	if !pipelineAutomationSettings(p.db).AutoPublisherEnabled {
		return
	}
	p.reconcilePublishedJobs()
	var jobs []models.QueueJob
	if err := p.db.Where("status = ? AND published_at IS NULL AND validation_status IN ?", JobStatusCompleted, []string{ValidationStatusPassed, ValidationStatusWarning}).Order("updated_at asc").Limit(10).Find(&jobs).Error; err != nil {
		return
	}
	for _, job := range jobs {
		_, _ = (PublisherHandler{db: p.db}).publishQueueJob(job, false)
	}
}

func (p *AutoPublisher) reconcilePublishedJobs() {
	var jobs []models.QueueJob
	if err := p.db.Where("published_at IS NOT NULL AND published_path <> '' AND publication_retired_at IS NULL").Order("published_at desc").Limit(100).Find(&jobs).Error; err != nil {
		return
	}
	for _, job := range jobs {
		changed := false
		if archivedPath, err := (PublisherHandler{db: p.db}).archivePublishedOriginal(job); err != nil {
			job.Notes = appendNote(job.Notes, "Published job reconciliation archive warning: "+err.Error())
			changed = true
		} else if archivedPath != "" {
			job.Notes = appendNote(job.Notes, "Published job reconciled; original archived: "+archivedPath)
			changed = true
		}
		if job.OutputPath != "" {
			if err := (PublisherHandler{db: p.db}).cleanupStagedJob(job); err != nil {
				job.Notes = appendNote(job.Notes, "Published job reconciliation cleanup warning: "+err.Error())
				changed = true
			}
		}
		if changed {
			_ = p.db.Save(&job).Error
		}
	}
}
