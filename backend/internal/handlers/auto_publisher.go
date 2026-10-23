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
	if err := p.processManualPublishRequests(); err != nil {
		tickErrors = append(tickErrors, err)
	}
	if !automation.AutoPublisherEnabled {
		return errors.Join(tickErrors...)
	}
	if err := p.recoverUnattemptedValidationHandoffs(); err != nil {
		tickErrors = append(tickErrors, err)
	}
	var jobs []models.QueueJob
	if err := p.db.Where(`status = ? AND published_at IS NULL
		AND NOT EXISTS (
			SELECT 1 FROM publisher_publications AS manual_publication
			WHERE manual_publication.queue_job_id = queue_jobs.id
			AND manual_publication.execution_number = queue_jobs.execution_number
			AND COALESCE(manual_publication.manual_request_status, '') IN (?, ?)
		)
		AND (
		(stage = ? AND TRIM(validation_run_id) <> '' AND (publication_state = '' OR publication_state = ?))
		OR EXISTS (
			SELECT 1 FROM publisher_publications AS publication
			WHERE publication.queue_job_id = queue_jobs.id
			AND publication.execution_number = queue_jobs.execution_number
			AND publication.state = ?
			)
		)`, JobStatusCompleted, publisherManualRequestAccepted, publisherManualRequestProcessing, JobStageReadyToPublish, PublicationReady, PublicationReady).Order("updated_at asc").Limit(10).Find(&jobs).Error; err != nil {
		return errors.Join(append(tickErrors, fmt.Errorf("load ready publications: %w", err))...)
	}
	for _, job := range jobs {
		if _, err := (PublisherHandler{db: p.db}).publishQueueJobWithMode(job, false, true); err != nil {
			tickErrors = append(tickErrors, fmt.Errorf("auto-publish queue job %d: %w", job.ID, err))
		}
	}
	return errors.Join(tickErrors...)
}

