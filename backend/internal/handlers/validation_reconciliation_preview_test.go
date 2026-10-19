package handlers

import (
    "testing"
    "time"

    "github.com/anuelvs/mvforge/backend/internal/models"
)

func TestHistoricalTrackReconciliationPreviewFailsClosedForCrossedSnapshot(t *testing.T) {
    job := models.QueueJob{ID: 1, Status: JobStatusCompleted, MediaPath: "/fixtures/one.mkv",
        TrackProfileSnapshot: models.JSONMap{"resolvedForAsset": "/fixtures/two.mkv"}}
    artifact := map[string]any{
        "resultIdentity": map[string]any{"artifactId":"result:1","generationToken":"token:1","generationVersion":float64(1),"outputRevision":"revision:1"},
        "executionBinding": map[string]any{"executionNumber":float64(1),"executionPlanId":float64(11),"executionPlanVersion":float64(1),"executionPlanFingerprint":"plan","frozenQueueFingerprint":"queue"},
        "outputProbe": map[string]any{"streams":[]any{map[string]any{"codec_type":"audio"},map[string]any{"codec_type":"audio"}}},
    }
    result := assessHistoricalTrackReconciliation(job, artifact, true)
    if result.Status != "NEEDS_EVIDENCE" || result.Observed.Audio != 2 {
        t.Fatalf("crossed snapshot was treated as recoverable: %#v", result)
    }
    // These three failures (extra/missing audio, subtitle and sidecar
    // expectations) are symptoms, not independent recovery authority.
    for _, recorded := range []models.JSONMap{
        {"resolvedForAsset": "/fixtures/two.mkv", "audioMode": "first"},
        {"resolvedForAsset": "/fixtures/two.mkv", "subtitleDisposition": "keep"},
        {"resolvedForAsset": "/fixtures/two.mkv", "subtitleSidecarFormats": models.JSONList{"srt","ass"}},
    } {
        job.TrackProfileSnapshot = recorded
        if got := assessHistoricalTrackReconciliation(job, artifact, true); got.Status != "NEEDS_EVIDENCE" {
            t.Fatalf("a changed historical track expectation granted recovery: %#v", got)
        }
    }
}

func TestHistoricalTrackReconciliationPreviewRejectsUnboundAndNormalJobs(t *testing.T) {
    job := models.QueueJob{ID: 2, Status: JobStatusCompleted, MediaPath: "/fixtures/one.mkv",
        TrackProfileSnapshot: models.JSONMap{"resolvedForAsset": "/fixtures/one.mkv"}}
    if got := assessHistoricalTrackReconciliation(job, nil, false); got.Status != "BLOCKED" {
        t.Fatalf("unbound Result was not blocked: %#v", got)
    }
    artifact := map[string]any{"resultIdentity": map[string]any{
        "artifactId":"result:2","generationToken":"token:2","generationVersion":float64(1),"outputRevision":"revision:2"},
        "executionBinding": map[string]any{"executionNumber":float64(1),"executionPlanId":float64(12),"executionPlanVersion":float64(1),"executionPlanFingerprint":"plan","frozenQueueFingerprint":"queue"},
    }
    if got := assessHistoricalTrackReconciliation(job, artifact, true); got.Status != "BLOCKED" {
        t.Fatalf("uncrossed job incorrectly declared recoverable: %#v", got)
    }
    now := time.Now()
    job.PublishedAt = &now
    if got := assessHistoricalTrackReconciliation(job, artifact, true); got.Status != "BLOCKED" {
        t.Fatalf("published job incorrectly declared recoverable: %#v", got)
    }
}
