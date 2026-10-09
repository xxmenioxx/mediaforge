package handlers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type housekeepingTestRoots struct {
	raw, library, archive, work, cache, reports, logs string
}

var housekeepingTestSequence uint64

func newHousekeepingTestDB(t *testing.T) (*gorm.DB, housekeepingTestRoots) {
	t.Helper()
	housekeepingTestSequence++
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:housekeeping-engine-%d?mode=memory&cache=shared", housekeepingTestSequence)), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.AppSetting{}, &models.QueueJob{}, &models.SchedulerReservation{}, &models.AssetRecord{},
		&models.AssetMaintenanceOperation{}, &models.PublisherPublication{}, &models.TestEncode{},
		&models.TestEncodeArtifact{}, &models.RuntimeSnapshot{},
	); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	roots := housekeepingTestRoots{
		raw: filepath.Join(base, "raw"), library: filepath.Join(base, "library"), archive: filepath.Join(base, "archive"),
		work: filepath.Join(base, "work"), cache: filepath.Join(base, "cache"), reports: filepath.Join(base, "reports"), logs: filepath.Join(base, "logs"),
	}
	for _, path := range []string{roots.raw, roots.library, roots.archive, roots.work, roots.cache, roots.reports, roots.logs} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	settings := []models.AppSetting{
		{Key: "storageRoles", Value: models.JSONMap{
			"raw": models.JSONMap{"path": roots.raw}, "library": models.JSONMap{"path": roots.library},
			"originals_archive": models.JSONMap{"path": roots.archive}, "work": models.JSONMap{"path": roots.work},
			"cache": models.JSONMap{"path": roots.cache}, "reports": models.JSONMap{"path": roots.reports}, "logs": models.JSONMap{"path": roots.logs},
		}},
		{Key: "housekeeping", Value: models.JSONMap{
			"autoEnabled": true, "intervalHours": 24, "failedRetentionDays": 0, "canceledRetentionDays": 0, "orphanRetentionDays": 0,
			"publishedWorkspaceRetentionDays": 0, "publishedJobLogRetentionDays": 14, "failedJobLogRetentionDays": 0,
			"canceledJobLogRetentionDays": 0, "orphanJobLogRetentionDays": 0,
			"emptyRawDirectoriesEnabled": true, "orphanStageArtifactsEnabled": true,
		}},
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatal(err)
	}
	return db, roots
}

func housekeepingOldTime() time.Time { return time.Now().UTC().Add(-30 * 24 * time.Hour) }

func housekeepingCreateJob(t *testing.T, db *gorm.DB, status string, overrides models.QueueJob) models.QueueJob {
	t.Helper()
	old := housekeepingOldTime()
	job := models.QueueJob{MediaPath: "/synthetic/raw/source.mkv", Status: status, Stage: status, CreatedAt: old, UpdatedAt: old, FinishedAt: &old}
	if overrides.MediaPath != "" {
		job.MediaPath = overrides.MediaPath
	}
	if overrides.Stage != "" {
		job.Stage = overrides.Stage
	}
	if overrides.WorkerName != "" {
		job.WorkerName = overrides.WorkerName
	}
	if overrides.ValidationStatus != "" {
		job.ValidationStatus = overrides.ValidationStatus
	}
	if overrides.ValidationOutcome != "" {
		job.ValidationOutcome = overrides.ValidationOutcome
	}
	job.ValidationActionable = overrides.ValidationActionable
	job.ValidationOverride = overrides.ValidationOverride
	job.OutputPath = overrides.OutputPath
	job.PublishedPath = overrides.PublishedPath
	if overrides.PublishedAt != nil {
		job.PublishedAt = overrides.PublishedAt
	}
	if overrides.FinishedAt == nil {
		job.FinishedAt = &old
	} else {
		job.FinishedAt = overrides.FinishedAt
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&job).UpdateColumn("updated_at", old).Error; err != nil {
		t.Fatal(err)
	}
	return job
}

func housekeepingWriteFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func housekeepingWorkspace(t *testing.T, root string, id uint, fileName string) string {
	t.Helper()
	path := filepath.Join(root, "job-"+strconv.FormatUint(uint64(id), 10))
	housekeepingWriteFile(t, filepath.Join(path, fileName), "fixture")
	old := housekeepingOldTime()
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	return path
}

