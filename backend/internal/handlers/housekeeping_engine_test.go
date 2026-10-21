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
	if overrides.StartedAt != nil {
		job.StartedAt = overrides.StartedAt
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

func TestHousekeepingTerminalWorkerProvenanceDoesNotBlockWorkspaceOrLogRetention(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	old := housekeepingOldTime()
	jobs := []models.QueueJob{
		housekeepingCreateJob(t, db, JobStatusCompleted, models.QueueJob{WorkerName: "local-worker", PublishedAt: &old, StartedAt: &old, FinishedAt: &old}),
		housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{WorkerName: "local-worker", StartedAt: &old, FinishedAt: &old}),
		housekeepingCreateJob(t, db, JobStatusCanceled, models.QueueJob{WorkerName: "local-worker", StartedAt: &old, FinishedAt: &old}),
	}
	workspaces := map[uint]string{}
	logs := map[uint]string{}
	for _, job := range jobs {
		workspaces[job.ID] = housekeepingWorkspace(t, roots.work, job.ID, "retained-output.bin")
		logs[job.ID] = filepath.Join(roots.logs, fmt.Sprintf("job-%d.log", job.ID))
		housekeepingWriteFile(t, logs[job.ID], "historical worker log")
	}

	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if _, err := os.Lstat(workspaces[job.ID]); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("terminal workspace %d with historical WorkerName remains: %v", job.ID, err)
		}
		if _, err := os.Lstat(logs[job.ID]); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("terminal job log %d with historical WorkerName remains: %v", job.ID, err)
		}
		for path, kind := range map[string]string{workspaces[job.ID]: housekeepingKindWorkspace, logs[job.ID]: housekeepingKindJobLog} {
			candidate, ok := housekeepingCandidateByPath(report, path, kind)
			if !ok || candidate.SafeToDelete || candidate.Outcome != "cleaned" {
				t.Errorf("terminal job %d cleanup outcome was not retained for %s: %#v", job.ID, kind, candidate)
			}
		}
	}
	var historicalJobs int64
	if err := db.Model(&models.QueueJob{}).Count(&historicalJobs).Error; err != nil || historicalJobs != int64(len(jobs)) {
		t.Fatalf("QueueJob history or WorkerName provenance changed: count=%d err=%v", historicalJobs, err)
	}
}

