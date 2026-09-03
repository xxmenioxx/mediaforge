package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

type TrackProfilePreviewInput struct {
	AssetPath string         `json:"assetPath" binding:"required"`
	Profile   models.JSONMap `json:"profile" binding:"required"`
}

type TrackProfilePreviewDecision struct {
	Index  int    `json:"index"`
	Type   string `json:"type"`
	Kept   bool   `json:"kept"`
	Reason string `json:"reason"`
}

type TrackProfileResolutionPreview struct {
	AssetPath           string                        `json:"assetPath"`
	KeepVideoStreams    []int                         `json:"keepVideoStreams"`
	KeepAudioStreams    []int                         `json:"keepAudioStreams"`
	KeepSubtitleStreams []int                         `json:"keepSubtitleStreams"`
	Video               []TrackProfilePreviewDecision `json:"video"`
	Audio               []TrackProfilePreviewDecision `json:"audio"`
	Subtitle            []TrackProfilePreviewDecision `json:"subtitle"`
	Warnings            []string                      `json:"warnings"`
	ResolvedTrackPlan   ResolvedTrackPlan             `json:"resolvedTrackPlan"`
}

func (h QueueHandler) PreviewTrackProfileResolution(c *gin.Context) {
	var input TrackProfilePreviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	path := filepath.Clean(strings.TrimSpace(input.AssetPath))
	if path == "." || !filepath.IsAbs(path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assetPath must be absolute"})
		return
	}
	profile := map[string]any(input.Profile)
	var scan models.ScanResult
	if err := h.db.Where("path = ?", path).Order("updated_at desc, id desc").First(&scan).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "a cached asset snapshot is required to preview Path track rules"})
		return
	}
	resolved := profile
	if storedSettingProfileScope(profile) == "path" {
		resolved = resolveTrackProfileForAsset(h.db, path, profile)
	}
	if _, err := canonicalTrackDispositionProfile(resolved); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	plan, err := resolveTrackPlan(scan, resolved)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, buildTrackProfileResolutionPreview(scan, resolved, plan))
}