func housekeepingCandidateByPath(report HousekeepingReport, path, kind string) (HousekeepingCandidate, bool) {
	for _, candidate := range report.Candidates {
		if sameHousekeepingPath(candidate.Path, path) && candidate.Kind == kind {
			return candidate, true
		}
	}
	return HousekeepingCandidate{}, false
}

func TestHousekeepingPublishedWorkspaceCleanupAndUnpublishedProtection(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	old := housekeepingOldTime()
	published := housekeepingCreateJob(t, db, JobStatusCompleted, models.QueueJob{PublishedAt: &old})
	unpublished := housekeepingCreateJob(t, db, JobStatusCompleted, models.QueueJob{})
	publishedPath := housekeepingWorkspace(t, roots.work, published.ID, "published.part")
	unpublishedPath := housekeepingWorkspace(t, roots.work, unpublished.ID, "result.mkv")

	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(publishedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published workspace remains: %v", err)
	}
	if _, err := os.Stat(unpublishedPath); err != nil {
		t.Fatalf("completed unpublished output was removed: %v", err)
	}
	candidate, ok := housekeepingCandidateByPath(report, unpublishedPath, housekeepingKindWorkspace)
	if !ok || candidate.SafeToDelete || candidate.BlockedReason == "" {
		t.Fatalf("unpublished workspace was not reported as blocked: %#v", candidate)
	}
	var historicalJobs int64
	if err := db.Model(&models.QueueJob{}).Count(&historicalJobs).Error; err != nil || historicalJobs != 2 {
		t.Fatalf("QueueJob history was changed: count=%d err=%v", historicalJobs, err)
	}
}

func TestHousekeepingFailedCanceledRetentionAndActiveOwnershipGuards(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	failed := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{})
	canceled := housekeepingCreateJob(t, db, JobStatusCanceled, models.QueueJob{})
	worker := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{WorkerName: "worker-a"})
	reserved := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{})
	paths := map[uint]string{
		failed.ID:   housekeepingWorkspace(t, roots.work, failed.ID, "failed.bin"),
		canceled.ID: housekeepingWorkspace(t, roots.work, canceled.ID, "canceled.bin"),
		worker.ID:   housekeepingWorkspace(t, roots.work, worker.ID, "worker.bin"),
		reserved.ID: housekeepingWorkspace(t, roots.work, reserved.ID, "reserved.bin"),
	}
	if err := db.Create(&models.SchedulerReservation{JobID: reserved.ID, AssetKey: reserved.MediaPath, State: "active", WorkerName: "worker-b"}).Error; err != nil {
		t.Fatal(err)
	}

	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{failed.ID, canceled.ID} {
		if _, err := os.Lstat(paths[id]); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expired terminal workspace %d remains: %v", id, err)
		}
	}
	for _, id := range []uint{worker.ID, reserved.ID} {
		if _, err := os.Stat(paths[id]); err != nil {
			t.Errorf("active ownership workspace %d was removed: %v", id, err)
		}
		candidate, ok := housekeepingCandidateByPath(report, paths[id], housekeepingKindWorkspace)
		if !ok || candidate.SafeToDelete || candidate.BlockedReason == "" {
			t.Errorf("active ownership was not reported for job %d: %#v", id, candidate)
		}
	}
}

