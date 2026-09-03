package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

type SubtitleArtifact struct {
	ArtifactID              string `json:"artifactId"`
	Type                    string `json:"type"`
	StreamIndex             int    `json:"streamIndex"`
	SourceCodec             string `json:"sourceCodec"`
	Format                  string `json:"format"`
	Mode                    string `json:"mode"`
	Language                string `json:"language"`
	Default                 bool   `json:"default"`
	Forced                  bool   `json:"forced"`
	Title                   string `json:"title,omitempty"`
	StagedPath              string `json:"stagedPath"`
	PublishedPath           string `json:"publishedPath,omitempty"`
	SizeBytes               int64  `json:"sizeBytes"`
	Status                  string `json:"status"`
	Error                   string `json:"error,omitempty"`
	DisplayName             string `json:"displayName,omitempty"`
	FontAttachmentsExported bool   `json:"fontAttachmentsExported"`
	OCRLanguage             string `json:"ocrLanguage,omitempty"`
	OCRMode                 string `json:"ocrMode,omitempty"`
}

type FontAttachmentArtifact struct {
	ArtifactID        string `json:"artifactId"`
	Type              string `json:"type"`
	StreamIndex       int    `json:"streamIndex"`
	AttachmentOrdinal int    `json:"attachmentOrdinal"`
	SourceCodec       string `json:"sourceCodec,omitempty"`
	OriginalName      string `json:"originalName,omitempty"`
	MIMEType          string `json:"mimeType,omitempty"`
	FontFormat        string `json:"fontFormat"`
	SafeFilename      string `json:"safeFilename"`
	RelativePath      string `json:"relativePath"`
	StagedPath        string `json:"stagedPath"`
	PublishedPath     string `json:"publishedPath,omitempty"`
	SizeBytes         int64  `json:"sizeBytes"`
	Status            string `json:"status"`
	Error             string `json:"error,omitempty"`
	DisplayName       string `json:"displayName,omitempty"`
}

type subtitleArtifactProgress func(SubtitleArtifact)

func subtitleArtifactID(streamIndex int, mode, format string) string {
	return fmt.Sprintf("subtitle:%d:%s:%s", streamIndex, fallback(strings.ToLower(strings.TrimSpace(mode)), "original"), strings.ToLower(strings.TrimSpace(format)))
}

func fontAttachmentArtifactID(streamIndex int) string {
	return fmt.Sprintf("font-attachment:%d", streamIndex)
}

func fontAttachmentRelativePath(mediaPath, safeFilename string) string {
	base := filepath.Base(strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath)))
	return filepath.Join(base+".fonts", safeFilename)
}

