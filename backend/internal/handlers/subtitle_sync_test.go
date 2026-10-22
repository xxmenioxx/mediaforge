package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestApplySubtitleSyncTransformSRTOnlyChangesCueTimes(t *testing.T) {
	source := "1\r\n00:00:00,250 --> 00:00:01,750 align:start position:10%\r\nHello <i>world</i>\r\ncontinued\r\n\r\n2\r\n00:00:02,500 --> 00:00:03,000\r\nSecond line\r\n"
	want := "1\r\n00:00:01,670 --> 00:00:03,170 align:start position:10%\r\nHello <i>world</i>\r\ncontinued\r\n\r\n2\r\n00:00:03,920 --> 00:00:04,420\r\nSecond line\r\n"
	got, timings, err := applySubtitleSyncTransform("srt", []byte(source), 1420)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("unexpected SRT output:\n%s", got)
	}
	if len(timings) != 2 || timings[0].StartMilliseconds != 1670 || timings[1].EndMilliseconds != 4420 {
		t.Fatalf("unexpected transformed timings: %#v", timings)
	}
}

func TestApplySubtitleSyncTransformASSPreservesStructureAndDialogue(t *testing.T) {
	source := "[Script Info]\r\nTitle: untouched\r\n[V4+ Styles]\r\nFormat: Name, Fontname, Fontsize\r\nStyle: Default,Arial,40\r\n[Events]\r\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\r\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0000,0000,0000,,{\\pos(20,40)}Hello\\N{\\k40}world\r\nComment: 0,0:00:03.00,0:00:04.00,Default,,0000,0000,0000,,comment\r\n"
	want := "[Script Info]\r\nTitle: untouched\r\n[V4+ Styles]\r\nFormat: Name, Fontname, Fontsize\r\nStyle: Default,Arial,40\r\n[Events]\r\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\r\nDialogue: 0,0:00:02.42,0:00:03.42,Default,,0000,0000,0000,,{\\pos(20,40)}Hello\\N{\\k40}world\r\nComment: 0,0:00:03.00,0:00:04.00,Default,,0000,0000,0000,,comment\r\n"
	got, timings, err := applySubtitleSyncTransform("ass", []byte(source), 1420)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("unexpected ASS output:\n%s", got)
	}
	if len(timings) != 1 || timings[0].StartMilliseconds != 2420 || timings[0].EndMilliseconds != 3420 {
		t.Fatalf("unexpected transformed timings: %#v", timings)
	}
}

func TestApplySubtitleSyncTransformSSAAndRejectsUnsafeIntervals(t *testing.T) {
	ssa := "[Script Info]\n[V4 Styles]\nFormat: Name, Fontname\nStyle: Default,Arial\n[Events]\nFormat: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: Marked=0,0:00:01.00,0:00:02.00,Default,,0000,0000,0000,,{\\p1}m 0 0 l 10 10{\\p0}\n"
	got, _, err := applySubtitleSyncTransform("ssa", []byte(ssa), 250)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Dialogue: Marked=0,0:00:01.25,0:00:02.25") || !strings.Contains(string(got), "{\\p1}m 0 0 l 10 10{\\p0}") {
		t.Fatalf("SSA timing or drawing was not preserved: %s", got)
	}
	if _, _, err := applySubtitleSyncTransform("srt", []byte("1\n00:00:00,100 --> 00:00:01,000\ntext\n"), -200); err == nil {
		t.Fatal("expected negative start to fail closed")
	}
	if _, _, err := applySubtitleSyncTransform("ass", []byte(ssa), 1); err == nil {
		t.Fatal("expected non-centisecond ASS offset to be rejected")
	}
}

