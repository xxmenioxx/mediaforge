package handlers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

var (
	ocrBitmapCountPattern = regexp.MustCompile(
		`(?i)Running tesseract OCR on\s+(\d+)\s+.*image(?:s|\(s\))?`,
	)
	ocrDroppedCountPattern = regexp.MustCompile(`(?i)(\d+)\s+image\(s\) produced no OCR text`)
)

type OCRProgressFunc func(processed, total int)

func isBitmapSubtitleCodecName(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "dvd_subtitle", "dvb_subtitle", "hdmv_pgs_subtitle", "pgssub":
		return true
	default:
		return false
	}
}

func scanLinesAndCarriageReturns(
	data []byte,
	atEOF bool,
) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}

	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}

	return 0, nil, nil
}

func generateBitmapSubtitleSidecar(
	ctx context.Context,
	sourceMediaPath string,
	destinationMediaPath string,
	stream FFProbeStream,
	input SubtitleExtractionInput,
	onProgress OCRProgressFunc,
) (SubtitleExtractionResult, error) {
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
		strings.TrimSuffix(destinationMediaPath, filepath.Ext(destinationMediaPath)),
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
	rawTempPath := filepath.Join(tempDir, "ocr.raw."+format)

	message, err := runBitmapOCR(
		ctx,
		sourceMediaPath,
		stream,
		format,
		language,
		input.OCRMode,
		tempPath,
		rawTempPath,
		onProgress,
	)
	if err != nil {
		return result, fmt.Errorf("OCR failed for subtitle stream %d (%s): %s", stream.Index, stream.CodecName, fallback(message, err.Error()))
	}

	content, err := os.ReadFile(tempPath)
	if err != nil {
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
	if normalizedOCRMode(input.OCRMode) != "raw" {
		rawOutputPath := strings.TrimSuffix(outputPath, "."+format) + ".raw." + format
		if rawContent, readErr := os.ReadFile(rawTempPath); readErr == nil && validSubtitleSidecar(format, rawContent) {
			if linkErr := os.Link(rawTempPath, rawOutputPath); linkErr == nil {
				result.Created = append(result.Created, rawOutputPath)
			} else if os.IsExist(linkErr) {
				result.Existing = append(result.Existing, rawOutputPath)
			}
		}
	}
	return result, nil
}

func generateBitmapSubtitleAtPath(ctx context.Context, mediaPath string, stream FFProbeStream, format string, ocrLanguage string, ocrMode string, outputPath string, segmentStart float64, segmentDuration int) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format != "srt" && format != "ass" {
		return fmt.Errorf("OCR output format must be srt or ass")
	}
	language := normalizedOCRLanguage(ocrLanguage, stream.Tags["language"])
	if _, err := exec.LookPath("seconv"); err != nil {
		return fmt.Errorf("OCR is not available in this backend image (seconv was not found)")
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		return fmt.Errorf("OCR is not available in this backend image (Tesseract was not found)")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("cannot create OCR output directory: %w", err)
	}
	tempDir, err := os.MkdirTemp(filepath.Dir(outputPath), ".mvforge-ocr-*")
	if err != nil {
		return fmt.Errorf("cannot create OCR workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)
	tempPath := filepath.Join(tempDir, "ocr."+format)
	rawTempPath := filepath.Join(tempDir, "ocr.raw."+format)
	ocrMediaPath := mediaPath
	ocrStream := stream
	if segmentDuration > 0 {
		segmentPath := filepath.Join(tempDir, "subtitle-segment.mkv")
		args := bitmapSubtitleSegmentArgs(mediaPath, stream.Index, segmentPath, segmentStart, segmentDuration)
		output, runErr := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
		if runErr != nil {
			return fmt.Errorf("cannot isolate subtitle test window for stream %d: %s", stream.Index, fallback(strings.TrimSpace(string(output)), runErr.Error()))
		}
		windowStreams, probeErr := probeSubtitleStreams(ctx, segmentPath)
		if probeErr != nil || len(windowStreams) == 0 {
			return fmt.Errorf("subtitle test window for stream %d has no readable subtitle track", stream.Index)
		}
		ocrMediaPath, ocrStream = segmentPath, windowStreams[0]
	}

	message, runErr := runBitmapOCR(
		ctx,
		ocrMediaPath,
		ocrStream,
		format,
		language,
		ocrMode,
		tempPath,
		rawTempPath,
		nil,
	)

	if runErr != nil {
		return fmt.Errorf("OCR failed for subtitle stream %d (%s): %s", stream.Index, stream.CodecName, fallback(message, runErr.Error()))
	}
	content, err := os.ReadFile(tempPath)
	if err != nil {
		if summary := emptyOCRTrackMessage(stream.Index, language, message); summary != "" {
			return fmt.Errorf("%s", summary)
		}
		return fmt.Errorf("OCR produced no %s subtitle for stream %d: %s", strings.ToUpper(format), stream.Index, conciseCommandOutput(fallback(message, "SeConv completed without reporting a reason"), 1200))
	}
	if !validSubtitleSidecar(format, content) {
		return fmt.Errorf("OCR output for subtitle stream %d did not contain valid timed subtitle text", stream.Index)
	}
	_ = os.Remove(outputPath)
	if err := os.Rename(tempPath, outputPath); err != nil {
		return fmt.Errorf("cannot stage OCR subtitle: %w", err)
	}
	return nil
}

func bitmapSubtitleSegmentArgs(mediaPath string, streamIndex int, outputPath string, segmentStart float64, segmentDuration int) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	inputSeek, outputTrim := segmentedSeekWindow(segmentStart)
	if inputSeek > 0 {
		args = append(args, "-ss", strconv.FormatFloat(inputSeek, 'f', -1, 64))
	}
	args = append(args, "-i", mediaPath)
	if outputTrim > 0 {
		args = append(args, "-ss", strconv.FormatFloat(outputTrim, 'f', -1, 64))
	}
	if segmentDuration > 0 {
		args = append(args, "-t", strconv.Itoa(segmentDuration))
	}
	return append(args, "-map", fmt.Sprintf("0:%d", streamIndex), "-c:s", "copy", outputPath)
}

func normalizedOCRMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "raw":
		return "raw"
	case "clean":
		return "clean"
	default:
		return "accurate"
	}
}