func plannedSubtitleArtifacts(job models.QueueJob) []SubtitleArtifact {
	plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || len(plan.SidecarOutputs) == 0 {
		return nil
	}
	base := strings.TrimSuffix(filepath.Base(job.MediaPath), filepath.Ext(job.MediaPath))
	used := map[string]struct{}{}
	artifacts := make([]SubtitleArtifact, 0, len(plan.SidecarOutputs))
	for _, decision := range plan.SidecarOutputs {
		artifact := SubtitleArtifact{
			ArtifactID: subtitleArtifactID(decision.StreamIndex, decision.Mode, decision.Format), Type: "subtitle_sidecar",
			StreamIndex: decision.StreamIndex, SourceCodec: decision.Codec, Format: decision.Format, Mode: decision.Mode,
			Language: decision.Language, Default: decision.Default, Forced: decision.Forced, Title: decision.Title, Status: "planned",
			OCRLanguage: decision.OCRLanguage, OCRMode: decision.OCRMode,
		}
		artifact.DisplayName = filepath.Base(resolvedSubtitleStagedPath(base, artifact, used))
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

func plannedFontAttachmentArtifacts(job models.QueueJob) []FontAttachmentArtifact {
	plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || len(plan.FontAttachments) == 0 {
		return nil
	}
	artifacts := make([]FontAttachmentArtifact, 0, len(plan.FontAttachments))
	for _, font := range plan.FontAttachments {
		relativePath := fontAttachmentRelativePath(job.MediaPath, font.SafeFilename)
		artifacts = append(artifacts, FontAttachmentArtifact{
			ArtifactID: font.ArtifactID, Type: "font_attachment", StreamIndex: font.StreamIndex, AttachmentOrdinal: font.AttachmentOrdinal,
			SourceCodec: font.Codec, OriginalName: font.OriginalName, MIMEType: font.MIMEType, FontFormat: font.FontFormat,
			SafeFilename: font.SafeFilename, RelativePath: relativePath, DisplayName: font.SafeFilename, Status: "planned",
		})
	}
	return artifacts
}

func plannedSidecarArtifactsJSON(job models.QueueJob) models.JSONList {
	return sidecarArtifactsJSON(plannedSubtitleArtifacts(job), plannedFontAttachmentArtifacts(job))
}

func generateSubtitleArtifacts(ctx context.Context, plan MediaJobPlan) ([]SubtitleArtifact, error) {
	return generateSubtitleArtifactsWithProgress(ctx, plan, nil)
}

func generateSubtitleArtifactsWithProgress(ctx context.Context, plan MediaJobPlan, progress subtitleArtifactProgress) ([]SubtitleArtifact, error) {
	// Persisted SubtitleTransforms predate original-representation extraction
	// and may intentionally request OCR or text conversion. Preserve that
	// compatibility path; new canonical sidecar decisions never synthesize a
	// transform and use stream-copy extraction below.
	if len(plan.Override.SubtitleTransforms) == 0 && plan.ResolvedTracks != nil {
		return generateResolvedSubtitleArtifacts(ctx, plan, progress)
	}
	if len(plan.Override.SubtitleTransforms) == 0 {
		return nil, nil
	}
	streams := map[int]MediaStream{}
	for _, stream := range plan.Streams.Subtitle {
		streams[stream.Index] = stream
	}
	base := strings.TrimSuffix(plan.OutputPath, filepath.Ext(plan.OutputPath))
	usedPaths := map[string]struct{}{}
	usedExistingSidecars := map[string]struct{}{}
	artifacts := make([]SubtitleArtifact, 0, len(plan.Override.SubtitleTransforms))
	completed := false
	defer func() {
		if completed {
			return
		}
		for _, artifact := range artifacts {
			_ = os.Remove(artifact.StagedPath)
		}
	}()
	var bitmapStreams map[int]FFProbeStream
	for _, transform := range plan.Override.SubtitleTransforms {
		stream, exists := streams[transform.StreamIndex]
		if !exists {
			return nil, fmt.Errorf("subtitle stream %d disappeared before extraction", transform.StreamIndex)
		}
		language := safeSubtitleFilenamePart(transform.Language)
		if language == "" {
			language = safeSubtitleFilenamePart(stream.Language)
		}
		if language == "" {
			language = "und"
		}
		if !plan.IndependentSubtitleArtifacts && existingSubtitleTransformSidecar(plan.SourceAssetPath, transform, language, usedExistingSidecars) {
			continue
		}
		suffix := "." + language
		if transform.MakeDefault {
			suffix += ".default"
		}
		stagedPath := base + suffix + "." + transform.Format
		if _, duplicate := usedPaths[stagedPath]; duplicate {
			stagedPath = fmt.Sprintf("%s.%s.%d.%s", base, language, stream.Index, transform.Format)
		}
		usedPaths[stagedPath] = struct{}{}
		_ = os.Remove(stagedPath)
		if isBitmapSubtitleCodecName(stream.Codec) {
			if bitmapStreams == nil {
				probed, probeErr := probeSubtitleStreams(ctx, plan.InputPath)
				if probeErr != nil {
					return nil, fmt.Errorf("OCR subtitle scan failed: %w", probeErr)
				}
				bitmapStreams = map[int]FFProbeStream{}
				for _, candidate := range probed {
					bitmapStreams[candidate.Index] = candidate
				}
			}
			bitmapStream, found := bitmapStreams[stream.Index]
			if !found {
				return nil, fmt.Errorf("bitmap subtitle stream %d disappeared before OCR", stream.Index)
			}
			ocrLanguage := transform.OCRLanguage
			if strings.TrimSpace(ocrLanguage) == "" {
				ocrLanguage = transform.Language
			}
			if err := generateBitmapSubtitleAtPath(ctx, plan.InputPath, bitmapStream, transform.Format, ocrLanguage, transform.OCRMode, stagedPath, plan.SegmentStartSeconds, plan.SegmentDurationSeconds); err != nil {
				return nil, err
			}
		} else if !subtitleCanConvertText(stream.Codec) {
			return nil, fmt.Errorf("subtitle stream %d (%s) cannot be converted to %s", stream.Index, stream.Codec, strings.ToUpper(transform.Format))
		} else {
			args := textSubtitleExtractionArgs(plan, stream.Index, transform.Format, stagedPath)
			cmd := exec.CommandContext(ctx, "ffmpeg", args...)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				_ = os.Remove(stagedPath)
				return nil, fmt.Errorf("subtitle stream %d extraction failed: %s", stream.Index, fallback(strings.TrimSpace(stderr.String()), err.Error()))
			}
		}
		info, err := os.Stat(stagedPath)
		if err != nil || info.IsDir() || info.Size() == 0 {
			_ = os.Remove(stagedPath)
			if emptySubtitleArtifactCanBeSkipped(plan) {
				continue
			}
			return nil, fmt.Errorf("subtitle stream %d produced an empty sidecar", stream.Index)
		}
		content, err := os.ReadFile(stagedPath)
		if err != nil || !validSubtitleSidecar(transform.Format, content) {
			_ = os.Remove(stagedPath)
			return nil, fmt.Errorf("subtitle stream %d produced an invalid %s sidecar", stream.Index, strings.ToUpper(transform.Format))
		}
		artifacts = append(artifacts, SubtitleArtifact{
			Type: "subtitle_sidecar", StreamIndex: stream.Index, SourceCodec: stream.Codec, Format: transform.Format,
			Language: language, Default: transform.MakeDefault, StagedPath: stagedPath, SizeBytes: info.Size(), Status: "ready",
		})
	}
	completed = true
	return artifacts, nil
}

func generateResolvedSubtitleArtifacts(ctx context.Context, plan MediaJobPlan, progress subtitleArtifactProgress) ([]SubtitleArtifact, error) {
	if plan.ResolvedTracks == nil || len(plan.ResolvedTracks.SidecarOutputs) == 0 {
		return nil, nil
	}
	streams := map[int]MediaStream{}
	for _, stream := range plan.Streams.Subtitle {
		streams[stream.Index] = stream
	}
	base := strings.TrimSuffix(plan.OutputPath, filepath.Ext(plan.OutputPath))
	usedPaths := map[string]struct{}{}
	artifacts := make([]SubtitleArtifact, 0, len(plan.ResolvedTracks.SidecarOutputs))
	var bitmapStreams map[int]FFProbeStream
	completed := false
	defer func() {
		if completed {
			return
		}
		for index, artifact := range artifacts {
			if artifact.StagedPath != "" {
				_ = os.Remove(artifact.StagedPath)
			}
			if artifacts[index].Status == "ready" {
				artifacts[index].Status = "rolled_back"
				artifacts[index].Error = "sidecar set rolled back after another extraction failed"
			}
		}
	}()
	for _, decision := range plan.ResolvedTracks.SidecarOutputs {
		stream, exists := streams[decision.StreamIndex]
		artifact := SubtitleArtifact{
			ArtifactID:  subtitleArtifactID(decision.StreamIndex, decision.Mode, decision.Format),
			Type:        "subtitle_sidecar",
			StreamIndex: decision.StreamIndex, SourceCodec: decision.Codec, Format: decision.Format,
			Language: decision.Language, Default: decision.Default, Forced: decision.Forced, Title: decision.Title, Mode: decision.Mode,
			OCRLanguage: decision.OCRLanguage, OCRMode: decision.OCRMode,
			Status: "planned",
		}
		artifacts = append(artifacts, artifact)
		current := &artifacts[len(artifacts)-1]
		if !exists {
			current.Status, current.Error = "failed", fmt.Sprintf("subtitle stream %d disappeared before extraction", decision.StreamIndex)
			return artifacts, fmt.Errorf("%s", current.Error)
		}
		format := strings.ToLower(strings.TrimSpace(decision.Format))
		mode := strings.ToLower(strings.TrimSpace(decision.Mode))
		if mode == "" {
			mode = "original"
		}
		executionKind := resolvedSubtitleExecutionKind(stream.Codec, mode)
		if mode == "original" {
			originalFormat, _, supported := originalSubtitleExtractionFormat(stream.Codec)
			if !supported || (format != "" && !strings.EqualFold(format, originalFormat)) {
				current.Status = "unsupported"
				current.Error = fmt.Sprintf("subtitle stream %d codec %s does not support original %s sidecar extraction", stream.Index, stream.Codec, format)
				return artifacts, fmt.Errorf("%s", current.Error)
			}
			format = originalFormat
		} else if format != "srt" || executionKind == "unsupported" {
			current.Status = "unsupported"
			current.Error = fmt.Sprintf("subtitle stream %d codec %s cannot generate converted %s sidecar", stream.Index, stream.Codec, format)
			return artifacts, fmt.Errorf("%s", current.Error)
		}
		current.Format, current.Mode = format, mode
		current.SourceCodec = stream.Codec
		if current.Language == "" {
			current.Language = stream.Language
		}
		current.StagedPath = resolvedSubtitleStagedPath(base, *current, usedPaths)
		current.DisplayName = filepath.Base(current.StagedPath)
		_ = os.Remove(current.StagedPath)
		current.Status = "generating"
		if progress != nil {
			progress(*current)
		}
		if executionKind == "bitmap_ocr" {
			bitmapStream, probeErr := bitmapSubtitleStreamByIndex(ctx, plan.InputPath, stream.Index, &bitmapStreams)
			if probeErr != nil {
				current.Status, current.Error = "failed", probeErr.Error()
				return artifacts, probeErr
			}
			ocrLanguage := decision.OCRLanguage
			if strings.EqualFold(strings.TrimSpace(ocrLanguage), "auto") {
				ocrLanguage = ""
			}
			if err := generateBitmapSubtitleAtPath(ctx, plan.InputPath, bitmapStream, format, ocrLanguage, decision.OCRMode, current.StagedPath, plan.SegmentStartSeconds, plan.SegmentDurationSeconds); err != nil {
				current.Status, current.Error = "failed", err.Error()
				if progress != nil {
					progress(*current)
				}
				return artifacts, err
			}
		} else {
			// Build arguments after the deterministic output path is known.
			var args []string
			if mode == "original" {
				_, muxer, _ := originalSubtitleExtractionFormat(stream.Codec)
				args = originalSubtitleExtractionArgs(plan, stream.Index, muxer, current.StagedPath)
			} else {
				args = textSubtitleExtractionArgs(plan, stream.Index, format, current.StagedPath)
			}
			cmd := exec.CommandContext(ctx, "ffmpeg", args...)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				current.Status = "failed"
				current.Error = fmt.Sprintf("subtitle stream %d extraction failed: %s", stream.Index, fallback(strings.TrimSpace(stderr.String()), err.Error()))
				if progress != nil {
					progress(*current)
				}
				return artifacts, fmt.Errorf("%s", current.Error)
			}
		}
		info, err := os.Stat(current.StagedPath)
		if err != nil || info.IsDir() || info.Size() == 0 {
			_ = os.Remove(current.StagedPath)
			if emptySubtitleArtifactCanBeSkipped(plan) {
				artifacts = artifacts[:len(artifacts)-1]
				continue
			}
			current.Status, current.Error = "failed", fmt.Sprintf("subtitle stream %d produced an empty original sidecar", stream.Index)
			return artifacts, fmt.Errorf("%s", current.Error)
		}
		content, readErr := os.ReadFile(current.StagedPath)
		if readErr != nil || !validOriginalSubtitleSidecar(format, content) {
			current.Status, current.Error = "failed", fmt.Sprintf("subtitle stream %d produced an invalid %s sidecar", stream.Index, strings.ToUpper(format))
			return artifacts, fmt.Errorf("%s", current.Error)
		}
		current.SizeBytes, current.Status = info.Size(), "ready"
		if progress != nil {
			progress(*current)
		}
	}
	completed = true
	return artifacts, nil
}

