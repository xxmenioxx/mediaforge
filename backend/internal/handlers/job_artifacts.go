package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type jobArtifact struct {
	GeneratedAt     time.Time       `json:"generatedAt"`
	Kind            string          `json:"kind"`
	Job             models.QueueJob `json:"job"`
	Command         string          `json:"command,omitempty"`
	ExecutionEngine string          `json:"executionEngine,omitempty"`
	Profile         models.Profile  `json:"profile"`
	AudioProfileKey string          `json:"audioProfileKey,omitempty"`
	ProcessingMode  string          `json:"processingMode,omitempty"`
	SourceProbe     map[string]any  `json:"sourceProbe,omitempty"`
	OutputProbe     map[string]any  `json:"outputProbe,omitempty"`
	Result          map[string]any  `json:"result,omitempty"`
	Notes           string          `json:"notes,omitempty"`
}

type jobArtifactsResponse struct {
	JobID      uint           `json:"jobId"`
	AsIs       map[string]any `json:"asIs,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	AsIsPath   string         `json:"asIsPath,omitempty"`
	ResultPath string         `json:"resultPath,omitempty"`
	Warnings   []string       `json:"warnings,omitempty"`
}

type analysisBackfillResponse struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Total    int `json:"total"`
}

func JobArtifacts(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var job models.QueueJob
		if err := db.First(&job, c.Param("id")).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		response := jobArtifactsResponse{JobID: job.ID}
		warnings := []string{}
		if artifact, path, err := readLatestJobArtifact(db, job, "as-is"); err == nil {
			response.AsIs = artifact
			response.AsIsPath = path
		} else if err != os.ErrNotExist {
			warnings = append(warnings, "AS-IS artifact: "+err.Error())
		}
		if artifact, path, err := readLatestJobArtifact(db, job, "result"); err == nil {
			response.Result = artifact
			response.ResultPath = path
		} else if err != os.ErrNotExist {
			warnings = append(warnings, "Result artifact: "+err.Error())
		}
		response.Warnings = warnings
		c.JSON(http.StatusOK, response)
	}
}

func BackfillAnalysisFromAsIsReports(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := backfillAnalysisRecordsFromAsIsReports(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func writeJobAsIsArtifact(db *gorm.DB, job models.QueueJob, profile models.Profile, audioProfile *audioEnhancementProfile, command string, processingMode string) error {
	sourceProbe := ffprobeJSON(job.MediaPath)
	artifact := jobArtifact{
		GeneratedAt:     time.Now(),
		Kind:            "as-is",
		Job:             job,
		Command:         command,
		ExecutionEngine: "FFmpeg",
		Profile:         profile,
		ProcessingMode:  processingMode,
		SourceProbe:     sourceProbe,
		Notes:           job.Notes,
	}
	if audioProfile != nil {
		artifact.AudioProfileKey = audioProfile.Key
	}
	appendAnalysisRecordForJob(db, job, sourceProbe)
	return writeJobArtifact(db, job, "as-is", artifact)
}

func appendAnalysisRecordForJob(db *gorm.DB, job models.QueueJob, sourceProbe map[string]any) {
	if len(sourceProbe) == 0 || sourceProbe["error"] != nil {
		return
	}

	var probe FFProbeResult
	content, err := json.Marshal(sourceProbe)
	if err != nil || json.Unmarshal(content, &probe) != nil {
		return
	}

	size := int64(0)
	if info, err := os.Stat(job.MediaPath); err == nil {
		size = info.Size()
	}
	scan := buildScanResult(job.MediaPath, size, probe, models.JSONMap(sourceProbe))
	record := models.JSONMap{
		"id":        strconv.FormatInt(time.Now().UnixMilli(), 10) + "-" + job.MediaPath,
		"assetPath": job.MediaPath,
		"assetName": scan.FileName,
		"decision":  "queued-conversion-as-is",
		"notes":     "Automatically captured before MediaForge job #" + strconv.FormatUint(uint64(job.ID), 10),
		"scan":      scan,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}

	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "analysisRecords").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			_ = db.Create(&models.AppSetting{Key: "analysisRecords", Value: models.JSONMap{"records": []any{record}}}).Error
		}
		return
	}

	records, _ := setting.Value["records"].([]any)
	next := append([]any{record}, records...)
	if len(next) > 200 {
		next = next[:200]
	}
	setting.Value = models.JSONMap{"records": next}
	_ = db.Save(&setting).Error
}

func backfillAnalysisRecordsFromAsIsReports(db *gorm.DB) (analysisBackfillResponse, error) {
	response := analysisBackfillResponse{}
	dir, err := jobReportPath(db, "as-is")
	if err != nil {
		return response, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return response, nil
		}
		return response, err
	}

	files := []os.DirEntry{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		files = append(files, entry)
	}
	sort.SliceStable(files, func(i, j int) bool {
		left, leftErr := files[i].Info()
		right, rightErr := files[j].Info()
		if leftErr != nil || rightErr != nil {
			return files[i].Name() > files[j].Name()
		}
		return left.ModTime().After(right.ModTime())
	})

	records, seenIDs, err := existingAnalysisRecords(db)
	if err != nil {
		return response, err
	}

	imported := []any{}
	for _, file := range files {
		response.Total++
		record, ok := analysisRecordFromAsIsReport(filepath.Join(dir, file.Name()), file.Name())
		if !ok {
			response.Skipped++
			continue
		}
		id := stringFromUnknown(record["id"])
		if id == "" {
			response.Skipped++
			continue
		}
		if _, exists := seenIDs[id]; exists {
			response.Skipped++
			continue
		}
		seenIDs[id] = struct{}{}
		imported = append(imported, record)
		response.Imported++
	}

	if response.Imported == 0 {
		return response, nil
	}

	next := append(imported, records...)
	if len(next) > 200 {
		next = next[:200]
	}
	value := models.JSONMap{"records": next}

	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "analysisRecords").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return response, db.Create(&models.AppSetting{Key: "analysisRecords", Value: value}).Error
		}
		return response, err
	}
	setting.Value = value
	return response, db.Save(&setting).Error
}

func analysisRecordFromAsIsReport(path string, fileName string) (models.JSONMap, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var artifact jobArtifact
	if err := json.Unmarshal(content, &artifact); err != nil {
		return nil, false
	}
	if artifact.Kind != "as-is" || len(artifact.SourceProbe) == 0 || artifact.SourceProbe["error"] != nil {
		return nil, false
	}

	var probe FFProbeResult
	probeContent, err := json.Marshal(artifact.SourceProbe)
	if err != nil || json.Unmarshal(probeContent, &probe) != nil {
		return nil, false
	}

	assetPath := strings.TrimSpace(artifact.Job.MediaPath)
	if assetPath == "" {
		assetPath = strings.TrimSpace(probe.Format.Filename)
	}
	if assetPath == "" {
		return nil, false
	}

	size := probeSizeBytes(artifact.SourceProbe)
	if info, err := os.Stat(assetPath); err == nil {
		size = info.Size()
	}

	generatedAt := artifact.GeneratedAt
	if generatedAt.IsZero() {
		if info, err := os.Stat(path); err == nil {
			generatedAt = info.ModTime()
		} else {
			generatedAt = time.Now()
		}
	}

	scan := buildScanResult(assetPath, size, probe, models.JSONMap(artifact.SourceProbe))
	jobID := strconv.FormatUint(uint64(artifact.Job.ID), 10)
	if artifact.Job.ID == 0 {
		jobID = "unknown"
	}
	return models.JSONMap{
		"id":        "as-is-report-" + strings.TrimSuffix(fileName, filepath.Ext(fileName)),
		"assetPath": assetPath,
		"assetName": scan.FileName,
		"decision":  "queued-conversion-as-is",
		"notes":     "Imported from AS-IS report for MediaForge job #" + jobID + ".",
		"scan":      scan,
		"createdAt": generatedAt.UTC().Format(time.RFC3339),
	}, true
}

func existingAnalysisRecords(db *gorm.DB) ([]any, map[string]struct{}, error) {
	seen := map[string]struct{}{}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "analysisRecords").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return []any{}, seen, nil
		}
		return nil, nil, err
	}

	records, _ := setting.Value["records"].([]any)
	for _, record := range records {
		if mapped, ok := record.(map[string]any); ok {
			if id := stringFromUnknown(mapped["id"]); id != "" {
				seen[id] = struct{}{}
			}
		}
	}
	return records, seen, nil
}

func probeSizeBytes(raw map[string]any) int64 {
	format, ok := raw["format"].(map[string]any)
	if !ok {
		return 0
	}
	size, err := strconv.ParseInt(stringFromUnknown(format["size"]), 10, 64)
	if err != nil {
		return 0
	}
	return size
}

func writeJobResultArtifact(db *gorm.DB, job models.QueueJob, result map[string]any) error {
	artifact := jobArtifact{
		GeneratedAt: time.Now(),
		Kind:        "result",
		Job:         job,
		OutputProbe: ffprobeJSON(job.OutputPath),
		Result:      result,
		Notes:       job.Notes,
	}
	return writeJobArtifact(db, job, "result", artifact)
}

func writeJobArtifact(db *gorm.DB, job models.QueueJob, kind string, artifact jobArtifact) error {
	dir, err := jobReportPath(db, kind)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, jobReportFileName(job, kind)), append(content, '\n'), 0o644)
}

func ffprobeJSON(path string) map[string]any {
	if path == "" {
		return nil
	}
	output, err := exec.Command(
		"ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	).Output()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	var value map[string]any
	if err := json.Unmarshal(output, &value); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return value
}

func jobReportPath(db *gorm.DB, kind string) (string, error) {
	setting := models.AppSetting{}
	values := models.JSONMap{}
	if err := db.First(&setting, "key = ?", "paths").Error; err == nil && setting.Value != nil {
		values = setting.Value
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
	}

	key := "resultsReportsPath"
	fallback := "/media/reports/results"
	if kind == "as-is" {
		key = "asIsReportsPath"
		fallback = "/media/reports/as-is"
	}
	if value := strings.TrimSpace(stringFromUnknown(values[key])); value != "" {
		return value, nil
	}
	return fallback, nil
}

func readLatestJobArtifact(db *gorm.DB, job models.QueueJob, kind string) (map[string]any, string, error) {
	dir, err := jobReportPath(db, kind)
	if err != nil {
		return nil, "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", err
	}

	prefix := strconv.FormatUint(uint64(job.ID), 10) + "-job-" + strconv.FormatUint(uint64(job.ID), 10) + "-"
	matches := []os.DirEntry{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		matches = append(matches, entry)
	}
	if len(matches) == 0 {
		return nil, "", os.ErrNotExist
	}

	sort.SliceStable(matches, func(i, j int) bool {
		left, leftErr := matches[i].Info()
		right, rightErr := matches[j].Info()
		if leftErr != nil || rightErr != nil {
			return matches[i].Name() > matches[j].Name()
		}
		return left.ModTime().After(right.ModTime())
	})

	path := filepath.Join(dir, matches[0].Name())
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var artifact map[string]any
	if err := json.Unmarshal(content, &artifact); err != nil {
		return nil, "", err
	}
	return artifact, path, nil
}

func jobReportFileName(job models.QueueJob, kind string) string {
	assetName := strings.TrimSuffix(filepath.Base(job.MediaPath), filepath.Ext(job.MediaPath))
	if strings.TrimSpace(assetName) == "" {
		assetName = "asset"
	}
	date := time.Now().Format("20060102-150405")
	jobID := strconv.FormatUint(uint64(job.ID), 10)
	return jobID + "-job-" + jobID + "-" + safeReportName(assetName) + "-" + date + ".json"
}

var unsafeReportNamePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func safeReportName(value string) string {
	name := unsafeReportNamePattern.ReplaceAllString(strings.TrimSpace(value), "-")
	name = strings.Trim(name, ".-_")
	if name == "" {
		return "asset"
	}
	return name
}
