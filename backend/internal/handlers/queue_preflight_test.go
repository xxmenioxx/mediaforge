package handlers

import (
	"bytes"
	"encoding/json"
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
	if err := db.Create(&scan).Error; err != nil {
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