func resolvedSubtitleExecutionKind(codec, mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "original") || strings.TrimSpace(mode) == "" {
		return "original"
	}
	if !strings.EqualFold(strings.TrimSpace(mode), "converted") {
		return "unsupported"
	}
	if isBitmapSubtitleCodecName(codec) {
		return "bitmap_ocr"
	}
	if subtitleCanConvertText(codec) {
		return "text_ffmpeg"
	}
	return "unsupported"
}

func bitmapSubtitleStreamByIndex(ctx context.Context, inputPath string, streamIndex int, cached *map[int]FFProbeStream) (FFProbeStream, error) {
	if *cached == nil {
		streams, err := probeSubtitleStreams(ctx, inputPath)
		if err != nil {
			return FFProbeStream{}, fmt.Errorf("OCR subtitle scan failed: %w", err)
		}
		*cached = map[int]FFProbeStream{}
		for _, stream := range streams {
			if isBitmapSubtitleCodecName(stream.CodecName) {
				(*cached)[stream.Index] = stream
			}
		}
	}
	if stream, ok := (*cached)[streamIndex]; ok {
		return stream, nil
	}
	return FFProbeStream{}, fmt.Errorf("bitmap subtitle stream %d disappeared before OCR", streamIndex)
}

func originalSubtitleExtractionFormat(codec string) (format, muxer string, supported bool) {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "ass":
		return "ass", "ass", true
	case "ssa":
		return "ssa", "ass", true
	case "subrip", "srt":
		return "srt", "srt", true
	case "hdmv_pgs_subtitle", "pgs", "pgssub":
		return "sup", "sup", true
	default:
		return "", "", false
	}
}

