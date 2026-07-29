package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	ocrBitmapCountPattern  = regexp.MustCompile(`(?i)Running tesseract OCR on\s+(\d+)\s+MKV VobSub image`)
	ocrDroppedCountPattern = regexp.MustCompile(`(?i)(\d+)\s+image\(s\) produced no OCR text`)
)

func isBitmapSubtitleCodecName(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "dvd_subtitle", "dvb_subtitle", "hdmv_pgs_subtitle", "pgssub":
		return true
	default:
		return false
	}
}

func generateBitmapSubtitleSidecar(ctx context.Context, mediaPath string, stream FFProbeStream, input SubtitleExtractionInput) (SubtitleExtractionResult, error) {
	format := strings.ToLower(strings.TrimSpace(input.Format))
	if format == "" {
		format = "srt"
	}
	if format != "srt" && format != "ass" {
		return SubtitleExtractionResult{}, fmt.Errorf("OCR output format must be srt or ass")
	}

	language := normalizedOCRLanguage(input.OCRLanguage, stream.Tags["language"])
	fileLanguage := safeSubtitleFilenamePart(stream.Tags["language"])
	if fileLanguage == "" {
		fileLanguage = language
	}
	outputPath := fmt.Sprintf(
		"%s.%s.%d.%s",
		strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath)),
		fileLanguage,
		stream.Index,
		format,
	)
	result := SubtitleExtractionResult{Created: []string{}, Existing: []string{}, Unsupported: []string{}}
	if info, err := os.Stat(outputPath); err == nil && !info.IsDir() {
		result.Existing = append(result.Existing, outputPath)
		return result, nil
	} else if err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("%s", mediaPathReadError(err))
	}

	if _, err := exec.LookPath("seconv"); err != nil {
		return result, fmt.Errorf("OCR is not available in this backend image (seconv was not found)")
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		return result, fmt.Errorf("OCR is not available in this backend image (Tesseract was not found)")
	}

	tempDir, err := os.MkdirTemp(filepath.Dir(outputPath), ".mvforge-ocr-*")
	if err != nil {
		return result, fmt.Errorf("cannot create OCR workspace beside the asset: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempName := "ocr." + format
	tempPath := filepath.Join(tempDir, tempName)
	args := []string{
		mediaPath,
		seconvFormat(format),
		"--track-number:" + strconv.Itoa(matroskaTrackNumber(stream)),
		"--ocr-engine:tesseract",
		"--ocr-language:" + language,
		"--output-filename:" + tempPath,
		"--overwrite",
	}
	cmd := exec.CommandContext(ctx, "seconv", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(strings.Join([]string{stderr.String(), stdout.String()}, "\n"))
		if message == "" {
			message = err.Error()
		}
		return result, fmt.Errorf("OCR failed for subtitle stream %d (%s): %s", stream.Index, stream.CodecName, message)
	}

	content, err := os.ReadFile(tempPath)
	if err != nil {
		message := strings.TrimSpace(strings.Join([]string{stderr.String(), stdout.String()}, "\n"))
		if summary := emptyOCRTrackMessage(stream.Index, language, message); summary != "" {
			return result, fmt.Errorf("%s", summary)
		}
		if message == "" {
			message = "SeConv completed without reporting a reason"
		}
		message = conciseCommandOutput(message, 1200)
		return result, fmt.Errorf("OCR produced no %s subtitle for stream %d: %s", strings.ToUpper(format), stream.Index, message)
	}
	if !validSubtitleSidecar(format, content) {
		return result, fmt.Errorf("OCR output for subtitle stream %d did not contain valid timed subtitle text", stream.Index)
	}
	if err := os.Link(tempPath, outputPath); err != nil {
		if os.IsExist(err) {
			result.Existing = append(result.Existing, outputPath)
			return result, nil
		}
		return result, fmt.Errorf("cannot publish OCR subtitle beside the asset: %w", err)
	}
	result.Created = append(result.Created, outputPath)
	return result, nil
}

func emptyOCRTrackMessage(streamIndex int, language string, output string) string {
	dropped := ocrDroppedCountPattern.FindStringSubmatch(output)
	if len(dropped) != 2 {
		return ""
	}
	count := dropped[1]
	if total := ocrBitmapCountPattern.FindStringSubmatch(output); len(total) == 2 {
		count = total[1]
	}
	noun := "bitmap event"
	if count != "1" {
		noun = "bitmap events"
	}
	return fmt.Sprintf(
		"OCR completed for subtitle stream %d, but its %s %s contained no text recognizable as %s. "+
			"The track may be empty or decorative, or the OCR language may be incorrect; no subtitle file was created.",
		streamIndex, count, noun, language,
	)
}

func conciseCommandOutput(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "…"
}

func seconvFormat(format string) string {
	if format == "ass" {
		return "assa"
	}
	return "subrip"
}

func normalizedOCRLanguage(requested string, detected string) string {
	value := strings.ToLower(strings.TrimSpace(requested))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(detected))
	}
	switch value {
	case "es", "spa", "esp", "spanish":
		return "spa"
	case "ja", "jp", "jpn", "japanese":
		return "jpn"
	case "en", "eng", "english":
		return "eng"
	default:
		return "eng"
	}
}

func matroskaTrackNumber(stream FFProbeStream) int {
	switch id := stream.ID.(type) {
	case string:
		if value, err := strconv.ParseInt(strings.TrimSpace(id), 0, 32); err == nil && value > 0 {
			return int(value)
		}
	case float64:
		if id > 0 && id == float64(int(id)) {
			return int(id)
		}
	}
	// FFprobe indexes all streams from zero, while Matroska TrackNumber starts at one.
	return stream.Index + 1
}