func buildTrackProfileResolutionPreview(scan models.ScanResult, resolved map[string]any, plan ResolvedTrackPlan) TrackProfileResolutionPreview {
	videoKept, audioKept, subtitleKept := map[int]bool{}, map[int]bool{}, map[int]bool{}
	for _, stream := range plan.VideoStreams {
		videoKept[stream.StreamIndex] = true
	}
	for _, stream := range plan.AudioStreams {
		audioKept[stream.StreamIndex] = true
	}
	for _, stream := range plan.SubtitleStreams {
		if stream.Action.KeepsEmbedded() {
			subtitleKept[stream.StreamIndex] = true
		}
	}
	preview := TrackProfileResolutionPreview{
		AssetPath:        filepath.Clean(scan.Path),
		KeepVideoStreams: sortedIndexSet(videoKept), KeepAudioStreams: sortedIndexSet(audioKept), KeepSubtitleStreams: sortedIndexSet(subtitleKept),
		Video: []TrackProfilePreviewDecision{}, Audio: []TrackProfilePreviewDecision{}, Subtitle: []TrackProfilePreviewDecision{}, Warnings: []string{},
		ResolvedTrackPlan: plan,
	}
	preview.Warnings = append(preview.Warnings, plan.Warnings...)
	if plan.FontAttachmentExportPolicy == FontAttachmentExportNone && hasSupportedFontAttachments(plan.AttachmentStreams) {
		for _, sidecar := range plan.SidecarOutputs {
			if strings.EqualFold(sidecar.Mode, "original") && (strings.EqualFold(sidecar.Format, "ass") || strings.EqualFold(sidecar.Format, "ssa")) {
				preview.Warnings = append(preview.Warnings, fmt.Sprintf("Extracted %s subtitle stream %d may reference custom source fonts; font attachments are not exported by this operation.", strings.ToUpper(sidecar.Format), sidecar.StreamIndex))
			}
		}
	}
	videoPosition := 0
	for _, raw := range scan.VideoStreams {
		stream := settingProfileObject(raw)
		if stream == nil {
			continue
		}
		index := streamIndexValue(stream["index"])
		kept := videoKept[index]
		reason := "additional_video"
		if kept && workerStringValue(resolved["videoMode"]) == "all" {
			reason = "all_video"
		} else if kept && videoPosition == 0 {
			reason = "first_video"
		}
		preview.Video = append(preview.Video, TrackProfilePreviewDecision{Index: index, Type: "video", Kept: kept, Reason: reason})
		videoPosition++
	}
	for _, raw := range scan.AudioStreams {
		stream := settingProfileObject(raw)
		if stream == nil {
			continue
		}
		index := streamIndexValue(stream["index"])
		kept := audioKept[index]
		preview.Audio = append(preview.Audio, TrackProfilePreviewDecision{Index: index, Type: "audio", Kept: kept, Reason: audioTrackPreviewReason(stream, resolved, kept)})
	}
	for _, raw := range scan.SubtitleStreams {
		stream := settingProfileObject(raw)
		if stream == nil {
			continue
		}
		index := streamIndexValue(stream["index"])
		kept := subtitleKept[index]
		preview.Subtitle = append(preview.Subtitle, TrackProfilePreviewDecision{Index: index, Type: "subtitle", Kept: kept, Reason: subtitleTrackPreviewReason(stream, resolved, kept)})
	}
	if boolValue(resolved["audioRequired"], true) && len(preview.KeepAudioStreams) == 0 {
		preview.Warnings = append(preview.Warnings, "No audio stream matches this required Path rule.")
	}
	if boolValue(resolved["subtitlesRequired"], false) && len(preview.KeepSubtitleStreams) == 0 {
		preview.Warnings = append(preview.Warnings, "No subtitle stream matches this required Path rule.")
	}
	return preview
}

func hasSupportedFontAttachments(attachments []ResolvedAttachmentStream) bool {
	for _, attachment := range attachments {
		if strings.EqualFold(attachment.AttachmentKind, "FONT") && supportedFontAttachmentFormat(attachment.FontFormat) {
			return true
		}
	}
	return false
}

func resolvedStreamIndexSet(value any) map[int]bool {
	result := map[int]bool{}
	for _, raw := range workerSliceValue(value) {
		if index := streamIndexValue(raw); index >= 0 {
			result[index] = true
		}
	}
	return result
}

func sortedIndexSet(values map[int]bool) []int {
	result := make([]int, 0, len(values))
	for index := range values {
		result = append(result, index)
	}
	sort.Ints(result)
	return result
}

func audioTrackPreviewReason(stream, profile map[string]any, kept bool) string {
	if boolValue(profile["dropCommentary"], true) && (boolValue(stream["comment"], false) || strings.Contains(strings.ToLower(workerStringValue(stream["title"])), "commentary")) {
		return "commentary"
	}
	switch workerStringValue(profile["audioMode"]) {
	case "none":
		return "audio_disabled"
	case "default":
		if kept {
			return "default_track"
		}
		return "not_default"
	case "languages":
		if kept {
			return "language_match"
		}
		return "language_not_selected"
	default:
		return "all_audio"
	}
}

func subtitleTrackPreviewReason(stream, profile map[string]any, kept bool) string {
	mode := workerStringValue(profile["subtitleMode"])
	forced := boolValue(stream["forced"], false)
	if kept && forced && (mode == "forced" || mode == "forced-or-languages") {
		return "forced"
	}
	switch mode {
	case "none":
		return "subtitle_disabled"
	case "forced":
		return "not_forced"
	case "languages", "forced-or-languages":
		if kept {
			return "language_match"
		}
		return "language_not_selected"
	default:
		return "all_subtitles"
	}
}