func originalSubtitleExtractionArgs(plan MediaJobPlan, streamIndex int, muxer, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	inputSeek, outputTrim := segmentedSeekWindow(plan.SegmentStartSeconds)
	if inputSeek > 0 {
		args = append(args, "-ss", fmt.Sprintf("%g", inputSeek))
	}
	args = append(args, "-i", plan.InputPath)
	if outputTrim > 0 {
		args = append(args, "-ss", fmt.Sprintf("%g", outputTrim))
	}
	if plan.SegmentDurationSeconds > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", plan.SegmentDurationSeconds))
	}
	return append(args, "-map", fmt.Sprintf("0:%d", streamIndex), "-vn", "-an", "-c:s", "copy", "-f", muxer, outputPath)
}

func fontAttachmentExtractionArgs(inputPath string, attachmentOrdinal int, outputPath string) []string {
	return []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		fmt.Sprintf("-dump_attachment:t:%d", attachmentOrdinal), outputPath,
		"-i", inputPath, "-map", "0:v:0?", "-frames:v", "0", "-f", "null", "-",
	}
}

func generateFontAttachmentArtifactsWithProgress(
	ctx context.Context,
	plan MediaJobPlan,
	frozenArtifacts []FontAttachmentArtifact,
	progress func(FontAttachmentArtifact),
) ([]FontAttachmentArtifact, error) {
	if len(frozenArtifacts) == 0 {
		return nil, nil
	}

	base := strings.TrimSuffix(plan.OutputPath, filepath.Ext(plan.OutputPath)) + ".fonts"
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("prepare font attachment staging directory: %w", err)
	}

	artifacts := make([]FontAttachmentArtifact, 0, len(frozenArtifacts))
	completed := false

	defer func() {
		if completed {
			return
		}

		for index := range artifacts {
			if artifacts[index].StagedPath != "" {
				_ = os.Remove(artifacts[index].StagedPath)
			}
			if artifacts[index].Status == "ready" {
				artifacts[index].Status = "rolled_back"
				artifacts[index].Error = "font attachment set rolled back after another extraction failed"
			}
		}

		_ = os.Remove(base)
	}()

	for _, frozen := range frozenArtifacts {
		artifact := frozen
		artifact.StagedPath = filepath.Join(base, artifact.SafeFilename)
		artifact.Status = "generating"
		artifact.Error = ""
		artifact.SizeBytes = 0

		artifacts = append(artifacts, artifact)
		current := &artifacts[len(artifacts)-1]

		_ = os.Remove(current.StagedPath)

		if progress != nil {
			progress(*current)
		}

		cmd := exec.CommandContext(
			ctx,
			"ffmpeg",
			fontAttachmentExtractionArgs(
				plan.InputPath,
				current.AttachmentOrdinal,
				current.StagedPath,
			)...,
		)

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			current.Status = "failed"
			current.Error = fmt.Sprintf(
				"font attachment stream %d extraction failed: %s",
				current.StreamIndex,
				fallback(strings.TrimSpace(stderr.String()), err.Error()),
			)

			if progress != nil {
				progress(*current)
			}

			return artifacts, fmt.Errorf("%s", current.Error)
		}

		info, err := os.Stat(current.StagedPath)
		if err != nil || info.IsDir() || info.Size() == 0 {
			_ = os.Remove(current.StagedPath)
			current.Status = "failed"
			current.Error = fmt.Sprintf(
				"font attachment stream %d produced an empty artifact",
				current.StreamIndex,
			)
			return artifacts, fmt.Errorf("%s", current.Error)
		}

		current.SizeBytes = info.Size()
		current.Status = "ready"

		if progress != nil {
			progress(*current)
		}
	}

	completed = true
	return artifacts, nil
}

