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
	Type                    string `json:"type"`
	StreamIndex             int    `json:"streamIndex"`
	SourceCodec             string `json:"sourceCodec"`
	Format                  string `json:"format"`
	Language                string `json:"language"`
	Default                 bool   `json:"default"`
	Forced                  bool   `json:"forced"`
	Title                   string `json:"title,omitempty"`
	StagedPath              string `json:"stagedPath"`
	PublishedPath           string `json:"publishedPath,omitempty"`
	SizeBytes               int64  `json:"sizeBytes"`
	Status                  string `json:"status"`
	Error                   string `json:"error,omitempty"`
	FontAttachmentsExported bool   `json:"fontAttachmentsExported"`
}

func generateSubtitleArtifacts(ctx context.Context, plan MediaJobPlan) ([]SubtitleArtifact, error) {
	// Persisted SubtitleTransforms predate original-representation extraction
	// and may intentionally request OCR or text conversion. Preserve that
	// compatibility path; new canonical sidecar decisions never synthesize a
	// transform and use stream-copy extraction below.
	if len(plan.Override.SubtitleTransforms) == 0 && plan.ResolvedTracks != nil {
		return generateResolvedSubtitleArtifacts(ctx, plan)
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

func generateResolvedSubtitleArtifacts(ctx context.Context, plan MediaJobPlan) ([]SubtitleArtifact, error) {
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
			Type:        "subtitle_sidecar",
			StreamIndex: decision.StreamIndex, SourceCodec: decision.Codec, Format: decision.Format,
			Language: decision.Language, Default: decision.Default, Forced: decision.Forced, Title: decision.Title,
			Status: "planned",
		}
		artifacts = append(artifacts, artifact)
		current := &artifacts[len(artifacts)-1]
		if !exists {
			current.Status, current.Error = "failed", fmt.Sprintf("subtitle stream %d disappeared before extraction", decision.StreamIndex)
			return artifacts, fmt.Errorf("%s", current.Error)
		}
		format, muxer, supported := originalSubtitleExtractionFormat(stream.Codec)
		if !supported || (decision.Format != "" && !strings.EqualFold(decision.Format, format)) {
			current.Status = "unsupported"
			current.Error = fmt.Sprintf("subtitle stream %d codec %s does not support original sidecar extraction", stream.Index, stream.Codec)
			return artifacts, fmt.Errorf("%s", current.Error)
		}
		current.Format = format
		current.SourceCodec = stream.Codec
		if current.Language == "" {
			current.Language = stream.Language
		}
		current.StagedPath = resolvedSubtitleStagedPath(base, *current, usedPaths)
		_ = os.Remove(current.StagedPath)
		args := originalSubtitleExtractionArgs(plan, stream.Index, muxer, current.StagedPath)
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			current.Status = "failed"
			current.Error = fmt.Sprintf("subtitle stream %d original extraction failed: %s", stream.Index, fallback(strings.TrimSpace(stderr.String()), err.Error()))
			return artifacts, fmt.Errorf("%s", current.Error)
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
	}
	completed = true
	return artifacts, nil
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
			"type": value.Type, "streamIndex": value.StreamIndex, "sourceCodec": value.SourceCodec, "format": value.Format,
			"language": value.Language, "default": value.Default, "forced": value.Forced, "title": value.Title,
			"stagedPath": value.StagedPath, "publishedPath": value.PublishedPath, "sizeBytes": value.SizeBytes,
			"status": value.Status, "error": value.Error, "fontAttachmentsExported": value.FontAttachmentsExported,
		})
	}
	return result
}

func subtitleArtifactsFromJSON(values models.JSONList) []SubtitleArtifact {
	result := []SubtitleArtifact{}
	for _, raw := range values {
		value, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, SubtitleArtifact{
			Type:                    fallback(stringFromUnknown(value["type"]), "subtitle_sidecar"),
			StreamIndex:             intValueSetting(value["streamIndex"], -1),
			SourceCodec:             stringFromUnknown(value["sourceCodec"]),
			Format:                  stringFromUnknown(value["format"]),
			Language:                stringFromUnknown(value["language"]),
			Default:                 boolSetting(value["default"], false),
			Forced:                  boolSetting(value["forced"], false),
			Title:                   stringFromUnknown(value["title"]),
			StagedPath:              stringFromUnknown(value["stagedPath"]),
			PublishedPath:           stringFromUnknown(value["publishedPath"]),
			SizeBytes:               int64(intValueSetting(value["sizeBytes"], 0)),
			Status:                  stringFromUnknown(value["status"]),
			Error:                   stringFromUnknown(value["error"]),
			FontAttachmentsExported: boolSetting(value["fontAttachmentsExported"], false),
		})
	}
	return result
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
	job.SubtitleArtifacts = subtitleArtifactsJSON(artifacts)
	return published, nil
}
