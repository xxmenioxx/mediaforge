package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLogFileCategoriesCoverSystemAndPipelineSources(t *testing.T) {
	tests := map[string]string{"backend.log": "backend", "system.log": "system", "scheduler.log": "scheduler", "workers.log": "workers", "pipeline.log": "pipeline", "jobs.log": "jobs", "job-42.log": "jobs"}
	for name, expected := range tests {
		if actual := logFileCategory(name); actual != expected {
			t.Fatalf("%s category=%s expected=%s", name, actual, expected)
		}
	}
}

func TestReadJobLogReturnsCanonicalTailForEachJobStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:job-log-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	logDir := t.TempDir()
	t.Setenv("MVFORGE_LOG_DIR", logDir)
	statuses := []string{JobStatusRunning, JobStatusCompleted, JobStatusFailed, JobStatusCanceled}
	jobs := make([]models.QueueJob, 0, len(statuses)+1)
	for _, status := range statuses {
		job := models.QueueJob{MediaPath: "/raw/" + status + ".mkv", Status: status, Notes: "only-" + status}
		if err := db.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, job)
	}
	other := models.QueueJob{MediaPath: "/raw/other.mkv", Status: JobStatusCompleted, Notes: "other-job-secret"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.GET("/api/queue/jobs/:id/log", NewLogHandler(db).ReadJobLog)
	for _, job := range jobs {
		if job.Status == JobStatusRunning {
			writer, err := openPersistentJobLogWriter(db, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Write([]byte("live-output\n")); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/queue/jobs/%d/log?tail=1000", job.ID), nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "only-"+job.Status) {
			t.Fatalf("status %s response=%d %s", job.Status, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "other-job-secret") {
			t.Fatalf("job %d log included another job", job.ID)
		}
		if _, err := os.Stat(filepath.Join(logDir, fmt.Sprintf("job-%d.log", job.ID))); err != nil {
			t.Fatalf("canonical log for job %d was not persisted: %v", job.ID, err)
		}
	}
}

func TestReadJobLogValidatesIdentityAndReportsMissingStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:job-log-missing-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/raw/one.mkv", Status: JobStatusCompleted}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_LOG_DIR", blocker)
	router := gin.New()
	router.GET("/api/queue/jobs/:id/log", NewLogHandler(db).ReadJobLog)
	for _, test := range []struct {
		path string
	}{
		{path: "/api/queue/jobs/0/log"},
		{path: "/api/queue/jobs/1%2F..%2Fetc/log"},
		{path: "/api/queue/jobs/9999/log"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code == http.StatusOK {
			t.Fatalf("%s unexpectedly returned a job log", test.path)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/queue/jobs/%d/log", job.ID), nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":false`) {
		t.Fatalf("unavailable storage response=%d %s", response.Code, response.Body.String())
	}
}

func TestReadJobLogBoundsTail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:job-log-tail-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	var notes strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&notes, "line-%02d\n", i)
	}
	job := models.QueueJob{MediaPath: "/raw/tail.mkv", Status: JobStatusCompleted, Notes: notes.String()}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_LOG_DIR", t.TempDir())
	router := gin.New()
	router.GET("/api/queue/jobs/:id/log", NewLogHandler(db).ReadJobLog)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/queue/jobs/%d/log?tail=5", job.ID), nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "line-16") || !strings.Contains(response.Body.String(), "line-19") || !strings.Contains(response.Body.String(), `"truncated":true`) {
		t.Fatalf("tail response=%d %s", response.Code, response.Body.String())
	}
}

func TestReadJobLogRejectsCanonicalFilenameSymlink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:job-log-symlink-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/raw/symlink.mkv", Status: JobStatusRunning}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(root, "outside.log")
	if err := os.WriteFile(secret, []byte("must-not-be-exposed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(logDir, fmt.Sprintf("job-%d.log", job.ID))); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_LOG_DIR", logDir)
	router := gin.New()
	router.GET("/api/queue/jobs/:id/log", NewLogHandler(db).ReadJobLog)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/queue/jobs/%d/log", job.ID), nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":false`) || strings.Contains(response.Body.String(), "must-not-be-exposed") {
		t.Fatalf("symlink response=%d %s", response.Code, response.Body.String())
	}
}

func TestPersistentJobLogWriterCapturesOutputAndBoundsStorage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:job-log-writer-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/raw/captured.mkv", Status: JobStatusRunning}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	logDir := t.TempDir()
	t.Setenv("MVFORGE_LOG_DIR", logDir)
	writer, err := openPersistentJobLogWriter(db, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(strings.Repeat("x", 2*maxLogReadBytes) + "\nfinal-ffmpeg-output\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("job-%d.log", job.ID))
	info, err := os.Stat(logPath)
	if err != nil || info.Size() > 2*maxLogReadBytes {
		t.Fatalf("persistent log size=%v err=%v; expected bounded storage", info, err)
	}
	response := httptest.NewRecorder()
	router := gin.New()
	router.GET("/api/queue/jobs/:id/log", NewLogHandler(db).ReadJobLog)
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/queue/jobs/%d/log", job.ID), nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "final-ffmpeg-output") {
		t.Fatalf("captured output response=%d %s", response.Code, response.Body.String())
	}
}

func TestConsolidatedLogsExposeSchedulerWorkerAndPipelineState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:consolidated-logs?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ExecutionPlan{}, &models.SchedulerReservation{}, &models.RuntimeSnapshot{}, &models.WorkerNode{}, &models.QueueJob{}); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/raw/show.mkv", Status: JobStatusRunning, Stage: JobStageConverting, WorkerName: "local", Priority: 1, StageHistory: models.JSONList{models.JSONMap{"stage": JobStageConverting}}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ExecutionPlan{JobID: job.ID, Version: 1, Status: "dispatched", SelectedEncoder: "libx265"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.WorkerNode{Name: "local", Status: "online", MaxConcurrentJobs: 1, LastSeenAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(schedulerLog(db), "libx265") || !strings.Contains(workersLog(db), "worker=local") || !strings.Contains(pipelineLog([]models.QueueJob{job}), "converting") {
		t.Fatal("consolidated subsystem state is missing")
	}
}