func resolvedSubtitleStagedPath(base string, artifact SubtitleArtifact, used map[string]struct{}) string {
	language := safeSubtitleFilenamePart(artifact.Language)
	if language == "" {
		language = "und"
	}
	suffix := "." + language
	if artifact.Forced {
		suffix += ".forced"
	} else if artifact.Default {
		suffix += ".default"
	}
	path := base + suffix + "." + artifact.Format
	if _, duplicate := used[path]; duplicate {
		suffix = fmt.Sprintf(".%s.%d", language, artifact.StreamIndex)
		if artifact.Forced {
			suffix += ".forced"
		} else if artifact.Default {
			suffix += ".default"
		}
		path = base + suffix + "." + artifact.Format
	}
	used[path] = struct{}{}
	return path
}

func validOriginalSubtitleSidecar(format string, content []byte) bool {
	if len(content) == 0 {
		return false
	}
	switch format {
	case "ass", "ssa":
		return strings.Contains(string(content), "[Events]") && strings.Contains(string(content), "Dialogue:")
	case "srt":
		return strings.Contains(string(content), "-->")
	case "sup":
		return len(content) >= 2 && content[0] == 'P' && content[1] == 'G'
	default:
		return false
	}
}

func emptySubtitleArtifactCanBeSkipped(plan MediaJobPlan) bool {
	return plan.AllowEmptySubtitleArtifacts && plan.SegmentDurationSeconds > 0
}

