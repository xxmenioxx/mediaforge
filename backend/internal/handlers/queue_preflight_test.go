package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type queuePreflightFixture struct {
	db      *gorm.DB
	profile models.Profile
	library models.Library
	paths   []string
}

func newQueuePreflightFixture(t *testing.T, sourceCount int) queuePreflightFixture {
	t.Helper()
	return newQueuePreflightFixtureWithDB(t, sourceCount, queueSelectedAssetsTestDB(t))
}

func newQueuePreflightFixtureWithDB(t *testing.T, sourceCount int, db *gorm.DB) queuePreflightFixture {
	t.Helper()
	sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Preflight library", SourcePath: sourceRoot, DestinationPath: destinationRoot}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, sourceCount)
	for index := 0; index < sourceCount; index++ {
		path := filepath.Join(sourceRoot, fmt.Sprintf("Episode%02d.mkv", index+1))
		if err := os.WriteFile(path, []byte("stable fixture source"), 0o600); err != nil {
			t.Fatal(err)
		}
		record := models.AssetRecord{Path: path, RootPath: sourceRoot, GroupPath: filepath.Base(sourceRoot), FileName: filepath.Base(path), Status: "unprocessed"}
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
		scan := models.ScanResult{
			Path: path, FileName: filepath.Base(path), VideoCodec: "h264", Width: 1920, Height: 1080,
			VideoStreams: models.JSONList{models.JSONMap{
				"index": 0, "codec": "h264", "width": 1920, "height": 1080,
				"avgFrameRate": "24/1", "realFrameRate": "24/1", "fieldOrder": "progressive",
			}},
			InterlaceAnalysis:     models.JSONMap{"version": float64(interlaceAnalysisVersion), "status": "progressive"},
			CadenceAnalysis:       models.JSONMap{"version": float64(cadenceAnalysisVersion), "type": "native"},
			CadenceRecommendation: models.JSONMap{"version": 1.0, "operation": "preserve"},
		}
		if err := db.Create(&scan).Error; err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	state := models.RuntimeState{
		SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: time.Now(), SelectedProfile: "desktop_safe",
		StableFingerprint: "batch-preflight-test-runtime", FingerprintVersion: "v1",
		Encoders: models.JSONMap{"libx265": map[string]any{"Listed": true, "Usable": true}},
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	return queuePreflightFixture{db: db, profile: profile, library: library, paths: paths}
}

func queuePreflightRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewQueueHandler(db)
	router.POST("/queue/jobs", handler.Create)
	router.POST("/preflights", handler.CreateBatchPreflight)
	router.POST("/preflights/selected-assets", handler.PreflightSelectedAssets)
	router.GET("/preflights/:id", handler.GetBatchPreflight)
	router.PUT("/preflights/:id", handler.UpdateBatchPreflight)
	router.POST("/preflights/:id/edit", handler.EditBatchPreflight)
	router.POST("/preflights/:id/accept", handler.AcceptBatchPreflight)
	router.POST("/preflights/:id/remove", handler.RemoveBatchPreflightAsset)
	router.POST("/preflights/:id/discard", handler.DiscardBatchPreflight)
	router.POST("/batches", handler.CreateBatch)
	router.POST("/selected-assets", handler.QueueSelectedAssets)
	return router
}

func queuePreflightRequest(t *testing.T, router *gin.Engine, method, target string, input any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func saveSubtitleSelectionThroughAssetTracks(t *testing.T, db *gorm.DB, path, body string) {
	t.Helper()
	router := gin.New()
	router.PATCH("/api/assets/tracks", NewAssetHandler(db).UpdateTracks)
	response := assetTracksPatchRequest(t, router, path, body)
	if response.Code != http.StatusOK {
		t.Fatalf("Save Tracks status=%d body=%s", response.Code, response.Body.String())
	}
}

func queuePreflightInput(fixture queuePreflightFixture, batchName string) QueueBatchPreflightInput {
	jobs := make([]QueueJobInput, 0, len(fixture.paths))
	for _, path := range fixture.paths {
		jobs = append(jobs, QueueJobInput{
			MediaPath: path, LibraryID: fixture.library.ID, ProfileID: fixture.profile.ID,
			ProcessingMode: ProcessingModeFullEncode, Priority: 5,
		})
	}
	return QueueBatchPreflightInput{BatchID: "preflight-test-batch", BatchName: batchName, Jobs: jobs}
}

func TestQueuePreflightResponseSerializesNilTrackProfileSnapshot(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	snapshot, err := scheduler.CaptureProfileSnapshot(fixture.profile, time.Now().UTC(), "nil-track-preflight-response-test")
	if err != nil {
		t.Fatal(err)
	}
	response := queuePreflightResponse(models.QueueBatchPreflight{
		ID: "candidate-nil-track", Revision: 12, Resolved: true, Status: models.QueueBatchPreflightStatusActive,
	}, queueBatchPreflightState{
		BatchID: "logical-series-title", BatchName: "Complete Series",
		Assets: []queueBatchPreflightAsset{{
			ID: "asset-nil-track", MediaPath: fixture.paths[0], BatchID: "season0-group", BatchName: "Season0",
			Input: QueueJobInput{MediaPath: fixture.paths[0]}, Status: queuePreflightReady,
			Blockers: []string{}, Warnings: []string{},
			ResolvedJob: &models.QueueJob{
				ProfileSnapshot: snapshot, ProfileVersion: 1,
				TrackProfileSnapshot: nil, AudioProfileSnapshot: nil,
				ProfileResolution: nil, ResolvedDestinationSnapshot: nil,
			},
		}},
	})
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Assets []struct {
			Plan map[string]json.RawMessage `json:"plan"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Assets) != 1 {
		t.Fatalf("response assets=%d want=1: %s", len(decoded.Assets), payload)
	}
	for _, field := range []string{"trackProfileSnapshot", "audioProfileSnapshot", "profileResolution", "destination"} {
		if got := string(decoded.Assets[0].Plan[field]); got != "null" {
			t.Errorf("%s JSON=%q want null: %s", field, got, payload)
		}
	}
	var subtitlePlan QueueBatchPreflightSubtitlePlan
	if err := json.Unmarshal(decoded.Assets[0].Plan["subtitlePlan"], &subtitlePlan); err != nil || subtitlePlan.State != "unavailable" {
		t.Errorf("legacy snapshot subtitle plan=%#v err=%v want explicit unavailable state", subtitlePlan, err)
	}
}

func TestBatchPreflightAcceptsTheExactReviewedFrozenPlan(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	fixture.profile.OptimizationIntent = "balanced"
	if err := fixture.db.Model(&models.Profile{}).Where("id = ?", fixture.profile.ID).Update("optimization_intent", fixture.profile.OptimizationIntent).Error; err != nil {
		t.Fatal(err)
	}
	addQueuePreflightBitrateScan(t, fixture.db, fixture.paths[0], 1_000_000)
	var trackScan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&trackScan).Error; err != nil {
		t.Fatal(err)
	}
	trackScan.SubtitleStreams = models.JSONList{
		models.JSONMap{"index": 2, "codec": "subrip", "language": "eng"},
		models.JSONMap{"index": 3, "codec": "ass", "language": "eng"},
		models.JSONMap{"index": 4, "codec": "hdmv_pgs_subtitle", "language": "spa"},
	}
	if err := fixture.db.Save(&trackScan).Error; err != nil {
		t.Fatal(err)
	}
	saveSubtitleSelectionThroughAssetTracks(t, fixture.db, fixture.paths[0], `{"keepSubtitleStreams":[2,4]}`)
	var source models.AssetRecord
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	sourceRevision, err := queuePreflightSourceRevision(fixture.paths[0])
	if err != nil {
		t.Fatal(err)
	}
	router := queuePreflightRouter(fixture.db)
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", queuePreflightInput(fixture, "Season 1"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightReady || candidate.Assets[0].Plan == nil {
		t.Fatalf("candidate did not resolve a ready plan: %#v", candidate.Assets)
	}
	if candidate.Assets[0].Plan.ProfileVersion < 1 || candidate.Assets[0].Plan.ProfileCapturedAt == nil {
		t.Fatalf("candidate did not expose frozen profile version and capture time: %#v", candidate.Assets[0].Plan)
	}
	var reviewedOverride AssetConversionOverrideState
	reviewedOverrideJSON, err := json.Marshal(candidate.Assets[0].Plan.ProfileSnapshot[assetConversionOverrideSnapshotKey])
	if err != nil || json.Unmarshal(reviewedOverrideJSON, &reviewedOverride) != nil || !reflect.DeepEqual(reviewedOverride.KeepSubtitleStreams, []int{2, 4}) || !boolValue(candidate.Assets[0].Plan.ProfileSnapshot[assetSubtitleSelectionExplicitSnapshotKey], false) {
		t.Fatalf("Preflight did not freeze the exact explicit asset subtitle selection: override=%#v snapshot=%#v err=%v", reviewedOverride, candidate.Assets[0].Plan.ProfileSnapshot, err)
	}
	frozenProfile, err := scheduler.RestoreProfileSnapshot(candidate.Assets[0].Plan.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	profileIdentity, ok := profileIdentityFromSnapshot(candidate.Assets[0].Plan.ProfileSnapshot)
	if !ok || profileIdentity.DefinitionFingerprint == "" || profileIdentity.EffectiveProfileFingerprint == "" {
		t.Fatalf("prepared Queue snapshot omitted versioned profile identities: %#v", candidate.Assets[0].Plan.ProfileSnapshot[profileIdentitySnapshotKey])
	}
	qualCandidate, qualEvidence := profileQualificationFixtureForProfile(t, frozenProfile, profileIdentity.DefinitionFingerprint, candidate.Assets[0].Plan.SelectedEncoder, true, 95)
	qualCandidate.SourceAssetID, qualCandidate.SourcePath, qualCandidate.SourceRevision = source.ID, fixture.paths[0], sourceRevision
	qualCandidate.Profile = frozenProfile
	qualCandidate.EffectiveProfileFingerprint = profileIdentity.EffectiveProfileFingerprint
	qualCandidate.EffectiveEncoder = candidate.Assets[0].Plan.SelectedEncoder
	qualEvidence = addQualificationArtifactDefaults(qualEvidence, "medium")
	for index := range qualEvidence.ArtifactEvidence {
		if qualEvidence.ArtifactEvidence[index].Kind == "temporal_instability" {
			qualEvidence.ArtifactEvidence[index].Status = "unsupported"
			qualEvidence.ArtifactEvidence[index].Scope = ""
			qualEvidence.ArtifactEvidence[index].PairIdentity = nil
		}
	}
	qualEvidence, err = ComputePerceptualEvidenceSetIdentity(qualEvidence)
	if err != nil {
		t.Fatal(err)
	}
	qualificationAssessment := EvaluateProfileQualification(qualCandidate, qualEvidence)
	if qualificationAssessment.Outcome != models.QualityOutcomePassWithWarnings {
		t.Fatalf("qualification fixture outcome=%s rules=%#v", qualificationAssessment.Outcome, qualificationAssessment.Rules)
	}
	testEncode := profileQualificationTestEncode(qualCandidate, qualEvidence)
	if err := fixture.db.Create(&testEncode).Error; err != nil {
		t.Fatal(err)
	}
	qualification := profileQualificationModel(qualCandidate, qualEvidence, qualificationAssessment, time.Now().UTC())
	if err := fixture.db.Create(&qualification).Error; err != nil {
		t.Fatal(err)
	}
	updated := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, QueueBatchPreflightInput{
		BatchID: candidate.BatchID, BatchName: candidate.BatchName, Revision: candidate.Revision,
		Jobs: queuePreflightInput(fixture, candidate.BatchName).Jobs,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("resolve qualification update status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.Assets[0].Plan.QualityEvidence == nil {
		t.Fatal("candidate omitted persisted source bitrate and BPPPF evidence")
	}
	qualificationEvidence, _ := snapshotCacheMap(candidate.Assets[0].Plan.QualityEvidence["profileQualification"])
	if stringFromUnknown(qualificationEvidence["status"]) != models.QualityOutcomePassWithWarnings || stringFromUnknown(qualificationEvidence["qualificationId"]) != qualification.ID {
		t.Fatalf("candidate omitted current Profile Qualification provenance: %#v", qualificationEvidence)
	}
	sourceEvidence, _ := snapshotCacheMap(candidate.Assets[0].Plan.QualityEvidence["sourceBitrateEvidence"])
	if stringFromUnknown(sourceEvidence["method"]) != quality.SourceBitrateMethodStreamMetadata || jsonInt64(sourceEvidence["bitrate"]) != 1_000_000 {
		t.Fatalf("candidate source evidence changed: %#v", sourceEvidence)
	}
	assessment, _ := snapshotCacheMap(candidate.Assets[0].Plan.QualityEvidence["bpppfAssessment"])
	if stringFromUnknown(assessment["status"]) != quality.BPPPFStatusHighRisk {
		t.Fatalf("expected a frozen high-risk planning warning, got %#v", assessment)
	}
	if len(candidate.Assets[0].Warnings) == 0 {
		t.Fatal("Preflight omitted its BPPPF warning")
	}
	changedSnapshot := queuePreflightCloneMap(candidate.Assets[0].Plan.ProfileSnapshot)
	changedWorkerConfig, ok := snapshotCacheMap(changedSnapshot["workerConfig"])
	if !ok {
		t.Fatal("frozen Queue snapshot omitted the effective worker configuration")
	}
	changedWorkerConfig["hevcLevelEffective"] = "5.2"
	changedSnapshot["workerConfig"] = changedWorkerConfig
	changedProfileAsset := []queueBatchPreflightAsset{{
		ID: "changed-profile", MediaPath: fixture.paths[0], AssetRecordID: &source.ID,
		SourceRevision: sourceRevision, Constraints: models.JSONMap{"ready": true},
		SelectedEncoder: candidate.Assets[0].Plan.SelectedEncoder,
		ResolvedJob:     &models.QueueJob{ProfileSnapshot: changedSnapshot},
	}}
	if err := attachPreflightProfileQualifications(fixture.db, changedProfileAsset); err != nil {
		t.Fatal(err)
	}
	changedProfileEvidence, ok := changedProfileAsset[0].QualityEvidence["profileQualification"].(profileQualificationPreflightView)
	if !ok || changedProfileEvidence.Status != "STALE" || !strings.Contains(changedProfileEvidence.Message, "does not match its frozen profile snapshot") {
		t.Fatalf("changed effective profile did not fail closed with a cause: %#v", changedProfileEvidence)
	}
	otherRecord := models.AssetRecord{
		Path:     filepath.Join(filepath.Dir(fixture.paths[0]), "OtherAsset.mkv"),
		FileName: "OtherAsset.mkv", Status: "unprocessed",
	}
	if err := fixture.db.Create(&otherRecord).Error; err != nil {
		t.Fatal(err)
	}
	otherAssetID := otherRecord.ID
	crossAsset := []queueBatchPreflightAsset{{
		ID: "same-path-different-asset", MediaPath: fixture.paths[0], AssetRecordID: &otherAssetID,
		SourceRevision: sourceRevision, Constraints: models.JSONMap{"ready": true},
		ResolvedJob: &models.QueueJob{ProfileSnapshot: candidate.Assets[0].Plan.ProfileSnapshot},
	}}
	if err := attachPreflightProfileQualifications(fixture.db, crossAsset); err != nil {
		t.Fatal(err)
	}
	crossAssetEvidence, ok := crossAsset[0].QualityEvidence["profileQualification"].(profileQualificationPreflightView)
	if !ok || crossAssetEvidence.Status != "STALE" {
		t.Fatalf("Qualification from another AssetRecord produced %#v", crossAssetEvidence)
	}
	if message := crossAssetEvidence.Message; !strings.Contains(message, "AssetRecord path differs from the frozen source path") {
		t.Fatalf("cross-asset mismatch omitted its cause: %#v", crossAssetEvidence)
	}
	var queuedCount, executionPlanCount int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&queuedCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.ExecutionPlan{}).Count(&executionPlanCount).Error; err != nil {
		t.Fatal(err)
	}
	if queuedCount != 0 || executionPlanCount != 0 {
		t.Fatalf("draft created Queue state before acceptance: jobs=%d plans=%d", queuedCount, executionPlanCount)
	}

	reviewedSnapshot := candidate.Assets[0].Plan.ProfileSnapshot
	reviewedConstraints := candidate.Assets[0].Plan.Constraints
	mutatedDestination := filepath.Join(t.TempDir(), "changed-destination")
	if err := os.Mkdir(mutatedDestination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.Profile{}).Where("id = ?", fixture.profile.ID).Update("quality_value", 31).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.Library{}).Where("id = ?", fixture.library.ID).Update("destination_path", mutatedDestination).Error; err != nil {
		t.Fatal(err)
	}

	accepted := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{
		"revision": candidate.Revision, "assetIds": []string{candidate.Assets[0].ID},
	})
	if accepted.Code != http.StatusCreated {
		t.Fatalf("accept status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var acceptedCandidate QueueBatchPreflightResponse
	if err := json.Unmarshal(accepted.Body.Bytes(), &acceptedCandidate); err != nil {
		t.Fatal(err)
	}
	if acceptedCandidate.Status != models.QueueBatchPreflightStatusCompleted || len(acceptedCandidate.Jobs) != 1 {
		t.Fatalf("accepted jobs=%d want=1", len(acceptedCandidate.Jobs))
	}
	job := acceptedCandidate.Jobs[0]
	if !reflect.DeepEqual(job.ProfileSnapshot, reviewedSnapshot) {
		t.Fatalf("accepted Queue snapshot differs from reviewed snapshot\nreviewed=%#v\naccepted=%#v", reviewedSnapshot, job.ProfileSnapshot)
	}
	var acceptedOverride AssetConversionOverrideState
	acceptedOverrideJSON, err := json.Marshal(job.ProfileSnapshot[assetConversionOverrideSnapshotKey])
	if err != nil || json.Unmarshal(acceptedOverrideJSON, &acceptedOverride) != nil || !reflect.DeepEqual(acceptedOverride.KeepSubtitleStreams, []int{2, 4}) || !boolValue(job.ProfileSnapshot[assetSubtitleSelectionExplicitSnapshotKey], false) {
		t.Fatalf("accepted job changed the reviewed asset subtitle selection: override=%#v snapshot=%#v err=%v", acceptedOverride, job.ProfileSnapshot, err)
	}
	profile, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if profile.QualityValue != fixture.profile.QualityValue {
		t.Fatalf("accepted profile quality=%d want reviewed=%d", profile.QualityValue, fixture.profile.QualityValue)
	}
	if !reflect.DeepEqual(acceptedCandidate.Assets[0].Plan.Constraints, reviewedConstraints) {
		t.Fatal("accepted plan constraints changed from reviewed constraints")
	}
	wantOutput := filepath.Join(fixture.library.DestinationPath, filepath.Base(fixture.paths[0]))
	if got := workerStringValue(job.ResolvedDestinationSnapshot["outputPath"]); got != wantOutput {
		t.Fatalf("accepted output path=%q want frozen destination %q", got, wantOutput)
	}
	changedDefaults := defaultAssetDefaultsValue()
	videoDefaults := changedDefaults["video"].(models.JSONMap)
	videoDefaults["custom"] = models.JSONMap{"videoCodec": "copy"}
	changedDefaults["video"] = videoDefaults
	if err := fixture.db.Create(&models.AppSetting{Key: assetDefaultsSettingKey, Value: changedDefaults}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.Profile{}).Where("id = ?", fixture.profile.ID).Update("quality_value", 42).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(job.MediaPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(job.MediaPath, []byte("source changed after review"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolvedProfile, err := resolveQueueVideoProfile(fixture.db, job.MediaPath, job, profile, AssetConversionOverrideState{}, "mutated-encoder")
	if err != nil {
		t.Fatal(err)
	}
	if resolvedProfile.QualityValue != fixture.profile.QualityValue || workerStringValue(resolvedProfile.WorkerConfig["videoEncoder"]) != "libx265" {
		t.Fatalf("worker changed reviewed profile after acceptance: %#v", resolvedProfile)
	}
	manifest, manifestOK := snapshotCacheMap(job.ProfileSnapshot[models.QueueSourceAnalysisEvidenceKey])
	frozenBitrate, bitrateOK := decodeSourceBitrateEvidence(manifest["sourceBitrateEvidence"])
	frozenRail, railOK := snapshotCacheMap(manifest["bpppfAssessment"])
	if !manifestOK || !bitrateOK || frozenBitrate.Bitrate != 1_000_000 || !railOK || stringFromUnknown(frozenRail["status"]) != quality.BPPPFStatusHighRisk {
		t.Fatalf("worker altered or discarded reviewed evidence after source change: manifest=%#v", manifest)
	}
	var acceptedPlan models.ExecutionPlan
	if err := fixture.db.First(&acceptedPlan, *job.ActiveExecutionPlanID).Error; err != nil {
		t.Fatal(err)
	}
	acceptedQualityEvidence, _ := snapshotCacheMap(acceptedPlan.Evaluation["qualityEvidence"])
	acceptedQualification, _ := snapshotCacheMap(acceptedQualityEvidence["profileQualification"])
	if stringFromUnknown(acceptedQualification["qualificationId"]) != qualification.ID || stringFromUnknown(acceptedQualification["policyVersion"]) != profileQualificationPolicyVersion {
		t.Fatalf("accepted execution plan lost the exact qualification reviewed in Preflight: %#v", acceptedQualification)
	}
	acceptedEvidenceJSON, acceptedEvidenceErr := json.Marshal(acceptedPlan.Evaluation["qualityEvidence"])
	reviewedEvidenceJSON, reviewedEvidenceErr := json.Marshal(candidate.Assets[0].Plan.QualityEvidence)
	if acceptedEvidenceErr != nil || reviewedEvidenceErr != nil || !bytes.Equal(acceptedEvidenceJSON, reviewedEvidenceJSON) {
		t.Fatalf("accepted ExecutionPlan evidence differs from reviewed evidence: %#v", acceptedPlan.Evaluation["qualityEvidence"])
	}
	terminalEdit := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/edit", map[string]any{"revision": acceptedCandidate.Revision})
	terminalUpdate := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, QueueBatchPreflightInput{
		Revision: acceptedCandidate.Revision, BatchID: candidate.BatchID, Jobs: []QueueJobInput{candidate.Assets[0].Input},
	})
	terminalAccept := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{
		"revision": acceptedCandidate.Revision, "assetIds": []string{candidate.Assets[0].ID},
	})
	terminalRemove := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/remove", map[string]any{
		"revision": acceptedCandidate.Revision, "assetId": candidate.Assets[0].ID,
	})
	terminalDiscard := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/discard", map[string]any{"revision": acceptedCandidate.Revision})
	for operation, response := range map[string]*httptest.ResponseRecorder{
		"edit": terminalEdit, "resolve": terminalUpdate, "accept": terminalAccept, "remove": terminalRemove, "discard": terminalDiscard,
	} {
		if response.Code != http.StatusConflict {
			t.Fatalf("terminal candidate %s returned status=%d body=%s", operation, response.Code, response.Body.String())
		}
	}
	var unchangedJobs int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&unchangedJobs).Error; err != nil || unchangedJobs != 1 {
		t.Fatalf("terminal mutations changed accepted QueueJobs: count=%d err=%v", unchangedJobs, err)
	}
}

func TestQueuePreflightFreezesAssetSubtitleExclusionAheadOfTrackProfileRules(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	addQueuePreflightBitrateScan(t, fixture.db, fixture.paths[0], 1_000_000)
	var scan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	scan.SubtitleStreams = models.JSONList{
		models.JSONMap{"index": 2, "codec": "subrip", "language": "eng"},
		models.JSONMap{"index": 3, "codec": "ass", "language": "eng"},
		models.JSONMap{"index": 4, "codec": "hdmv_pgs_subtitle", "language": "spa"},
	}
	if err := fixture.db.Save(&scan).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&models.AppSetting{Key: "trackProfiles", Value: models.JSONMap{"profiles": models.JSONList{models.JSONMap{
		"key": "subtitle-rules", "scope": "path", "trackDispositionVersion": 1,
		"subtitleDisposition": "keep",
		"subtitleRules": models.JSONList{
			models.JSONMap{"streamIndex": 2, "action": "keep_and_extract"},
			models.JSONMap{"streamIndex": 3, "action": "extract"},
			models.JSONMap{"streamIndex": 4, "action": "keep"},
		},
		"attachmentPolicy": "auto", "chapterPolicy": "keep",
	}}}}).Error; err != nil {
		t.Fatal(err)
	}
	saveSubtitleSelectionThroughAssetTracks(t, fixture.db, fixture.paths[0], `{"keepSubtitleStreams":[2,4]}`)
	input := queuePreflightInput(fixture, "Track profile exclusion")
	input.Jobs[0].TrackProfileKey = "subtitle-rules"
	response := queuePreflightRequest(t, queuePreflightRouter(fixture.db), http.MethodPost, "/preflights", input)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(response.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightReady || candidate.Assets[0].Plan == nil {
		t.Fatalf("Track Profile candidate did not resolve a ready plan: %#v", candidate.Assets)
	}
	frozen, ok := ResolvedTrackPlanFromSnapshot(candidate.Assets[0].Plan.TrackProfileSnapshot)
	if !ok {
		t.Fatalf("Preflight omitted its canonical frozen track plan: %#v", candidate.Assets[0].Plan.TrackProfileSnapshot)
	}
	var excludedAction SubtitleDisposition
	for _, stream := range frozen.SubtitleStreams {
		if stream.StreamIndex == 3 {
			excludedAction = stream.Action
		}
	}
	if excludedAction != SubtitleDispositionRemove {
		t.Fatalf("Track Profile rule restored excluded stream 3 in Preflight: %#v", frozen.SubtitleStreams)
	}
	for _, sidecar := range frozen.SidecarOutputs {
		if sidecar.StreamIndex == 3 {
			t.Fatalf("Preflight froze a sidecar for excluded stream 3: %#v", frozen.SidecarOutputs)
		}
	}
	var override AssetConversionOverrideState
	encoded, err := json.Marshal(candidate.Assets[0].Plan.ProfileSnapshot[assetConversionOverrideSnapshotKey])
	if err != nil || json.Unmarshal(encoded, &override) != nil || !reflect.DeepEqual(override.KeepSubtitleStreams, []int{2, 4}) || !boolValue(candidate.Assets[0].Plan.ProfileSnapshot[assetSubtitleSelectionExplicitSnapshotKey], false) {
		t.Fatalf("Preflight lost the reviewed asset selection: override=%#v snapshot=%#v err=%v", override, candidate.Assets[0].Plan.ProfileSnapshot, err)
	}
}

func TestBatchPreflightProjectsSubtitleActionsAndFreezesQueueOnlyOverride(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 2)
	addQueuePreflightBitrateScan(t, fixture.db, fixture.paths[0], 1_000_000)
	for _, path := range fixture.paths {
		var scan models.ScanResult
		if err := fixture.db.Where("path = ?", path).First(&scan).Error; err != nil {
			t.Fatal(err)
		}
		scan.SubtitleStreams = models.JSONList{
			models.JSONMap{"index": 2, "codec": "subrip", "language": "spa", "title": "Spanish", "default": true},
			models.JSONMap{"index": 3, "codec": "subrip", "language": "eng"},
			models.JSONMap{"index": 4, "codec": "hdmv_pgs_subtitle", "language": "jpn", "forced": true},
			models.JSONMap{"index": 5, "codec": "ass", "language": "fra"},
		}
		if err := fixture.db.Save(&scan).Error; err != nil {
			t.Fatal(err)
		}
	}
	trackProfile := models.JSONMap{
		"key": "subtitle-review", "scope": "path", "trackDispositionVersion": 1,
		"subtitleMode": "all", "subtitleDisposition": "extract", "subtitleSidecarFormats": models.JSONList{"both"},
		"defaultSubtitleLanguage": "spa", "attachmentPolicy": "auto", "chapterPolicy": "keep",
		"preferredSubtitleRules": models.JSONList{
			models.JSONMap{"language": "spa", "action": "extract"},
			models.JSONMap{"language": "eng", "action": "extract"},
			models.JSONMap{"language": "jpn", "action": "keep_and_extract"},
		},
	}
	if err := fixture.db.Create(&models.AppSetting{Key: "trackProfiles", Value: models.JSONMap{"profiles": models.JSONList{trackProfile}}}).Error; err != nil {
		t.Fatal(err)
	}
	input := queuePreflightInput(fixture, "Subtitle review")
	input.Jobs = input.Jobs[:1]
	input.Jobs[0].TrackProfileKey = "subtitle-review"
	router := queuePreflightRouter(fixture.db)
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 1 || candidate.Assets[0].Plan == nil || candidate.Assets[0].Plan.SubtitlePlan == nil {
		t.Fatalf("candidate omitted subtitle projection: %#v", candidate.Assets)
	}
	initial := candidate.Assets[0].Plan.SubtitlePlan
	if initial.Version != 1 || initial.State != "planned" || initial.SourceStreamCount != 4 || initial.SidecarStreams != 4 || initial.KeptInMKV != 1 || initial.KeepAndExtract != 1 || initial.PolicySource != "Track Profile" {
		t.Fatalf("incorrect effective subtitle summary: %#v", initial)
	}
	if initial.Decisions[0].Action != SubtitleDispositionExtract || initial.Decisions[1].Action != SubtitleDispositionExtract || initial.Decisions[2].Action != SubtitleDispositionKeepAndExtract || initial.Decisions[3].Action != SubtitleDispositionExtract {
		t.Fatalf("preferred rules or unmatched Extract fallback were misreported: %#v", initial.Decisions)
	}
	if initial.Decisions[0].Title != "Spanish" || !initial.Decisions[0].SourceDefault || initial.DefaultSubtitle.RequestedLanguage != "spa" || initial.DefaultSubtitle.Status != "missing" {
		t.Fatalf("source metadata/default flag preference was not represented honestly: %#v", initial)
	}
	if len(initial.Sidecars) != 8 || !initial.Sidecars[4].OCRRequired || len(initial.Warnings) != 1 || !strings.Contains(initial.Warnings[0], "FRA") {
		t.Fatalf("formats, OCR, or unmatched-language warning missing: %#v", initial)
	}
	if candidate.Assets[0].SourceRevision == "" || candidate.Assets[0].SourceRevision != initialDecisionSourceRevision(candidate.Assets[0], 5) {
		t.Fatalf("candidate stream projection lacks its source revision identity: %#v", candidate.Assets[0])
	}

	updatedInput := candidate.Assets[0].Input
	updatedInput.CandidateSubtitleDispositions = []CandidateSubtitleDispositionOverride{{
		StreamIndex: 5, StreamSignature: initial.Decisions[3].StreamSignature,
		SourceRevision: candidate.Assets[0].SourceRevision, Action: SubtitleDispositionKeep,
	}}
	updated := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, QueueBatchPreflightInput{
		Revision: candidate.Revision, BatchID: candidate.BatchID, BatchName: candidate.BatchName,
		Jobs: []QueueJobInput{updatedInput},
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("Queue-only subtitle update status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	updatedAsset := candidate.Assets[0]
	if updatedAsset.Plan == nil || updatedAsset.Plan.SubtitlePlan == nil {
		t.Fatalf("candidate update omitted subtitle projection: %#v", updatedAsset)
	}
	decision := updatedAsset.Plan.SubtitlePlan.Decisions[3]
	if decision.Action != SubtitleDispositionKeep || decision.ActionSource != "candidate_override" || decision.InheritedAction == nil || *decision.InheritedAction != SubtitleDispositionExtract {
		t.Fatalf("candidate override lacks the reviewed before/after plan: %#v", decision)
	}
	if updatedAsset.Plan.SubtitlePlan.KeptInMKV != 2 || updatedAsset.Plan.SubtitlePlan.SidecarStreams != 3 || updatedAsset.Plan.SubtitlePlan.KeepAndExtract != 1 {
		t.Fatalf("candidate override summary counts are incorrect: %#v", updatedAsset.Plan.SubtitlePlan)
	}
	for _, sidecar := range updatedAsset.Plan.SubtitlePlan.Sidecars {
		if sidecar.StreamIndex == 5 {
			t.Fatalf("Keep override retained a sidecar for stream 5: %#v", sidecar)
		}
	}
	if _, exists := assetConversionOverrides(fixture.db)[filepath.Clean(fixture.paths[0])]; exists {
		t.Fatalf("Queue-only action wrote a persistent Asset Tracks override: %#v", assetConversionOverrides(fixture.db))
	}

	accepted := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{
		"revision": candidate.Revision, "assetIds": []string{candidate.Assets[0].ID},
	})
	if accepted.Code != http.StatusCreated {
		t.Fatalf("accept status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var acceptedCandidate QueueBatchPreflightResponse
	if err := json.Unmarshal(accepted.Body.Bytes(), &acceptedCandidate); err != nil {
		t.Fatal(err)
	}
	if len(acceptedCandidate.Jobs) != 1 {
		t.Fatalf("accepted jobs=%d want=1", len(acceptedCandidate.Jobs))
	}
	frozen, ok := ResolvedTrackPlanFromSnapshot(acceptedCandidate.Jobs[0].TrackProfileSnapshot)
	if !ok || frozen.SubtitleStreams[3].Action != SubtitleDispositionKeep || frozen.SubtitleStreams[3].ActionSource != "candidate_override" {
		t.Fatalf("accepted Queue snapshot differs from reviewed subtitle plan: %#v", acceptedCandidate.Jobs[0].TrackProfileSnapshot)
	}
	assertQueueJobSubtitleArtifactsMatchFrozenPlan(t, acceptedCandidate.Jobs[0])
	for _, artifact := range subtitleArtifactsFromJSON(acceptedCandidate.Jobs[0].SubtitleArtifacts) {
		if artifact.StreamIndex == 5 {
			t.Fatalf("accepted Queue job retained the inherited Extract artifact after Keep: %#v", artifact)
		}
	}
	if !hasSubtitleArtifactStream(acceptedCandidate.Jobs[0].SubtitleArtifacts, 4) {
		t.Fatalf("Keep + Extract sidecar was lost from accepted Queue job: %#v", acceptedCandidate.Jobs[0].SubtitleArtifacts)
	}
	if _, exists := assetConversionOverrides(fixture.db)[filepath.Clean(fixture.paths[0])]; exists {
		t.Fatalf("accepting candidate action wrote a persistent Asset Tracks override: %#v", assetConversionOverrides(fixture.db))
	}

	var setting models.AppSetting
	if err := fixture.db.First(&setting, "key = ?", "trackProfiles").Error; err != nil {
		t.Fatal(err)
	}
	profiles := workerSliceValue(setting.Value["profiles"])
	settingProfileObject(profiles[0])["subtitleDisposition"] = "remove"
	setting.Value["profiles"] = profiles
	if err := fixture.db.Save(&setting).Error; err != nil {
		t.Fatal(err)
	}
	removeInput := QueueBatchPreflightInput{
		BatchID: "preflight-remove-fallback",
		Jobs: []QueueJobInput{{
			MediaPath: fixture.paths[1], LibraryID: fixture.library.ID, ProfileID: fixture.profile.ID,
			ProcessingMode: ProcessingModeFullEncode, Priority: 5, TrackProfileKey: "subtitle-review",
		}},
	}
	removeCandidateResponse := queuePreflightRequest(t, router, http.MethodPost, "/preflights", removeInput)
	if removeCandidateResponse.Code != http.StatusCreated {
		t.Fatalf("Remove fallback create status=%d body=%s", removeCandidateResponse.Code, removeCandidateResponse.Body.String())
	}
	var removeCandidate QueueBatchPreflightResponse
	if err := json.Unmarshal(removeCandidateResponse.Body.Bytes(), &removeCandidate); err != nil {
		t.Fatal(err)
	}
	removePlan := removeCandidate.Assets[0].Plan.SubtitlePlan
	if removePlan == nil || removePlan.State != "planned" || removePlan.SidecarStreams != 3 || removePlan.Removed != 1 || len(removePlan.Sidecars) != 6 {
		t.Fatalf("Remove fallback projection planned work for unmatched stream(s): %#v", removePlan)
	}
	if removePlan.Decisions[3].Action != SubtitleDispositionRemove {
		t.Fatalf("unmatched FRA disposition=%q want remove", removePlan.Decisions[3].Action)
	}
	for _, sidecar := range removePlan.Sidecars {
		if sidecar.StreamIndex == 5 {
			t.Fatalf("Remove fallback produced a sidecar for stream 5: %#v", sidecar)
		}
	}
}

func TestQueuePreflightSubtitleDispositionReconcilesArtifactsAndWorkerRetry(t *testing.T) {
	fontScan := models.ScanResult{
		Path: "/raw/Episode.mkv", FileName: "Episode.mkv",
		SubtitleStreams: models.JSONList{
			models.JSONMap{"index": 2, "codec": "ass", "language": "spa"},
			models.JSONMap{"index": 3, "codec": "subrip", "language": "eng"},
		},
		AttachmentStreams: models.JSONList{models.JSONMap{
			"index": 7, "type": "attachment", "codec": "ttf", "filename": "Planned.ttf",
			"mimeType": "font/ttf", "attachmentKind": "FONT", "fontFormat": "TTF",
		}},
		AttachmentInventoryAvailable: true,
	}
	singleStreamScan := models.ScanResult{
		Path: "/raw/Single.mkv", FileName: "Single.mkv",
		SubtitleStreams: models.JSONList{models.JSONMap{"index": 2, "codec": "subrip", "language": "eng"}},
	}
	tests := []struct {
		name         string
		fallback     string
		scan         models.ScanResult
		streamIndex  int
		action       SubtitleDisposition
		wantAction   SubtitleDisposition
		wantSidecars int
		wantFonts    int
	}{
		{name: "Extract to Keep preserves unrelated font and sidecar artifacts", fallback: "extract", scan: fontScan, streamIndex: 3, action: SubtitleDispositionKeep, wantAction: SubtitleDispositionKeep, wantSidecars: 1, wantFonts: 1},
		{name: "Extract to Remove preserves unrelated font and sidecar artifacts", fallback: "extract", scan: fontScan, streamIndex: 3, action: SubtitleDispositionRemove, wantAction: SubtitleDispositionRemove, wantSidecars: 1, wantFonts: 1},
		{name: "Remove to Extract adds the newly planned artifact", fallback: "remove", scan: fontScan, streamIndex: 3, action: SubtitleDispositionExtract, wantAction: SubtitleDispositionExtract, wantSidecars: 1},
		{name: "Keep plus Extract creates a sidecar while retaining the stream", fallback: "extract", scan: fontScan, streamIndex: 3, action: SubtitleDispositionKeepAndExtract, wantAction: SubtitleDispositionKeepAndExtract, wantSidecars: 2, wantFonts: 1},
		{name: "last Extract sidecar to Keep clears the artifact list", fallback: "extract", scan: singleStreamScan, streamIndex: 2, action: SubtitleDispositionKeep, wantAction: SubtitleDispositionKeep},
		{name: "last Extract sidecar to Remove clears the artifact list", fallback: "extract", scan: singleStreamScan, streamIndex: 2, action: SubtitleDispositionRemove, wantAction: SubtitleDispositionRemove},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := map[string]any{
				"trackDispositionVersion": 1, "subtitleMode": "all", "subtitleDisposition": test.fallback,
				"subtitleSidecarFormats": []string{"original"}, "attachmentPolicy": "remove",
				"fontAttachmentExportPolicy": "all", "chapterPolicy": "keep",
			}
			initialPlan, err := resolveTrackPlan(test.scan, profile)
			if err != nil {
				t.Fatal(err)
			}
			initialPlanMap, err := resolvedTrackPlanMap(initialPlan)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := models.JSONMap{}
			for key, value := range profile {
				snapshot[key] = value
			}
			snapshot[resolvedTrackPlanSnapshotKey] = initialPlanMap
			job := &models.QueueJob{MediaPath: test.scan.Path, TrackProfileSnapshot: snapshot}
			job.SubtitleArtifacts = plannedSidecarArtifactsJSON(*job)
			if len(initialPlan.SidecarOutputs) > 0 && len(job.SubtitleArtifacts) == 0 {
				t.Fatal("fixture did not begin with the inherited sidecar artifact")
			}
			initialFonts := fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts)
			streamSignature := ""
			for _, stream := range initialPlan.SubtitleStreams {
				if stream.StreamIndex == test.streamIndex {
					streamSignature = stream.StreamSignature
					break
				}
			}
			if streamSignature == "" {
				t.Fatalf("canonical plan omitted stream %d identity", test.streamIndex)
			}

			handler := QueueHandler{}
			err = handler.applyQueuePreflightSubtitleDispositions(job, test.scan, "source-revision-1", []CandidateSubtitleDispositionOverride{{
				StreamIndex: test.streamIndex, StreamSignature: streamSignature,
				SourceRevision: "source-revision-1", Action: test.action,
			}})
			if err != nil {
				t.Fatal(err)
			}
			plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
			if !ok {
				t.Fatal("candidate update lost the canonical resolved plan")
			}
			var actualAction SubtitleDisposition
			for _, stream := range plan.SubtitleStreams {
				if stream.StreamIndex == test.streamIndex {
					actualAction = stream.Action
					break
				}
			}
			if actualAction != test.wantAction {
				t.Fatalf("resolved action=%q want=%q plan=%#v", actualAction, test.wantAction, plan.SubtitleStreams)
			}
			if got := len(plannedSubtitleArtifacts(*job)); got != test.wantSidecars {
				t.Fatalf("canonical sidecar count=%d want=%d", got, test.wantSidecars)
			}
			assertQueueJobSubtitleArtifactsMatchFrozenPlan(t, *job)
			if test.wantSidecars == 0 && len(job.SubtitleArtifacts) != 0 {
				t.Fatalf("QueueJob retained stale artifacts after its last sidecar was removed: %#v", job.SubtitleArtifacts)
			}
			if got := len(fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts)); got != test.wantFonts {
				t.Fatalf("font artifacts=%#v want count=%d", fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts), test.wantFonts)
			}
			if test.wantFonts > 0 && !reflect.DeepEqual(fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts), initialFonts) {
				t.Fatalf("candidate subtitle edit changed unrelated planned font artifacts: before=%#v after=%#v", initialFonts, fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts))
			}

			// Worker retries/restarts consume the frozen plan and must not restore
			// artifacts from the inherited pre-edit plan.
			ensurePlannedSidecarArtifacts(job)
			ensurePlannedSidecarArtifacts(job)
			assertQueueJobSubtitleArtifactsMatchFrozenPlan(t, *job)
		})
	}
}

func assertQueueJobSubtitleArtifactsMatchFrozenPlan(t *testing.T, job models.QueueJob) {
	t.Helper()
	got, err := json.Marshal(job.SubtitleArtifacts)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(plannedSidecarArtifactsJSON(job))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("QueueJob subtitle artifacts differ from frozen plan: got=%s want=%s", got, want)
	}
}

func hasSubtitleArtifactStream(artifacts models.JSONList, streamIndex int) bool {
	for _, artifact := range subtitleArtifactsFromJSON(artifacts) {
		if artifact.StreamIndex == streamIndex {
			return true
		}
	}
	return false
}

func initialDecisionSourceRevision(asset QueueBatchPreflightAssetResponse, streamIndex int) string {
	for _, decision := range asset.Plan.SubtitlePlan.Decisions {
		if decision.StreamIndex == streamIndex {
			return asset.SourceRevision
		}
	}
	return ""
}

func TestBatchPreflightUnqualifiedProfileAddsWarningWithoutBlocking(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	var source models.AssetRecord
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	revision, err := queuePreflightSourceRevision(fixture.paths[0])
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := scheduler.CaptureProfileSnapshot(fixture.profile, time.Now().UTC(), "profile-qualification-preflight-test")
	if err != nil {
		t.Fatal(err)
	}
	assets := []queueBatchPreflightAsset{{
		ID: "unqualified", MediaPath: fixture.paths[0], AssetRecordID: &source.ID, SourceRevision: revision,
		Status: queuePreflightReady, Blockers: []string{}, Warnings: []string{}, Constraints: models.JSONMap{"ready": true},
		ResolvedJob: &models.QueueJob{ProfileSnapshot: snapshot},
	}}
	if err := attachPreflightProfileQualifications(fixture.db, assets); err != nil {
		t.Fatal(err)
	}
	qualification, ok := assets[0].QualityEvidence["profileQualification"].(profileQualificationPreflightView)
	if !ok || qualification.Status != "NOT_QUALIFIED" {
		t.Fatalf("missing qualification was not surfaced as unqualified: %#v", assets[0].QualityEvidence)
	}
	if !strings.Contains(qualification.Message, "predates the versioned reusable profile identity") {
		t.Fatalf("legacy Queue snapshot omitted the identity cause: %#v", qualification)
	}
	if assets[0].Status != queuePreflightReady || len(assets[0].Blockers) != 0 || len(assets[0].Warnings) == 0 {
		t.Fatalf("unqualified evidence changed Queue blocking behavior: %#v", assets[0])
	}
}

func addQueuePreflightBitrateScan(t *testing.T, db *gorm.DB, path string, bitrate int64) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	evidence := quality.SourceBitrateEvidence{
		SchemaVersion: quality.SourceBitrateEvidenceSchemaVersion, SamplingPolicy: quality.SourceBitrateSamplingPolicyVersion,
		Bitrate: bitrate, Method: quality.SourceBitrateMethodStreamMetadata, Confidence: "high",
		StreamIndex: intPointer(0), DurationSeconds: 90, SampledWindows: []quality.SourceBitrateSampleWindow{},
	}
	scan := models.ScanResult{
		Path: path, FileName: filepath.Base(path), Container: "matroska", SizeBytes: info.Size(), Duration: 90,
		Bitrate: 10_000_000, SourceBitrateEvidence: sourceBitrateEvidenceMap(evidence), VideoCodec: "h264",
		Width: 1920, Height: 1080, VideoStreams: models.JSONList{models.JSONMap{
			"index": 0, "codec": "h264", "bitrate": bitrate, "width": 1920, "height": 1080,
			"avgFrameRate": "24/1", "realFrameRate": "24/1", "duration": 90,
		}}, AudioStreams: models.JSONList{}, RawProbe: models.JSONMap{},
	}
	stampSnapshotCacheMetadata(&scan, path, info)
	var existing models.ScanResult
	if err := db.Where("path = ?", path).First(&existing).Error; err == nil {
		scan.ID = existing.ID
		scan.InterlaceAnalysis = existing.InterlaceAnalysis
		scan.CadenceAnalysis = existing.CadenceAnalysis
		scan.CadenceRecommendation = existing.CadenceRecommendation
		if err := db.Save(&scan).Error; err != nil {
			t.Fatal(err)
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := db.Create(&scan).Error; err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal(err)
	}
}

func TestBatchPreflightEditInvalidatesThenReresolvesSavedConfiguration(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	if err := fixture.db.Create(&models.ProfileAssignment{TargetType: "path", TargetPath: filepath.Dir(fixture.paths[0]), MediaType: "video", Selection: "profile", VideoProfileID: fixture.profile.ID}).Error; err != nil {
		t.Fatal(err)
	}
	router := queuePreflightRouter(fixture.db)
	initialInput := queuePreflightInput(fixture, "Original batch")
	initialInput.Jobs[0].ResolveProfileAssignments = true
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", initialInput)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	edit := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/edit", map[string]any{"revision": candidate.Revision})
	if edit.Code != http.StatusOK {
		t.Fatalf("edit status=%d body=%s", edit.Code, edit.Body.String())
	}
	var invalidated QueueBatchPreflightResponse
	if err := json.Unmarshal(edit.Body.Bytes(), &invalidated); err != nil {
		t.Fatal(err)
	}
	if invalidated.Resolved || invalidated.Revision != candidate.Revision+1 {
		t.Fatalf("Edit did not invalidate prior resolution: resolved=%v revision=%d want=%d", invalidated.Resolved, invalidated.Revision, candidate.Revision+1)
	}
	if invalidated.Assets[0].Input.ProfileID != fixture.profile.ID || invalidated.Assets[0].Input.LibraryID != fixture.library.ID {
		t.Fatalf("Edit lost candidate configuration: %#v", invalidated.Assets[0].Input)
	}
	if len(invalidated.Assets[0].Input.ProfileResolution) == 0 {
		t.Fatal("candidate did not persist effective assignment provenance")
	}
	input := queuePreflightInput(fixture, "Edited batch")
	input.Revision = invalidated.Revision
	input.Jobs[0].Notes = "retained user selection"
	updated := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, input)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	var resolved QueueBatchPreflightResponse
	if err := json.Unmarshal(updated.Body.Bytes(), &resolved); err != nil {
		t.Fatal(err)
	}
	if !resolved.Resolved || resolved.Revision != invalidated.Revision+1 || resolved.BatchName != "Edited batch" || resolved.Assets[0].Input.Notes != "retained user selection" || resolved.Assets[0].Plan == nil {
		t.Fatalf("Edit did not persist and re-resolve candidate inputs: %#v", resolved)
	}
	if !reflect.DeepEqual(resolved.Assets[0].Input.ProfileResolution, candidate.Assets[0].Input.ProfileResolution) {
		t.Fatal("Edit discarded frozen profile assignment provenance")
	}
	if resolved.Assets[0].Plan == nil || resolved.Assets[0].Plan.Profile.ID != fixture.profile.ID {
		t.Fatalf("Edit lost effective assigned profile: %#v", resolved.Assets[0].Plan)
	}
	staleAccept := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{"revision": candidate.Revision, "assetIds": []string{resolved.Assets[0].ID}})
	if staleAccept.Code != http.StatusConflict {
		t.Fatalf("stale candidate accepted: status=%d body=%s", staleAccept.Code, staleAccept.Body.String())
	}
}

func TestBatchPreflightEditReviewsPathAssignmentBeforeCandidateOverride(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	oldPath := fixture.paths[0]
	sourceRoot := filepath.Dir(oldPath)
	documentariesPath := filepath.Join(sourceRoot, "documentaries")
	assetGroupPath := filepath.Join(documentariesPath, "Cobain - Montage of Heck")
	assetPath := filepath.Join(assetGroupPath, "Cobain - Montage of Heck.mkv")
	if err := os.MkdirAll(assetGroupPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldPath, assetPath); err != nil {
		t.Fatal(err)
	}
	var asset models.AssetRecord
	if err := fixture.db.Where("path = ?", oldPath).First(&asset).Error; err != nil {
		t.Fatal(err)
	}
	asset.Path, asset.SourcePath = assetPath, assetPath
	asset.RootPath, asset.GroupPath = sourceRoot, filepath.Join("documentaries", "Cobain - Montage of Heck")
	asset.LogicalGroupPath, asset.FileName = documentariesPath, filepath.Base(assetPath)
	if err := fixture.db.Save(&asset).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.ScanResult{}).Where("path = ?", oldPath).Updates(map[string]any{"path": assetPath, "file_name": filepath.Base(assetPath)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.AutoMigrate(&models.SourceGroup{}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "test", Encoders: models.JSONMap{"libx265": models.JSONMap{"listed": true, "usable": true}}}).Error; err != nil {
		t.Fatal(err)
	}
	fixture.paths[0] = assetPath
	pathProfile := fixture.profile
	pathProfile.Name = "Path Profile A"
	pathProfile.ProfileVersion = 3
	if err := fixture.db.Model(&models.Profile{}).Where("id = ?", pathProfile.ID).Updates(map[string]any{"name": pathProfile.Name, "profile_version": pathProfile.ProfileVersion}).Error; err != nil {
		t.Fatal(err)
	}
	candidateProfile := authoritativeTestProfile()
	candidateProfile.Name = "Candidate Override"
	candidateProfile.ProfileVersion = 4
	if err := fixture.db.Create(&candidateProfile).Error; err != nil {
		t.Fatal(err)
	}
	sourceProfile := authoritativeTestProfile()
	sourceProfile.Name, sourceProfile.ProfileVersion = "Source Group Profile B", 2
	if err := fixture.db.Create(&sourceProfile).Error; err != nil {
		t.Fatal(err)
	}
	sourceGroup := models.SourceGroup{Name: "Documentaries", RelativePath: "documentaries", SourcePath: documentariesPath, Enabled: true}
	if err := fixture.db.Create(&sourceGroup).Error; err != nil {
		t.Fatal(err)
	}
	asset.SourceGroupID = sourceGroup.ID
	if err := fixture.db.Save(&asset).Error; err != nil {
		t.Fatal(err)
	}
	assignments := []models.ProfileAssignment{
		{TargetType: assetScopeSourceGroup, TargetPath: sourceGroup.SourcePath, MediaType: "video", Selection: "profile", VideoProfileID: sourceProfile.ID},
		{TargetType: assetScopeLogicalGroup, TargetPath: asset.LogicalGroupPath, MediaType: "video", Selection: "profile", VideoProfileID: candidateProfile.ID},
		{TargetType: assetScopePath, TargetPath: assetGroupPath, MediaType: "video", Selection: "profile", VideoProfileID: pathProfile.ID},
	}
	if err := fixture.db.Create(&assignments).Error; err != nil {
		t.Fatal(err)
	}
	router := queuePreflightRouter(fixture.db)
	input := queuePreflightInput(fixture, "Path assignment override")
	input.Jobs[0].SourceAssetRecordID = &asset.ID
	input.Jobs[0].ResolveProfileAssignments = true
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.Assets[0].Plan == nil {
		t.Fatal("initial candidate has no frozen plan")
	}
	if got := queuePreflightAssignmentValue(candidate.Assets[0].Plan.ProfileResolution["video"]); got.Source != assetScopePath || got.ProfileID != pathProfile.ID || filepath.Clean(got.TargetPath) != filepath.Clean(assetGroupPath) {
		t.Fatalf("initial candidate plan did not retain the Path assignment: %#v", got)
	}

	edit := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/edit", map[string]any{"revision": candidate.Revision})
	if edit.Code != http.StatusOK {
		t.Fatalf("edit status=%d body=%s", edit.Code, edit.Body.String())
	}
	var editable QueueBatchPreflightResponse
	if err := json.Unmarshal(edit.Body.Bytes(), &editable); err != nil {
		t.Fatal(err)
	}
	updatedJob := editable.Assets[0].Input
	updatedJob.ProfileID = candidateProfile.ID
	update := QueueBatchPreflightInput{
		Revision: editable.Revision, BatchID: editable.BatchID, BatchName: editable.BatchName,
		Jobs: []QueueJobInput{updatedJob},
	}
	updated := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, update)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	responseAsset := candidate.Assets[0]
	if responseAsset.AssignmentReview == nil || !responseAsset.AssignmentReview.Required || len(responseAsset.AssignmentReview.Changes) != 1 {
		t.Fatalf("candidate override did not require reviewing the changed Path assignment: %#v", responseAsset.AssignmentReview)
	}
	change := responseAsset.AssignmentReview.Changes[0]
	wantBefore := QueueBatchPreflightAssignmentValue{
		Selection: "profile", Source: assetScopePath, TargetPath: assetGroupPath,
		ProfileID: pathProfile.ID, ProfileName: pathProfile.Name, ProfileVersion: pathProfile.ProfileVersion,
		ProcessingMode: ProcessingModeFullEncode, SelectedEncoder: "libx265",
	}
	wantAfter := QueueBatchPreflightAssignmentValue{
		Selection: "profile", Source: "queue_batch_preflight_edit", AssignmentOrigin: assetScopePath,
		TargetPath: assetGroupPath, ProfileID: candidateProfile.ID,
		ProfileName: candidateProfile.Name, ProfileVersion: candidateProfile.ProfileVersion,
		ProcessingMode: ProcessingModeFullEncode, SelectedEncoder: "libx265", CandidateOverride: true,
	}
	if change.Domain != "video" || !reflect.DeepEqual(change.Before, wantBefore) || !reflect.DeepEqual(change.After, wantAfter) {
		t.Fatalf("Path assignment review lost original or candidate-only provenance: got=%#v before=%#v after=%#v", change, change.Before, change.After)
	}
	if responseAsset.Plan == nil || responseAsset.Plan.Profile.ID != candidateProfile.ID || responseAsset.Plan.Profile.Name != candidateProfile.Name || responseAsset.Plan.ProfileVersion != candidateProfile.ProfileVersion {
		t.Fatalf("reviewed candidate did not freeze the explicit profile identity: %#v", responseAsset.Plan)
	}
}

func TestSelectedAssetPreflightKeepsPathProfileAndReviewsExplicitRefresh(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	oldPath := fixture.paths[0]
	sourceRoot := filepath.Dir(oldPath)
	documentariesPath := filepath.Join(sourceRoot, "documentaries")
	assetGroupPath := filepath.Join(documentariesPath, "Cobain - Montage of Heck")
	assetPath := filepath.Join(assetGroupPath, "Cobain - Montage of Heck.mkv")
	if err := os.MkdirAll(assetGroupPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldPath, assetPath); err != nil {
		t.Fatal(err)
	}
	var asset models.AssetRecord
	if err := fixture.db.Where("path = ?", oldPath).First(&asset).Error; err != nil {
		t.Fatal(err)
	}
	asset.Path, asset.SourcePath = assetPath, assetPath
	asset.RootPath, asset.GroupPath = sourceRoot, filepath.Join("documentaries", "Cobain - Montage of Heck")
	asset.LogicalGroupPath, asset.FileName = documentariesPath, filepath.Base(assetPath)
	if err := fixture.db.Save(&asset).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.ScanResult{}).Where("path = ?", oldPath).Updates(map[string]any{"path": assetPath, "file_name": filepath.Base(assetPath)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.AutoMigrate(&models.SourceGroup{}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "test", Encoders: models.JSONMap{"libx265": models.JSONMap{"listed": true, "usable": true}}}).Error; err != nil {
		t.Fatal(err)
	}

	pathProfile := fixture.profile
	pathProfile.Name, pathProfile.ProfileVersion = "Path Profile A", 3
	if err := fixture.db.Model(&models.Profile{}).Where("id = ?", pathProfile.ID).Updates(map[string]any{"name": pathProfile.Name, "profile_version": pathProfile.ProfileVersion}).Error; err != nil {
		t.Fatal(err)
	}
	sourceProfile := authoritativeTestProfile()
	sourceProfile.Name, sourceProfile.ProfileVersion = "Source Group Profile B", 2
	if err := fixture.db.Create(&sourceProfile).Error; err != nil {
		t.Fatal(err)
	}
	logicalProfile := authoritativeTestProfile()
	logicalProfile.Name, logicalProfile.ProfileVersion = "Logical Group Profile C", 4
	if err := fixture.db.Create(&logicalProfile).Error; err != nil {
		t.Fatal(err)
	}
	sourceGroup := models.SourceGroup{Name: "Documentaries", RelativePath: "documentaries", SourcePath: documentariesPath, Enabled: true}
	if err := fixture.db.Create(&sourceGroup).Error; err != nil {
		t.Fatal(err)
	}
	asset.SourceGroupID = sourceGroup.ID
	if err := fixture.db.Save(&asset).Error; err != nil {
		t.Fatal(err)
	}
	assignments := []models.ProfileAssignment{
		{TargetType: assetScopeSourceGroup, TargetPath: sourceGroup.SourcePath, MediaType: "video", Selection: "profile", VideoProfileID: sourceProfile.ID},
		{TargetType: assetScopeLogicalGroup, TargetPath: asset.LogicalGroupPath, MediaType: "video", Selection: "profile", VideoProfileID: logicalProfile.ID},
		{TargetType: assetScopePath, TargetPath: assetGroupPath, MediaType: "video", Selection: "profile", VideoProfileID: pathProfile.ID},
	}
	if err := fixture.db.Create(&assignments).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&models.AssetScopeConfiguration{
		ScopeType: assetScopeSourceGroup, ScopeKey: sourceGroup.SourcePath,
		DestinationSelection: configSelectionValue, DestinationLibraryID: fixture.library.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	assertPathProfile := func(stage string, result QueueBatchPreflightResponse, expectedID uint, expectedName string) {
		t.Helper()
		item := result.Assets[0]
		video, _ := item.Input.ProfileResolution["video"].(map[string]any)
		if item.Input.ProfileID != expectedID || video["source"] != assetScopePath || filepath.Clean(workerStringValue(video["targetPath"])) != filepath.Clean(assetGroupPath) {
			t.Fatalf("%s did not preserve the canonical Path assignment: input=%#v resolution=%#v", stage, item.Input, item.Input.ProfileResolution)
		}
		if item.Plan == nil || item.Plan.Profile.ID != expectedID || item.Plan.Profile.Name != expectedName {
			t.Fatalf("%s frozen plan profile=%#v want %s (%d), status=%s blockers=%v warnings=%v", stage, item.Plan, expectedName, expectedID, item.Status, item.Blockers, item.Warnings)
		}
	}

	router := queuePreflightRouter(fixture.db)
	selected := queuePreflightRequest(t, router, http.MethodPost, "/preflights/selected-assets", map[string]any{"assetIds": []uint{asset.ID}})
	if selected.Code != http.StatusCreated {
		t.Fatalf("selected-asset preflight status=%d body=%s", selected.Code, selected.Body.String())
	}
	var selectedCandidate QueueBatchPreflightResponse
	if err := json.Unmarshal(selected.Body.Bytes(), &selectedCandidate); err != nil {
		t.Fatal(err)
	}
	assertPathProfile("selected-assets route", selectedCandidate, pathProfile.ID, pathProfile.Name)
	discarded := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+selectedCandidate.ID+"/discard", map[string]any{"revision": selectedCandidate.Revision})
	if discarded.Code != http.StatusOK {
		t.Fatalf("discard selected-assets candidate status=%d body=%s", discarded.Code, discarded.Body.String())
	}
	jobInput := QueueJobInput{
		MediaPath: assetPath, SourceAssetRecordID: &asset.ID, LibraryID: fixture.library.ID,
		ProfileID: sourceProfile.ID, ProcessingMode: ProcessingModeFullEncode,
		Priority: 5, ResolveProfileAssignments: true,
	}
	directInput := QueueBatchPreflightInput{
		BatchID: "cobain-single-asset", BatchName: "Single asset: Cobain - Montage of Heck.mkv",
		Jobs: []QueueJobInput{jobInput},
	}
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", directInput)
	if created.Code != http.StatusCreated {
		t.Fatalf("single-asset preflight status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	assertPathProfile("single-asset creation", candidate, pathProfile.ID, pathProfile.Name)

	edit := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/edit", map[string]any{"revision": candidate.Revision})
	if edit.Code != http.StatusOK {
		t.Fatalf("edit status=%d body=%s", edit.Code, edit.Body.String())
	}
	var edited QueueBatchPreflightResponse
	if err := json.Unmarshal(edit.Body.Bytes(), &edited); err != nil {
		t.Fatal(err)
	}
	update := QueueBatchPreflightInput{Revision: edited.Revision, BatchID: edited.BatchID, BatchName: edited.BatchName, Jobs: []QueueJobInput{edited.Assets[0].Input}}
	resolved := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, update)
	if resolved.Code != http.StatusOK {
		t.Fatalf("unchanged assignment update status=%d body=%s", resolved.Code, resolved.Body.String())
	}
	if err := json.Unmarshal(resolved.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	assertPathProfile("unchanged edit", candidate, pathProfile.ID, pathProfile.Name)

	if err := fixture.db.Model(&models.ProfileAssignment{}).Where("target_type = ? AND target_path = ? AND media_type = ?", assetScopePath, assetGroupPath, "video").Update("video_profile_id", sourceProfile.ID).Error; err != nil {
		t.Fatal(err)
	}
	stale := queuePreflightRequest(t, router, http.MethodGet, "/preflights/"+candidate.ID, map[string]any{})
	if stale.Code != http.StatusOK {
		t.Fatalf("stale read status=%d body=%s", stale.Code, stale.Body.String())
	}
	if err := json.Unmarshal(stale.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets[0].AssignmentChanges) == 0 || candidate.Assets[0].AssignmentChanges[0].Reason != "assignment_changed" {
		t.Fatalf("changed path assignment was not surfaced: %#v", candidate.Assets[0].AssignmentChanges)
	}
	staleAccept := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{"revision": candidate.Revision, "assetIds": []string{candidate.Assets[0].ID}})
	if staleAccept.Code != http.StatusConflict {
		t.Fatalf("stale assignment candidate accepted: status=%d body=%s", staleAccept.Code, staleAccept.Body.String())
	}
	edit = queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/edit", map[string]any{"revision": candidate.Revision})
	if edit.Code != http.StatusOK {
		t.Fatalf("stale edit status=%d body=%s", edit.Code, edit.Body.String())
	}
	if err := json.Unmarshal(edit.Body.Bytes(), &edited); err != nil {
		t.Fatal(err)
	}
	update = QueueBatchPreflightInput{Revision: edited.Revision, BatchID: edited.BatchID, BatchName: edited.BatchName, ReResolveAssignments: true, Jobs: []QueueJobInput{edited.Assets[0].Input}}
	refreshed := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, update)
	if refreshed.Code != http.StatusOK {
		t.Fatalf("explicit re-resolve status=%d body=%s", refreshed.Code, refreshed.Body.String())
	}
	if err := json.Unmarshal(refreshed.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	assertPathProfile("explicit path refresh", candidate, sourceProfile.ID, sourceProfile.Name)
	if candidate.Assets[0].AssignmentReview == nil || !candidate.Assets[0].AssignmentReview.Required {
		t.Fatalf("path profile change did not require review: %#v", candidate.Assets[0].AssignmentReview)
	}
	if len(candidate.Assets[0].AssignmentReview.Changes) == 0 || candidate.Assets[0].AssignmentReview.Changes[0].Before.ProfileName != pathProfile.Name || candidate.Assets[0].AssignmentReview.Changes[0].After.ProfileName != sourceProfile.Name {
		t.Fatalf("before-to-after profile provenance was not retained: %#v", candidate.Assets[0].AssignmentReview.Changes)
	}
	edit = queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/edit", map[string]any{"revision": candidate.Revision})
	if edit.Code != http.StatusOK {
		t.Fatalf("explicit profile edit status=%d body=%s", edit.Code, edit.Body.String())
	}
	if err := json.Unmarshal(edit.Body.Bytes(), &edited); err != nil {
		t.Fatal(err)
	}
	explicitInput := edited.Assets[0].Input
	explicitInput.ProfileID = logicalProfile.ID
	update = QueueBatchPreflightInput{Revision: edited.Revision, BatchID: edited.BatchID, BatchName: edited.BatchName, Jobs: []QueueJobInput{explicitInput}}
	explicit := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, update)
	if explicit.Code != http.StatusOK {
		t.Fatalf("explicit profile update status=%d body=%s", explicit.Code, explicit.Body.String())
	}
	if err := json.Unmarshal(explicit.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	videoResolution := queuePreflightAssignmentValue(candidate.Assets[0].Input.ProfileResolution["video"])
	if candidate.Assets[0].Input.ProfileID != logicalProfile.ID || videoResolution.Source != "queue_batch_preflight_edit" || videoResolution.AssignmentOrigin != assetScopePath || candidate.Assets[0].Plan == nil || candidate.Assets[0].Plan.Profile.ID != logicalProfile.ID || candidate.Assets[0].Plan.Profile.Name != logicalProfile.Name {
		t.Fatalf("explicit candidate override did not retain its Path origin and frozen identity: input=%#v resolution=%#v plan=%#v", candidate.Assets[0].Input, videoResolution, candidate.Assets[0].Plan)
	}
	if candidate.Assets[0].AssignmentReview == nil || !candidate.Assets[0].AssignmentReview.Required || len(candidate.Assets[0].AssignmentReview.Changes) == 0 || candidate.Assets[0].AssignmentReview.Changes[0].Before.ProfileName != pathProfile.Name || candidate.Assets[0].AssignmentReview.Changes[0].After.ProfileName != logicalProfile.Name {
		t.Fatalf("explicit profile edit did not retain the reviewed before-and-after values: %#v", candidate.Assets[0].AssignmentReview)
	}
	acceptURL := "/preflights/" + candidate.ID + "/accept"
	withoutReview := queuePreflightRequest(t, router, http.MethodPost, acceptURL, map[string]any{"revision": candidate.Revision, "assetIds": []string{candidate.Assets[0].ID}})
	if withoutReview.Code != http.StatusConflict {
		t.Fatalf("unreviewed assignment change accepted: status=%d body=%s", withoutReview.Code, withoutReview.Body.String())
	}
	accepted := queuePreflightRequest(t, router, http.MethodPost, acceptURL, map[string]any{"revision": candidate.Revision, "assetIds": []string{candidate.Assets[0].ID}, "reviewedAssignmentChanges": true})
	if accepted.Code != http.StatusCreated {
		t.Fatalf("reviewed assignment change status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var acceptedCandidate QueueBatchPreflightResponse
	if err := json.Unmarshal(accepted.Body.Bytes(), &acceptedCandidate); err != nil {
		t.Fatal(err)
	}
	if len(acceptedCandidate.Jobs) != 1 {
		t.Fatalf("accepted jobs=%d want=1", len(acceptedCandidate.Jobs))
	}
	job := acceptedCandidate.Jobs[0]
	frozenProfile, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if job.ProfileID != logicalProfile.ID || frozenProfile.ID != logicalProfile.ID || frozenProfile.Name != logicalProfile.Name || queuePreflightAssignmentValue(job.ProfileResolution["video"]).Source != "queue_batch_preflight_edit" || queuePreflightAssignmentValue(job.ProfileResolution["video"]).AssignmentOrigin != assetScopePath {
		t.Fatalf("accepted Queue job diverged from displayed candidate override: job=%#v profile=%#v", job, frozenProfile)
	}
	if job.LibraryID != fixture.library.ID || job.ResolvedDestinationSnapshot["libraryName"] != fixture.library.Name {
		t.Fatalf("accepted destination differs from reviewed library: %#v", job.ResolvedDestinationSnapshot)
	}
	if job.ActiveExecutionPlanID == nil {
		t.Fatal("accepted Queue job did not freeze an ExecutionPlan")
	}
	var plan models.ExecutionPlan
	if err := fixture.db.First(&plan, *job.ActiveExecutionPlanID).Error; err != nil {
		t.Fatal(err)
	}
	if plan.LibraryID != job.LibraryID || plan.SelectedEncoder != candidate.Assets[0].Plan.SelectedEncoder || plan.ProfileVersion != job.ProfileVersion {
		t.Fatalf("accepted ExecutionPlan diverged from reviewed Queue job: plan=%#v job=%#v", plan, job)
	}
}

func TestBatchPreflightAllowsReadyOnlyAndRetainsBlockedAssets(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 2)
	input := queuePreflightInput(fixture, "Partial batch")
	input.Jobs[1].LibraryID = 99999
	router := queuePreflightRouter(fixture.db)
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	var readyID, blockedID string
	blockedCount := 0
	for _, asset := range candidate.Assets {
		if asset.Status == queuePreflightReady {
			readyID = asset.ID
		} else if asset.Status == queuePreflightBlocked {
			blockedID = asset.ID
			blockedCount++
		}
	}
	if readyID == "" || blockedCount != 1 {
		t.Fatalf("expected one ready and one blocked asset: %#v", candidate.Assets)
	}
	blockedAccept := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{"revision": candidate.Revision, "assetIds": []string{blockedID}})
	if blockedAccept.Code != http.StatusConflict {
		t.Fatalf("blocked asset was accepted: status=%d body=%s", blockedAccept.Code, blockedAccept.Body.String())
	}
	var queuedBeforeAcceptance int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&queuedBeforeAcceptance).Error; err != nil {
		t.Fatal(err)
	}
	if queuedBeforeAcceptance != 0 {
		t.Fatalf("blocked acceptance created %d Queue jobs", queuedBeforeAcceptance)
	}
	accepted := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{"revision": candidate.Revision, "assetIds": []string{readyID}})
	if accepted.Code != http.StatusCreated {
		t.Fatalf("partial accept status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var partial QueueBatchPreflightResponse
	if err := json.Unmarshal(accepted.Body.Bytes(), &partial); err != nil {
		t.Fatal(err)
	}
	queued, blocked := 0, 0
	for _, asset := range partial.Assets {
		if asset.Status == queuePreflightQueued {
			queued++
		}
		if asset.Status == queuePreflightBlocked {
			blocked++
		}
	}
	if partial.Status != models.QueueBatchPreflightStatusActive || queued != 1 || blocked != 1 {
		t.Fatalf("partial acceptance did not retain unresolved batch context: %#v", partial.Assets)
	}
}

func TestBatchPreflightSurfacesMissingProfileAndLibraryWithoutReplacement(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	router := queuePreflightRouter(fixture.db)
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", queuePreflightInput(fixture, "Missing references"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Delete(&models.Profile{}, fixture.profile.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Delete(&models.Library{}, fixture.library.ID).Error; err != nil {
		t.Fatal(err)
	}
	read := queuePreflightRequest(t, router, http.MethodGet, "/preflights/"+candidate.ID, map[string]any{})
	if read.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}
	if err := json.Unmarshal(read.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	missingProfile, missingLibrary := false, false
	for _, change := range candidate.Assets[0].AssignmentChanges {
		missingProfile = missingProfile || change.Domain == "video" && change.Reason == "profile_unavailable"
		missingLibrary = missingLibrary || change.Domain == "destination" && change.Reason == "library_unavailable"
	}
	if !missingProfile || !missingLibrary {
		t.Fatalf("missing candidate references were not retained and reported: %#v", candidate.Assets[0].AssignmentChanges)
	}
	accepted := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{"revision": candidate.Revision, "assetIds": []string{candidate.Assets[0].ID}})
	if accepted.Code != http.StatusConflict {
		t.Fatalf("candidate with unavailable references was accepted or replaced: status=%d body=%s", accepted.Code, accepted.Body.String())
	}
}

func TestBatchPreflightRequiresReviewForJob2191AmbiguousIVTC(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	profile := fixture.profile
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	profile.WorkerConfig["fieldStructureMode"] = "preserve"
	profile.WorkerConfig["hevcLevelMode"] = "auto"
	profile.WorkerConfig["hevcLevelEffective"] = "4.0"
	profile.WorkerConfig["hevcLevelTier"] = "main"
	profile.WorkerConfig["hevcLevelResolutionState"] = hevcLevelResolutionResolved
	profile.WorkerConfig["hevcLevelInputFingerprint"] = "historical-fingerprint"
	if err := fixture.db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var scan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	ambiguous := job2191AmbiguousIVTCScanResult(t, fixture.paths[0])
	ambiguous.ID = scan.ID
	if err := fixture.db.Save(&ambiguous).Error; err != nil {
		t.Fatal(err)
	}

	input := queuePreflightInput(fixture, "Unresolved HEVC")
	created := queuePreflightRequest(t, queuePreflightRouter(fixture.db), http.MethodPost, "/preflights", input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightRequiresReview || candidate.Assets[0].ReviewCode != "ivtc_field_order_required" {
		if len(candidate.Assets) != 1 {
			t.Fatalf("preflight returned %d assets, want one", len(candidate.Assets))
		}
		asset := candidate.Assets[0]
		t.Fatalf("ambiguous IVTC should require review before Queue acceptance: status=%q reviewCode=%q blockers=%v", asset.Status, asset.ReviewCode, asset.Blockers)
	}
	blocker := strings.Join(candidate.Assets[0].Blockers, " ")
	warning := strings.Join(candidate.Assets[0].Warnings, " ")
	if !strings.Contains(blocker, "explicit or validated field order") || !strings.Contains(warning, "effective IVTC filters") || !strings.Contains(warning, "output FPS") || !strings.Contains(warning, "HEVC Level/Tier") {
		t.Fatalf("preflight blocker omitted the actionable IVTC cause: %q", blocker)
	}
	if candidate.Assets[0].Plan != nil {
		t.Fatalf("review-required candidate retained a queueable plan: %#v", candidate.Assets[0].Plan)
	}
	review := candidate.Assets[0].CadenceReview
	if review == nil || review.CadenceType != "hard_telecine" || review.DeclaredFrameRate != "30000/1001" || review.RecommendedOperation != "inverse_telecine" || review.RecommendedFrameRate != "24000/1001" || review.ReportedFieldOrder != "tt" || review.ValidatedSelectedOrder != "" || review.ValidatedWindows != 1 || review.ObservedWindows != 3 || !review.EffectiveOutputFrameRateUnknown || review.HEVCLevelResolutionState != hevcLevelResolutionUnresolved || review.HEVCLevelEffective != "" || review.HEVCLevelTier != "" {
		t.Fatalf("review details do not describe #2191 without treating reported field order as validated: %#v", review)
	}
	var jobs, plans, reservations int64
	for name, query := range map[string]func() *gorm.DB{
		"jobs":         func() *gorm.DB { return fixture.db.Model(&models.QueueJob{}) },
		"plans":        func() *gorm.DB { return fixture.db.Model(&models.ExecutionPlan{}) },
		"reservations": func() *gorm.DB { return fixture.db.Model(&models.SchedulerReservation{}) },
	} {
		var count int64
		if err := query().Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		switch name {
		case "jobs":
			jobs = count
		case "plans":
			plans = count
		case "reservations":
			reservations = count
		}
	}
	if jobs != 0 || plans != 0 || reservations != 0 {
		t.Fatalf("review-required Preflight persisted Queue state: jobs=%d plans=%d reservations=%d", jobs, plans, reservations)
	}
}

func TestSingleQueueCreateRequiresReviewForJob2191AmbiguousIVTC(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	profile := fixture.profile
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	profile.WorkerConfig["fieldStructureMode"] = "preserve"
	profile.WorkerConfig["hevcLevelMode"] = "auto"
	profile.WorkerConfig["hevcLevelEffective"] = "4.0"
	profile.WorkerConfig["hevcLevelTier"] = "main"
	profile.WorkerConfig["hevcLevelResolutionState"] = hevcLevelResolutionResolved
	profile.WorkerConfig["hevcLevelInputFingerprint"] = "historical-fingerprint"
	if err := fixture.db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var scan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	ambiguous := job2191AmbiguousIVTCScanResult(t, fixture.paths[0])
	ambiguous.ID = scan.ID
	if err := fixture.db.Save(&ambiguous).Error; err != nil {
		t.Fatal(err)
	}

	response := queuePreflightRequest(t, queuePreflightRouter(fixture.db), http.MethodPost, "/queue/jobs", QueueJobInput{
		MediaPath: fixture.paths[0], LibraryID: fixture.library.ID, ProfileID: fixture.profile.ID,
		ProcessingMode: ProcessingModeFullEncode, Priority: 5,
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("single Queue Create status=%d want conflict; body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "REVIEW_REQUIRED" || body["reasonCode"] != "ivtc_field_order_required" || !strings.Contains(stringFromUnknown(body["error"]), "explicit or validated field order") {
		t.Fatalf("single Queue Create omitted actionable REVIEW_REQUIRED response: %#v", body)
	}
	missingEvidence, _ := body["missingEvidence"].([]any)
	if len(missingEvidence) != 1 || missingEvidence[0] != "explicit or validated IVTC field order" {
		t.Fatalf("single Queue Create missingEvidence=%#v", body["missingEvidence"])
	}
	var jobs, plans, reservations int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.ExecutionPlan{}).Count(&plans).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.SchedulerReservation{}).Count(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if jobs != 0 || plans != 0 || reservations != 0 {
		t.Fatalf("REVIEW_REQUIRED request persisted Queue state: jobs=%d plans=%d reservations=%d", jobs, plans, reservations)
	}

	input := QueueJobInput{
		MediaPath: fixture.paths[0], LibraryID: fixture.library.ID, ProfileID: fixture.profile.ID,
		ProcessingMode: ProcessingModeFullEncode, Priority: 5, CadenceFieldOrder: "tff",
	}
	explicit := queuePreflightRequest(t, queuePreflightRouter(fixture.db), http.MethodPost, "/queue/jobs", input)
	var explicitBody map[string]any
	if err := json.Unmarshal(explicit.Body.Bytes(), &explicitBody); err != nil {
		t.Fatal(err)
	}
	if explicit.Code != http.StatusConflict || explicitBody["status"] != "PREFLIGHT_REQUIRED" || explicitBody["code"] != "single_queue_preflight_required" {
		t.Fatalf("explicit order bypassed reviewed Queue Preflight: status=%d body=%s", explicit.Code, explicit.Body.String())
	}
}

func TestSingleQueuePreflightReviewsExplicitJob2191IVTCPlanBeforeAccept(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	profile := fixture.profile
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	profile.WorkerConfig["fieldStructureMode"] = "preserve"
	profile.WorkerConfig["hevcLevelMode"] = "auto"
	profile.WorkerConfig["hevcLevelEffective"] = "4.0"
	profile.WorkerConfig["hevcLevelTier"] = "main"
	profile.WorkerConfig["hevcLevelResolutionState"] = hevcLevelResolutionResolved
	profile.WorkerConfig["hevcLevelInputFingerprint"] = "historical-fingerprint"
	if err := fixture.db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var scan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	ambiguous := job2191AmbiguousIVTCScanResult(t, fixture.paths[0])
	ambiguous.ID = scan.ID
	if err := fixture.db.Save(&ambiguous).Error; err != nil {
		t.Fatal(err)
	}
	router := queuePreflightRouter(fixture.db)
	input := queuePreflightInput(fixture, "Sin Limite single asset")
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightRequiresReview {
		t.Fatalf("ambiguous candidate status=%#v", candidate.Assets)
	}
	input.Jobs[0].CadenceFieldOrder = "tff"
	updated := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, QueueBatchPreflightInput{
		BatchID: candidate.BatchID, BatchName: candidate.BatchName, Revision: candidate.Revision, Jobs: input.Jobs,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("explicit TFF resolution status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightReady || candidate.Assets[0].Plan == nil || candidate.Assets[0].Input.CadenceFieldOrder != "tff" {
		t.Fatalf("explicit TFF candidate did not resolve to a reviewable ready plan: %#v", candidate.Assets)
	}
	reviewedSnapshot := queuePreflightCloneMap(candidate.Assets[0].Plan.ProfileSnapshot)
	frozenProfile, err := scheduler.RestoreProfileSnapshot(reviewedSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if got := workerStringValue(frozenProfile.WorkerConfig["effectiveCadenceFieldOrder"]); got != "tff" {
		t.Fatalf("frozen effective field order=%q want tff", got)
	}
	if got := workerStringValue(frozenProfile.WorkerConfig["effectiveOutputFrameRate"]); got != "24000/1001" || profileWorkerBool(frozenProfile, "effectiveOutputFrameRateUnknown", false) {
		t.Fatalf("frozen cadence output rate=%q unknown=%v", got, frozenProfile.WorkerConfig["effectiveOutputFrameRateUnknown"])
	}
	filters := workerStringValue(frozenProfile.WorkerConfig["videoFilters"])
	if !strings.Contains(filters, "fieldmatch=order=tff") || !strings.Contains(filters, "decimate") {
		t.Fatalf("reviewed filters do not show the selected TFF IVTC: %q", filters)
	}
	if got := workerStringValue(frozenProfile.WorkerConfig["hevcLevelResolutionState"]); got != hevcLevelResolutionResolved {
		t.Fatalf("HEVC decision was not recalculated after cadence selection: state=%q config=%#v", got, frozenProfile.WorkerConfig)
	}
	if got := workerStringValue(frozenProfile.WorkerConfig["hevcLevelInputFingerprint"]); got == "" || got == "historical-fingerprint" {
		t.Fatalf("stale HEVC fingerprint survived candidate resolution: %q", got)
	}
	if err := fixture.db.First(&profile, fixture.profile.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got := workerStringValue(profile.WorkerConfig["cadenceFieldOrder"]); got != "auto" {
		t.Fatalf("candidate order leaked into the saved profile: %q", got)
	}

	accepted := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{
		"revision": candidate.Revision, "assetIds": []string{candidate.Assets[0].ID}, "reviewedAssignmentChanges": true,
	})
	if accepted.Code != http.StatusCreated {
		t.Fatalf("accept status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var acceptedCandidate QueueBatchPreflightResponse
	if err := json.Unmarshal(accepted.Body.Bytes(), &acceptedCandidate); err != nil {
		t.Fatal(err)
	}
	if len(acceptedCandidate.Jobs) != 1 || acceptedCandidate.Jobs[0].ActiveExecutionPlanID == nil {
		t.Fatalf("accepted candidate did not bind Queue job to an ExecutionPlan: %#v", acceptedCandidate.Jobs)
	}
	job := acceptedCandidate.Jobs[0]
	if !reflect.DeepEqual(job.ProfileSnapshot, reviewedSnapshot) {
		t.Fatal("accepted Queue snapshot differs from the reviewed TFF plan")
	}
	var executionPlan models.ExecutionPlan
	if err := fixture.db.First(&executionPlan, *job.ActiveExecutionPlanID).Error; err != nil {
		t.Fatal(err)
	}
	if executionPlan.JobID != job.ID || executionPlan.ProfileVersion != job.ProfileVersion {
		t.Fatalf("ExecutionPlan binding differs from accepted Queue job: plan=%#v job=%#v", executionPlan, job)
	}
	if len(executionPlan.Constraints) == 0 {
		t.Fatal("ExecutionPlan omitted constraints resolved from the frozen candidate")
	}
}

func TestSingleQueuePreflightKeepsJob2191DeepRatiosReviewRequiredWithoutQueueState(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	profile := fixture.profile
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	profile.WorkerConfig["fieldStructureMode"] = "preserve"
	profile.WorkerConfig["hevcLevelMode"] = "auto"
	profile.WorkerConfig["hevcLevelEffective"] = "4.0"
	profile.WorkerConfig["hevcLevelTier"] = "main"
	profile.WorkerConfig["hevcLevelResolutionState"] = hevcLevelResolutionResolved
	profile.WorkerConfig["hevcLevelInputFingerprint"] = "job-2191-historical-level"
	if err := fixture.db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var scan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	job2191 := job2191AmbiguousIVTCScanResult(t, fixture.paths[0])
	job2191.ID = scan.ID
	info, err := os.Stat(fixture.paths[0])
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ivtcSourceIdentityForSnapshot(fixture.db, scan, fixture.paths[0], info)
	if err != nil {
		t.Fatal(err)
	}
	var interlace InterlaceAnalysis
	if !decodeJSONValue(job2191.InterlaceAnalysis, &interlace) || interlace.IVTCValidation == nil {
		t.Fatal("could not decode the synthetic Job #2191 evidence")
	}
	validation := interlace.IVTCValidation
	validation.DeepAnalysisVersion = ivtcDeepEvidenceVersion
	validation.DeepAnalysisStatus = "review_required"
	validation.SourceIdentity = identity
	validation.SourceFrameRate = "30000/1001"
	validation.ExpectedOutputFrameRate = "24000/1001"
	validation.DurationSeconds = 6287.072
	validation.WindowSeconds = 5
	validation.BaselineWindowCount = 3
	validation.AttemptedWindows = 5
	validation.InconclusiveWindows = 4
	validation.ContradictoryWindows = 0
	validation.ValidatedWindows = 1
	validation.ValidWindows = 1
	validation.SelectedOrder = ""
	validation.Windows = append(validation.Windows,
		IVTCWindowValidation{Start: 1687.509, TFFProgressive: 49, TFFClassified: 123, TFFResidualInterlaced: 74,
			BFFProgressive: 5, BFFClassified: 123, BFFResidualInterlaced: 118,
			TFFProgressiveRatio: 49.0 / 123, BFFProgressiveRatio: 5.0 / 123, Outcome: ivtcWindowOutcomeInconclusive},
		IVTCWindowValidation{Start: 4579.563, TFFProgressive: 18, TFFClassified: 122, TFFResidualInterlaced: 104,
			BFFProgressive: 4, BFFClassified: 122, BFFResidualInterlaced: 118,
			TFFProgressiveRatio: 18.0 / 122, BFFProgressiveRatio: 4.0 / 122, Outcome: ivtcWindowOutcomeInconclusive},
	)
	updatedAt := time.Now().UTC()
	validation.EvidenceSnapshotUpdatedAt = updatedAt.Format(time.RFC3339Nano)
	validation.SourceIdentity.SnapshotUpdatedAt = validation.EvidenceSnapshotUpdatedAt
	interlace.Status = "telecine_suspected"
	interlace.DetectedFieldOrder = ""
	interlace.FieldOrderMismatch = false
	interlace.RecommendedMode = ""
	interlace.RecommendedAction = "review"
	interlace.RecommendedFilter = ""
	interlace.AutomaticFilter = ""
	interlace.DecisionReason = "deep IVTC evidence remained insufficient"
	encoded, err := json.Marshal(interlace)
	if err != nil {
		t.Fatal(err)
	}
	var interlaceValue models.JSONMap
	if err := json.Unmarshal(encoded, &interlaceValue); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.ScanResult{}).Where("id = ?", scan.ID).Updates(map[string]any{
		"interlace_analysis":     interlaceValue,
		"cadence_analysis":       job2191.CadenceAnalysis,
		"cadence_recommendation": job2191.CadenceRecommendation,
		"updated_at":             updatedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	var current models.ScanResult
	if err := fixture.db.First(&current, scan.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !deepCadenceIdentityCurrent(fixture.db, current, fixture.paths[0], info, identity) {
		t.Fatal("synthetic five-window evidence is not current for the local fixture")
	}

	router := queuePreflightRouter(fixture.db)
	input := queuePreflightInput(fixture, "Sin Limite #2191 cadence evidence")
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightRequiresReview || candidate.Assets[0].Plan != nil {
		t.Fatalf("historical ratios must stay REVIEW_REQUIRED without a plan: %#v", candidate.Assets)
	}
	if review := candidate.Assets[0].CadenceReview; review == nil || review.ValidatedSelectedOrder != "" || review.ValidWindows != 1 || review.InconclusiveWindows != 4 {
		t.Fatalf("historical window outcomes were not retained in Queue review: %#v", review)
	}
	accepted := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{
		"revision": candidate.Revision, "assetIds": []string{candidate.Assets[0].ID},
	})
	if accepted.Code == http.StatusCreated {
		t.Fatalf("review-required evidence unexpectedly accepted: %s", accepted.Body.String())
	}
	var jobs, plans, reservations int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.ExecutionPlan{}).Count(&plans).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.SchedulerReservation{}).Count(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if jobs != 0 || plans != 0 || reservations != 0 {
		t.Fatalf("unvalidated historical cadence created Queue state: jobs=%d plans=%d reservations=%d", jobs, plans, reservations)
	}
}

func TestSingleQueuePreflightAcceptsValidatedJob2191IVTCOrder(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	profile := fixture.profile
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	profile.WorkerConfig["fieldStructureMode"] = "preserve"
	profile.WorkerConfig["hevcLevelMode"] = "auto"
	profile.WorkerConfig["hevcLevelEffective"] = "5.1"
	profile.WorkerConfig["hevcLevelTier"] = "high"
	profile.WorkerConfig["hevcLevelResolutionState"] = hevcLevelResolutionResolved
	profile.WorkerConfig["hevcLevelInputFingerprint"] = "historical-unresolved-rate"
	if err := fixture.db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var scan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	validated := job2191AmbiguousIVTCScanResult(t, fixture.paths[0])
	validated.ID = scan.ID
	info, err := os.Stat(fixture.paths[0])
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ivtcSourceIdentityForSnapshot(fixture.db, scan, fixture.paths[0], info)
	if err != nil {
		t.Fatal(err)
	}
	var interlace InterlaceAnalysis
	encoded, err := json.Marshal(validated.InterlaceAnalysis)
	if err != nil || json.Unmarshal(encoded, &interlace) != nil {
		t.Fatalf("decode synthetic IVTC evidence: %v", err)
	}
	interlace.Status = "telecine"
	interlace.FieldOrder = "tff"
	interlace.DetectedFieldOrder = "tff"
	interlace.RecommendedMode = "ivtc_tff"
	interlace.RecommendedAction = "ivtc"
	interlace.IVTCValidation.DeepAnalysisVersion = ivtcDeepEvidenceVersion
	interlace.IVTCValidation.DeepAnalysisStatus = "resolved"
	interlace.IVTCValidation.SourceIdentity = identity
	evidenceUpdatedAt := time.Now().UTC()
	interlace.IVTCValidation.EvidenceSnapshotUpdatedAt = evidenceUpdatedAt.Format(time.RFC3339Nano)
	interlace.IVTCValidation.SourceFrameRate = "30000/1001"
	interlace.IVTCValidation.ExpectedOutputFrameRate = "24000/1001"
	interlace.IVTCValidation.DurationSeconds = 90
	interlace.IVTCValidation.WindowSeconds = 5
	interlace.IVTCValidation.BaselineWindowCount = 3
	interlace.IVTCValidation.AttemptedWindows = 5
	interlace.IVTCValidation.InconclusiveWindows = 2
	interlace.IVTCValidation.ValidatedWindows = 3
	interlace.IVTCValidation.ValidWindows = 3
	interlace.IVTCValidation.SelectedOrder = "tff"
	interlace.IVTCValidation.Windows[0].SelectedOrder = "tff"
	interlace.IVTCValidation.Windows[0].Outcome = ivtcWindowOutcomeValid
	interlace.IVTCValidation.Windows[0].Confidence = .96
	interlace.IVTCValidation.Windows[1].Outcome = ivtcWindowOutcomeInconclusive
	interlace.IVTCValidation.Windows[2].Outcome = ivtcWindowOutcomeInconclusive
	interlace.IVTCValidation.Windows = append(interlace.IVTCValidation.Windows,
		IVTCWindowValidation{Start: 15, SelectedOrder: "tff", Outcome: ivtcWindowOutcomeValid, Confidence: .96},
		IVTCWindowValidation{Start: 45, SelectedOrder: "tff", Outcome: ivtcWindowOutcomeValid, Confidence: .96},
	)
	for index := range interlace.IVTCValidation.Windows {
		window := &interlace.IVTCValidation.Windows[index]
		window.DiagnosticsAvailable = true
		window.TFFCanonicalOutputFrames, window.TFFCanonicalOutputProgressive = 120, 120
		window.TFFCanonicalOutputFrameRate = 24000.0 / 1001.0
		window.TFFCanonicalIDETClassified = 120
		window.BFFCanonicalOutputFrames, window.BFFCanonicalOutputProgressive = 120, 120
		window.BFFCanonicalOutputFrameRate = 24000.0 / 1001.0
		window.BFFCanonicalIDETClassified = 120
		if window.Outcome == ivtcWindowOutcomeValid {
			window.TFFCanonicalIDETInterlaced = 3
			window.BFFCanonicalIDETInterlaced = 80
		}
	}
	for _, window := range interlace.IVTCValidation.Windows {
		if window.Outcome == ivtcWindowOutcomeValid && window.SelectedOrder != "tff" {
			t.Fatalf("deep fixture has conflicting IVTC evidence: %#v", window)
		}
	}
	interlaceBytes, err := json.Marshal(interlace)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(interlaceBytes, &validated.InterlaceAnalysis); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.ScanResult{}).Where("id = ?", validated.ID).Updates(map[string]any{
		"interlace_analysis":     validated.InterlaceAnalysis,
		"cadence_analysis":       validated.CadenceAnalysis,
		"cadence_recommendation": validated.CadenceRecommendation,
		"updated_at":             evidenceUpdatedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	var persistedDeep models.ScanResult
	if err := fixture.db.Where("id = ?", validated.ID).First(&persistedDeep).Error; err != nil {
		t.Fatal(err)
	}
	var persistedInterlace InterlaceAnalysis
	if !decodeJSONValue(persistedDeep.InterlaceAnalysis, &persistedInterlace) || !deepCadenceIdentityCurrent(fixture.db, persistedDeep, fixture.paths[0], info, persistedInterlace.IVTCValidation.SourceIdentity) || persistedInterlace.IVTCValidation.EvidenceSnapshotUpdatedAt != persistedDeep.UpdatedAt.UTC().Format(time.RFC3339Nano) {
		device, inode, stable := filesystemDeviceInode(info)
		identityCurrent := deepCadenceIdentityCurrent(fixture.db, persistedDeep, fixture.paths[0], info, persistedInterlace.IVTCValidation.SourceIdentity)
		identityInfoMatches := ivtcSourceIdentityMatchesInfo(persistedInterlace.IVTCValidation.SourceIdentity, fixture.paths[0], info, persistedDeep.ID)
		var asset models.AssetRecord
		assetErr := fixture.db.Where("path = ?", filepath.Clean(fixture.paths[0])).First(&asset).Error
		t.Fatalf("synthetic deep evidence identity fence is invalid: identityCurrent=%v infoMatches=%v table=%v assetErr=%v evidenceUpdatedAt=%q snapshotUpdatedAt=%q scanId=%d identity=%#v stat=(%d,%d,%v,size=%d,mode=%d) assetId=%d", identityCurrent, identityInfoMatches, fixture.db.Migrator().HasTable(&models.AssetRecord{}), assetErr, persistedInterlace.IVTCValidation.EvidenceSnapshotUpdatedAt, persistedDeep.UpdatedAt.UTC().Format(time.RFC3339Nano), persistedDeep.ID, persistedInterlace.IVTCValidation.SourceIdentity, device, inode, stable, info.Size(), uint32(info.Mode()), asset.ID)
	}
	created := queuePreflightRequest(t, queuePreflightRouter(fixture.db), http.MethodPost, "/preflights", queuePreflightInput(fixture, "validated IVTC"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightReady || candidate.Assets[0].Plan == nil {
		t.Fatalf("validated IVTC order should resolve without manual review: assets=%#v cadenceReview=%#v", candidate.Assets, candidate.Assets[0].CadenceReview)
	}
	if review := candidate.Assets[0].CadenceReview; review == nil || review.DeepAnalysisVersion != ivtcDeepEvidenceVersion || review.DeepAnalysisStatus != "resolved" || review.SourceFrameRate != "30000/1001" || review.ExpectedOutputFrameRate != "24000/1001" || review.ValidatedWindows != 3 || review.AttemptedWindows != 5 {
		t.Fatalf("resolved cadence evidence was not exposed for review: %#v", review)
	}
	if review := candidate.Assets[0].CadenceReview; review == nil || len(review.Windows) != 5 || !review.Windows[0].DiagnosticsAvailable || review.Windows[0].TFFCanonicalIDETInterlaced != 3 || review.Windows[0].TFFCanonicalIDETClassified != 120 || review.Windows[0].TFFCanonicalOutputFrames != 120 {
		t.Fatalf("canonical residual and timeline diagnostics were omitted from the Queue review contract: %#v", review)
	}
	profile, err = scheduler.RestoreProfileSnapshot(candidate.Assets[0].Plan.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if got := workerStringValue(profile.WorkerConfig["effectiveCadenceFieldOrder"]); got != "tff" {
		t.Fatalf("validated order was not frozen: %q", got)
	}
	if got := workerStringValue(profile.WorkerConfig["effectiveOutputFrameRate"]); got != "24000/1001" || profileWorkerBool(profile, "effectiveOutputFrameRateUnknown", false) {
		t.Fatalf("deep IVTC did not resolve the canonical output FPS: rate=%q unknown=%v", got, profileWorkerBool(profile, "effectiveOutputFrameRateUnknown", false))
	}
	if got := workerStringValue(profile.WorkerConfig["hevcLevelResolutionState"]); got != hevcLevelResolutionResolved {
		t.Fatalf("HEVC Level/Tier remained unresolved after deep cadence resolution: %q", got)
	}
	if got := workerStringValue(profile.WorkerConfig["hevcLevelInputFingerprint"]); got == "historical-unresolved-rate" || got == "" {
		t.Fatalf("HEVC Level/Tier reused a stale input fingerprint after FPS resolution: %q", got)
	}
	accepted := queuePreflightRequest(t, queuePreflightRouter(fixture.db), http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{
		"revision": candidate.Revision, "assetIds": []string{candidate.Assets[0].ID},
	})
	if accepted.Code != http.StatusCreated {
		t.Fatalf("accept current deep cadence evidence status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var queued int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&queued).Error; err != nil || queued != 1 {
		t.Fatalf("deep cadence acceptance did not create exactly one Queue intent: count=%d err=%v", queued, err)
	}
}

func TestSingleQueuePreflightRejectsSourceChangeBeforeAccept(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	profile := fixture.profile
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	profile.WorkerConfig["fieldStructureMode"] = "preserve"
	if err := fixture.db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var scan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	ambiguous := job2191AmbiguousIVTCScanResult(t, fixture.paths[0])
	ambiguous.ID = scan.ID
	if err := fixture.db.Save(&ambiguous).Error; err != nil {
		t.Fatal(err)
	}
	router := queuePreflightRouter(fixture.db)
	input := queuePreflightInput(fixture, "stale single asset")
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	input.Jobs[0].CadenceFieldOrder = "tff"
	updated := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, QueueBatchPreflightInput{
		BatchID: candidate.BatchID, BatchName: candidate.BatchName, Revision: candidate.Revision, Jobs: input.Jobs,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("explicit TFF resolution status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.Assets[0].Status != queuePreflightReady {
		t.Fatalf("expected ready candidate before source mutation: %#v", candidate.Assets[0])
	}
	if err := os.WriteFile(fixture.paths[0], []byte("changed source identity after review"), 0o600); err != nil {
		t.Fatal(err)
	}
	accepted := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{
		"revision": candidate.Revision, "assetIds": []string{candidate.Assets[0].ID},
	})
	if accepted.Code != http.StatusConflict {
		t.Fatalf("candidate accepted after source revision changed: status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var jobs, plans, reservations int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.ExecutionPlan{}).Count(&plans).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.SchedulerReservation{}).Count(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if jobs != 0 || plans != 0 || reservations != 0 {
		t.Fatalf("stale candidate acceptance persisted Queue state: jobs=%d plans=%d reservations=%d", jobs, plans, reservations)
	}
}

func TestSingleQueuePreflightPreserveCadenceUsesKnownSourceRateAndResolvesHEVC(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	profile := fixture.profile
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	profile.WorkerConfig["fieldStructureMode"] = "preserve"
	profile.WorkerConfig["hevcLevelMode"] = "auto"
	profile.WorkerConfig["hevcLevelEffective"] = "4.0"
	profile.WorkerConfig["hevcLevelTier"] = "main"
	profile.WorkerConfig["hevcLevelResolutionState"] = hevcLevelResolutionResolved
	profile.WorkerConfig["hevcLevelInputFingerprint"] = "historical-fingerprint"
	if err := fixture.db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var scan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	ambiguous := job2191AmbiguousIVTCScanResult(t, fixture.paths[0])
	ambiguous.ID = scan.ID
	if err := fixture.db.Save(&ambiguous).Error; err != nil {
		t.Fatal(err)
	}
	router := queuePreflightRouter(fixture.db)
	input := queuePreflightInput(fixture, "Preserve cadence review")
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	input.Jobs[0].CadenceModeOverride = "preserve"
	input.Jobs[0].CadenceFieldOrder = "auto"
	updated := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, QueueBatchPreflightInput{
		BatchID: candidate.BatchID, BatchName: candidate.BatchName, Revision: candidate.Revision, Jobs: input.Jobs,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("preserve cadence resolution status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightReady || candidate.Assets[0].Plan == nil {
		t.Fatalf("preserve cadence with known source rate should resolve a reviewable plan: %#v", candidate.Assets)
	}
	resolved, err := scheduler.RestoreProfileSnapshot(candidate.Assets[0].Plan.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if workerStringValue(resolved.WorkerConfig["cadenceMode"]) != "preserve" || workerStringValue(resolved.WorkerConfig["hevcLevelResolutionState"]) != hevcLevelResolutionResolved || profileWorkerBool(resolved, "effectiveOutputFrameRateUnknown", false) {
		t.Fatalf("known source cadence did not resolve to a safe preserve/HEVC plan: %#v", resolved.WorkerConfig)
	}
	if strings.Contains(workerStringValue(resolved.WorkerConfig["videoFilters"]), "fieldmatch=") || strings.Contains(workerStringValue(resolved.WorkerConfig["videoFilters"]), "decimate") {
		t.Fatalf("preserve cadence unexpectedly planned IVTC filters: %q", resolved.WorkerConfig["videoFilters"])
	}
	if got := workerStringValue(resolved.WorkerConfig["hevcLevelInputFingerprint"]); got == "" || got == "historical-fingerprint" {
		t.Fatalf("stale HEVC fingerprint survived preserve resolution: %q", got)
	}
}

func TestSingleQueuePreflightPreserveCadenceWithUnknownFPSRemainsReviewRequired(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	profile := fixture.profile
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	profile.WorkerConfig["fieldStructureMode"] = "preserve"
	profile.WorkerConfig["videoFilters"] = "fps=fps*2"
	profile.WorkerConfig["hevcLevelMode"] = "auto"
	profile.WorkerConfig["hevcLevelEffective"] = "4.0"
	profile.WorkerConfig["hevcLevelTier"] = "main"
	profile.WorkerConfig["hevcLevelResolutionState"] = hevcLevelResolutionResolved
	profile.WorkerConfig["hevcLevelInputFingerprint"] = "historical-fingerprint"
	if err := fixture.db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var scan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	ambiguous := job2191AmbiguousIVTCScanResult(t, fixture.paths[0])
	ambiguous.ID = scan.ID
	if err := fixture.db.Save(&ambiguous).Error; err != nil {
		t.Fatal(err)
	}
	router := queuePreflightRouter(fixture.db)
	input := queuePreflightInput(fixture, "Preserve cadence unknown FPS")
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	input.Jobs[0].CadenceModeOverride = "preserve"
	input.Jobs[0].CadenceFieldOrder = "auto"
	updated := queuePreflightRequest(t, router, http.MethodPut, "/preflights/"+candidate.ID, QueueBatchPreflightInput{
		BatchID: candidate.BatchID, BatchName: candidate.BatchName, Revision: candidate.Revision, Jobs: input.Jobs,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("preserve cadence resolution status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightRequiresReview || candidate.Assets[0].ReviewCode != "cadence_output_fps_unknown" || candidate.Assets[0].Plan != nil {
		t.Fatalf("unknown preserve cadence did not remain review-required: %#v", candidate.Assets)
	}
	blockers := strings.Join(candidate.Assets[0].Blockers, " ")
	if !strings.Contains(blockers, "effective output FPS is unknown") {
		t.Fatalf("preserve cadence blocker omitted its unresolved FPS cause: %q", blockers)
	}
	var jobs, plans, reservations int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.ExecutionPlan{}).Count(&plans).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.SchedulerReservation{}).Count(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if jobs != 0 || plans != 0 || reservations != 0 {
		t.Fatalf("unresolved preserve candidate persisted Queue state: jobs=%d plans=%d reservations=%d", jobs, plans, reservations)
	}
}

func TestClaimPreparationBlocksJob2191IVTCBeforeExecutionPlanDispatch(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	profile := fixture.profile
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	profile.WorkerConfig["fieldStructureMode"] = "preserve"
	profile.WorkerConfig["hevcLevelMode"] = "auto"
	if err := fixture.db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var scan models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	ambiguous := job2191AmbiguousIVTCScanResult(t, fixture.paths[0])
	ambiguous.ID = scan.ID
	if err := fixture.db.Save(&ambiguous).Error; err != nil {
		t.Fatal(err)
	}
	var record models.AssetRecord
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&record).Error; err != nil {
		t.Fatal(err)
	}
	recordID := record.ID
	job := models.QueueJob{
		MediaPath: fixture.paths[0], SourceAssetRecordID: &recordID, LibraryID: fixture.library.ID,
		ProfileID: profile.ID, ProcessingMode: ProcessingModeFullEncode, Status: JobStatusRunning, Stage: JobStageClaimed,
	}
	if err := fixture.db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkerHandler(fixture.db).prepareClaimedQueueJob(job); err == nil || !strings.Contains(err.Error(), "explicit or validated field order") {
		t.Fatalf("claim preparation did not reject unresolved HEVC before plan creation: %v", err)
	}
	var plans int64
	if err := fixture.db.Model(&models.ExecutionPlan{}).Where("job_id = ?", job.ID).Count(&plans).Error; err != nil || plans != 0 {
		t.Fatalf("unresolved Queue job created an execution plan: count=%d err=%v", plans, err)
	}
	var stored models.QueueJob
	if err := fixture.db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if queueJobFreezeComplete(stored) || stored.ActiveExecutionPlanID != nil {
		t.Fatalf("unresolved Queue job persisted a dispatched freeze: %#v", stored)
	}
}

func TestLegacyBatchCreationRequiresPreflight(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	router := queuePreflightRouter(fixture.db)
	response := queuePreflightRequest(t, router, http.MethodPost, "/batches", queuePreflightInput(fixture, "Direct batch"))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "batch_preflight_required") {
		t.Fatalf("direct batch status=%d body=%s", response.Code, response.Body.String())
	}
	var count int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("direct batch created %d Queue jobs", count)
	}
	var asset models.AssetRecord
	if err := fixture.db.Where("path = ?", fixture.paths[0]).First(&asset).Error; err != nil {
		t.Fatal(err)
	}
	selectedCommit := queuePreflightRequest(t, router, http.MethodPost, "/selected-assets", map[string]any{"assetIds": []uint{asset.ID}, "commit": true})
	if selectedCommit.Code != http.StatusConflict || !strings.Contains(selectedCommit.Body.String(), "batch_preflight_required") {
		t.Fatalf("direct selected-asset commit status=%d body=%s", selectedCommit.Code, selectedCommit.Body.String())
	}
	selectedCandidate := queuePreflightRequest(t, router, http.MethodPost, "/preflights/selected-assets", map[string]any{"assetIds": []uint{asset.ID}})
	if selectedCandidate.Code != http.StatusCreated {
		t.Fatalf("selected-asset Preflight status=%d body=%s", selectedCandidate.Code, selectedCandidate.Body.String())
	}
	if err := fixture.db.Model(&models.QueueJob{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("selected-asset Preflight created %d Queue jobs before acceptance", count)
	}
}

func TestBatchPreflightTestEncodeUsesReviewedCandidateSnapshot(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	router := queuePreflightRouter(fixture.db)
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", queuePreflightInput(fixture, "Test Encode context"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.Profile{}).Where("id = ?", fixture.profile.ID).Update("quality_value", 32).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&models.AppSetting{Key: "assetConversionOverrides", Value: models.JSONMap{
		fixture.paths[0]: models.JSONMap{"frameStructureMaxBFrames": 4, "frameStructureBFrameMode": "custom"},
	}}).Error; err != nil {
		t.Fatal(err)
	}
	resolved, err := (AssetHandler{db: fixture.db}).resolveTestEncodeRequest(testEncodeRequest{
		SourcePath: fixture.paths[0], LibraryID: fixture.library.ID, ConfigurationSource: "batch_preflight",
		PreflightCandidateID: candidate.ID, PreflightRevision: candidate.Revision, PreflightAssetID: candidate.Assets[0].ID,
		StartMode: "beginning", DurationSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.profile.QualityValue != fixture.profile.QualityValue || resolved.job.ProfileID != fixture.profile.ID || workerStringValue(resolved.profile.WorkerConfig["videoEncoder"]) != "libx265" {
		t.Fatalf("Test Encode re-resolved material candidate fields: profile=%#v job=%#v", resolved.profile, resolved.job)
	}
	if resolved.requested["batchPreflightCandidateId"] != candidate.ID || resolved.requested["batchPreflightAssetId"] != candidate.Assets[0].ID {
		t.Fatalf("Test Encode did not retain exact candidate identity: %#v", resolved.requested)
	}
	stale, err := (AssetHandler{db: fixture.db}).resolveTestEncodeRequest(testEncodeRequest{
		SourcePath: fixture.paths[0], LibraryID: fixture.library.ID, ConfigurationSource: "batch_preflight",
		PreflightCandidateID: candidate.ID, PreflightRevision: candidate.Revision + 1, PreflightAssetID: candidate.Assets[0].ID,
		StartMode: "beginning", DurationSeconds: 60,
	})
	if err == nil || stale.job.ID != 0 {
		t.Fatalf("stale candidate context unexpectedly resolved: result=%#v err=%v", stale, err)
	}
}

func TestCompatiblePreviewUsesExactBatchPreflightContext(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 1)
	router := queuePreflightRouter(fixture.db)
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", queuePreflightInput(fixture, "Preview context"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	changed := fixture.profile
	changed.QualityValue = 99
	preview := compatiblePreviewRequest{
		Path: fixture.paths[0], Profile: &changed,
		PreflightCandidateID: candidate.ID, PreflightRevision: candidate.Revision, PreflightAssetID: candidate.Assets[0].ID,
	}
	if err := resolveCompatiblePreviewPreflight(fixture.db, &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Profile == nil || preview.Profile.QualityValue != fixture.profile.QualityValue || preview.ProfileID != 0 || workerStringValue(preview.Profile.WorkerConfig["videoEncoder"]) != "libx265" {
		t.Fatalf("Preview did not use the exact reviewed profile: %#v", preview)
	}
	preview.PreflightRevision++
	if err := resolveCompatiblePreviewPreflight(fixture.db, &preview); err == nil {
		t.Fatal("Preview accepted a stale Batch Preflight revision")
	}
}

func createQueuePreflightCandidate(t *testing.T, router *gin.Engine, input QueueBatchPreflightInput) QueueBatchPreflightResponse {
	t.Helper()
	created := queuePreflightRequest(t, router, http.MethodPost, "/preflights", input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(created.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	return candidate
}

func TestBatchPreflightRemovesBlockedAndFinalAssetsWithoutQueueing(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 2)
	input := queuePreflightInput(fixture, "Remove blocked candidate item")
	input.Jobs[1].LibraryID = 99999
	router := queuePreflightRouter(fixture.db)
	candidate := createQueuePreflightCandidate(t, router, input)
	var blocked QueueBatchPreflightAssetResponse
	var ready QueueBatchPreflightAssetResponse
	for _, asset := range candidate.Assets {
		if asset.Status == queuePreflightBlocked {
			blocked = asset
		} else if asset.Status == queuePreflightReady {
			ready = asset
		}
	}
	if blocked.ID == "" || ready.ID == "" || ready.Plan == nil {
		t.Fatalf("expected one blocked and one ready asset with a plan: %#v", candidate.Assets)
	}
	readyPlan := ready.Plan
	blockedBytes, err := os.ReadFile(blocked.MediaPath)
	if err != nil {
		t.Fatal(err)
	}
	removed := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/remove", map[string]any{"revision": candidate.Revision, "assetId": blocked.ID})
	if removed.Code != http.StatusOK {
		t.Fatalf("remove blocked status=%d body=%s", removed.Code, removed.Body.String())
	}
	var remaining QueueBatchPreflightResponse
	if err := json.Unmarshal(removed.Body.Bytes(), &remaining); err != nil {
		t.Fatal(err)
	}
	if remaining.Status != models.QueueBatchPreflightStatusActive || remaining.Revision != candidate.Revision+1 || len(remaining.Assets) != 1 || remaining.Assets[0].ID != ready.ID || !reflect.DeepEqual(remaining.Assets[0].Plan, readyPlan) {
		t.Fatalf("removing blocked item changed the remaining reviewed plan: %#v", remaining)
	}
	duplicate := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/remove", map[string]any{"revision": candidate.Revision, "assetId": blocked.ID})
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate stale removal status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	last := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/remove", map[string]any{"revision": remaining.Revision, "assetId": ready.ID})
	if last.Code != http.StatusOK {
		t.Fatalf("remove final candidate item status=%d body=%s", last.Code, last.Body.String())
	}
	var terminal QueueBatchPreflightResponse
	if err := json.Unmarshal(last.Body.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Status != models.QueueBatchPreflightStatusDiscarded || terminal.Revision != remaining.Revision+1 || len(terminal.Assets) != 0 {
		t.Fatalf("removing final item did not safely terminalize the candidate: %#v", terminal)
	}
	var jobs int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("removal created %d QueueJobs", jobs)
	}
	if got, err := os.ReadFile(blocked.MediaPath); err != nil || !reflect.DeepEqual(got, blockedBytes) {
		t.Fatalf("removal changed or removed source media: bytes=%q err=%v", got, err)
	}
	loaded := queuePreflightRequest(t, router, http.MethodGet, "/preflights/"+candidate.ID, nil)
	if loaded.Code != http.StatusOK || !strings.Contains(loaded.Body.String(), `"status":"discarded"`) {
		t.Fatalf("terminal candidate was not persisted: status=%d body=%s", loaded.Code, loaded.Body.String())
	}
	fresh := createQueuePreflightCandidate(t, router, queuePreflightInput(fixture, "Fresh after discard"))
	if fresh.ID == candidate.ID || fresh.Status != models.QueueBatchPreflightStatusActive {
		t.Fatalf("new Preflight reused a discarded candidate: %#v", fresh)
	}
}

func TestBatchPreflightDiscardsPartialRemainderWithoutChangingAcceptedWork(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 2)
	input := queuePreflightInput(fixture, "Partial accept then discard")
	input.Jobs[1].LibraryID = 99999
	router := queuePreflightRouter(fixture.db)
	candidate := createQueuePreflightCandidate(t, router, input)
	var readyID, blockedID string
	for _, asset := range candidate.Assets {
		if asset.Status == queuePreflightReady {
			readyID = asset.ID
		} else if asset.Status == queuePreflightBlocked {
			blockedID = asset.ID
		}
	}
	if readyID == "" || blockedID == "" {
		t.Fatalf("expected ready and blocked assets: %#v", candidate.Assets)
	}
	acceptedRecorder := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/accept", map[string]any{"revision": candidate.Revision, "assetIds": []string{readyID}})
	if acceptedRecorder.Code != http.StatusCreated {
		t.Fatalf("partial acceptance status=%d body=%s", acceptedRecorder.Code, acceptedRecorder.Body.String())
	}
	var accepted QueueBatchPreflightResponse
	if err := json.Unmarshal(acceptedRecorder.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	var jobsBefore []models.QueueJob
	var plansBefore []models.ExecutionPlan
	var reservationsBefore []models.SchedulerReservation
	if err := fixture.db.Order("id ASC").Find(&jobsBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Order("id ASC").Find(&plansBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Order("id ASC").Find(&reservationsBefore).Error; err != nil {
		t.Fatal(err)
	}
	if len(jobsBefore) != 1 || len(plansBefore) == 0 || len(reservationsBefore) == 0 {
		t.Fatalf("partial acceptance did not create expected existing work: jobs=%d plans=%d reservations=%d", len(jobsBefore), len(plansBefore), len(reservationsBefore))
	}
	queuedSource, err := os.ReadFile(fixture.paths[0])
	if err != nil {
		t.Fatal(err)
	}
	discardedRecorder := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/discard", map[string]any{"revision": accepted.Revision})
	if discardedRecorder.Code != http.StatusOK {
		t.Fatalf("discard partial candidate status=%d body=%s", discardedRecorder.Code, discardedRecorder.Body.String())
	}
	var discarded QueueBatchPreflightResponse
	if err := json.Unmarshal(discardedRecorder.Body.Bytes(), &discarded); err != nil {
		t.Fatal(err)
	}
	if discarded.Status != models.QueueBatchPreflightStatusDiscarded || discarded.Revision != accepted.Revision+1 || len(discarded.Assets) != 1 || discarded.Assets[0].Status != queuePreflightQueued || discarded.Assets[0].ID != readyID {
		t.Fatalf("discard did not preserve accepted candidate history: %#v", discarded)
	}
	var jobsAfter []models.QueueJob
	var plansAfter []models.ExecutionPlan
	var reservationsAfter []models.SchedulerReservation
	if err := fixture.db.Order("id ASC").Find(&jobsAfter).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Order("id ASC").Find(&plansAfter).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Order("id ASC").Find(&reservationsAfter).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jobsAfter, jobsBefore) || !reflect.DeepEqual(plansAfter, plansBefore) || !reflect.DeepEqual(reservationsAfter, reservationsBefore) {
		t.Fatal("discard changed accepted QueueJobs, ExecutionPlans, or SchedulerReservations")
	}
	if got, err := os.ReadFile(fixture.paths[0]); err != nil || !reflect.DeepEqual(got, queuedSource) {
		t.Fatalf("discard changed accepted source media: bytes=%q err=%v", got, err)
	}
	stale := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+candidate.ID+"/discard", map[string]any{"revision": accepted.Revision})
	if stale.Code != http.StatusConflict {
		t.Fatalf("duplicate discard status=%d body=%s", stale.Code, stale.Body.String())
	}
}

func TestBatchPreflightRemoveAndDiscardPersistAcrossDatabaseRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "preflight-restart.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.AssetRecord{}, &models.Profile{}, &models.ProfileAssignment{}, &models.AssetScopeConfiguration{},
		&models.TestEncode{}, &models.ProfileQualification{}, &models.QueueJob{}, &models.ExecutionPlan{},
		&models.SchedulerReservation{}, &models.AppSetting{}, &models.ScanResult{}, &models.AssetMaintenanceOperation{},
		&models.Library{}, &models.RuntimeSnapshot{}, &models.RuntimeState{}, &models.QueueBatchPreflight{},
	); err != nil {
		t.Fatal(err)
	}
	fixture := newQueuePreflightFixtureWithDB(t, 1, db)
	input := queuePreflightInput(fixture, "Persisted discarded candidate")
	input.Jobs[0].LibraryID = 99999
	router := queuePreflightRouter(db)
	removedCandidate := createQueuePreflightCandidate(t, router, input)
	removed := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+removedCandidate.ID+"/remove", map[string]any{"revision": removedCandidate.Revision, "assetId": removedCandidate.Assets[0].ID})
	if removed.Code != http.StatusOK {
		t.Fatalf("remove status=%d body=%s", removed.Code, removed.Body.String())
	}
	discardedCandidate := createQueuePreflightCandidate(t, router, input)
	discarded := queuePreflightRequest(t, router, http.MethodPost, "/preflights/"+discardedCandidate.ID+"/discard", map[string]any{"revision": discardedCandidate.Revision})
	if discarded.Code != http.StatusOK {
		t.Fatalf("discard status=%d body=%s", discarded.Code, discarded.Body.String())
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	restartedDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := restartedDB.AutoMigrate(&models.QueueBatchPreflight{}); err != nil {
		t.Fatal(err)
	}
	restartedRouter := queuePreflightRouter(restartedDB)
	for _, candidateID := range []string{removedCandidate.ID, discardedCandidate.ID} {
		restarted := queuePreflightRequest(t, restartedRouter, http.MethodGet, "/preflights/"+candidateID, nil)
		if restarted.Code != http.StatusOK || !strings.Contains(restarted.Body.String(), `"status":"discarded"`) || !strings.Contains(restarted.Body.String(), `"revision":2`) {
			t.Fatalf("candidate lifecycle did not survive restart: status=%d body=%s", restarted.Code, restarted.Body.String())
		}
	}
	var jobs int64
	if err := restartedDB.Model(&models.QueueJob{}).Count(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("discard created %d QueueJobs", jobs)
	}
}

func TestBatchPreflightConcurrentRemoveRequestsUseRevisionFence(t *testing.T) {
	fixture := newQueuePreflightFixture(t, 2)
	input := queuePreflightInput(fixture, "Concurrent removal")
	input.Jobs[0].LibraryID = 99999
	input.Jobs[1].LibraryID = 99999
	router := queuePreflightRouter(fixture.db)
	candidate := createQueuePreflightCandidate(t, router, input)
	if len(candidate.Assets) != 2 {
		t.Fatalf("candidate assets=%d want=2", len(candidate.Assets))
	}
	statuses := make(chan int, 2)
	for _, asset := range candidate.Assets {
		asset := asset
		go func() {
			body, _ := json.Marshal(map[string]any{"revision": candidate.Revision, "assetId": asset.ID})
			request := httptest.NewRequest(http.MethodPost, "/preflights/"+candidate.ID+"/remove", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			statuses <- response.Code
		}()
	}
	first, second := <-statuses, <-statuses
	if !((first == http.StatusOK && second == http.StatusConflict) || (first == http.StatusConflict && second == http.StatusOK)) {
		t.Fatalf("concurrent remove status codes were %d and %d", first, second)
	}
	var persisted models.QueueBatchPreflight
	if err := fixture.db.First(&persisted, "id = ?", candidate.ID).Error; err != nil {
		t.Fatal(err)
	}
	state, err := queuePreflightStateFromRecord(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Revision != candidate.Revision+1 || persisted.Status != models.QueueBatchPreflightStatusActive || len(state.Assets) != 1 {
		t.Fatalf("concurrent stale request lost a revision or asset: revision=%d status=%s assets=%d", persisted.Revision, persisted.Status, len(state.Assets))
	}
}