func TestHousekeepingFailedCanceledRetentionAndActiveOwnershipGuards(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	failed := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{})
	canceled := housekeepingCreateJob(t, db, JobStatusCanceled, models.QueueJob{})
	reserved := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{})
	processStarted := housekeepingOldTime()
	process := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{WorkerName: "local-worker", StartedAt: &processStarted})
	if err := db.Model(&process).UpdateColumn("finished_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	paths := map[uint]string{
		failed.ID:   housekeepingWorkspace(t, roots.work, failed.ID, "failed.bin"),
		canceled.ID: housekeepingWorkspace(t, roots.work, canceled.ID, "canceled.bin"),
		reserved.ID: housekeepingWorkspace(t, roots.work, reserved.ID, "reserved.bin"),
		process.ID:  housekeepingWorkspace(t, roots.work, process.ID, "process.bin"),
	}
	logs := map[uint]string{}
	for _, job := range []models.QueueJob{reserved, process} {
		logs[job.ID] = filepath.Join(roots.logs, fmt.Sprintf("job-%d.log", job.ID))
		housekeepingWriteFile(t, logs[job.ID], "owned by active lifecycle")
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
	for _, id := range []uint{reserved.ID, process.ID} {
		if _, err := os.Stat(paths[id]); err != nil {
			t.Errorf("active ownership workspace %d was removed: %v", id, err)
		}
		candidate, ok := housekeepingCandidateByPath(report, paths[id], housekeepingKindWorkspace)
		if !ok || candidate.SafeToDelete || candidate.BlockedReason == "" {
			t.Errorf("active ownership was not reported for job %d: %#v", id, candidate)
		}
		if _, err := os.Stat(logs[id]); err != nil {
			t.Errorf("active ownership job log %d was removed: %v", id, err)
		}
		logCandidate, ok := housekeepingCandidateByPath(report, logs[id], housekeepingKindJobLog)
		if !ok || logCandidate.SafeToDelete || logCandidate.BlockedReason == "" {
			t.Errorf("active ownership was not reported for job log %d: %#v", id, logCandidate)
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
		if !ok || candidate.SafeToDelete || candidate.Outcome != "cleaned" {
			t.Errorf("owned stage artifact cleanup outcome was not recorded: %#v", candidate)
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
	queuedReferenceDir := filepath.Join(roots.raw, "queued-reference")
	processReferenceDir := filepath.Join(roots.raw, "active-process-reference")
	reservedReferenceDir := filepath.Join(roots.raw, "active-reservation-reference")
	publisherReferenceDir := filepath.Join(roots.raw, "active-publisher-reference")
	rootReferenceDir := filepath.Join(roots.raw, "asset-root-reference")
	groupReferenceDir := filepath.Join(roots.raw, "asset-group-reference")
	maintenanceReferenceDir := filepath.Join(roots.raw, "active-maintenance-reference")
	for _, path := range []string{assetReferenceDir, queueReferenceDir, queuedReferenceDir, processReferenceDir, reservedReferenceDir, publisherReferenceDir, rootReferenceDir, groupReferenceDir, maintenanceReferenceDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	assetRecord := models.AssetRecord{Path: filepath.Join(assetReferenceDir, "current.mkv"), SourcePath: filepath.Join(assetReferenceDir, "source.mkv"), RootPath: roots.raw, Status: "unprocessed", Missing: false}
	if err := db.Create(&assetRecord).Error; err != nil {
		t.Fatal(err)
	}
	rootGroupRecord := models.AssetRecord{Path: "/synthetic/movie.mkv", RootPath: rootReferenceDir, GroupPath: groupReferenceDir, Status: "unprocessed", Missing: false}
	if err := db.Create(&rootGroupRecord).Error; err != nil {
		t.Fatal(err)
	}
	old := housekeepingOldTime()
	historicalPublished := housekeepingCreateJob(t, db, JobStatusCompleted, models.QueueJob{
		MediaPath: filepath.Join(queueReferenceDir, "restore-target.mkv"), WorkerName: "local-worker", PublishedAt: &old, StartedAt: &old, FinishedAt: &old,
	})
	if historicalPublished.WorkerName != "local-worker" {
		t.Fatal("historical QueueJob WorkerName provenance was not retained")
	}
	queuedJob := housekeepingCreateJob(t, db, JobStatusQueued, models.QueueJob{MediaPath: filepath.Join(queuedReferenceDir, "queued.mkv")})
	processJob := housekeepingCreateJob(t, db, JobStatusFailed, models.QueueJob{MediaPath: filepath.Join(processReferenceDir, "processing.mkv"), WorkerName: "local-worker", StartedAt: &old})
	if err := db.Model(&processJob).UpdateColumn("finished_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	reservedJob := housekeepingCreateJob(t, db, JobStatusCompleted, models.QueueJob{MediaPath: filepath.Join(reservedReferenceDir, "reserved.mkv"), PublishedAt: &old, StartedAt: &old, FinishedAt: &old})
	reservation := models.SchedulerReservation{JobID: reservedJob.ID, AssetKey: reservedJob.MediaPath, State: "active", WorkerName: "local-worker"}
	if err := db.Create(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	maintenanceOperation := models.AssetMaintenanceOperation{
		ID: "maintenance-raw-active", AssetPath: filepath.Join(maintenanceReferenceDir, "working.mkv"),
		TemporaryPath: filepath.Join(maintenanceReferenceDir, ".working.mkv.maintenance-raw-active.tmp.mkv"),
		BackupPath:    filepath.Join(maintenanceReferenceDir, ".working.mkv.maintenance-raw-active.backup.mkv"),
		Status:        maintenanceStatusRunning, Phase: "committing",
	}
	if err := db.Create(&maintenanceOperation).Error; err != nil {
		t.Fatal(err)
	}
	publisherJob := housekeepingCreateJob(t, db, JobStatusCompleted, models.QueueJob{MediaPath: filepath.Join(publisherReferenceDir, "publishing.mkv"), PublishedAt: &old, StartedAt: &old, FinishedAt: &old})
	if err := db.Create(&models.PublisherPublication{
		PublicationID: "active-publisher-publication", QueueJobID: publisherJob.ID, ExecutionNumber: 1,
		ExecutionPlanID: 1, ExecutionPlanVersion: 1, ExecutionPlanFingerprint: "plan-fingerprint", LibraryID: 1,
		DestinationRoot: roots.library, DestinationPath: filepath.Join(roots.library, "published.mkv"), PublishMode: "standard",
		ResultArtifactID: "result-artifact", ResultGenerationToken: "generation-token", ResultGenerationVersion: 1,
		ResultOutputRevision: "output-revision", ValidationRunID: "validation-run", SourcePath: publisherJob.MediaPath,
		State: "publishing", ActiveAttemptID: "attempt-1", Manifest: models.JSONList{},
	}).Error; err != nil {
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
	if _, err := os.Lstat(queueReferenceDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("historical published QueueJob kept an empty RAW directory: %v", err)
	}
	for _, path := range []string{assetReferenceDir, queuedReferenceDir, processReferenceDir, reservedReferenceDir, publisherReferenceDir, rootReferenceDir, groupReferenceDir, maintenanceReferenceDir, filepath.Join(roots.raw, "contains-mkv"), filepath.Join(roots.raw, "contains-mp4"), filepath.Join(roots.raw, "contains-srt"), filepath.Join(roots.raw, "contains-nfo"), filepath.Join(roots.raw, "contains-artwork")} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("nonempty or referenced RAW directory was removed: %s (%v)", path, err)
		}
	}
	for _, path := range []string{queuedReferenceDir, processReferenceDir, reservedReferenceDir, publisherReferenceDir, maintenanceReferenceDir} {
		candidate, ok := housekeepingCandidateByPath(report, path, housekeepingKindEmptyDirectory)
		if !ok || candidate.SafeToDelete {
			t.Errorf("current Queue ownership did not protect RAW directory %s: %#v", path, candidate)
		}
	}
	if _, ok := housekeepingCandidateByPath(report, roots.raw, housekeepingKindEmptyDirectory); ok {
		t.Fatal("configured RAW root appeared as a candidate")
	}
	assetCandidate, ok := housekeepingCandidateByPath(report, assetReferenceDir, housekeepingKindEmptyDirectory)
	if !ok || assetCandidate.SafeToDelete || !housekeepingHasProtectionReference(assetCandidate, "AssetRecord", strconv.FormatUint(uint64(assetRecord.ID), 10), "path") {
		t.Fatalf("AssetRecord-referenced empty directory was not protected: %#v", assetCandidate)
	}
	if !housekeepingHasProtectionReference(assetCandidate, "AssetRecord", strconv.FormatUint(uint64(assetRecord.ID), 10), "source_path") {
		t.Fatalf("AssetRecord source_path was not identified: %#v", assetCandidate)
	}
	for _, reference := range []struct {
		path  string
		field string
	}{{rootReferenceDir, "root_path"}, {groupReferenceDir, "group_path"}} {
		candidate, ok := housekeepingCandidateByPath(report, reference.path, housekeepingKindEmptyDirectory)
		if !ok || candidate.SafeToDelete || !housekeepingHasProtectionReference(candidate, "AssetRecord", strconv.FormatUint(uint64(rootGroupRecord.ID), 10), reference.field) {
			t.Errorf("AssetRecord %s was not identified: %#v", reference.field, candidate)
		}
	}
	queuedCandidate, ok := housekeepingCandidateByPath(report, queuedReferenceDir, housekeepingKindEmptyDirectory)
	if !ok || !housekeepingHasProtectionReference(queuedCandidate, "QueueJob", strconv.FormatUint(uint64(queuedJob.ID), 10), "media_path") {
		t.Fatalf("QueueJob source reference was not identified: %#v", queuedCandidate)
	}
	reservedCandidate, ok := housekeepingCandidateByPath(report, reservedReferenceDir, housekeepingKindEmptyDirectory)
	if !ok || !housekeepingHasProtectionReference(reservedCandidate, "SchedulerReservation", strconv.FormatUint(uint64(reservation.ID), 10), "asset_key") {
		t.Fatalf("active reservation was not identified: %#v", reservedCandidate)
	}
	publisherCandidate, ok := housekeepingCandidateByPath(report, publisherReferenceDir, housekeepingKindEmptyDirectory)
	if !ok || !housekeepingHasProtectionReference(publisherCandidate, "PublisherPublication", "active-publisher-publication", "source_path") {
		t.Fatalf("active Publisher lease was not identified: %#v", publisherCandidate)
	}
	maintenanceCandidate, ok := housekeepingCandidateByPath(report, maintenanceReferenceDir, housekeepingKindEmptyDirectory)
	if !ok || !housekeepingHasProtectionReference(maintenanceCandidate, "AssetMaintenanceOperation", maintenanceOperation.ID, "asset_path") {
		t.Fatalf("active maintenance operation was not identified: %#v", maintenanceCandidate)
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

func TestHousekeepingCompletedLibraryMaintenanceTemporaryIsAlreadyAbsentNoop(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	assetPath := filepath.Join(roots.library, "Akira", "Akira_t00.mkv")
	temporaryPath := filepath.Join(filepath.Dir(assetPath), ".Akira_t00.mkv.maintenance-noop.tmp.mkv")
	backupPath := filepath.Join(filepath.Dir(assetPath), ".Akira_t00.mkv.maintenance-noop.backup.mkv")
	finished := housekeepingOldTime()
	operation := models.AssetMaintenanceOperation{
		ID: "maintenance-noop", OperationType: "remove_tracks", AssetPath: assetPath,
		TemporaryPath: temporaryPath, BackupPath: backupPath, Status: maintenanceStatusComplete, Phase: "completed",
		FinishedAt: &finished, CreatedAt: finished, UpdatedAt: finished,
	}
	if err := db.Create(&operation).Error; err != nil {
		t.Fatal(err)
	}

	report, err := PreviewHousekeeping(db)
	if err != nil {
		t.Fatal(err)
	}
	candidate, ok := housekeepingCandidateByPath(report, temporaryPath, housekeepingKindStageArtifact)
	if !ok || candidate.SafeToDelete || candidate.Outcome != "already_absent" || candidate.SizeBytes != 0 || candidate.BlockedReason != "" {
		t.Fatalf("completed missing temporary was not reported as a zero-byte no-op: %#v", candidate)
	}
	if candidate.Maintenance == nil || candidate.Maintenance.Status != "completed" || candidate.Maintenance.Phase != "completed" || candidate.Maintenance.TemporaryPresence != "absent" || candidate.Maintenance.BackupPresence != "absent" {
		t.Fatalf("maintenance state was not diagnosed: %#v", candidate.Maintenance)
	}
	for _, summary := range report.Summaries {
		if summary.Kind == housekeepingKindStageArtifact && summary.RecoverableBytes != 0 {
			t.Fatalf("missing temporary contributed recoverable bytes: %#v", summary)
		}
	}
	result, err := ExecuteHousekeepingPreview(db, report.PreviewID)
	if err != nil {
		t.Fatal(err)
	}
	noOp, ok := housekeepingCandidateByPath(result, temporaryPath, housekeepingKindStageArtifact)
	if !ok || result.Outcome != "already_absent" || noOp.Outcome != "already_absent" || result.RecoveredBytes != 0 || len(result.RemovedPaths) != 0 {
		t.Fatalf("already absent outcome was not retained durably: candidate=%#v report=%#v", noOp, result)
	}
}

func TestHousekeepingCompletedLibraryMaintenanceTemporaryRequiresManualBoundCleanup(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	assetPath := filepath.Join(roots.library, "Akira", "Akira_t00.mkv")
	content := []byte("completed maintenance result fixture")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	temporaryPath := filepath.Join(filepath.Dir(assetPath), ".Akira_t00.mkv.maintenance-residual.tmp.mkv")
	backupPath := filepath.Join(filepath.Dir(assetPath), ".Akira_t00.mkv.maintenance-residual.backup.mkv")
	if err := os.WriteFile(temporaryPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := mediaFileFingerprint(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: assetPath, RootPath: roots.library, FileName: filepath.Base(assetPath), Status: "processed", SizeBytes: int64(len(content)), Missing: false}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	finished := housekeepingOldTime()
	operation := models.AssetMaintenanceOperation{
		ID: "maintenance-residual", OperationType: "remove_tracks", AssetRecordID: record.ID, AssetPath: assetPath,
		OriginalFingerprint: "original-fingerprint", ResultFingerprint: fingerprint,
		TemporaryPath: temporaryPath, BackupPath: backupPath, Status: maintenanceStatusComplete, Phase: "completed",
		FinishedAt: &finished, CreatedAt: finished, UpdatedAt: finished,
	}
	if err := db.Create(&operation).Error; err != nil {
		t.Fatal(err)
	}

	automatic, err := runAutomaticHousekeeping(db)
	if err != nil {
		t.Fatal(err)
	}
	automaticCandidate, ok := housekeepingCandidateByPath(automatic, temporaryPath, housekeepingKindStageArtifact)
	if !ok || automaticCandidate.SafeToDelete || !automaticCandidate.ManualOnly || automaticCandidate.Outcome != "blocked" {
		t.Fatalf("automatic housekeeping did not retain Library maintenance output: %#v", automaticCandidate)
	}
	if _, err := os.Stat(temporaryPath); err != nil {
		t.Fatalf("automatic housekeeping removed Library maintenance output: %v", err)
	}

	preview, err := PreviewHousekeeping(db)
	if err != nil {
		t.Fatal(err)
	}
	candidate, ok := housekeepingCandidateByPath(preview, temporaryPath, housekeepingKindStageArtifact)
	if !ok || !candidate.SafeToDelete || !candidate.ManualOnly || candidate.Maintenance == nil || candidate.Maintenance.AssetRecordID != record.ID || candidate.Maintenance.TemporaryPresence != "present" || candidate.Maintenance.BackupPresence != "absent" {
		t.Fatalf("verified Library residual was not eligible in manual preview: %#v", candidate)
	}
	result, err := ExecuteHousekeepingPreview(db, preview.PreviewID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(temporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("verified temporary was not removed: %v", err)
	}
	if current, err := os.ReadFile(assetPath); err != nil || string(current) != string(content) {
		t.Fatalf("canonical Library asset changed: %q err=%v", current, err)
	}
	var persisted models.AssetMaintenanceOperation
	if err := db.Where("id = ?", operation.ID).First(&persisted).Error; err != nil || persisted.Status != maintenanceStatusComplete {
		t.Fatalf("maintenance history was modified: %#v err=%v", persisted, err)
	}
	cleaned, ok := housekeepingCandidateByPath(result, temporaryPath, housekeepingKindStageArtifact)
	if !ok || cleaned.SafeToDelete || cleaned.Outcome != "cleaned" || result.Outcome != "cleaned" || result.RecoveredBytes != int64(len(content)) {
		t.Fatalf("cleanup outcome or reclaimed bytes were not persisted: candidate=%#v report=%#v", cleaned, result)
	}
}

func TestHousekeepingLibraryMaintenanceRejectsBackupFingerprintAndReplacementChanges(t *testing.T) {
	for _, testCase := range []string{"backup", "fingerprint", "hardlink", "symlink", "replacement", "operation-deleted", "operation-changed", "reference-added", "failed", "running", "unknown"} {
		t.Run(testCase, func(t *testing.T) {
			db, roots := newHousekeepingTestDB(t)
			assetPath := filepath.Join(roots.library, "film.mkv")
			content := []byte("verified result bytes")
			if err := os.WriteFile(assetPath, content, 0o644); err != nil {
				t.Fatal(err)
			}
			temporaryPath := filepath.Join(roots.library, ".film.mkv.maintenance-race.tmp.mkv")
			backupPath := filepath.Join(roots.library, ".film.mkv.maintenance-race.backup.mkv")
			if err := os.WriteFile(temporaryPath, content, 0o644); err != nil {
				t.Fatal(err)
			}
			fingerprint, err := mediaFileFingerprint(assetPath)
			if err != nil {
				t.Fatal(err)
			}
			if testCase == "fingerprint" {
				if err := os.WriteFile(temporaryPath, []byte("different output"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if testCase == "hardlink" {
				if err := os.Link(temporaryPath, temporaryPath+".alias"); err != nil {
					t.Fatal(err)
				}
			}
			if testCase == "symlink" {
				alternate := temporaryPath + ".target"
				if err := os.WriteFile(alternate, content, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(temporaryPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(alternate, temporaryPath); err != nil {
					t.Fatal(err)
				}
			}
			if testCase == "backup" {
				if err := os.WriteFile(backupPath, []byte("recovery backup"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			record := models.AssetRecord{Path: assetPath, RootPath: roots.library, FileName: filepath.Base(assetPath), Status: "processed", Missing: false}
			if err := db.Create(&record).Error; err != nil {
				t.Fatal(err)
			}
			finished := housekeepingOldTime()
			status, phase := maintenanceStatusComplete, "completed"
			switch testCase {
			case "failed":
				status, phase = maintenanceStatusFailed, "interrupted"
			case "running":
				status, phase = maintenanceStatusRunning, "remuxing"
			case "unknown":
				status, phase = "mystery", "mystery"
			}
			operation := models.AssetMaintenanceOperation{
				ID: "maintenance-race", OperationType: "remove_tracks", AssetRecordID: record.ID, AssetPath: assetPath,
				ResultFingerprint: fingerprint, TemporaryPath: temporaryPath, BackupPath: backupPath,
				Status: status, Phase: phase, FinishedAt: &finished, CreatedAt: finished, UpdatedAt: finished,
			}
			if err := db.Create(&operation).Error; err != nil {
				t.Fatal(err)
			}
			preview, err := PreviewHousekeeping(db)
			if err != nil {
				t.Fatal(err)
			}
			candidate, ok := housekeepingCandidateByPath(preview, temporaryPath, housekeepingKindStageArtifact)
			if !ok || candidate.SafeToDelete {
				if ok && candidate.SafeToDelete && (testCase == "replacement" || testCase == "operation-deleted" || testCase == "operation-changed" || testCase == "reference-added") {
					switch testCase {
					case "replacement":
						alternate := temporaryPath + ".replacement"
						if err := os.WriteFile(alternate, content, 0o644); err != nil {
							t.Fatal(err)
						}
						if err := os.Remove(temporaryPath); err != nil {
							t.Fatal(err)
						}
						if err := os.Rename(alternate, temporaryPath); err != nil {
							t.Fatal(err)
						}
					case "operation-deleted":
						if err := db.Delete(&models.AssetMaintenanceOperation{}, "id = ?", operation.ID).Error; err != nil {
							t.Fatal(err)
						}
					case "operation-changed":
						if err := db.Model(&models.AssetMaintenanceOperation{}).Where("id = ?", operation.ID).Updates(map[string]any{"status": maintenanceStatusRunning, "phase": "committing"}).Error; err != nil {
							t.Fatal(err)
						}
					case "reference-added":
						if err := db.Create(&models.AssetRecord{Path: temporaryPath, RootPath: roots.library, FileName: filepath.Base(temporaryPath), Status: "processed", Missing: false}).Error; err != nil {
							t.Fatal(err)
						}
					}
					result, err := ExecuteHousekeepingPreview(db, preview.PreviewID)
					if err != nil {
						t.Fatal(err)
					}
					changed, _ := housekeepingCandidateByPath(result, temporaryPath, housekeepingKindStageArtifact)
					if changed.SafeToDelete || changed.Outcome != "blocked" || changed.BlockedReason == "" {
						t.Fatalf("replacement was not blocked: %#v", changed)
					}
					if _, err := os.Stat(temporaryPath); err != nil {
						t.Fatalf("replacement file was removed: %v", err)
					}
					return
				}
				t.Fatalf("unexpected preview eligibility: %#v", candidate)
			}
			if testCase == "backup" && (candidate.Maintenance == nil || candidate.Maintenance.BackupPresence != "present") {
				t.Fatalf("backup presence was not diagnosed: %#v", candidate.Maintenance)
			}
			if testCase == "fingerprint" && candidate.BlockedReason == "" {
				t.Fatal("fingerprint mismatch was not explained")
			}
			if (testCase == "hardlink" || testCase == "symlink") && candidate.BlockedReason == "" {
				t.Fatalf("unsafe temporary path was not explained: %#v", candidate)
			}
			if _, err := os.Stat(temporaryPath); err != nil {
				t.Fatalf("blocked temporary was removed: %v", err)
			}
		})
	}
}

func TestHousekeepingLibraryMaintenanceRejectsTraversalAndRoleOverlap(t *testing.T) {
	for _, testCase := range []string{"traversal", "overlap"} {
		t.Run(testCase, func(t *testing.T) {
			db, roots := newHousekeepingTestDB(t)
			libraryRoot := roots.library
			if testCase == "overlap" {
				libraryRoot = filepath.Join(roots.raw, "nested-library")
				if err := os.MkdirAll(libraryRoot, 0o755); err != nil {
					t.Fatal(err)
				}
				var setting models.AppSetting
				if err := db.Where("key = ?", "storageRoles").First(&setting).Error; err != nil {
					t.Fatal(err)
				}
				setting.Value["library"] = map[string]interface{}{"path": libraryRoot}
				if err := db.Save(&setting).Error; err != nil {
					t.Fatal(err)
				}
			}
			assetPath := filepath.Join(libraryRoot, "film.mkv")
			if err := os.WriteFile(assetPath, []byte("asset"), 0o644); err != nil {
				t.Fatal(err)
			}
			temporaryPath := filepath.Join(libraryRoot, ".film.mkv.maintenance-unsafe.tmp.mkv")
			if err := os.WriteFile(temporaryPath, []byte("asset"), 0o644); err != nil {
				t.Fatal(err)
			}
			if testCase == "traversal" {
				temporaryPath = libraryRoot + string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(libraryRoot) + string(os.PathSeparator) + filepath.Base(temporaryPath)
			}
			finished := housekeepingOldTime()
			operation := models.AssetMaintenanceOperation{
				ID: "maintenance-unsafe", AssetPath: assetPath, TemporaryPath: temporaryPath,
				BackupPath:        filepath.Join(libraryRoot, ".film.mkv.maintenance-unsafe.backup.mkv"),
				ResultFingerprint: "unused", Status: maintenanceStatusComplete, Phase: "completed", FinishedAt: &finished, CreatedAt: finished, UpdatedAt: finished,
			}
			if err := db.Create(&operation).Error; err != nil {
				t.Fatal(err)
			}
			report, err := PreviewHousekeeping(db)
			if err != nil {
				t.Fatal(err)
			}
			candidate, ok := housekeepingCandidateByPath(report, temporaryPath, housekeepingKindStageArtifact)
			if !ok || candidate.SafeToDelete || candidate.BlockedReason == "" {
				t.Fatalf("unsafe Library role/path was not blocked: %#v", candidate)
			}
			if _, err := os.Stat(filepath.Join(libraryRoot, ".film.mkv.maintenance-unsafe.tmp.mkv")); err != nil {
				t.Fatalf("unsafe path was modified: %v", err)
			}
		})
	}
}

func TestHousekeepingRawDirectoryDiagnosticsRetainStaleAndConcurrentReferences(t *testing.T) {
	db, roots := newHousekeepingTestDB(t)
	staleDirectory := filepath.Join(roots.raw, "stale-asset")
	if err := os.MkdirAll(staleDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := models.AssetRecord{Path: filepath.Join(staleDirectory, "missing.mkv"), RootPath: roots.raw, Status: "unprocessed", Missing: true}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewHousekeeping(db)
	if err != nil {
		t.Fatal(err)
	}
	staleCandidate, ok := housekeepingCandidateByPath(preview, staleDirectory, housekeepingKindEmptyDirectory)
	if !ok || staleCandidate.SafeToDelete || !housekeepingHasProtectionReference(staleCandidate, "AssetRecord", strconv.FormatUint(uint64(stale.ID), 10), "path") || staleCandidate.ProtectionReferences[0].State != "stale_reference_suspected" {
		t.Fatalf("stale AssetRecord blocker was not identified: %#v", staleCandidate)
	}
	if _, err := os.Stat(staleDirectory); err != nil {
		t.Fatalf("stale reference allowed empty RAW directory removal: %v", err)
	}

	concurrentDB, concurrentRoots := newHousekeepingTestDB(t)
	concurrentDirectory := filepath.Join(concurrentRoots.raw, "new-reference")
	if err := os.MkdirAll(concurrentDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	concurrentPreview, err := PreviewHousekeeping(concurrentDB)
	if err != nil {
		t.Fatal(err)
	}
	initial, ok := housekeepingCandidateByPath(concurrentPreview, concurrentDirectory, housekeepingKindEmptyDirectory)
	if !ok || !initial.SafeToDelete {
		t.Fatalf("unreferenced empty directory was not eligible: %#v", initial)
	}
	newReference := models.AssetRecord{Path: filepath.Join(concurrentDirectory, "new.mkv"), RootPath: concurrentRoots.raw, Status: "unprocessed", Missing: false}
	if err := concurrentDB.Create(&newReference).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ExecuteHousekeepingPreview(concurrentDB, concurrentPreview.PreviewID)
	if err != nil {
		t.Fatal(err)
	}
	changed, ok := housekeepingCandidateByPath(result, concurrentDirectory, housekeepingKindEmptyDirectory)
	if !ok || changed.SafeToDelete || changed.Outcome != "blocked" || !housekeepingHasProtectionReference(changed, "AssetRecord", strconv.FormatUint(uint64(newReference.ID), 10), "path") {
		t.Fatalf("new reference was not rechecked before removal: %#v", changed)
	}
	if _, err := os.Stat(concurrentDirectory); err != nil {
		t.Fatalf("concurrent reference did not protect RAW directory: %v", err)
	}
}

func TestHousekeepingRawBatchRevalidatesReferencesBeforeEachRemoval(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(fmt.Sprintf("manual-%v", manual), func(t *testing.T) {
			db, roots := newHousekeepingTestDB(t)
			firstDirectory := filepath.Join(roots.raw, "a-first")
			secondDirectory := filepath.Join(roots.raw, "b-second")
			for _, path := range []string{firstDirectory, secondDirectory} {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			preview, err := PreviewHousekeeping(db)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{firstDirectory, secondDirectory} {
				candidate, ok := housekeepingCandidateByPath(preview, path, housekeepingKindEmptyDirectory)
				if !ok || !candidate.SafeToDelete {
					t.Fatalf("directory was not initially eligible: path=%q candidate=%#v", path, candidate)
				}
			}

			var insertedReference *models.AssetRecord
			var callbackErr error
			callbackName := fmt.Sprintf("test:housekeeping-raw-batch-reference-%d", housekeepingTestSequence)
			if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if insertedReference != nil || tx.Statement.Table != "asset_records" {
					return
				}
				if _, err := os.Stat(firstDirectory); !os.IsNotExist(err) {
					return
				}
				if _, err := os.Stat(secondDirectory); err != nil {
					return
				}
				reference := models.AssetRecord{
					Path:     filepath.Join(secondDirectory, "new-active-reference.mkv"),
					RootPath: roots.raw, Status: "unprocessed", Missing: false,
				}
				if err := db.Create(&reference).Error; err != nil {
					callbackErr = err
					return
				}
				insertedReference = &reference
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

			var report HousekeepingReport
			if manual {
				report, err = ExecuteHousekeepingPreview(db, preview.PreviewID)
			} else {
				report, err = RunHousekeeping(db, false)
			}
			if err != nil {
				t.Fatal(err)
			}
			if callbackErr != nil {
				t.Fatalf("could not create the inter-candidate reference: %v", callbackErr)
			}
			if insertedReference == nil {
				t.Fatal("no reference was inserted after the first directory was processed")
			}

			if _, err := os.Stat(firstDirectory); !os.IsNotExist(err) {
				t.Fatalf("first eligible directory was not removed: stat error=%v", err)
			}
			if info, err := os.Lstat(secondDirectory); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				t.Fatalf("second directory was not preserved as a directory: info=%v error=%v", info, err)
			}
			second, ok := housekeepingCandidateByPath(report, secondDirectory, housekeepingKindEmptyDirectory)
			if !ok || second.SafeToDelete || second.Outcome != "blocked" ||
				!housekeepingHasProtectionReference(second, "AssetRecord", strconv.FormatUint(uint64(insertedReference.ID), 10), "path") {
				t.Fatalf("new inter-candidate reference did not block deletion: %#v", second)
			}
		})
	}
}

func housekeepingHasProtectionReference(candidate HousekeepingCandidate, source, recordID, field string) bool {
	for _, reference := range candidate.ProtectionReferences {
		if reference.Source == source && reference.RecordID == recordID && reference.Field == field {
			return true
		}
	}
	return false
}

func TestHousekeepingRawDirectoryExecutionRejectsEntryAndIdentityChanges(t *testing.T) {
	for _, testCase := range []string{"nonempty", "symlink", "replacement"} {
		t.Run(testCase, func(t *testing.T) {
			db, roots := newHousekeepingTestDB(t)
			directory := filepath.Join(roots.raw, "mutable")
			if err := os.MkdirAll(directory, 0o755); err != nil {
				t.Fatal(err)
			}
			preview, err := PreviewHousekeeping(db)
			if err != nil {
				t.Fatal(err)
			}
			candidate, ok := housekeepingCandidateByPath(preview, directory, housekeepingKindEmptyDirectory)
			if !ok || !candidate.SafeToDelete {
				t.Fatalf("empty RAW directory was not eligible before mutation: %#v", candidate)
			}
			switch testCase {
			case "nonempty":
				housekeepingWriteFile(t, filepath.Join(directory, "poster.nfo"), "preserve sidecar")
			case "symlink":
				target := filepath.Join(t.TempDir(), "target")
				if err := os.MkdirAll(target, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(directory); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, directory); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				replacement := filepath.Join(roots.raw, "replacement")
				if err := os.MkdirAll(replacement, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(directory); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, directory); err != nil {
					t.Fatal(err)
				}
			}
			result, err := ExecuteHousekeepingPreview(db, preview.PreviewID)
			if err != nil {
				t.Fatal(err)
			}
			changed, ok := housekeepingCandidateByPath(result, directory, housekeepingKindEmptyDirectory)
			if !ok || changed.SafeToDelete || changed.Outcome != "blocked" || changed.BlockedReason == "" {
				t.Fatalf("changed RAW directory was not reported as blocked: %#v", changed)
			}
			if _, err := os.Lstat(directory); err != nil {
				t.Fatalf("changed RAW directory or replacement was removed: %v", err)
			}
		})
	}

	db, roots := newHousekeepingTestDB(t)
	directory := filepath.Join(roots.raw, "symlink-directory")
	target := filepath.Join(t.TempDir(), "outside-target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, directory); err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewHousekeeping(db)
	if err != nil {
		t.Fatal(err)
	}
	symlinkCandidate, ok := housekeepingCandidateByPath(preview, directory, housekeepingKindEmptyDirectory)
	if !ok || symlinkCandidate.SafeToDelete || symlinkCandidate.Outcome != "blocked" || symlinkCandidate.BlockedReason == "" {
		t.Fatalf("RAW symlink directory was not shown as blocked: %#v", symlinkCandidate)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target changed during preview: %v", err)
	}
}