func textSubtitleExtractionArgs(plan MediaJobPlan, streamIndex int, format, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	inputSeek, outputTrim := segmentedSeekWindow(plan.SegmentStartSeconds)
	if inputSeek > 0 {
		args = append(args, "-ss", fmt.Sprintf("%g", inputSeek))
	}
	args = append(args, "-i", plan.InputPath)
	if outputTrim > 0 {
		args = append(args, "-ss", fmt.Sprintf("%g", outputTrim))
	}
	if plan.SegmentDurationSeconds > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", plan.SegmentDurationSeconds))
	}
	return append(args,
		"-map", fmt.Sprintf("0:%d", streamIndex),
		"-vn", "-an", "-c:s", format, "-f", format, outputPath,
	)
}

func existingSubtitleTransformSidecar(mediaPath string, transform SubtitleTransform, language string, used map[string]struct{}) bool {
	mediaPath = strings.TrimSpace(mediaPath)
	if mediaPath == "" {
		return false
	}
	values, err := externalSubtitlesForMedia(mediaPath)
	if err != nil {
		return false
	}
	for _, sidecar := range values {
		if !strings.EqualFold(sidecar.Format, transform.Format) || !strings.EqualFold(sidecar.Language, language) || sidecar.Default != transform.MakeDefault {
			continue
		}
		if _, alreadyUsed := used[sidecar.Path]; alreadyUsed {
			continue
		}
		content, err := os.ReadFile(sidecar.Path)
		if err == nil && validSubtitleSidecar(transform.Format, content) {
			used[sidecar.Path] = struct{}{}
			return true
		}
	}
	return false
}

func validSubtitleSidecar(format string, content []byte) bool {
	text := string(content)
	if format == "ass" {
		return strings.Contains(text, "[Events]") && strings.Contains(text, "Dialogue:")
	}
	return strings.Contains(text, "-->")
}

func subtitleArtifactsJSON(values []SubtitleArtifact) models.JSONList {
	if len(values) == 0 {
		return nil
	}
	result := make(models.JSONList, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]interface{}{
			"artifactId": value.ArtifactID,
			"type":       value.Type, "streamIndex": value.StreamIndex, "sourceCodec": value.SourceCodec, "format": value.Format, "mode": value.Mode,
			"language": value.Language, "default": value.Default, "forced": value.Forced, "title": value.Title,
			"ocrLanguage": value.OCRLanguage, "ocrMode": value.OCRMode,
			"stagedPath": value.StagedPath, "publishedPath": value.PublishedPath, "sizeBytes": value.SizeBytes,
			"status": value.Status, "error": value.Error, "displayName": value.DisplayName, "fontAttachmentsExported": value.FontAttachmentsExported,
		})
	}
	return result
}

func fontAttachmentArtifactsJSON(values []FontAttachmentArtifact) models.JSONList {
	if len(values) == 0 {
		return nil
	}
	result := make(models.JSONList, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]interface{}{
			"artifactId": value.ArtifactID, "type": "font_attachment", "streamIndex": value.StreamIndex,
			"attachmentOrdinal": value.AttachmentOrdinal, "sourceCodec": value.SourceCodec, "originalName": value.OriginalName,
			"mimeType": value.MIMEType, "fontFormat": value.FontFormat, "safeFilename": value.SafeFilename,
			"relativePath": value.RelativePath, "stagedPath": value.StagedPath, "publishedPath": value.PublishedPath,
			"sizeBytes": value.SizeBytes, "status": value.Status, "error": value.Error, "displayName": value.DisplayName,
		})
	}
	return result
}

func sidecarArtifactsJSON(subtitles []SubtitleArtifact, fonts []FontAttachmentArtifact) models.JSONList {
	values := models.JSONList{}
	values = append(values, subtitleArtifactsJSON(subtitles)...)
	values = append(values, fontAttachmentArtifactsJSON(fonts)...)
	if len(values) == 0 {
		return nil
	}
	return values
}