func TestHousekeepingJobLogLifecycleRetentionAndSystemLogExclusion(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	old := housekeepingOldTime()
	failed := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{})
	canceled := housekeepingCreateJob(t, db, JobStatusCanceled, models.QueueJob{})
	publishedOld := housekeepingCreateJob(t, db, JobStatusCompleted, models.QueueJob{PublishedAt: &old})
	publishedRecentTime := time.Now().UTC().Add(-48 * time.Hour)
	publishedRecent := housekeepingCreateJob(t, db, JobStatusCompleted, models.QueueJob{PublishedAt: &publishedRecentTime})
	active := housekeepingCreateJob(t, db, JobStatusQueued, models.QueueJob{FinishedAt: nil})
	blocked := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{Stage: "validation_review", ValidationOutcome: "BLOCKED"})
	paths := map[uint]string{}
	for _, job := range []models.QueueJob{failed, canceled, publishedOld, publishedRecent, active, blocked} {
		path := filepath.Join(roots.logs, fmt.Sprintf("job-%d.log", job.ID))
		housekeepingWriteFile(t, path, "log")
		paths[job.ID] = path
	}
	orphanPath := filepath.Join(roots.logs, "job-991991.log")
	housekeepingWriteFile(t, orphanPath, "orphan log")
	for _, name := range []string{"backend.log", "scheduler.log", "workers.log"} {
		housekeepingWriteFile(t, filepath.Join(roots.logs, name), "system log")
	}

	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{failed.ID, canceled.ID, publishedOld.ID} {
		if _, err := os.Lstat(paths[id]); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expired terminal log for job %d remains: %v", id, err)
		}
	}
	if _, err := os.Lstat(orphanPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expired orphan log remains: %v", err)
	}
	for _, id := range []uint{publishedRecent.ID, active.ID, blocked.ID} {
		if _, err := os.Stat(paths[id]); err != nil {
			t.Errorf("protected job log %d was removed: %v", id, err)
		}
		candidate, ok := housekeepingCandidateByPath(report, paths[id], housekeepingKindJobLog)
		if !ok || candidate.SafeToDelete || candidate.BlockedReason == "" {
			t.Errorf("protected log %d was not reported as blocked: %#v", id, candidate)
		}
	}
	for _, name := range []string{"backend.log", "scheduler.log", "workers.log"} {
		path := filepath.Join(roots.logs, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("system log %s was touched: %v", name, err)
		}
		if _, exists := housekeepingCandidateByPath(report, path, housekeepingKindJobLog); exists {
			t.Errorf("system log %s was classified as a job log", name)
		}
	}
	var jobs int64
	if err := db.Model(&models.QueueJob{}).Count(&jobs).Error; err != nil || jobs != 6 {
		t.Fatalf("job lifecycle history was changed: count=%d err=%v", jobs, err)
	}
}

func TestHousekeepingCleansPersistedAbandonedStageArtifactsAndPreservesRecords(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	old := housekeepingOldTime()
	maintenancePath := filepath.Join(roots.cache, "subtitle", "operation-owned.data")
	housekeepingWriteFile(t, maintenancePath, "subtitle temp")
	operation := models.AssetMaintenanceOperation{
		ID: "op-abandoned-subtitle", OperationType: "subtitle_ocr", AssetPath: "/synthetic/raw/movie.mkv",
		TemporaryPath: maintenancePath, Status: "interrupted", Phase: "interrupted", StartedAt: &old, FinishedAt: &old, CreatedAt: old, UpdatedAt: old,
	}
	if err := db.Create(&operation).Error; err != nil {
		t.Fatal(err)
	}
	archiveStage := filepath.Join(roots.archive, "publisher-stage.part")
	archivedOriginal := filepath.Join(roots.archive, "original.mkv")
	housekeepingWriteFile(t, archiveStage, "abandoned copy")
	housekeepingWriteFile(t, archivedOriginal, "preserve original")
	publication := models.PublisherPublication{
		PublicationID: "pub-terminal", QueueJobID: 9001, ExecutionNumber: 1, ExecutionPlanID: 1, ExecutionPlanVersion: 1,
		ExecutionPlanFingerprint: "plan-fingerprint", LibraryID: 1, DestinationRoot: roots.library, DestinationPath: filepath.Join(roots.library, "movie.mkv"),
		PublishMode: "standard", ResultArtifactID: "result-1", ResultGenerationToken: "generation-1", ResultGenerationVersion: 1,
		ResultOutputRevision: "revision-1", ValidationRunID: "validation-1", State: PublicationPublished, Manifest: models.JSONList{},
		OriginalArchiveStatus: "failed", OriginalArchiveTemporaryPath: archiveStage, OriginalArchivedPath: archivedOriginal,
		CreatedAt: old, UpdatedAt: old,
	}
	if err := db.Create(&publication).Error; err != nil {
		t.Fatal(err)
	}

	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{maintenancePath, archiveStage} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("abandoned stage artifact remains: %s (%v)", path, err)
		}
	}
	if _, err := os.Stat(archivedOriginal); err != nil {
		t.Fatalf("canonical archived original was removed: %v", err)
	}
	var operationCount, publicationCount int64
	if err := db.Model(&models.AssetMaintenanceOperation{}).Count(&operationCount).Error; err != nil || operationCount != 1 {
		t.Fatalf("maintenance audit row was removed: count=%d err=%v", operationCount, err)
	}
	if err := db.Model(&models.PublisherPublication{}).Count(&publicationCount).Error; err != nil || publicationCount != 1 {
		t.Fatalf("publication lifecycle row was removed: count=%d err=%v", publicationCount, err)
	}
	for _, path := range []string{maintenancePath, archiveStage} {
		candidate, ok := housekeepingCandidateByPath(report, path, housekeepingKindStageArtifact)
		if !ok || !candidate.SafeToDelete {
			t.Errorf("owned stage artifact was not previewed as safe: %#v", candidate)
		}
	}
}

