package scheduler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
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
	p.refreshScheduleWaiters()
	p.refreshWorkspaceWaiters()
}

func (p *ReviewPlanner) refreshWorkspaceWaiters() {
	var plans []models.ExecutionPlan
	if err := p.db.Where("status = ? AND waiting_state = ? AND approval_status IN ?", ExecutionPlanWaiting, "WAITING_WORKSPACE", []string{ApprovalAutoApproved, ApprovalManual}).Limit(25).Find(&plans).Error; err != nil {
		return
	}
	for i := range plans {
		decision, err := EvaluateWorkspace(p.db, plans[i])
		if err != nil {
			continue
		}
		plans[i].Evaluation["workspace"] = decision
		plans[i].WorkspaceMode = decision.Mode
		if !decision.Allowed {
			_ = p.db.Save(&plans[i]).Error
			continue
		}
		workingDecision, classification, err := EvaluatePlanWorkingHours(p.db, time.Now())
		if err != nil {
			continue
		}
		plans[i].Evaluation["workingHours"], plans[i].Evaluation["jobClassification"] = workingDecision, classification
		if !workingDecision.Allowed {
			plans[i].WaitingState = "WAITING_SCHEDULE_WINDOW"
		} else {
			plans[i].Status, plans[i].WaitingState = ExecutionPlanReady, ""
			if decision, resourceErr := EvaluateResources(p.db, &plans[i]); resourceErr == nil && !decision.Allowed {
				applyResourceWait(&plans[i], decision)
			}
		}
		_ = p.db.Save(&plans[i]).Error
	}
}

func (p *ReviewPlanner) refreshScheduleWaiters() {
	var plans []models.ExecutionPlan
	if err := p.db.Where("status = ? AND waiting_state = ? AND approval_status IN ?", ExecutionPlanWaiting, "WAITING_SCHEDULE_WINDOW", []string{ApprovalAutoApproved, ApprovalManual}).Limit(25).Find(&plans).Error; err != nil {
		return
	}
	for i := range plans {
		decision, classification, err := EvaluatePlanWorkingHours(p.db, time.Now())
		if err != nil {
			continue
		}
		plans[i].Evaluation["workingHours"] = decision
		plans[i].Evaluation["jobClassification"] = classification
		if !decision.Allowed {
			continue
		}
		workspaceDecision, workspaceErr := EvaluateWorkspace(p.db, plans[i])
		if workspaceErr != nil {
			continue
		}
		plans[i].Evaluation["workspace"], plans[i].WorkspaceMode = workspaceDecision, workspaceDecision.Mode
		if !workspaceDecision.Allowed {
			plans[i].WaitingState = "WAITING_SSD_SPACE"
			_ = p.db.Save(&plans[i]).Error
			continue
		}
		plans[i].Status, plans[i].WaitingState = ExecutionPlanReady, ""
		if resourceDecision, resourceErr := EvaluateResources(p.db, &plans[i]); resourceErr == nil && !resourceDecision.Allowed {
			applyResourceWait(&plans[i], resourceDecision)
		} else {
			plans[i].DecisionReasons = append(plans[i].DecisionReasons, decision.Reason)
		}
		_ = p.db.Save(&plans[i]).Error
	}
}