func subtitleArtifactsFromJSON(values models.JSONList) []SubtitleArtifact {
	result := []SubtitleArtifact{}
	for _, raw := range values {
		value, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		artifactType := strings.TrimSpace(stringFromUnknown(value["type"]))
		if artifactType != "" && artifactType != "subtitle_sidecar" {
			continue
		}
		result = append(result, SubtitleArtifact{
			ArtifactID:              stringFromUnknown(value["artifactId"]),
			Type:                    fallback(stringFromUnknown(value["type"]), "subtitle_sidecar"),
			StreamIndex:             intValueSetting(value["streamIndex"], -1),
			SourceCodec:             stringFromUnknown(value["sourceCodec"]),
			Format:                  stringFromUnknown(value["format"]),
			Mode:                    stringFromUnknown(value["mode"]),
			Language:                stringFromUnknown(value["language"]),
			Default:                 boolSetting(value["default"], false),
			Forced:                  boolSetting(value["forced"], false),
			Title:                   stringFromUnknown(value["title"]),
			StagedPath:              stringFromUnknown(value["stagedPath"]),
			PublishedPath:           stringFromUnknown(value["publishedPath"]),
			SizeBytes:               int64(intValueSetting(value["sizeBytes"], 0)),
			Status:                  stringFromUnknown(value["status"]),
			Error:                   stringFromUnknown(value["error"]),
			DisplayName:             stringFromUnknown(value["displayName"]),
			FontAttachmentsExported: boolSetting(value["fontAttachmentsExported"], false),
			OCRLanguage:             stringFromUnknown(value["ocrLanguage"]), OCRMode: stringFromUnknown(value["ocrMode"]),
		})
	}
	return result
}

func fontAttachmentArtifactsFromJSON(values models.JSONList) []FontAttachmentArtifact {
	result := []FontAttachmentArtifact{}
	for _, raw := range values {
		value, ok := raw.(map[string]interface{})
		if !ok || stringFromUnknown(value["type"]) != "font_attachment" {
			continue
		}
		result = append(result, FontAttachmentArtifact{
			ArtifactID: stringFromUnknown(value["artifactId"]), Type: "font_attachment",
			StreamIndex: intValueSetting(value["streamIndex"], -1), AttachmentOrdinal: intValueSetting(value["attachmentOrdinal"], -1),
			SourceCodec: stringFromUnknown(value["sourceCodec"]), OriginalName: stringFromUnknown(value["originalName"]), MIMEType: stringFromUnknown(value["mimeType"]),
			FontFormat: stringFromUnknown(value["fontFormat"]), SafeFilename: stringFromUnknown(value["safeFilename"]), RelativePath: stringFromUnknown(value["relativePath"]),
			StagedPath: stringFromUnknown(value["stagedPath"]), PublishedPath: stringFromUnknown(value["publishedPath"]), SizeBytes: int64(intValueSetting(value["sizeBytes"], 0)),
			Status: stringFromUnknown(value["status"]), Error: stringFromUnknown(value["error"]), DisplayName: stringFromUnknown(value["displayName"]),
		})
	}
	return result
}

func mergeFontAttachmentArtifactProgress(current []FontAttachmentArtifact, update FontAttachmentArtifact) []FontAttachmentArtifact {
	if update.ArtifactID == "" {
		update.ArtifactID = fontAttachmentArtifactID(update.StreamIndex)
	}

	for index := range current {
		if current[index].ArtifactID == update.ArtifactID {
			current[index].StagedPath = update.StagedPath
			current[index].PublishedPath = update.PublishedPath
			current[index].SizeBytes = update.SizeBytes
			current[index].Status = update.Status
			current[index].Error = update.Error
			return current
		}
	}

	return append(current, update)
}

func mergeSubtitleArtifactProgress(current []SubtitleArtifact, update SubtitleArtifact) []SubtitleArtifact {
	if update.ArtifactID == "" {
		update.ArtifactID = subtitleArtifactID(update.StreamIndex, update.Mode, update.Format)
	}
	for index := range current {
		artifactID := current[index].ArtifactID
		if artifactID == "" {
			artifactID = subtitleArtifactID(current[index].StreamIndex, current[index].Mode, current[index].Format)
		}
		if artifactID == update.ArtifactID {
			current[index] = update
			return current
		}
	}
	return append(current, update)
}

