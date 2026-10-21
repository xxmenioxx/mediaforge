package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

type validationStagingFixture struct {
	validation queueValidationDismissalFixture
	workspace  string
}

func newValidationStagingFixture(t *testing.T) validationStagingFixture {
	t.Helper()
	fixture := newQueueValidationDismissalFixture(t, models.QualityOutcomeFail)
	db := fixture.delivery.db
	if err := db.AutoMigrate(
		&models.SchedulerReservation{}, &models.AssetMaintenanceOperation{}, &models.TestEncode{}, &models.TestEncodeArtifact{},
		&models.CompatiblePreviewArtifactRecord{}, &models.ValidationStagingCleanupPreview{}, &models.ValidationStagingCleanupEvidence{},
	); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Dir(fixture.delivery.output)
	sidecar := strings.TrimSuffix(fixture.delivery.output, filepath.Ext(fixture.delivery.output)) + ".eng.ass"
	if err := os.WriteFile(sidecar, []byte("previewed staged sidecar"), 0o600); err != nil {
		t.Fatal(err)
	}
	return validationStagingFixture{validation: fixture, workspace: workspace}
}

func dismissValidationStagingFixture(t *testing.T, fixture validationStagingFixture) {
	t.Helper()
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	router := gin.New()
	router.DELETE("/api/queue/jobs/:id", NewQueueHandler(fixture.validation.delivery.db).Dismiss)
	request := httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/"+strconv.Itoa(int(fixture.validation.delivery.job.ID)), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Dismiss status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidationStagingCleanupDismissThenCleansOnlyPreviewedWorkspace(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	db := fixture.validation.delivery.db
	preview, err := createValidationStagingPreview(db, fixture.validation.delivery.job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.State != "ready" || preview.PreviewID == "" || len(preview.Files) != 2 || preview.TotalBytes == 0 {
		t.Fatalf("preview did not bind the exact workspace files: %#v output=%q workspace=%q", preview, fixture.validation.delivery.job.OutputPath, fixture.workspace)
	}
	dismissValidationStagingFixture(t, fixture)
	result, err := executeValidationStagingCleanup(db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != "cleaned" || result.RecoveredBytes != preview.TotalBytes || len(result.RemovedPaths) < 3 {
		t.Fatalf("cleanup result=%#v preview=%#v", result, preview)
	}
	if _, err := os.Stat(fixture.workspace); !os.IsNotExist(err) {
		t.Fatalf("workspace remains after cleanup, stat err=%v", err)
	}
	if _, err := os.Stat(fixture.validation.delivery.original); err != nil {
		t.Fatalf("RAW original was changed by cleanup: %v", err)
	}
	if _, err := os.Stat(fixture.validation.resultPath); err != nil {
		t.Fatalf("Result report was changed by cleanup: %v", err)
	}
	var job models.QueueJob
	if err := db.First(&job, fixture.validation.delivery.job.ID).Error; err != nil || job.DismissedAt == nil || job.ValidationRunID != fixture.validation.run.RunID {
		t.Fatalf("Dismiss/Validation history changed: job=%#v err=%v", job, err)
	}
	var evidence []models.ValidationStagingCleanupEvidence
	if err := db.Where("job_id = ?", job.ID).Order("id asc").Find(&evidence).Error; err != nil || len(evidence) != 2 || evidence[0].Event != "started" || evidence[1].Outcome != "cleaned" {
		t.Fatalf("cleanup evidence is incomplete: %#v err=%v", evidence, err)
	}
	repeated, err := executeValidationStagingCleanup(db, job.ID, preview.PreviewID, "local_operator")
	if err != nil || repeated.Outcome != "cleaned" || repeated.RecoveredBytes != result.RecoveredBytes {
		t.Fatalf("repeated confirmation was not idempotent: result=%#v err=%v", repeated, err)
	}
	var policy HousekeepingPolicy
	candidate, eligible, err := housekeepingCandidate(db, job.ID, fixture.workspace, time.Now(), policy, time.Now())
	if err != nil || eligible || candidate.SafeToDelete {
		t.Fatalf("global Housekeeping protection changed for dismissed job: %#v eligible=%v err=%v", candidate, eligible, err)
	}
}

func TestValidationStagingCleanupBlocksChangedManifestAndActiveReferences(t *testing.T) {
	for _, mode := range []string{"changed_manifest", "active_reservation", "other_job_sidecar"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newValidationStagingFixture(t)
			db := fixture.validation.delivery.db
			preview, err := createValidationStagingPreview(db, fixture.validation.delivery.job.ID)
			if err != nil || preview.State != "ready" {
				t.Fatalf("preview=%#v err=%v", preview, err)
			}
			dismissValidationStagingFixture(t, fixture)
			if mode == "changed_manifest" {
				if err := os.WriteFile(filepath.Join(fixture.workspace, "new-file.mkv"), []byte("newly staged"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if mode == "active_reservation" {
				if err := db.Create(&models.SchedulerReservation{JobID: fixture.validation.delivery.job.ID, AssetKey: fixture.validation.delivery.job.MediaPath, State: "active", WorkerName: "worker"}).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				other := models.QueueJob{MediaPath: "/raw/other.mkv", Status: JobStatusCompleted, SubtitleArtifacts: models.JSONList{filepath.Join(fixture.workspace, "output.eng.ass")}}
				if err := db.Create(&other).Error; err != nil {
					t.Fatal(err)
				}
			}
			result, err := executeValidationStagingCleanup(db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
			if err != nil || result.Outcome != "blocked" {
				t.Fatalf("unsafe cleanup was not blocked: result=%#v err=%v", result, err)
			}
			if _, err := os.Stat(fixture.validation.delivery.output); err != nil {
				t.Fatalf("workspace content was deleted after block: %v", err)
			}
			var job models.QueueJob
			if err := db.First(&job, fixture.validation.delivery.job.ID).Error; err != nil || job.DismissedAt == nil {
				t.Fatalf("cleanup block rolled back Dismiss: job=%#v err=%v", job, err)
			}
		})
	}
}

func TestValidationStagingCleanupNoopForAbsentWorkspace(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	if err := os.Remove(fixture.validation.delivery.output); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(strings.TrimSuffix(fixture.validation.delivery.output, filepath.Ext(fixture.validation.delivery.output)) + ".eng.ass"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.workspace); err != nil {
		t.Fatal(err)
	}
	preview, err := createValidationStagingPreview(fixture.validation.delivery.db, fixture.validation.delivery.job.ID)
	if err != nil || preview.State != "noop" || preview.TotalBytes != 0 || preview.PreviewID == "" {
		t.Fatalf("absent workspace preview=%#v err=%v", preview, err)
	}
	dismissValidationStagingFixture(t, fixture)
	result, err := executeValidationStagingCleanup(fixture.validation.delivery.db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
	if err != nil || result.Outcome != "noop" || result.RecoveredBytes != 0 {
		t.Fatalf("absent workspace cleanup=%#v err=%v", result, err)
	}
}

func TestValidationStagingCleanupRejectsSymlinkWorkspaceEntry(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	outside := filepath.Join(fixture.validation.delivery.root, "outside.txt")
	if err := os.WriteFile(outside, []byte("must remain"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(fixture.workspace, "escape.txt")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	preview, err := createValidationStagingPreview(fixture.validation.delivery.db, fixture.validation.delivery.job.ID)
	if err != nil || preview.State != "blocked" || len(preview.Blockers) == 0 || preview.PreviewID != "" {
		t.Fatalf("unsafe symlink preview=%#v err=%v", preview, err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside target changed: %v", err)
	}
}

func TestValidationStagingCleanupPartialRemovalReportsBytesAndFreshRetry(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	files, _, total, err := validationStagingManifest(fixture.workspace)
	if err != nil || len(files) != 3 {
		t.Fatalf("manifest=%#v total=%d err=%v", files, total, err)
	}
	ordered := append([]validationStagingFile(nil), files...)
	sort.Slice(ordered, func(i, j int) bool {
		leftDepth, rightDepth := pathDepth(ordered[i].Relative), pathDepth(ordered[j].Relative)
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind == "file"
		}
		return ordered[i].Relative > ordered[j].Relative
	})
	if ordered[0].Kind != "file" || ordered[1].Kind != "file" {
		t.Fatalf("expected two previewed files before the workspace root: %#v", ordered)
	}
	partialManifest := append([]validationStagingFile(nil), files...)
	for index := range partialManifest {
		if partialManifest[index].Relative == ordered[1].Relative {
			partialManifest[index].Identity = "changed-after-preview"
		}
	}
	removed, recovered, removeErr := removeValidationStagingManifest(filepath.Dir(fixture.workspace), fixture.workspace, partialManifest)
	if removeErr == nil || len(removed) != 1 || recovered != ordered[0].SizeBytes {
		t.Fatalf("partial removal was not accurately reported: removed=%#v recovered=%d err=%v", removed, recovered, removeErr)
	}
	remaining, _, remainingBytes, err := validationStagingManifest(fixture.workspace)
	if err != nil || len(remaining) != 2 || remainingBytes != total-recovered {
		t.Fatalf("remaining manifest=%#v bytes=%d total=%d err=%v", remaining, remainingBytes, total, err)
	}
	removed, retriedBytes, removeErr := removeValidationStagingManifest(filepath.Dir(fixture.workspace), fixture.workspace, remaining)
	if removeErr != nil || retriedBytes != remainingBytes || len(removed) != 2 {
		t.Fatalf("fresh retry did not remove only the remaining workspace: removed=%#v bytes=%d err=%v", removed, retriedBytes, removeErr)
	}
}

func TestValidationStagingCleanupConcurrentConfirmationConsumesPreviewOnce(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	db := fixture.validation.delivery.db
	preview, err := createValidationStagingPreview(db, fixture.validation.delivery.job.ID)
	if err != nil || preview.PreviewID == "" {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	dismissValidationStagingFixture(t, fixture)
	results := make([]validationStagingCleanupResponse, 2)
	errorsSeen := make([]error, 2)
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index], errorsSeen[index] = executeValidationStagingCleanup(db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
		}(index)
	}
	group.Wait()
	for index := range results {
		if errorsSeen[index] != nil || results[index].Outcome != "cleaned" || results[index].RecoveredBytes != preview.TotalBytes {
			t.Fatalf("confirmation %d was not idempotently fenced: result=%#v err=%v", index, results[index], errorsSeen[index])
		}
	}
	var events int64
	if err := db.Model(&models.ValidationStagingCleanupEvidence{}).Where("job_id = ? AND event = ?", fixture.validation.delivery.job.ID, "completed").Count(&events).Error; err != nil || events != 1 {
		t.Fatalf("preview was consumed more than once: events=%d err=%v", events, err)
	}
}

func TestValidationStagingCleanupResponseContractSerializes(t *testing.T) {
	encoded, err := json.Marshal(validationStagingCleanupResponse{Outcome: "blocked", RemovedPaths: []string{}, RecoveredBytes: 0})
	if err != nil || !strings.Contains(string(encoded), `"outcome":"blocked"`) || !strings.Contains(string(encoded), `"recoveredBytes":0`) {
		t.Fatalf("cleanup result response contract=%s err=%v", encoded, err)
	}
}

func setValidationStagingExecutionNumber(t *testing.T, fixture *validationStagingFixture, number uint) {
	t.Helper()
	db := fixture.validation.delivery.db
	job := fixture.validation.delivery.job
	job.ExecutionNumber = &number
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeJobResultArtifact(db, job, map[string]any{"status": "completed"}); err != nil {
		t.Fatal(err)
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		t.Fatal(err)
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || !bindingOK || binding.ExecutionNumber != number {
		t.Fatalf("new Result identity was not bound to execution %d: identity=%#v binding=%#v", number, identity, binding)
	}
	var run models.ValidationRun
	if err := db.Where("run_id = ? AND job_id = ?", job.ValidationRunID, job.ID).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	run.ExecutionNumber = binding.ExecutionNumber
	run.ExecutionPlanID = binding.ExecutionPlanID
	run.ExecutionPlanVersion = binding.ExecutionPlanVersion
	run.ExecutionPlanFingerprint = binding.ExecutionPlanFingerprint
	run.ResultArtifactID = identity.ArtifactID
	run.ResultGenerationToken = identity.GenerationToken
	run.ResultGenerationVersion = identity.GenerationVersion
	run.ResultOutputRevision = identity.OutputRevision
	if err := db.Save(&run).Error; err != nil {
		t.Fatal(err)
	}
	fixture.validation.delivery.job = job
	fixture.validation.run = run
}

func TestValidationStagingCleanupUsesQueueJobIDNotExecutionNumber(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	setValidationStagingExecutionNumber(t, &fixture, 42)
	preview, err := createValidationStagingPreview(fixture.validation.delivery.db, fixture.validation.delivery.job.ID)
	if err != nil || preview.State != "ready" {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	workRoot, err := filepath.EvalSymlinks(filepath.Dir(fixture.workspace))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(workRoot, "job-"+strconv.FormatUint(uint64(fixture.validation.delivery.job.ID), 10))
	if preview.ExecutionNumber != 42 || preview.WorkspacePath != want || preview.WorkspacePath == filepath.Join(workRoot, "job-42") {
		t.Fatalf("workspace ownership used the wrong identity: preview=%#v want=%q", preview, want)
	}
	dismissValidationStagingFixture(t, fixture)
	result, err := executeValidationStagingCleanup(fixture.validation.delivery.db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
	if err != nil || result.Outcome != "cleaned" {
		t.Fatalf("QueueJob ID scoped cleanup failed: result=%#v err=%v", result, err)
	}
}

func TestValidationStagingCleanupRejectsChangedQueueIdentity(t *testing.T) {
	for _, mode := range []string{"execution_number", "validation_run", "output_path"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newValidationStagingFixture(t)
			db := fixture.validation.delivery.db
			preview, err := createValidationStagingPreview(db, fixture.validation.delivery.job.ID)
			if err != nil || preview.State != "ready" {
				t.Fatalf("preview=%#v err=%v", preview, err)
			}
			dismissValidationStagingFixture(t, fixture)
			var job models.QueueJob
			if err := db.First(&job, fixture.validation.delivery.job.ID).Error; err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "execution_number":
				changed := *job.ExecutionNumber + 1
				job.ExecutionNumber = &changed
			case "validation_run":
				job.ValidationRunID = "validation-run:stale"
			case "output_path":
				job.OutputPath = filepath.Join(fixture.validation.delivery.root, "outside.mkv")
			}
			if err := db.Save(&job).Error; err != nil {
				t.Fatal(err)
			}
			result, err := executeValidationStagingCleanup(db, job.ID, preview.PreviewID, "local_operator")
			if err != nil || result.Outcome != "blocked" {
				t.Fatalf("changed identity was not blocked: result=%#v err=%v", result, err)
			}
			if _, err := os.Stat(fixture.validation.delivery.output); err != nil {
				t.Fatalf("staged output was deleted after identity changed: %v", err)
			}
		})
	}
}

func TestValidationStagingCleanupExpiredPreviewNeverDeletesWorkspace(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	db := fixture.validation.delivery.db
	preview, err := createValidationStagingPreview(db, fixture.validation.delivery.job.ID)
	if err != nil || preview.PreviewID == "" {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	dismissValidationStagingFixture(t, fixture)
	var stored models.ValidationStagingCleanupPreview
	if err := db.Where("preview_id = ?", preview.PreviewID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	stored.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	if err := db.Save(&stored).Error; err != nil {
		t.Fatal(err)
	}
	result, err := executeValidationStagingCleanup(db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
	if err != nil || result.Outcome != "blocked" {
		t.Fatalf("expired preview was not blocked: result=%#v err=%v", result, err)
	}
	if _, err := os.Stat(fixture.validation.delivery.output); err != nil {
		t.Fatalf("workspace changed after expired preview: %v", err)
	}
	latest, err := latestValidationStagingEvidence(db, fixture.validation.delivery.job.ID, preview.PreviewID)
	if err != nil || latest.Outcome != "blocked" || !strings.Contains(latest.Reason, "expired") {
		t.Fatalf("expired preview did not preserve an auditable blocked outcome: latest=%#v err=%v", latest, err)
	}
}

func TestValidationStagingCleanupBlocksUnsafeOrChangedWorkRole(t *testing.T) {
	for _, mode := range []string{"unsafe_preview_root", "root_changed_after_preview"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newValidationStagingFixture(t)
			db := fixture.validation.delivery.db
			var setting models.AppSetting
			if err := db.Where("key = ?", "paths").First(&setting).Error; err != nil {
				t.Fatal(err)
			}
			if mode == "root_changed_after_preview" {
				preview, err := createValidationStagingPreview(db, fixture.validation.delivery.job.ID)
				if err != nil || preview.State != "ready" {
					t.Fatalf("preview=%#v err=%v", preview, err)
				}
				dismissValidationStagingFixture(t, fixture)
				setting.Value["stagingPath"] = fixture.validation.delivery.rawRoot
				if err := db.Save(&setting).Error; err != nil {
					t.Fatal(err)
				}
				result, err := executeValidationStagingCleanup(db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
				if err != nil || result.Outcome != "blocked" {
					t.Fatalf("changed Work role was not blocked: result=%#v err=%v", result, err)
				}
			} else {
				setting.Value["stagingPath"] = fixture.validation.delivery.rawRoot
				if err := db.Save(&setting).Error; err != nil {
					t.Fatal(err)
				}
				preview, err := createValidationStagingPreview(db, fixture.validation.delivery.job.ID)
				if err != nil || preview.State != "blocked" || preview.PreviewID != "" {
					t.Fatalf("unsafe Work role produced a usable preview: preview=%#v err=%v", preview, err)
				}
			}
			if _, err := os.Stat(fixture.validation.delivery.original); err != nil {
				t.Fatalf("RAW original changed after unsafe role block: %v", err)
			}
			if _, err := os.Stat(fixture.validation.delivery.output); err != nil {
				t.Fatalf("staged output changed after unsafe role block: %v", err)
			}
		})
	}
}

func TestValidationStagingCleanupBlocksEveryActiveReferenceSurface(t *testing.T) {
	for _, mode := range []string{"maintenance", "publisher", "test_encode", "test_encode_artifact", "other_job_output", "asset_record", "other_execution_plan", "other_publisher_manifest", "active_compatible_preview"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newValidationStagingFixture(t)
			db := fixture.validation.delivery.db
			job := fixture.validation.delivery.job
			var referencedOriginal string
			if mode == "asset_record" {
				referencedOriginal = filepath.Join(fixture.workspace, "preserved-original.mkv")
				if err := os.WriteFile(referencedOriginal, []byte("synthetic RAW original"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			preview, err := createValidationStagingPreview(db, job.ID)
			if err != nil || preview.State != "ready" {
				t.Fatalf("preview=%#v err=%v", preview, err)
			}
			dismissValidationStagingFixture(t, fixture)
			referencePath := filepath.Join(fixture.workspace, "output.mkv")
			switch mode {
			case "maintenance":
				err = db.Create(&models.AssetMaintenanceOperation{ID: "maintenance-race", OperationType: "archive", AssetPath: referencePath, Status: maintenanceStatusRunning, Phase: "running"}).Error
			case "publisher":
				artifact, _, readErr := readLatestJobArtifact(db, job, "result")
				if readErr != nil {
					t.Fatal(readErr)
				}
				identity, identityOK := resultIdentityFromArtifact(artifact)
				binding, bindingOK := resultBindingFromArtifact(artifact)
				if !identityOK || !bindingOK {
					t.Fatalf("fixture Result binding is unavailable: %#v", artifact)
				}
				now := time.Now().UTC()
				err = db.Create(&models.PublisherPublication{
					PublicationID: "publication-race", QueueJobID: job.ID, ExecutionNumber: *job.ExecutionNumber,
					ExecutionPlanID: binding.ExecutionPlanID, ExecutionPlanVersion: binding.ExecutionPlanVersion,
					ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint, LibraryID: job.LibraryID,
					DestinationRoot: fixture.validation.delivery.libraryRoot, DestinationPath: fixture.validation.delivery.destination,
					PublishMode: job.PublishMode, ResultArtifactID: identity.ArtifactID,
					ResultGenerationToken: identity.GenerationToken, ResultGenerationVersion: identity.GenerationVersion,
					ResultOutputRevision: identity.OutputRevision, ValidationRunID: job.ValidationRunID,
					SourcePath: job.MediaPath, State: "pending", Manifest: models.JSONList{}, CreatedAt: now, UpdatedAt: now,
				}).Error
			case "test_encode":
				err = db.Create(&models.TestEncode{SourcePath: job.MediaPath, LibraryID: job.LibraryID, ConfigurationSource: "profile", ConfigurationHash: "synthetic", Status: "running", Phase: "encoding", OutputPath: referencePath}).Error
			case "test_encode_artifact":
				var testEncode models.TestEncode
				if err = db.Create(&models.TestEncode{SourcePath: job.MediaPath, LibraryID: job.LibraryID, ConfigurationSource: "profile", ConfigurationHash: "synthetic", Status: "completed", Phase: "complete"}).Error; err == nil {
					err = db.First(&testEncode, "source_path = ?", job.MediaPath).Error
				}
				if err == nil {
					err = db.Create(&models.TestEncodeArtifact{TestEncodeID: testEncode.ID, LibraryID: job.LibraryID, Path: referencePath, Type: "output", Expected: true, State: "owned"}).Error
				}
			case "other_job_output":
				err = db.Create(&models.QueueJob{MediaPath: "/raw/other.mkv", Status: JobStatusCompleted, OutputPath: referencePath}).Error
			case "asset_record":
				err = db.Create(&models.AssetRecord{Path: referencedOriginal, SourcePath: referencedOriginal, RootPath: fixture.workspace, RelativePath: filepath.Base(referencedOriginal), FileName: filepath.Base(referencedOriginal), Extension: ".mkv", Status: "unprocessed"}).Error
			case "other_execution_plan":
				err = db.Create(&models.ExecutionPlan{JobID: job.ID + 1000, Version: 1, Status: "frozen", OutputPath: referencePath}).Error
			case "other_publisher_manifest":
				other := models.QueueJob{MediaPath: "/raw/other.mkv", Status: JobStatusCompleted}
				if err = db.Create(&other).Error; err == nil {
					artifact, _, readErr := readLatestJobArtifact(db, job, "result")
					if readErr != nil {
						t.Fatal(readErr)
					}
					identity, identityOK := resultIdentityFromArtifact(artifact)
					binding, bindingOK := resultBindingFromArtifact(artifact)
					if !identityOK || !bindingOK {
						t.Fatalf("fixture Result binding is unavailable: %#v", artifact)
					}
					now := time.Now().UTC()
					err = db.Create(&models.PublisherPublication{
						PublicationID: "other-publication-manifest", QueueJobID: other.ID, ExecutionNumber: 1,
						ExecutionPlanID: binding.ExecutionPlanID, ExecutionPlanVersion: binding.ExecutionPlanVersion,
						ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint, LibraryID: job.LibraryID,
						DestinationRoot: fixture.validation.delivery.libraryRoot, DestinationPath: filepath.Join(fixture.validation.delivery.libraryRoot, "other.mkv"),
						PublishMode: job.PublishMode, ResultArtifactID: identity.ArtifactID,
						ResultGenerationToken: identity.GenerationToken, ResultGenerationVersion: identity.GenerationVersion,
						ResultOutputRevision: identity.OutputRevision, ValidationRunID: "other-validation-run",
						SourcePath: other.MediaPath, State: "pending", Manifest: models.JSONList{referencePath}, CreatedAt: now, UpdatedAt: now,
					}).Error
				}
			case "active_compatible_preview":
				err = db.Create(&models.CompatiblePreviewArtifactRecord{
					ArtifactID: "active-preview-ref", CachePath: filepath.Join(fixture.validation.delivery.root, "cache", "preview.mkv"),
					SourcePath: referencePath, State: string(compatiblePreviewArtifactGenerating), ExpiresAt: time.Now().UTC().Add(time.Minute),
				}).Error
			}
			if err != nil {
				t.Fatalf("create %s reference: %v", mode, err)
			}
			result, err := executeValidationStagingCleanup(db, job.ID, preview.PreviewID, "local_operator")
			if err != nil || result.Outcome != "blocked" {
				t.Fatalf("%s reference did not block cleanup: result=%#v err=%v", mode, result, err)
			}
			if _, err := os.Stat(fixture.validation.delivery.output); err != nil {
				t.Fatalf("staged output was deleted despite %s reference: %v", mode, err)
			}
		})
	}
}
