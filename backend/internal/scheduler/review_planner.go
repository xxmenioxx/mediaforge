package scheduler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/capabilities"
	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/runtimeinfo"
	"gorm.io/gorm"
)

const (
	ApprovalPending      = "pending"
	ApprovalAutoApproved = "auto_approved"
	ApprovalManual       = "manually_approved"
	ApprovalRejected     = "rejected"
)

type ReviewPlanner struct {
	db   *gorm.DB
	mu   sync.Mutex
	stop chan struct{}
}

func StartReviewPlanner(db *gorm.DB) *ReviewPlanner {
	planner := &ReviewPlanner{db: db, stop: make(chan struct{})}
	go planner.run()
	return planner
}

func (p *ReviewPlanner) Stop() { close(p.stop) }

func (p *ReviewPlanner) run() {
	ticker := time.NewTicker(2 * time.Second)
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

func (p *ReviewPlanner) tick() {
	if !p.mu.TryLock() {
		return
	}
	defer p.mu.Unlock()
	var plans []models.ExecutionPlan
	if err := p.db.Where("status = ?", ExecutionPlanPendingEvaluation).Order("created_at asc").Limit(25).Find(&plans).Error; err != nil {
		return
	}
	for i := range plans {
		_ = EvaluateReviewPlan(p.db, &plans[i])
	}
	p.refreshResourceWaiters()
}

func (p *ReviewPlanner) refreshResourceWaiters() {
	var plans []models.ExecutionPlan
	if err := p.db.Where("status = ? AND waiting_state = ? AND approval_status IN ?", ExecutionPlanWaiting, "WAITING_RESOURCES", []string{ApprovalAutoApproved, ApprovalManual}).Limit(25).Find(&plans).Error; err != nil {
		return
	}
	for i := range plans {
		allowed, reasons, err := CanDispatch(p.db, &plans[i])
		if err != nil {
			continue
		}
		if allowed {
			plans[i].Status, plans[i].WaitingState = ExecutionPlanReady, ""
			plans[i].DecisionReasons = append(plans[i].DecisionReasons, "Required scheduler resources are available")
		} else {
			plans[i].Evaluation["resourceWaitReasons"] = reasons
		}
		_ = p.db.Save(&plans[i]).Error
	}
}

func EvaluateReviewPlan(db *gorm.DB, plan *models.ExecutionPlan) error {
	var job models.QueueJob
	if err := db.First(&job, plan.JobID).Error; err != nil {
		return err
	}
	var library models.Library
	if err := db.First(&library, job.LibraryID).Error; err != nil {
		return err
	}
	profile, err := RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		return err
	}
	if snapshot, runtimeErr := runtimeinfo.Latest(db); runtimeErr == nil {
		plan.RuntimeSnapshotID = &snapshot.ID
		plan.RuntimeProfile = snapshot.SelectedProfile
		plan.DecisionSources["runtimeProfile"] = "runtime_snapshot"
		plan.DecisionReasons = append(plan.DecisionReasons, snapshot.SelectionReasons...)
	} else {
		plan.RuntimeProfile = "desktop_safe"
		plan.DecisionSources["runtimeProfile"] = "safe_fallback"
		plan.Warnings = append(plan.Warnings, "No runtime snapshot was available; desktop_safe policy was selected")
	}
	encoderFailure := ""
	for _, candidate := range uniqueStrings(append([]string{profile.PreferredEncoder}, []string(profile.AllowedEncoders)...)) {
		capability := capabilities.CheckEncoder(candidate)
		if capability.Usable {
			plan.SelectedEncoder = candidate
			break
		}
		if capability.Reason != "" {
			encoderFailure = capability.Reason
		}
	}
	plan.DecisionSources["selectedEncoder"] = "profile_constraints_and_runtime_probe"
	inputSize := int64(0)
	if info, statErr := os.Stat(job.MediaPath); statErr == nil {
		inputSize = info.Size()
	}
	minRatio, maxRatio := estimateRatios(plan.CodecFamily, plan.QualityValue)
	minOutput := int64(float64(inputSize) * minRatio)
	maxOutput := int64(float64(inputSize) * maxRatio)
	plan.OutputPath = reviewOutputPath(job.MediaPath, library, profile.Container)
	plan.InputSizeBytes = inputSize
	plan.EstimatedOutputMinBytes = minOutput
	plan.EstimatedOutputMaxBytes = maxOutput
	plan.EstimatedWorkspaceBytes = inputSize + maxOutput + maxOutput/5
	plan.EstimateConfidence = "low"
	plan.Evaluation = models.JSONMap{"method": "profile_heuristic_v1", "requiresLabSampleForHigherConfidence": true}
	if plan.RuntimeSnapshotID != nil {
		plan.Evaluation["runtimeSnapshotId"] = *plan.RuntimeSnapshotID
	}
	plan.Reservation = BuildReservation(*plan)
	plan.DecisionReasons = append(plan.DecisionReasons,
		fmt.Sprintf("Estimated output range is %d-%d bytes using profile heuristics", minOutput, maxOutput),
		"A Lab sample is required for a medium or high confidence size estimate",
	)
	if plan.SelectedEncoder == "" {
		plan.Status, plan.WaitingState, plan.ApprovalStatus = ExecutionPlanWaiting, "WAITING_ENCODER", ApprovalPending
		plan.Warnings = append(plan.Warnings, encoderFailure)
		plan.DecisionReasons = append(plan.DecisionReasons, "No encoder allowed by the profile passed its runtime capability check")
		return db.Save(plan).Error
	}

	mode := ReviewMode(db)
	plan.ApprovalMode = mode
	now := time.Now()
	switch mode {
	case "automatic":
		plan.ApprovalStatus, plan.ApprovedAt, plan.Status = ApprovalAutoApproved, &now, ExecutionPlanReady
		plan.DecisionReasons = append(plan.DecisionReasons, "Automatically approved by review policy")
	case "conditional":
		if inputSize > 0 && maxOutput <= inputSize {
			plan.ApprovalStatus, plan.ApprovedAt, plan.Status = ApprovalAutoApproved, &now, ExecutionPlanReady
			plan.DecisionReasons = append(plan.DecisionReasons, "Automatically approved because estimated output does not exceed input size")
		} else {
			plan.ApprovalStatus, plan.Status, plan.WaitingState = ApprovalPending, ExecutionPlanWaiting, "WAITING_REVIEW"
		}
	default:
		plan.ApprovalStatus, plan.Status, plan.WaitingState = ApprovalPending, ExecutionPlanWaiting, "WAITING_REVIEW"
	}
	if plan.Status == ExecutionPlanReady {
		allowed, reasons, resourceErr := CanDispatch(db, plan)
		if resourceErr == nil && !allowed {
			plan.Status, plan.WaitingState = ExecutionPlanWaiting, "WAITING_RESOURCES"
			plan.Evaluation["resourceWaitReasons"] = reasons
			plan.DecisionReasons = append(plan.DecisionReasons, "Approved plan is waiting for scheduler resources")
		}
	}
	return db.Save(plan).Error
}