func publishSubtitleArtifacts(
	job *models.QueueJob,
	destinationMediaPath string,
	overwrite bool,
	backups *[]publishBackup,
) ([]string, error) {
	artifacts := subtitleArtifactsFromJSON(job.SubtitleArtifacts)
	if len(artifacts) == 0 {
		return nil, nil
	}
	base := strings.TrimSuffix(destinationMediaPath, filepath.Ext(destinationMediaPath))
	destinations := make([]string, len(artifacts))
	alreadyPublished := make([]bool, len(artifacts))
	usedDestinations := map[string]struct{}{}
	for index, artifact := range artifacts {
		if artifact.Status != "" && artifact.Status != "ready" {
			return nil, fmt.Errorf("subtitle artifact for stream %d is not ready: %s", artifact.StreamIndex, fallback(artifact.Error, artifact.Status))
		}
		if info, err := os.Stat(artifact.StagedPath); err != nil || info.IsDir() || info.Size() == 0 {
			return nil, fmt.Errorf("staged subtitle artifact is missing or empty: %s", artifact.StagedPath)
		}
		language := safeSubtitleFilenamePart(artifact.Language)
		if language == "" {
			language = "und"
		}
		suffix := "." + language
		if artifact.Forced {
			suffix += ".forced"
		} else if artifact.Default {
			suffix += ".default"
		}
		suffix += "." + artifact.Format
		preferredDestination := base + suffix
		if _, duplicate := usedDestinations[preferredDestination]; duplicate {
			suffix = fmt.Sprintf(".%s.%d", language, artifact.StreamIndex)
			if artifact.Forced {
				suffix += ".forced"
			} else if artifact.Default {
				suffix += ".default"
			}
			suffix += "." + artifact.Format
			preferredDestination = base + suffix
			for discriminator := 2; ; discriminator++ {
				if _, duplicate = usedDestinations[preferredDestination]; !duplicate {
					break
				}
				suffix = fmt.Sprintf(".%s.%d.%d", language, artifact.StreamIndex, discriminator)
				if artifact.Forced {
					suffix += ".forced"
				} else if artifact.Default {
					suffix += ".default"
				}
				suffix += "." + artifact.Format
				preferredDestination = base + suffix
			}
		}
		usedDestinations[preferredDestination] = struct{}{}
		if !overwrite {
			resolved, exists, err := resolveSidecarDestination(artifact.StagedPath, base, suffix)
			if err != nil {
				return nil, err
			}
			destinations[index] = resolved
			alreadyPublished[index] = exists
			if resolved != preferredDestination {
				job.Notes = appendNote(job.Notes, "Generated subtitle sidecar renamed to preserve existing Library content: "+resolved)
			}
		} else {
			destinations[index] = preferredDestination
		}
	}
	published := []string{}
	for index, artifact := range artifacts {
		if !alreadyPublished[index] {
			destinationHadBackup := false

			if overwrite && backups != nil {
				var err error

				destinationHadBackup, err = ensurePublishBackup(
					destinations[index],
					backups,
				)
				if err != nil {
					return published, err
				}
			}
			if err := copyPublishedFile(artifact.StagedPath, destinations[index], overwrite); err != nil {
				return published, err
			}

			if !overwrite || !destinationHadBackup {
				published = append(
					published,
					destinations[index],
				)
			}
		}
		artifacts[index].PublishedPath = destinations[index]
	}
	job.SubtitleArtifacts = sidecarArtifactsJSON(artifacts, fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts))
	return published, nil
}

func publishFontAttachmentArtifacts(job *models.QueueJob, destinationMediaPath string, overwrite bool, backups *[]publishBackup) ([]string, error) {
	artifacts := fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts)
	if len(artifacts) == 0 {
		return nil, nil
	}
	destinationDir := strings.TrimSuffix(destinationMediaPath, filepath.Ext(destinationMediaPath)) + ".fonts"
	directoryCreated := false
	if info, err := os.Stat(destinationDir); os.IsNotExist(err) {
		if err := os.MkdirAll(destinationDir, 0o755); err != nil {
			return nil, err
		}
		directoryCreated = true
	} else if err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("font attachment destination is not a directory: %s", destinationDir)
	}
	published := []string{}
	if directoryCreated {
		published = append(published, destinationDir)
	}
	for index, artifact := range artifacts {
		if artifact.Status != "" && artifact.Status != "ready" {
			return published, fmt.Errorf("font attachment for stream %d is not ready: %s", artifact.StreamIndex, fallback(artifact.Error, artifact.Status))
		}
		if info, err := os.Stat(artifact.StagedPath); err != nil || info.IsDir() || info.Size() == 0 {
			return published, fmt.Errorf("staged font attachment is missing or empty: %s", artifact.StagedPath)
		}
		destination := filepath.Join(destinationDir, artifact.SafeFilename)
		if !overwrite {
			if _, err := os.Stat(destination); err == nil {
				return published, fmt.Errorf("font attachment destination already exists: %s", destination)
			} else if !os.IsNotExist(err) {
				return published, err
			}
		} else if backups != nil {
			hadBackup, err := ensurePublishBackup(destination, backups)
			if err != nil {
				return published, err
			}
			if hadBackup {
				if err := copyPublishedFile(artifact.StagedPath, destination, true); err != nil {
					return published, err
				}
				artifacts[index].PublishedPath = destination
				continue
			}
		}
		if err := copyPublishedFile(artifact.StagedPath, destination, overwrite); err != nil {
			return published, err
		}
		published = append(published, destination)
		artifacts[index].PublishedPath = destination
	}
	job.SubtitleArtifacts = sidecarArtifactsJSON(subtitleArtifactsFromJSON(job.SubtitleArtifacts), artifacts)
	return published, nil
}
