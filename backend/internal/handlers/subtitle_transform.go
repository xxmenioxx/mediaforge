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
	StreamIndex   int    `json:"streamIndex"`
	SourceCodec   string `json:"sourceCodec"`
	Format        string `json:"format"`
	Language      string `json:"language"`
	Default       bool   `json:"default"`
	StagedPath    string `json:"stagedPath"`
	PublishedPath string `json:"publishedPath,omitempty"`
	SizeBytes     int64  `json:"sizeBytes"`
}

func generateSubtitleArtifacts(ctx context.Context, plan MediaJobPlan) ([]SubtitleArtifact, error) {
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
		if existingSubtitleTransformSidecar(plan.SourceAssetPath, transform, language, usedExistingSidecars) {
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
			if err := generateBitmapSubtitleAtPath(ctx, plan.InputPath, bitmapStream, transform.Format, ocrLanguage, transform.OCRMode, stagedPath); err != nil {
				return nil, err
			}
		} else if !subtitleCanConvertText(stream.Codec) {
			return nil, fmt.Errorf("subtitle stream %d (%s) cannot be converted to %s", stream.Index, stream.Codec, strings.ToUpper(transform.Format))
		} else {
			args := []string{
				"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
				"-i", plan.InputPath,
				"-map", fmt.Sprintf("0:%d", stream.Index),
				"-vn", "-an",
				"-c:s", transform.Format,
				"-f", transform.Format,
				stagedPath,
			}
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
			return nil, fmt.Errorf("subtitle stream %d produced an empty sidecar", stream.Index)
		}
		content, err := os.ReadFile(stagedPath)
		if err != nil || !validSubtitleSidecar(transform.Format, content) {
			_ = os.Remove(stagedPath)
			return nil, fmt.Errorf("subtitle stream %d produced an invalid %s sidecar", stream.Index, strings.ToUpper(transform.Format))
		}
		artifacts = append(artifacts, SubtitleArtifact{
			StreamIndex: stream.Index, SourceCodec: stream.Codec, Format: transform.Format,
			Language: language, Default: transform.MakeDefault, StagedPath: stagedPath, SizeBytes: info.Size(),
		})
	}
	return artifacts, nil
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
			"streamIndex": value.StreamIndex, "sourceCodec": value.SourceCodec, "format": value.Format,
			"language": value.Language, "default": value.Default, "stagedPath": value.StagedPath,
			"publishedPath": value.PublishedPath, "sizeBytes": value.SizeBytes,
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
			StreamIndex:   intValueSetting(value["streamIndex"], -1),
			SourceCodec:   stringFromUnknown(value["sourceCodec"]),
			Format:        stringFromUnknown(value["format"]),
			Language:      stringFromUnknown(value["language"]),
			Default:       boolSetting(value["default"], false),
			StagedPath:    stringFromUnknown(value["stagedPath"]),
			PublishedPath: stringFromUnknown(value["publishedPath"]),
			SizeBytes:     int64(intValueSetting(value["sizeBytes"], 0)),
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
		if info, err := os.Stat(artifact.StagedPath); err != nil || info.IsDir() || info.Size() == 0 {
			return nil, fmt.Errorf("staged subtitle artifact is missing or empty: %s", artifact.StagedPath)
		}
		suffix := "." + safeSubtitleFilenamePart(artifact.Language)
		if artifact.Default {
			suffix += ".default"
		}
		suffix += "." + artifact.Format
		preferredDestination := base + suffix
		if _, duplicate := usedDestinations[preferredDestination]; duplicate {
			return nil, fmt.Errorf("multiple subtitle transformations resolve to the same destination: %s", preferredDestination)
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
