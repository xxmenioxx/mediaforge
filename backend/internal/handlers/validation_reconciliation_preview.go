package handlers

import (
    "net/http"
    "path/filepath"
    "strings"

    "github.com/anuelvs/mvforge/backend/internal/models"
    "github.com/gin-gonic/gin"
)

// HistoricalTrackReconciliationPreview is diagnostic only. It cannot grant a
// recovered expected-track contract, a ValidationRun or Publisher eligibility.
type HistoricalTrackReconciliationPreview struct {
    JobID uint `json:"jobId"`
    Status string `json:"status"`
    Reasons []string `json:"reasons"`
    Observed struct {
        Audio int `json:"audio"`
        Subtitles int `json:"subtitles"`
    } `json:"observed"`
    Frozen struct {
        ResolvedForAsset string `json:"resolvedForAsset,omitempty"`
        Audio int `json:"audio"`
        Subtitles int `json:"subtitles"`
        Sidecars int `json:"sidecars"`
    } `json:"frozen"`
    ResultArtifactID string `json:"resultArtifactId,omitempty"`
    ResultOutputRevision string `json:"resultOutputRevision,omitempty"`
    ExecutionPlanID uint `json:"executionPlanId,omitempty"`
}

// assessHistoricalTrackReconciliation never treats worker output or the crossed
// frozen track plan as independent evidence of the original operator intent.
func assessHistoricalTrackReconciliation(job models.QueueJob, artifact map[string]any, bound bool) HistoricalTrackReconciliationPreview {
    preview := HistoricalTrackReconciliationPreview{JobID: job.ID, Status: "BLOCKED"}
    if job.Status != JobStatusCompleted || job.PublishedAt != nil {
        preview.Reasons = append(preview.Reasons, "Only completed, unpublished jobs may be inspected for historical reconciliation")
        return preview
    }
    identity, identityOK := resultIdentityFromArtifact(artifact)
    binding, bindingOK := resultBindingFromArtifact(artifact)
    if !identityOK || !bindingOK || !bound {
        preview.Reasons = append(preview.Reasons, "Canonical Result generation or frozen Queue/ExecutionPlan binding is unavailable or inconsistent")
        return preview
    }
    preview.ResultArtifactID = identity.ArtifactID
    preview.ResultOutputRevision = identity.OutputRevision
    preview.ExecutionPlanID = binding.ExecutionPlanID
    outputProbe := unknownRecord(artifact["outputProbe"])
    if outputProbe != nil {
        preview.Observed.Audio = countProbeStreams(outputProbe, "audio")
        preview.Observed.Subtitles = countProbeStreams(outputProbe, "subtitle")
    }
    preview.Frozen.ResolvedForAsset = strings.TrimSpace(workerStringValue(job.TrackProfileSnapshot["resolvedForAsset"]))
    if plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot); ok {
        preview.Frozen.Audio = len(plan.AudioStreams)
        for _, stream := range plan.SubtitleStreams {
            if stream.Action == SubtitleDispositionKeep || stream.Action == SubtitleDispositionKeepAndExtract {
                preview.Frozen.Subtitles++
            }
        }
        preview.Frozen.Sidecars = len(plan.SidecarOutputs)
    }
    resolved := preview.Frozen.ResolvedForAsset
    if resolved == "" {
        preview.Status = "NEEDS_EVIDENCE"
        preview.Reasons = append(preview.Reasons, "Frozen track plan lacks a verifiable resolvedForAsset; original per-asset intent is not independently proven")
        return preview
    }
    if filepath.Clean(resolved) != filepath.Clean(job.MediaPath) {
        preview.Status = "NEEDS_EVIDENCE"
        preview.Reasons = append(preview.Reasons,
            "Frozen TrackProfileSnapshot belongs to a different asset; output streams, AS-IS plan and current defaults cannot establish the original expected track/sidecar contract")
        return preview
    }
    preview.Reasons = append(preview.Reasons, "No crossed asset detected; historical reconciliation is not applicable")
    return preview
}

// PreviewHistoricalTrackReconciliation reads existing evidence only. In
// particular it does not hash the output, run ffprobe, persist state, or
// authorize a later commit/publish operation.
func (h ValidationHandler) PreviewHistoricalTrackReconciliation(c *gin.Context) {
    job, ok := h.job(c)
    if !ok { return }
    if job.Status != JobStatusCompleted || job.PublishedAt != nil {
        c.JSON(http.StatusOK, assessHistoricalTrackReconciliation(job, nil, false))
        return
    }
    artifact, _, err := readLatestJobArtifact(h.db, job, "result")
    if err != nil {
        c.JSON(http.StatusOK, HistoricalTrackReconciliationPreview{
            JobID: job.ID, Status: "NEEDS_EVIDENCE",
            Reasons: []string{"Canonical Result artifact is unavailable; no historical repair can be inferred"},
        })
        return
    }
    binding, bindingOK := resultBindingFromArtifact(artifact)
    bound := bindingOK && frozenBindingMatches(h.db, job, binding)
    c.JSON(http.StatusOK, assessHistoricalTrackReconciliation(job, artifact, bound))
}