func extractPGSSubtitleForOCR(
	ctx context.Context,
	mediaPath string,
	stream FFProbeStream,
	outputPath string,
) (string, error) {
	args := []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", mediaPath,
		"-map", fmt.Sprintf("0:%d", stream.Index),
		"-c:s", "copy",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf(
			"cannot extract PGS subtitle stream %d: %w",
			stream.Index,
			err,
		)
	}

	return strings.TrimSpace(string(output)), nil
}

func runBitmapOCR(
	ctx context.Context,
	mediaPath string,
	stream FFProbeStream,
	format, language, mode, outputPath, rawOutputPath string,
	onProgress OCRProgressFunc,
) (string, error) {
	mode = normalizedOCRMode(mode)
	dir := filepath.Dir(outputPath)

	ocrInputPath := mediaPath
	useTrackNumber := true
	extractionMessage := ""

	switch strings.ToLower(strings.TrimSpace(stream.CodecName)) {
	case "hdmv_pgs_subtitle", "pgssub":
		pgsPath := filepath.Join(
			dir,
			fmt.Sprintf("ocr-stream-%d.sup", stream.Index),
		)

		message, err := extractPGSSubtitleForOCR(
			ctx,
			mediaPath,
			stream,
			pgsPath,
		)
		if err != nil {
			return message, err
		}

		extractionMessage = message
		ocrInputPath = pgsPath
		useTrackNumber = false
	}

	first := filepath.Join(dir, "ocr-isolated."+format)
	message, err := runSeConvOCRPass(
		ctx,
		ocrInputPath,
		stream,
		format,
		language,
		first,
		false,
		useTrackNumber,
		onProgress,
	)

	message = strings.TrimSpace(extractionMessage + "\n" + message)

	if err != nil {
		return message, err
	}
	chosen := first
	if mode == "accurate" {
		second := filepath.Join(dir, "ocr-colors."+format)
		secondMessage, secondErr := runSeConvOCRPass(
			ctx,
			ocrInputPath,
			stream,
			format,
			language,
			second,
			true,
			useTrackNumber,
			onProgress,
		)
		message = strings.TrimSpace(message + "\n" + secondMessage)
		if secondErr == nil && preferSecondaryOCR(subtitleOCRFileScore(first, format), subtitleOCRFileScore(second, format)) {
			chosen = second
		}
	}
	if err := copyFileContents(chosen, rawOutputPath); err != nil {
		return message, err
	}
	if mode == "raw" {
		return message, copyFileContents(chosen, outputPath)
	}
	cleanMessage, cleanErr := runSeConvCleanup(ctx, chosen, format, language, outputPath)
	return strings.TrimSpace(message + "\n" + cleanMessage), cleanErr
}

func runSeConvOCRPass(
	ctx context.Context,
	inputPath string,
	stream FFProbeStream,
	format string,
	language string,
	outputPath string,
	preserveColors bool,
	useTrackNumber bool,
	onProgress OCRProgressFunc,
) (string, error) {
	args := []string{
		inputPath,
		seconvFormat(format),
	}

	if useTrackNumber {
		args = append(
			args,
			"--track-number:"+strconv.Itoa(matroskaTrackNumber(stream)),
		)
	}

	args = append(
		args,
		"--ocr-engine:tesseract",
		"--ocr-language:"+language,
		"--output-filename:"+outputPath,
		"--overwrite",
	)

	if preserveColors {
		args = append(
			args,
			"--no-vobsub-isolate-colors",
			"--no-pgs-isolate-colors",
		)
	}

	return runSeConvWithProgress(ctx, args, onProgress)
}