func TestHousekeepingAmbiguousActiveAndCanonicalArtifactsStayUntouched(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	outside := filepath.Join(t.TempDir(), "unowned.data")
	housekeepingWriteFile(t, outside, "unowned")
	symlinkTarget := filepath.Join(t.TempDir(), "escaped.data")
	housekeepingWriteFile(t, symlinkTarget, "outside target")
	if err := os.Symlink(filepath.Dir(symlinkTarget), filepath.Join(roots.cache, "escape")); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(roots.cache, "escape", filepath.Base(symlinkTarget))
	sharedPath := filepath.Join(roots.cache, "shared.data")
	housekeepingWriteFile(t, sharedPath, "active reference")
	resultPath := filepath.Join(roots.cache, "canonical-result.data")
	housekeepingWriteFile(t, resultPath, "canonical result")
	requiredSidecarPath := filepath.Join(roots.cache, "required-sidecar.srt")
	housekeepingWriteFile(t, requiredSidecarPath, "required subtitle sidecar")
	old := housekeepingOldTime()
	operations := []models.AssetMaintenanceOperation{
		{ID: "op-outside", OperationType: "ffmpeg", AssetPath: "/synthetic/raw/a.mkv", TemporaryPath: outside, Status: "failed", Phase: "failed", FinishedAt: &old, CreatedAt: old, UpdatedAt: old},
		{ID: "op-symlink", OperationType: "ocr", AssetPath: "/synthetic/raw/b.mkv", TemporaryPath: symlinkPath, Status: "failed", Phase: "failed", FinishedAt: &old, CreatedAt: old, UpdatedAt: old},
		{ID: "op-shared", OperationType: "analysis", AssetPath: "/synthetic/raw/c.mkv", TemporaryPath: sharedPath, Status: "failed", Phase: "failed", FinishedAt: &old, CreatedAt: old, UpdatedAt: old},
		{ID: "op-shared-active", OperationType: "analysis", AssetPath: "/synthetic/raw/c.mkv", TemporaryPath: sharedPath, Status: "running", Phase: "running", CreatedAt: old, UpdatedAt: time.Now().UTC()},
		{ID: "op-result", OperationType: "ffmpeg", AssetPath: "/synthetic/raw/d.mkv", TemporaryPath: resultPath, Status: "failed", Phase: "failed", FinishedAt: &old, CreatedAt: old, UpdatedAt: old},
		{ID: "op-required-sidecar", OperationType: "subtitle_ocr", AssetPath: "/synthetic/raw/e.mkv", TemporaryPath: requiredSidecarPath, Status: "failed", Phase: "failed", FinishedAt: &old, CreatedAt: old, UpdatedAt: old},
	}
	if err := db.Create(&operations).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.QueueJob{MediaPath: "/synthetic/raw/result.mkv", OutputPath: resultPath, Status: JobStatusFailed, FinishedAt: &old}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.QueueJob{MediaPath: "/synthetic/raw/sidecar.mkv", SubtitleArtifacts: models.JSONList{models.JSONMap{"path": requiredSidecarPath}}, Status: JobStatusFailed, FinishedAt: &old}).Error; err != nil {
		t.Fatal(err)
	}

	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{outside, symlinkTarget, sharedPath, resultPath, requiredSidecarPath} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("ambiguous or referenced artifact %s was removed: %v", path, err)
		}
		candidate, ok := housekeepingCandidateByPath(report, path, housekeepingKindStageArtifact)
		if !ok || candidate.SafeToDelete || candidate.BlockedReason == "" {
			t.Errorf("unsafe artifact %s was not reported as blocked: %#v", path, candidate)
		}
	}
}

