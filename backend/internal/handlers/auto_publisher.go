package handlers

import (
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

type AutoPublisher struct {
	db   *gorm.DB
	mu   sync.Mutex
	stop chan struct{}
}

func StartAutoPublisher(db *gorm.DB) *AutoPublisher {
	publisher := &AutoPublisher{db: db, stop: make(chan struct{})}
	go func() {
		publisher.tick()
		publisher.run()
	}()
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
	automation := pipelineAutomationSettings(p.db)
	// Durable Publisher recovery is part of the existing AutoPublisher cadence;
	// it queries persisted publication rows and never walks Library or RAW roots.
	p.recoverPublisherPublications()
	p.dispatchAssetPublishedOutbox()
	if automation.PublishedJobReconciliationEnabled {
		p.reconcilePublishedJobs()
	}
	if !automation.AutoPublisherEnabled {
		return
	}
	p.recoverUnattemptedValidationHandoffs()
	var jobs []models.QueueJob
	if err := p.db.Where(`status = ? AND published_at IS NULL AND (
		(stage = ? AND TRIM(validation_run_id) <> '' AND (publication_state = '' OR publication_state = ?))
		OR EXISTS (
			SELECT 1 FROM publisher_publications AS publication
			WHERE publication.queue_job_id = queue_jobs.id
			AND publication.execution_number = queue_jobs.execution_number
			AND publication.state = ?
		)
	)`, JobStatusCompleted, JobStageReadyToPublish, PublicationReady, PublicationReady).Order("updated_at asc").Limit(10).Find(&jobs).Error; err != nil {
		return
	}
	for _, job := range jobs {
		_, _ = (PublisherHandler{db: p.db}).publishQueueJobWithMode(job, false, true)
	}
}

func (p *AutoPublisher) recoverUnattemptedValidationHandoffs() {
	var publications []models.PublisherPublication
	if err := p.db.Where("state = ? AND failure_code = ? AND validation_run_id = '' AND attempt_count = 0 AND fencing_generation = 0 AND published_at IS NULL AND original_archive_status = ?", PublicationBlocked, "identity_changed", "pending").Order("updated_at asc").Limit(10).Find(&publications).Error; err != nil {
		return
	}
	for _, publication := range publications {
		var job models.QueueJob
		if err := p.db.First(&job, publication.QueueJobID).Error; err != nil || job.Status != JobStatusCompleted || job.Stage != JobStageReadyToPublish || job.ExecutionNumber == nil || *job.ExecutionNumber != publication.ExecutionNumber || strings.TrimSpace(job.ValidationRunID) == "" {
			continue
		}
		_, _ = (PublisherHandler{db: p.db}).publishQueueJobWithMode(job, false, true)
	}
}

func (p *AutoPublisher) reconcilePublishedJobs() {
	var jobs []models.QueueJob
	if err := p.db.Where("published_at IS NOT NULL AND published_path <> '' AND publication_retired_at IS NULL").Order("published_at desc").Limit(100).Find(&jobs).Error; err != nil {
		return
	}
	for _, job := range jobs {
		changed := false
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