func runSeConvCleanup(ctx context.Context, inputPath, format, language, outputPath string) (string, error) {
	// Keep this pass deliberately conservative. In SeConv 5.1,
	// --remove-unicode-control-chars can also remove valid subtitle line breaks,
	// while merge operations can join distinct signs sharing a timestamp.
	args := []string{inputPath, seconvFormat(format), "--output-filename:" + outputPath, "--overwrite", "--fix-common-errors-rules:FixEmptyLines,FixInvalidItalicTags,FixMissingSpaces,FixUnneededSpaces,NormalizeStrings,FixUppercaseIInsideWords,FixCommonOcrErrors", "--fce-language:" + fceLanguage(language)}
	if dictionary := ocrDictionaryFolder(); dictionary != "" {
		args = append(args, "--dictionary-folder:"+dictionary)
	}
	return runSeConv(ctx, args)
}

func runSeConv(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, "seconv", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return strings.TrimSpace(strings.Join([]string{stderr.String(), stdout.String()}, "\n")), err
}

func runSeConvWithProgress(
	ctx context.Context,
	args []string,
	onProgress OCRProgressFunc,
) (string, error) {
	cmd := exec.CommandContext(ctx, "seconv", args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	var output bytes.Buffer
	var wg sync.WaitGroup

	process := func(r io.Reader) {
		defer wg.Done()

		scanner := bufio.NewScanner(r)
		scanner.Split(scanLinesAndCarriageReturns)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())

			if line == "" {
				continue
			}

			output.WriteString(line)
			output.WriteByte('\n')

			fmt.Fprintf(os.Stderr, "\n[SECONV-OCR] %s\n", line)

			if match := ocrBitmapCountPattern.FindStringSubmatch(line); len(match) == 2 {
				if total, parseErr := strconv.Atoi(match[1]); parseErr == nil {
					fmt.Fprintf(
						os.Stderr,
						"\n[SECONV-PROGRESS] total=%d\n",
						total,
					)

					if onProgress != nil {
						onProgress(0, total)
					}
				}
			}
		}

		// for scanner.Scan() {
		// 	line := scanner.Text()

		// 	output.WriteString(line)
		// 	output.WriteByte('\n')

		// 	lower := strings.ToLower(line)

		// 	if strings.Contains(lower, "ocr") ||
		// 		strings.Contains(lower, "tesseract") ||
		// 		strings.Contains(lower, "image") ||
		// 		strings.Contains(lower, "%") {
		// 		fmt.Printf("[seconv-ocr] %s\n", line)
		// 	}

		// 	// Primero detectamos el total reportado por SeConv.
		// 	if match := ocrBitmapCountPattern.FindStringSubmatch(line); len(match) == 2 {
		// 		if total, parseErr := strconv.Atoi(match[1]); parseErr == nil {
		// 			if onProgress != nil {
		// 				onProgress(0, total)
		// 			}
		// 		}
		// 	}
		// }
	}

	wg.Add(2)

	go process(stdoutPipe)
	go process(stderrPipe)

	waitErr := cmd.Wait()
	wg.Wait()

	return strings.TrimSpace(output.String()), waitErr
}

func ocrDictionaryFolder() string {
	for _, path := range []string{"/usr/share/hunspell", "/usr/local/share/hunspell", "/opt/homebrew/share/hunspell"} {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

func fceLanguage(language string) string {
	if language == "spa" {
		return "es"
	}
	if language == "jpn" || language == "jpn_vert" {
		return "ja"
	}
	return "en"
}

func copyFileContents(source, destination string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, content, 0o644)
}

func subtitleOCRFileScore(path, format string) float64 {
	content, err := os.ReadFile(path)
	if err != nil || !validSubtitleSidecar(format, content) {
		return math.Inf(-1)
	}
	return subtitleOCRScore(string(content))
}

func subtitleOCRScore(content string) float64 {
	letters, digits, suspicious, replacement := 0, 0, 0, 0
	for _, r := range content {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r):
			digits++
		case r == unicode.ReplacementChar:
			replacement++
		case !unicode.IsSpace(r) && !unicode.IsPunct(r):
			suspicious++
		}
	}
	return float64(letters) + float64(digits)*0.1 - float64(suspicious)*6 - float64(replacement)*25
}

func preferSecondaryOCR(primary, secondary float64) bool {
	// Colour isolation is the reliable default for bitmap subtitles. Select the
	// alternate pass only when its structural score is materially better; tiny
	// differences usually indicate different OCR mistakes, not an improvement.
	margin := math.Max(25, math.Abs(primary)*0.05)
	return secondary > primary+margin
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
	case "jpn_vert", "ja_vert", "japanese_vertical":
		return "jpn_vert"
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