func TestHousekeepingRawDirectoriesRequireLiteralEmptinessAndProtectReferences(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	emptyChild := filepath.Join(roots.raw, "empty", "child")
	if err := os.MkdirAll(emptyChild, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"contains-mkv/movie.mkv": "mkv", "contains-mp4/movie.mp4": "mp4",
		"contains-srt/movie.srt": "srt", "contains-nfo/show.nfo": "nfo", "contains-artwork/poster.jpg": "artwork",
	} {
		housekeepingWriteFile(t, filepath.Join(roots.raw, name), contents)
	}
	assetReferenceDir := filepath.Join(roots.raw, "asset-referenced")
	queueReferenceDir := filepath.Join(roots.raw, "queue-referenced")
	if err := os.MkdirAll(assetReferenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(queueReferenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: filepath.Join(assetReferenceDir, "current.mkv"), RootPath: roots.raw, Status: "unprocessed", Missing: false}).Error; err != nil {
		t.Fatal(err)
	}
	old := housekeepingOldTime()
	if err := db.Create(&models.QueueJob{MediaPath: filepath.Join(queueReferenceDir, "restore-target.mkv"), Status: JobStatusFailed, FinishedAt: &old}).Error; err != nil {
		t.Fatal(err)
	}

	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{emptyChild, filepath.Join(roots.raw, "empty")} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("truly empty RAW directory remains after bottom-up cleanup: %s (%v)", path, err)
		}
	}
	if _, err := os.Stat(roots.raw); err != nil {
		t.Fatalf("configured RAW root was removed: %v", err)
	}
	for _, path := range []string{assetReferenceDir, queueReferenceDir, filepath.Join(roots.raw, "contains-mkv"), filepath.Join(roots.raw, "contains-mp4"), filepath.Join(roots.raw, "contains-srt"), filepath.Join(roots.raw, "contains-nfo"), filepath.Join(roots.raw, "contains-artwork")} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("nonempty or referenced RAW directory was removed: %s (%v)", path, err)
		}
	}
	if _, ok := housekeepingCandidateByPath(report, roots.raw, housekeepingKindEmptyDirectory); ok {
		t.Fatal("configured RAW root appeared as a candidate")
	}
	assetCandidate, ok := housekeepingCandidateByPath(report, assetReferenceDir, housekeepingKindEmptyDirectory)
	if !ok || assetCandidate.SafeToDelete {
		t.Fatalf("AssetRecord-referenced empty directory was not protected: %#v", assetCandidate)
	}
}