func SetPlanApproval(db *gorm.DB, jobID, planID uint, approve bool) (models.ExecutionPlan, error) {
	var job models.QueueJob
	if err := db.First(&job, jobID).Error; err != nil {
		return models.ExecutionPlan{}, err
	}
	if job.ActiveExecutionPlanID == nil || *job.ActiveExecutionPlanID != planID {
		return models.ExecutionPlan{}, errors.New("only the active execution plan can be reviewed")
	}
	var plan models.ExecutionPlan
	if err := db.First(&plan, planID).Error; err != nil {
		return models.ExecutionPlan{}, err
	}
	now := time.Now()
	plan.ApprovalMode = "manual"
	if approve {
		plan.ApprovalStatus, plan.ApprovedAt, plan.RejectedAt = ApprovalManual, &now, nil
		plan.Status, plan.WaitingState = ExecutionPlanReady, ""
		plan.DecisionReasons = append(plan.DecisionReasons, "Manually approved")
		if allowed, reasons, resourceErr := CanDispatch(db, &plan); resourceErr == nil && !allowed {
			plan.Status, plan.WaitingState = ExecutionPlanWaiting, "WAITING_RESOURCES"
			plan.Evaluation["resourceWaitReasons"] = reasons
		}
	} else {
		plan.ApprovalStatus, plan.RejectedAt, plan.ApprovedAt = ApprovalRejected, &now, nil
		plan.Status, plan.WaitingState = ExecutionPlanWaiting, "WAITING_REVIEW"
		plan.DecisionReasons = append(plan.DecisionReasons, "Manually rejected")
	}
	return plan, db.Save(&plan).Error
}

func ReviewMode(db *gorm.DB) string {
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "pipelineAutomation").Error; err != nil {
		return "conditional"
	}
	mode, _ := setting.Value["reviewMode"].(string)
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "conditional"
	}
	if mode != "automatic" && mode != "conditional" {
		return "manual"
	}
	return mode
}

func AutoExecutionEnabled(db *gorm.DB) bool {
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "pipelineAutomation").Error; err != nil {
		return true
	}
	value, exists := setting.Value["autoExecutionEnabled"]
	if !exists {
		return true
	}
	enabled, _ := value.(bool)
	return enabled
}

func estimateRatios(codec string, quality int) (float64, float64) {
	if codec == "copy" {
		return 0.98, 1.02
	}
	if quality <= 18 {
		return 0.60, 0.95
	}
	if quality >= 25 {
		return 0.30, 0.65
	}
	return 0.42, 0.80
}

func reviewOutputPath(inputPath string, library models.Library, container string) string {
	relative, err := filepath.Rel(library.SourcePath, inputPath)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		relative = filepath.Base(inputPath)
	}
	relative = strings.TrimSuffix(relative, filepath.Ext(relative)) + "." + container
	return filepath.Join(library.DestinationPath, relative)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