func (p *ReviewPlanner) refreshResourceWaiters() {
	var plans []models.ExecutionPlan
	resourceStates := []string{"WAITING_RESOURCES", "WAITING_RAM", "WAITING_SSD_SPACE", "WAITING_HDD_SPACE", "WAITING_PROFILE_LIMIT", "WAITING_WORKER", "WAITING_POWER"}
	if err := p.db.Where("status = ? AND waiting_state IN ? AND approval_status IN ?", ExecutionPlanWaiting, resourceStates, []string{ApprovalAutoApproved, ApprovalManual}).Limit(25).Find(&plans).Error; err != nil {
		return
	}
	for i := range plans {
		decision, err := EvaluateResources(p.db, &plans[i])
		if err != nil {
			continue
		}
		if decision.Allowed {
			workspaceDecision, workspaceErr := EvaluateWorkspace(p.db, plans[i])
			if workspaceErr != nil {
				continue
			}
			plans[i].Evaluation["workspace"], plans[i].WorkspaceMode = workspaceDecision, workspaceDecision.Mode
			if !workspaceDecision.Allowed {
				plans[i].Status, plans[i].WaitingState = ExecutionPlanWaiting, "WAITING_SSD_SPACE"
				_ = p.db.Save(&plans[i]).Error
				continue
			}
			workingDecision, classification, workingErr := EvaluatePlanWorkingHours(p.db, time.Now())
			if workingErr != nil {
				continue
			}
			plans[i].Evaluation["workingHours"] = workingDecision
			plans[i].Evaluation["jobClassification"] = classification
			if workingDecision.Allowed {
				plans[i].Status, plans[i].WaitingState = ExecutionPlanReady, ""
				plans[i].DecisionReasons = append(plans[i].DecisionReasons, "Required scheduler resources are available")
			} else {
				plans[i].Status, plans[i].WaitingState = ExecutionPlanWaiting, "WAITING_SCHEDULE_WINDOW"
			}
		} else {
			applyResourceWait(&plans[i], decision)
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
	runtimeEncoders := models.JSONMap{}
	if snapshot, runtimeErr := runtimeinfo.Latest(db); runtimeErr == nil {
		plan.RuntimeSnapshotID = &snapshot.ID
		plan.RuntimeProfile = snapshot.SelectedProfile
		runtimeEncoders = snapshot.Encoders
		plan.DecisionSources["runtimeProfile"] = "runtime_snapshot"
		plan.DecisionReasons = append(plan.DecisionReasons, snapshot.SelectionReasons...)
	} else {
		plan.RuntimeProfile = "desktop_safe"
		plan.DecisionSources["runtimeProfile"] = "safe_fallback"
		plan.Warnings = append(plan.Warnings, "No runtime snapshot was available; desktop_safe policy was selected")
	}
	encoderFailure := ""
	for _, candidate := range uniqueStrings(append([]string{profile.PreferredEncoder}, []string(profile.AllowedEncoders)...)) {
		capability, recorded := recordedEncoderCapability(runtimeEncoders, candidate)
		if !recorded {
			capability = capabilities.CheckEncoder(candidate)
		}
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
	estimateMethod := "profile_heuristic_v1"
	estimateConfidence := "low"
	if plan.Evaluation == nil {
		plan.Evaluation = models.JSONMap{}
	}
	if source, probed := probeMediaEstimate(job.MediaPath); probed {
		if estimate, estimated := estimatePlannedOutput(profile, plan.SelectedEncoder, source, inputSize); estimated {
			if sampledVideoBytes, sampleEncoder, found := persistedProfileSampleEstimate(db, job.MediaPath, profile); found && sampleEncoder == plan.SelectedEncoder {
				total := sampledVideoBytes + estimate.AudioBytes + estimate.SubtitleBytes
				overhead := maxInt64(16<<10, total/100)
				estimate.MinBytes, estimate.MaxBytes = int64(float64(total+overhead)*.98), int64(float64(total+overhead)*1.02)
				estimate.VideoBytes, estimate.Confidence, estimate.Method = sampledVideoBytes, "high", "five_distributed_profile_samples"
			}
			minOutput, maxOutput = estimate.MinBytes, estimate.MaxBytes
			estimateMethod, estimateConfidence = estimate.Method, estimate.Confidence
			plan.Evaluation["outputEstimate"] = models.JSONMap{
				"videoBytes": estimate.VideoBytes, "audioBytes": estimate.AudioBytes, "subtitleBytes": estimate.SubtitleBytes,
				"durationSeconds": source.DurationSeconds, "sourceVideoBitrate": source.VideoBitrate,
			}
		}
	}
	plan.OutputPath = reviewOutputPath(job.MediaPath, library, profile.Container)
	plan.InputSizeBytes = inputSize
	plan.EstimatedOutputMinBytes = minOutput
	plan.EstimatedOutputMaxBytes = maxOutput
	plan.EstimatedWorkspaceBytes = inputSize + maxOutput + maxOutput/5
	plan.EstimateConfidence = estimateConfidence
	plan.Evaluation["method"] = estimateMethod
	plan.Evaluation["requiresLabSampleForHigherConfidence"] = estimateConfidence != "high"
	if plan.RuntimeSnapshotID != nil {
		plan.Evaluation["runtimeSnapshotId"] = *plan.RuntimeSnapshotID
	}
	plan.Reservation = BuildReservation(*plan)
	classification, classificationErr := ClassifyJob(JobTypeVideoConversion)
	if classificationErr != nil {
		return classificationErr
	}
	plan.Evaluation["jobClassification"] = classification
	directPlayReport, directPlayErr := EvaluatePlannedDirectPlay(db, profile)
	if directPlayErr != nil {
		return directPlayErr
	}
	plan.Evaluation["directPlay"] = directPlayReport
	plan.DecisionSources["directPlay"] = "profile_preflight_estimate"
	if directPlayReport.Enabled {
		plan.DecisionReasons = append(plan.DecisionReasons, fmt.Sprintf("Estimated DirectPlay risk is %s with a lowest client score of %d", directPlayReport.Risk, directPlayReport.LowestScore))
		if directPlayReport.Risk != "low" {
			plan.Warnings = append(plan.Warnings, "DirectPlay compatibility is estimated from the planned profile; validate the final output for definitive results")
		}
	}
	plan.DecisionReasons = append(plan.DecisionReasons,
		fmt.Sprintf("Estimated output range is %d-%d bytes using %s", minOutput, maxOutput, estimateMethod),
	)
	if estimateConfidence != "high" {
		plan.DecisionReasons = append(plan.DecisionReasons, "A saved LAB measurement can raise this estimate to high confidence")
	}
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
	if directPlayReport.Blocked {
		plan.ApprovalStatus, plan.ApprovedAt = ApprovalPending, nil
		plan.Status, plan.WaitingState = ExecutionPlanWaiting, WaitingDirectPlayReview
		plan.DecisionReasons = append(plan.DecisionReasons, "DirectPlay policy requires manual review because the estimated score is below its threshold")
	}
	if plan.Status == ExecutionPlanReady {
		workspaceDecision, workspaceErr := EvaluateWorkspace(db, *plan)
		if workspaceErr != nil {
			return workspaceErr
		}
		plan.Evaluation["workspace"] = workspaceDecision
		plan.WorkspaceMode = workspaceDecision.Mode
		workingDecision, _, workingErr := EvaluatePlanWorkingHours(db, now)
		if workingErr != nil {
			return workingErr
		}
		plan.Evaluation["workingHours"] = workingDecision
		if !workspaceDecision.Allowed {
			plan.Status, plan.WaitingState = ExecutionPlanWaiting, "WAITING_SSD_SPACE"
			plan.DecisionReasons = append(plan.DecisionReasons, workspaceDecision.Reason)
		} else if !workingDecision.Allowed {
			plan.Status, plan.WaitingState = ExecutionPlanWaiting, "WAITING_SCHEDULE_WINDOW"
			plan.DecisionReasons = append(plan.DecisionReasons, workingDecision.Reason)
		} else if decision, resourceErr := EvaluateResources(db, plan); resourceErr == nil && !decision.Allowed {
			applyResourceWait(plan, decision)
			plan.DecisionReasons = append(plan.DecisionReasons, "Approved plan is waiting for scheduler resources")
		}
	}
	return db.Save(plan).Error
}

func recordedEncoderCapability(encoders models.JSONMap, encoder string) (capabilities.EncoderCapability, bool) {
	raw, ok := encoders[encoder]
	if !ok {
		return capabilities.EncoderCapability{}, false
	}
	var value map[string]any
	switch typed := raw.(type) {
	case models.JSONMap:
		value = typed
	case map[string]any:
		value = typed
	default:
		return capabilities.EncoderCapability{}, false
	}
	listed, listedOK := value["listed"].(bool)
	usable, usableOK := value["usable"].(bool)
	if !listedOK || !usableOK {
		return capabilities.EncoderCapability{}, false
	}
	reason, _ := value["reason"].(string)
	return capabilities.EncoderCapability{Listed: listed, Usable: usable, Reason: reason}, true
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
		workingDecision, _, workingErr := EvaluatePlanWorkingHours(db, now)
		if workingErr != nil {
			return models.ExecutionPlan{}, workingErr
		}
		plan.Evaluation["workingHours"] = workingDecision
		workspaceDecision, workspaceErr := EvaluateWorkspace(db, plan)
		if workspaceErr != nil {
			return models.ExecutionPlan{}, workspaceErr
		}
		plan.Evaluation["workspace"], plan.WorkspaceMode = workspaceDecision, workspaceDecision.Mode
		if !workspaceDecision.Allowed {
			plan.Status, plan.WaitingState = ExecutionPlanWaiting, "WAITING_SSD_SPACE"
		} else if !workingDecision.Allowed {
			plan.Status, plan.WaitingState = ExecutionPlanWaiting, "WAITING_SCHEDULE_WINDOW"
		} else if decision, resourceErr := EvaluateResources(db, &plan); resourceErr == nil && !decision.Allowed {
			applyResourceWait(&plan, decision)
		}
	} else {
		plan.ApprovalStatus, plan.RejectedAt, plan.ApprovedAt = ApprovalRejected, &now, nil
		plan.Status, plan.WaitingState = ExecutionPlanWaiting, "WAITING_REVIEW"
		plan.DecisionReasons = append(plan.DecisionReasons, "Manually rejected")
	}
	return plan, db.Save(&plan).Error
}

func applyResourceWait(plan *models.ExecutionPlan, decision ResourceDecision) {
	plan.Status, plan.WaitingState = ExecutionPlanWaiting, decision.WaitingState
	plan.Evaluation["resources"] = decision
	plan.Evaluation["resourceWaitReasons"] = decision.Reasons
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