func TestFixedOffsetSubtitleSyncProposalRequiresReferenceAndPreviewsBoundedCues(t *testing.T) {
	var source strings.Builder
	for index := 0; index < 20; index++ {
		start := index * 2
		_, _ = source.WriteString(strconv.Itoa(index+1) + "\n")
		_, _ = source.WriteString(fmt.Sprintf("00:00:%02d,000 --> 00:00:%02d,500\nCue %d\n\n", start, start+1, index+1))
	}
	input := SubtitleSyncAnalysisInput{
		SourceIdentity: "external:/asset/movie.es.srt:identity", SourceRevision: "revision-1", Representation: "srt",
		ReferenceAudio:     SubtitleSyncAudioReference{StreamIndex: 2, StreamID: "0x3", Codec: "aac", Language: "spa", Title: "Spanish"},
		OffsetMilliseconds: 1500, Source: []byte(source.String()),
	}
	engine := fixedOffsetSubtitleSyncEngine{}
	proposal, err := engine.Analyze(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Model != "fixed_offset" || proposal.QualityState != "manual_review" || proposal.ReferenceAudio.StreamIndex != 2 || len(proposal.Preview) != maxSubtitleSyncPreview {
		t.Fatalf("proposal omitted required review evidence: %#v", proposal)
	}
	if proposal.Preview[0].ProposedStartMs-proposal.Preview[0].CurrentStartMs != input.OffsetMilliseconds || proposal.Preview[len(proposal.Preview)-1].CueIndex != 20 {
		t.Fatalf("proposal preview is not representative: %#v", proposal.Preview)
	}
	input.ReferenceAudio = SubtitleSyncAudioReference{}
	if _, err := engine.Analyze(context.Background(), input); err == nil {
		t.Fatal("expected missing reference audio to fail")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.Analyze(cancelled, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestSubtitleSyncCapabilityReportsBoundedRuntimeAvailability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newSubtitleSyncTestContext(t, nil)
	(AssetHandler{}).SubtitleSyncCapability(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected capability status %d: %s", recorder.Code, recorder.Body.String())
	}
	var capability struct {
		Enabled                  bool     `json:"enabled"`
		FixedOffset              bool     `json:"fixedOffset"`
		AutomaticAlignment       bool     `json:"automaticAlignment"`
		AutomaticAlignmentReason string   `json:"automaticAlignmentReason"`
		AutomaticFormats         []string `json:"automaticFormats"`
		SegmentedAlignment       bool     `json:"segmentedAlignment"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &capability); err != nil {
		t.Fatal(err)
	}
	automaticAvailable := subtitleSyncAutomaticEngineAvailable()
	expectedReason := subtitleSyncAutomaticUnavailableReason()
	if !capability.Enabled || !capability.FixedOffset || capability.AutomaticAlignment != automaticAvailable || capability.SegmentedAlignment || capability.AutomaticAlignmentReason != expectedReason || len(capability.AutomaticFormats) != 1 || capability.AutomaticFormats[0] != "srt" {
		t.Fatalf("unexpected subtitle sync capability: %#v", capability)
	}
}

func TestSubtitleSyncPreviewIsReadOnlyAndApplyUsesReviewedProposal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	subtitlePath := filepath.Join(root, "episode.es.srt")
	original := "1\n00:00:01,000 --> 00:00:02,000\nHola\n"
	expected := "1\n00:00:02,000 --> 00:00:03,000\nHola\n"
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, subtitlePath, original)
	installSubtitleSyncFFProbe(t, root)
	db := translationLibrarySourceTestDB(t, root, mediaPath)
	if err := db.AutoMigrate(&models.SubtitleSyncProvenance{}); err != nil {
		t.Fatal(err)
	}
	handler := AssetHandler{db: db}

	previewInput := map[string]any{
		"path": mediaPath, "subtitlePath": subtitlePath, "referenceAudioStreamIndex": 1, "offsetMilliseconds": 1000,
	}
	previewContext, previewRecorder := newSubtitleSyncTestContext(t, previewInput)
	handler.AnalyzeSubtitleSync(previewContext)
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("preview failed with %d: %s", previewRecorder.Code, previewRecorder.Body.String())
	}
	var proposal SubtitleSyncProposal
	if err := json.Unmarshal(previewRecorder.Body.Bytes(), &proposal); err != nil {
		t.Fatal(err)
	}
	if proposal.Model != "fixed_offset" || proposal.OffsetMilliseconds != 1000 || proposal.ReferenceAudio.StreamIndex != 1 || len(proposal.Preview) != 1 || proposal.Preview[0].ProposedStartMs != 2000 {
		t.Fatalf("preview omitted the requested decision evidence: %#v", proposal)
	}
	current, err := os.ReadFile(subtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != original {
		t.Fatalf("preview mutated the subtitle: %s", current)
	}

	invalidInput := map[string]any{
		"path": mediaPath, "subtitlePath": subtitlePath, "referenceAudioStreamIndex": 1, "offsetMilliseconds": -2000,
	}
	invalidContext, invalidRecorder := newSubtitleSyncTestContext(t, invalidInput)
	handler.AnalyzeSubtitleSync(invalidContext)
	if invalidRecorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid timing to fail closed, got %d: %s", invalidRecorder.Code, invalidRecorder.Body.String())
	}
	current, err = os.ReadFile(subtitlePath)
	if err != nil || string(current) != original {
		t.Fatalf("invalid preview changed the source: content=%q err=%v", current, err)
	}

	applyInput := map[string]any{
		"path": mediaPath, "subtitlePath": subtitlePath, "referenceAudioStreamIndex": 1, "offsetMilliseconds": 1000,
		"sourceRevision": proposal.SourceRevision, "proposalId": proposal.ProposalID,
	}
	applyContext, applyRecorder := newSubtitleSyncTestContext(t, applyInput)
	handler.StartSubtitleSync(applyContext)
	if applyRecorder.Code != http.StatusAccepted {
		t.Fatalf("apply failed with %d: %s", applyRecorder.Code, applyRecorder.Body.String())
	}
	var started struct {
		OperationID string `json:"operationId"`
	}
	if err := json.Unmarshal(applyRecorder.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.OperationID == "" {
		t.Fatal("apply did not return an operation identity")
	}
	t.Cleanup(func() {
		subtitleExtractionOperations.Lock()
		if operation := subtitleExtractionOperations.items[started.OperationID]; operation != nil && operation.cancel != nil {
			operation.cancel()
		}
		delete(subtitleExtractionOperations.items, started.OperationID)
		subtitleExtractionOperations.Unlock()
	})

	deadline := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		subtitleExtractionOperations.RLock()
		operation := cloneSubtitleExtractionOperationSnapshot(subtitleExtractionOperations.items[started.OperationID])
		subtitleExtractionOperations.RUnlock()
		if operation.ID == "" {
			t.Fatal("subtitle sync operation disappeared before completion")
		}
		if operation.Status != "running" {
			if operation.Status != "completed" {
				t.Fatalf("sync operation ended as %s: %s", operation.Status, operation.Error)
			}
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("subtitle sync operation did not complete")
		case <-ticker.C:
		}
	}
	published, err := os.ReadFile(subtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(published) != expected {
		t.Fatalf("apply did not use the reviewed proposal exactly: %q", published)
	}
	var provenance models.SubtitleSyncProvenance
	if err := db.First(&provenance, "operation_id = ?", started.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	if provenance.SourceRevision != proposal.SourceRevision || provenance.ReferenceAudioStreamID != proposal.ReferenceAudio.StreamID || provenance.OffsetMilliseconds != proposal.OffsetMilliseconds {
		t.Fatalf("applied provenance does not match the reviewed proposal: %#v", provenance)
	}
}

func TestStartSubtitleSyncRejectsStaleProposalWithoutMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	subtitlePath := filepath.Join(root, "episode.es.srt")
	original := "1\n00:00:01,000 --> 00:00:02,000\nHola\n"
	changed := "1\n00:00:01,000 --> 00:00:02,000\nEdited after preview\n"
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, subtitlePath, original)
	installSubtitleSyncFFProbe(t, root)
	handler := AssetHandler{db: translationLibrarySourceTestDB(t, root, mediaPath)}
	previewContext, previewRecorder := newSubtitleSyncTestContext(t, map[string]any{
		"path": mediaPath, "subtitlePath": subtitlePath, "referenceAudioStreamIndex": 1, "offsetMilliseconds": 1000,
	})
	handler.AnalyzeSubtitleSync(previewContext)
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("preview failed with %d: %s", previewRecorder.Code, previewRecorder.Body.String())
	}
	var proposal SubtitleSyncProposal
	if err := json.Unmarshal(previewRecorder.Body.Bytes(), &proposal); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subtitlePath, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	applyContext, applyRecorder := newSubtitleSyncTestContext(t, map[string]any{
		"path": mediaPath, "subtitlePath": subtitlePath, "referenceAudioStreamIndex": 1, "offsetMilliseconds": 1000,
		"sourceRevision": proposal.SourceRevision, "proposalId": proposal.ProposalID,
	})
	handler.StartSubtitleSync(applyContext)
	if applyRecorder.Code != http.StatusConflict {
		t.Fatalf("expected stale proposal rejection, got %d: %s", applyRecorder.Code, applyRecorder.Body.String())
	}
	actual, err := os.ReadFile(subtitlePath)
	if err != nil || string(actual) != changed {
		t.Fatalf("stale proposal changed the current subtitle: content=%q err=%v", actual, err)
	}
}

func TestPublishSubtitleSyncStagePersistsProvenanceAndRechecksSource(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	subtitlePath := filepath.Join(root, "episode.es.srt")
	writeTestFile(t, mediaPath, "media fixture")
	original := "1\n00:00:01,000 --> 00:00:02,000\nHola\n"
	writeTestFile(t, subtitlePath, original)
	installSubtitleSyncFFProbe(t, root)
	db, err := gorm.Open(sqlite.Open("file:subtitle-sync-"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SubtitleSyncProvenance{}); err != nil {
		t.Fatal(err)
	}
	handler := AssetHandler{db: db}
	source, err := resolveExternalSubtitleTranslationSource(mediaPath, subtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	engine := fixedOffsetSubtitleSyncEngine{}
	proposal, err := engine.Analyze(context.Background(), SubtitleSyncAnalysisInput{
		SourceIdentity: source.SourceIdentity, SourceRevision: source.SourceRevision, Representation: "srt",
		ReferenceAudio:     SubtitleSyncAudioReference{StreamIndex: 1, StreamID: "0x2", Codec: "aac", Language: "spa", Title: "Spanish"},
		OffsetMilliseconds: 1000, Source: []byte(original),
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := engine.Apply(context.Background(), []byte(original), proposal)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := os.MkdirTemp(root, ".mvforge-subtitle-sync-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)
	stagePath := filepath.Join(workspace, "output.srt")
	if err := os.WriteFile(stagePath, output, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := handler.publishSubtitleSyncStage(context.Background(), "sync-test-operation", workspace, mediaPath, source, proposal, stagePath); err != nil {
		t.Fatal(err)
	}
	published, err := os.ReadFile(subtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(published), "00:00:02,000 --> 00:00:03,000") || !strings.Contains(string(published), "Hola") {
		t.Fatalf("unexpected published subtitle: %s", published)
	}
	var stored models.SubtitleSyncProvenance
	if err := db.First(&stored, "operation_id = ?", "sync-test-operation").Error; err != nil {
		t.Fatal(err)
	}
	if stored.SourceIdentity != proposal.SourceIdentity || stored.SourceRevision != proposal.SourceRevision ||
		stored.ReferenceAudioStreamIndex != 1 || stored.ReferenceAudioStreamID != "0x2" || stored.ReferenceAudioCodec != "aac" ||
		stored.Model != proposal.Model || stored.OffsetMilliseconds != 1000 || stored.QualityState != "manual_review" {
		t.Fatalf("unexpected persisted provenance: %#v", stored)
	}
	if len(stored.SegmentsJSON) > 64 || stored.AnalysisAt.IsZero() || stored.AppliedAt.IsZero() {
		t.Fatalf("provenance is incomplete or unbounded: %#v", stored)
	}
}

func TestPublishSubtitleSyncStageRejectsStaleSourceWithoutReplacement(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	subtitlePath := filepath.Join(root, "episode.es.srt")
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, subtitlePath, "1\n00:00:01,000 --> 00:00:02,000\nOriginal\n")
	installSubtitleSyncFFProbe(t, root)
	db, err := gorm.Open(sqlite.Open("file:subtitle-sync-stale-"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SubtitleSyncProvenance{}); err != nil {
		t.Fatal(err)
	}
	currentSource, err := resolveExternalSubtitleTranslationSource(mediaPath, subtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := (fixedOffsetSubtitleSyncEngine{}).Analyze(context.Background(), SubtitleSyncAnalysisInput{
		SourceIdentity: currentSource.SourceIdentity, SourceRevision: currentSource.SourceRevision, Representation: "srt",
		ReferenceAudio: SubtitleSyncAudioReference{StreamIndex: 1, StreamID: "0x2", Codec: "aac"}, OffsetMilliseconds: 1000,
		Source: []byte("1\n00:00:01,000 --> 00:00:02,000\nOriginal\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	changed := "1\n00:00:01,000 --> 00:00:02,000\nChanged\n"
	if err := os.WriteFile(subtitlePath, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := os.MkdirTemp(root, ".mvforge-subtitle-sync-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)
	stagePath := filepath.Join(workspace, "output.srt")
	if err := os.WriteFile(stagePath, []byte("1\n00:00:02,000 --> 00:00:03,000\nOriginal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = (AssetHandler{db: db}).publishSubtitleSyncStage(context.Background(), "sync-stale-operation", workspace, mediaPath, currentSource, proposal, stagePath)
	if !errors.Is(err, errTranslationSourceStale) {
		t.Fatalf("expected stale source error, got %v", err)
	}
	actual, readErr := os.ReadFile(subtitlePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(actual) != changed {
		t.Fatalf("stale apply changed the current subtitle: %s", actual)
	}
}

func TestPublishSubtitleSyncStageRejectsStaleAudioWithoutReplacement(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	subtitlePath := filepath.Join(root, "episode.es.srt")
	original := "1\n00:00:01,000 --> 00:00:02,000\nOriginal\n"
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, subtitlePath, original)
	installSubtitleSyncFFProbe(t, root)
	db, err := gorm.Open(sqlite.Open("file:subtitle-sync-stale-audio-"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SubtitleSyncProvenance{}); err != nil {
		t.Fatal(err)
	}
	handler := AssetHandler{db: db}
	source, err := resolveExternalSubtitleTranslationSource(mediaPath, subtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	audio, err := resolveSubtitleSyncAudio(context.Background(), mediaPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := (fixedOffsetSubtitleSyncEngine{}).Analyze(context.Background(), SubtitleSyncAnalysisInput{
		SubtitlePath: subtitlePath, SourceIdentity: source.SourceIdentity, SourceRevision: source.SourceRevision, Representation: "srt",
		ReferenceAudio: audio, OffsetMilliseconds: 1000, Source: []byte(original),
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := (fixedOffsetSubtitleSyncEngine{}).Apply(context.Background(), []byte(original), proposal)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := os.MkdirTemp(root, ".mvforge-subtitle-sync-stale-audio-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)
	stagePath := filepath.Join(workspace, "output.srt")
	if err := os.WriteFile(stagePath, output, 0o600); err != nil {
		t.Fatal(err)
	}
	probeResponse := `{"streams":[{"index":1,"id":"0x4","codec_type":"audio","codec_name":"aac","tags":{"language":"spa","title":"Spanish"}}]}`
	probeScript := "#!/bin/sh\nprintf '%s\\n' '" + probeResponse + "'\n"
	if err := os.WriteFile(filepath.Join(root, "ffprobe"), []byte(probeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := handler.publishSubtitleSyncStage(context.Background(), "sync-stale-audio-operation", workspace, mediaPath, source, proposal, stagePath); !errors.Is(err, errTranslationSourceStale) {
		t.Fatalf("expected stale audio identity error, got %v", err)
	}
	actual, err := os.ReadFile(subtitlePath)
	if err != nil || string(actual) != original {
		t.Fatalf("stale audio identity changed the current subtitle: content=%q err=%v", actual, err)
	}
	var count int64
	if err := db.Model(&models.SubtitleSyncProvenance{}).Where("operation_id = ?", "sync-stale-audio-operation").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale audio identity persisted provenance: count=%d", count)
	}
}

func TestPublishSubtitleSyncStageRollsBackWhenProvenanceWriteFails(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	subtitlePath := filepath.Join(root, "episode.es.srt")
	original := "1\n00:00:01,000 --> 00:00:02,000\nOriginal\n"
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, subtitlePath, original)
	installSubtitleSyncFFProbe(t, root)
	db, err := gorm.Open(sqlite.Open("file:subtitle-sync-rollback-"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := resolveExternalSubtitleTranslationSource(mediaPath, subtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	audio, err := resolveSubtitleSyncAudio(context.Background(), mediaPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := (fixedOffsetSubtitleSyncEngine{}).Analyze(context.Background(), SubtitleSyncAnalysisInput{
		SubtitlePath: subtitlePath, SourceIdentity: source.SourceIdentity, SourceRevision: source.SourceRevision, Representation: "srt",
		ReferenceAudio: audio, OffsetMilliseconds: 1000, Source: []byte(original),
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := (fixedOffsetSubtitleSyncEngine{}).Apply(context.Background(), []byte(original), proposal)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := os.MkdirTemp(root, ".mvforge-subtitle-sync-rollback-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)
	stagePath := filepath.Join(workspace, "output.srt")
	if err := os.WriteFile(stagePath, output, 0o600); err != nil {
		t.Fatal(err)
	}
	err = (AssetHandler{db: db}).publishSubtitleSyncStage(context.Background(), "sync-provenance-failure", workspace, mediaPath, source, proposal, stagePath)
	if err == nil {
		t.Fatal("expected provenance persistence failure")
	}
	actual, readErr := os.ReadFile(subtitlePath)
	if readErr != nil || string(actual) != original {
		t.Fatalf("publication failure did not restore the original subtitle: content=%q err=%v", actual, readErr)
	}
}

func installSubtitleSyncFFProbe(t *testing.T, directory string) {
	t.Helper()
	probePath := filepath.Join(directory, "ffprobe")
	content := "#!/bin/sh\nprintf '%s\\n' '{\"streams\":[{\"index\":1,\"id\":\"0x2\",\"codec_type\":\"audio\",\"codec_name\":\"aac\",\"tags\":{\"language\":\"spa\",\"title\":\"Spanish\"}}]}'\n"
	if err := os.WriteFile(probePath, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func newSubtitleSyncTestContext(t *testing.T, input any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/assets/subtitle-sync", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func TestSubtitleSyncProvenanceTimesAreWrittenInUTC(t *testing.T) {
	input := SubtitleSyncAnalysisInput{SourceIdentity: "source", SourceRevision: "revision", Representation: "srt",
		ReferenceAudio: SubtitleSyncAudioReference{StreamIndex: 1, Codec: "aac"}, OffsetMilliseconds: 0,
		Source: []byte("1\n00:00:01,000 --> 00:00:02,000\ncue\n")}
	proposal, err := (fixedOffsetSubtitleSyncEngine{}).Analyze(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.AnalyzedAt.Location() != time.UTC {
		t.Fatalf("expected UTC analysis time, got %s", proposal.AnalyzedAt.Location())
	}
}

func TestRunSubtitleSyncOperationHonorsCancellationWithoutChangingSource(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	subtitlePath := filepath.Join(root, "episode.es.srt")
	original := "1\n00:00:01,000 --> 00:00:02,000\nOriginal\n"
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, subtitlePath, original)
	source, err := resolveExternalSubtitleTranslationSource(mediaPath, subtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := os.MkdirTemp(root, ".mvforge-subtitle-sync-test-cancel-*")
	if err != nil {
		t.Fatal(err)
	}
	operationID := "subtitle-sync-cancel-" + strings.ReplaceAll(t.Name(), "/", "-")
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{ID: operationID, Status: "running"}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	(AssetHandler{}).runSubtitleSyncOperation(ctx, operationID, workspace, mediaPath, source, SubtitleSyncProposal{})

	actual, err := os.ReadFile(subtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != original {
		t.Fatalf("cancelled sync changed the current subtitle: %s", actual)
	}
	subtitleExtractionOperations.RLock()
	operation := cloneSubtitleExtractionOperationSnapshot(subtitleExtractionOperations.items[operationID])
	subtitleExtractionOperations.RUnlock()
	if operation.ID != operationID || operation.Status != "cancelled" || operation.Phase != "cancelled" {
		t.Fatalf("expected cancelled operation, got %#v", operation)
	}
}
