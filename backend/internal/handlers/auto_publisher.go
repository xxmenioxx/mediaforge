package handlers

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/applog"
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
		if err := publisher.tick(); err != nil {
			applog.Event("error", "auto_publisher", "tick_failed", nil, err)
		}
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
			if err := p.tick(); err != nil {
				applog.Event("error", "auto_publisher", "tick_failed", nil, err)
			}
		case <-p.stop:
			return
		}
	}
}

func (p *AutoPublisher) tick() error {
	if !p.mu.TryLock() {
		return nil
	}
	defer p.mu.Unlock()
	var tickErrors []error
	automation := pipelineAutomationSettings(p.db)
	// Durable Publisher recovery is part of the existing AutoPublisher cadence;
	// it queries persisted publication rows and never walks Library or RAW roots.
	if err := p.recoverPublisherPublications(); err != nil {
		tickErrors = append(tickErrors, err)
	}
	p.dispatchAssetPublishedOutbox()
	if automation.PublishedJobReconciliationEnabled {
		p.reconcilePublishedJobs()
	}
	if !automation.AutoPublisherEnabled {
		return errors.Join(tickErrors...)
	}
	if err := p.recoverUnattemptedValidationHandoffs(); err != nil {
		tickErrors = append(tickErrors, err)
	}
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
		return errors.Join(append(tickErrors, fmt.Errorf("load ready publications: %w", err))...)
	}
	for _, job := range jobs {
		if _, err := (PublisherHandler{db: p.db}).publishQueueJobWithMode(job, false, true); err != nil {
			tickErrors = append(tickErrors, fmt.Errorf("auto-publish queue job %d: %w", job.ID, err))
		}
	}
	return errors.Join(tickErrors...)
}

func (p *AutoPublisher) recoverUnattemptedValidationHandoffs() error {
	var recoveryErrors []error
	var publications []models.PublisherPublication
	if err := p.db.Where("state = ? AND failure_code = ? AND validation_run_id = '' AND attempt_count = 0 AND fencing_generation = 0 AND published_at IS NULL AND original_archive_status = ?", PublicationBlocked, "identity_changed", "pending").Order("updated_at asc").Limit(10).Find(&publications).Error; err != nil {
		return fmt.Errorf("load unattempted validation handoffs: %w", err)
	}
	for _, publication := range publications {
		var job models.QueueJob
		if err := p.db.First(&job, publication.QueueJobID).Error; err != nil || job.Status != JobStatusCompleted || job.Stage != JobStageReadyToPublish || job.ExecutionNumber == nil || *job.ExecutionNumber != publication.ExecutionNumber || strings.TrimSpace(job.ValidationRunID) == "" {
			continue
		}
		if _, err := (PublisherHandler{db: p.db}).publishQueueJobWithMode(job, false, true); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("recover publication %s: %w", publication.PublicationID, err))
		}
	}
	return errors.Join(recoveryErrors...)
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