func TestHousekeepingRemovesOnlyEmptyOperationalDirectories(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	rawEmpty := filepath.Join(roots.raw, "empty-but-disabled")
	if err := os.Mkdir(rawEmpty, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		filepath.Join(roots.work, "empty", "nested"), filepath.Join(roots.cache, "empty"),
		filepath.Join(roots.reports, "empty"), filepath.Join(roots.logs, "empty"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(roots.reports, "nonempty"), 0o755); err != nil {
		t.Fatal(err)
	}
	housekeepingWriteFile(t, filepath.Join(roots.reports, "nonempty", "report.json"), "report")
	var policy models.AppSetting
	if err := db.Where("key = ?", "housekeeping").First(&policy).Error; err != nil {
		t.Fatal(err)
	}
	policy.Value["emptyRawDirectoriesEnabled"] = false
	if err := db.Save(&policy).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := RunHousekeeping(db, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("empty operational directory remains: %s (%v)", path, err)
		}
	}
	for _, root := range []string{roots.work, roots.cache, roots.reports, roots.logs} {
		if _, err := os.Stat(root); err != nil {
			t.Errorf("configured role root was removed: %s (%v)", root, err)
		}
	}
	if _, err := os.Stat(rawEmpty); err != nil {
		t.Fatalf("disabled empty RAW directory was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(roots.reports, "nonempty", "report.json")); err != nil {
		t.Fatalf("nonempty report directory was touched: %v", err)
	}
}

func TestHousekeepingPreviewBindsExactCandidatesAndConsumesToken(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	job := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{})
	previewedPath := housekeepingWorkspace(t, roots.work, job.ID, "previewed.bin")
	preview, err := PreviewHousekeeping(db)
	if err != nil {
		t.Fatal(err)
	}
	if preview.PreviewID == "" || preview.PreviewExpiresAt == nil {
		t.Fatalf("preview did not return a persisted token and expiry: %#v", preview)
	}
	newPath := housekeepingWorkspace(t, roots.work, 882200, "late.bin")
	result, err := ExecuteHousekeepingPreview(db, preview.PreviewID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(previewedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previewed workspace was not deleted: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("newly discovered workspace outside preview set was deleted: %v", err)
	}
	if len(result.RemovedPaths) != 1 || !sameHousekeepingPath(result.RemovedPaths[0], previewedPath) {
		t.Fatalf("execution did not use the exact previewed candidate set: %#v", result.RemovedPaths)
	}
	if _, err := ExecuteHousekeepingPreview(db, preview.PreviewID); !errors.Is(err, errStaleHousekeepingPreview) {
		t.Fatalf("consumed preview token was reusable: %v", err)
	}
}

func TestHousekeepingRejectsStalePreviewAndRevalidatesTOCTOU(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	staleJob := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{})
	stalePath := housekeepingWorkspace(t, roots.work, staleJob.ID, "stale.bin")
	stalePreview, err := PreviewHousekeeping(db)
	if err != nil {
		t.Fatal(err)
	}
	var setting models.AppSetting
	if err := db.Where("key = ?", "housekeeping").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	setting.Value["failedRetentionDays"] = float64(1)
	if err := db.Save(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteHousekeepingPreview(db, stalePreview.PreviewID); !errors.Is(err, errStaleHousekeepingPreview) {
		t.Fatalf("policy change did not invalidate preview: %v", err)
	}
	if _, err := os.Stat(stalePath); err != nil {
		t.Fatalf("stale preview deleted data: %v", err)
	}

	toctouDB, toctouRoots := newHousekeepingTestDB(t)
	toctouJob := housekeepingCreateJob(t, toctouDB, JobStatusFailed, models.QueueJob{})
	toctouPath := housekeepingWorkspace(t, toctouRoots.work, toctouJob.ID, "mutable.bin")
	preview, err := PreviewHousekeeping(toctouDB)
	if err != nil {
		t.Fatal(err)
	}
	housekeepingWriteFile(t, filepath.Join(toctouPath, "mutable.bin"), "changed after preview to a different size")
	result, err := ExecuteHousekeepingPreview(toctouDB, preview.PreviewID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(toctouPath); err != nil {
		t.Fatalf("changed workspace was deleted after preview: %v", err)
	}
	candidate, ok := housekeepingCandidateByPath(result, toctouPath, housekeepingKindWorkspace)
	if !ok || candidate.SafeToDelete || candidate.BlockedReason == "" {
		t.Fatalf("TOCTOU change was not reported as blocked: %#v", candidate)
	}
}

func TestHousekeepingAutomaticAndManualExecutionShareActiveJobGuards(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(fmt.Sprintf("manual-%v", manual), func(t *testing.T) {
			db, roots := newHousekeepingTestDB(t)
			job := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{})
			path := housekeepingWorkspace(t, roots.work, job.ID, "locked.bin")
			if err := db.Create(&models.SchedulerReservation{JobID: job.ID, AssetKey: job.MediaPath, State: "active", WorkerName: "worker"}).Error; err != nil {
				t.Fatal(err)
			}
			var report HousekeepingReport
			var err error
			if manual {
				preview, previewErr := PreviewHousekeeping(db)
				if previewErr != nil {
					t.Fatal(previewErr)
				}
				report, err = ExecuteHousekeepingPreview(db, preview.PreviewID)
			} else {
				report, err = RunHousekeeping(db, false)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("reserved workspace was removed: %v", err)
			}
			candidate, ok := housekeepingCandidateByPath(report, path, housekeepingKindWorkspace)
			if !ok || candidate.SafeToDelete || candidate.BlockedReason == "" {
				t.Fatalf("execution path did not share the active reservation guard: %#v", candidate)
			}
		})
	}
}