func (p *AutoPublisher) processManualPublishRequests() error {
	var pending []models.PublisherPublication
	if err := p.db.Where("manual_request_status IN ?", []string{publisherManualRequestAccepted, publisherManualRequestProcessing}).Order("manual_requested_at asc").Limit(10).Find(&pending).Error; err != nil {
		return fmt.Errorf("load accepted manual Publisher requests: %w", err)
	}
	var requestErrors []error
	for _, publication := range pending {
		switch publication.State {
		case PublicationPublished:
			if err := p.updateManualPublishRequestStatus(publication, publisherManualRequestCompleted); err != nil {
				requestErrors = append(requestErrors, fmt.Errorf("complete manual Publisher request %s: %w", publication.ManualRequestID, err))
			}
			continue
		case PublicationHeld, PublicationPublishing:
			continue
		case PublicationFailed:
			if publication.AttemptCount > publication.ManualRequestAttemptCount {
				if err := p.updateManualPublishRequestStatus(publication, publisherManualRequestFailed); err != nil {
					requestErrors = append(requestErrors, fmt.Errorf("fail manual Publisher request %s: %w", publication.ManualRequestID, err))
				}
				continue
			}
		case PublicationBlocked:
			if publication.AttemptCount > publication.ManualRequestAttemptCount {
				if err := p.updateManualPublishRequestStatus(publication, publisherManualRequestFailed); err != nil {
					requestErrors = append(requestErrors, fmt.Errorf("fail blocked manual Publisher request %s: %w", publication.ManualRequestID, err))
				}
				continue
			}
		}
		if publication.ManualRequestStatus == publisherManualRequestAccepted {
			result := p.db.Model(&models.PublisherPublication{}).
				Where("publication_id = ? AND manual_request_id = ? AND manual_request_status = ?", publication.PublicationID, publication.ManualRequestID, publisherManualRequestAccepted).
				Updates(map[string]any{"manual_request_status": publisherManualRequestProcessing, "updated_at": time.Now().UTC()})
			if result.Error != nil {
				requestErrors = append(requestErrors, fmt.Errorf("claim manual Publisher request %s: %w", publication.ManualRequestID, result.Error))
				continue
			}
			if result.RowsAffected != 1 {
				continue
			}
		}
		var job models.QueueJob
		if err := p.db.First(&job, publication.QueueJobID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if updateErr := p.updateManualPublishRequestStatus(publication, publisherManualRequestFailed); updateErr != nil {
					requestErrors = append(requestErrors, fmt.Errorf("fail missing-job manual Publisher request %s: %w", publication.ManualRequestID, updateErr))
				}
			}
			requestErrors = append(requestErrors, fmt.Errorf("load Queue job %d for manual publication %s: %w", publication.QueueJobID, publication.PublicationID, err))
			continue
		}
		_, publishErr := (PublisherHandler{db: p.db}).handoffAndPublishWithReason(job, false, publication.ManualRequestReason)
		var current models.PublisherPublication
		if err := p.db.Where("publication_id = ?", publication.PublicationID).First(&current).Error; err != nil {
			requestErrors = append(requestErrors, fmt.Errorf("reload manual publication %s: %w", publication.PublicationID, err))
			continue
		}
		if publishErr != nil && current.State == PublicationReady && current.AttemptCount == current.ManualRequestAttemptCount {
			var preflightErr publishError
			if errors.As(publishErr, &preflightErr) && preflightErr.Status >= 400 && preflightErr.Status < 500 {
				(PublisherHandler{db: p.db}).markPublicationBlocked(current, "manual_request_preflight_failed", publishErr.Error(), publicationActorOperator, publicationActorOperator)
				if err := p.db.Where("publication_id = ?", publication.PublicationID).First(&current).Error; err != nil {
					requestErrors = append(requestErrors, fmt.Errorf("reload blocked manual publication %s: %w", publication.PublicationID, err))
					continue
				}
			}
		}
		requestStatus := publisherManualRequestAccepted
		switch current.State {
		case PublicationPublished:
			requestStatus = publisherManualRequestCompleted
		case PublicationBlocked:
			requestStatus = publisherManualRequestFailed
		case PublicationFailed:
			if current.AttemptCount > current.ManualRequestAttemptCount {
				requestStatus = publisherManualRequestFailed
			}
		case PublicationPublishing:
			requestStatus = publisherManualRequestProcessing
		case PublicationHeld:
			requestStatus = publisherManualRequestAccepted
		}
		if err := p.updateManualPublishRequestStatus(current, requestStatus); err != nil {
			requestErrors = append(requestErrors, fmt.Errorf("update manual Publisher request %s: %w", current.ManualRequestID, err))
		}
		if publishErr != nil {
			requestErrors = append(requestErrors, fmt.Errorf("manual publication %s: %w", publication.PublicationID, publishErr))
		}
	}
	return errors.Join(requestErrors...)
}

func (p *AutoPublisher) updateManualPublishRequestStatus(publication models.PublisherPublication, status string) error {
	now := time.Now().UTC()
	updates := map[string]any{"manual_request_status": status, "updated_at": now}
	if status == publisherManualRequestCompleted || status == publisherManualRequestFailed {
		updates["manual_request_completed_at"] = now
	} else {
		updates["manual_request_completed_at"] = nil
	}
	return p.db.Model(&models.PublisherPublication{}).
		Where("publication_id = ? AND manual_request_id = ?", publication.PublicationID, publication.ManualRequestID).
		Updates(updates).Error
}

func (p *AutoPublisher) recoverUnattemptedValidationHandoffs() error {
	var recoveryErrors []error
	var publications []models.PublisherPublication
	if err := p.db.Where("state = ? AND failure_code = ? AND validation_run_id = '' AND attempt_count = 0 AND fencing_generation = 0 AND published_at IS NULL AND original_archive_status = ? AND COALESCE(manual_request_status, '') NOT IN (?, ?)", PublicationBlocked, "identity_changed", "pending", publisherManualRequestAccepted, publisherManualRequestProcessing).Order("updated_at asc").Limit(10).Find(&publications).Error; err != nil {
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
