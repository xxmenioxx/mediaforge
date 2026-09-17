package handlers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

var (
	ocrBitmapCountPattern = regexp.MustCompile(
		`(?i)Running tesseract OCR on\s+(\d+)\s+.*image(?:s|\(s\))?`,
	)
	ocrDroppedCountPattern         = regexp.MustCompile(`(?i)(\d+)\s+image\(s\) produced no OCR text`)
	ocrZeroRecognitionPattern      = regexp.MustCompile(`(?i)\bno subtitles recogn(?:ised|ized)\b`)
	ocrDisqualifyingFailurePattern = regexp.MustCompile(`(?i)\b(?:cancel(?:led|ed)|interrupt(?:ed)?|malformed|corrupt(?:ed)?|invalid input|cannot read|unable to read|segmentation fault|panic|fatal|crash(?:ed)?|signal: (?:killed|terminated))\b`)
)

const maxOCRDiagnostics = 64 * 1024

type ocrPassResult struct {
	message         string
	err             error
	zeroRecognition bool
}

type bitmapOCRLanguageAttempt struct {
	message    string
	err        error
	chosenPath string
	zero       bool
}

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

func generateBitmapSubtitleBoth(ctx context.Context, sourceMediaPath, destinationMediaPath string, stream FFProbeStream, input SubtitleExtractionInput, onProgress OCRProgressFunc) (SubtitleExtractionResult, error) {
	base := strings.TrimSuffix(destinationMediaPath, filepath.Ext(destinationMediaPath))
	language := safeSubtitleFilenamePart(stream.Tags["language"])
	if language == "" {
		language = normalizedOCRLanguage(input.OCRLanguage, stream.Tags["language"])
	}
	srtPath := fmt.Sprintf("%s.%s.%d.srt", base, language, stream.Index)
	assPath := fmt.Sprintf("%s.%s.%d.ass", base, language, stream.Index)
	result := SubtitleExtractionResult{Created: []string{}, Existing: []string{}, Unsupported: []string{}}
	for _, path := range []string{srtPath, assPath} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			result.Existing = append(result.Existing, path)
		} else if err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("%s", mediaPathReadError(err))
		}
	}
	if len(result.Existing) == 2 {
		return result, nil
	}
	if _, err := exec.LookPath("seconv"); err != nil {
		return result, fmt.Errorf("OCR is not available in this backend image (seconv was not found)")
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		return result, fmt.Errorf("OCR is not available in this backend image (Tesseract was not found)")
	}
	tempDir, err := os.MkdirTemp(filepath.Dir(srtPath), ".mvforge-ocr-both-*")
	if err != nil {
		return result, fmt.Errorf("cannot create OCR workspace beside the asset: %w", err)
	}
	defer os.RemoveAll(tempDir)
	rawTempPath := filepath.Join(tempDir, "ocr.raw.srt")
	tempSRT := filepath.Join(tempDir, "ocr.srt")
	languageValue := normalizedOCRLanguage(input.OCRLanguage, stream.Tags["language"])
	message, err := runBitmapOCR(ctx, sourceMediaPath, stream, "srt", languageValue, input.OCRMode, tempSRT, rawTempPath, onProgress)
	if err != nil {
		return result, fmt.Errorf("OCR failed for subtitle stream %d (%s): %s", stream.Index, stream.CodecName, fallback(message, err.Error()))
	}
	content, err := os.ReadFile(tempSRT)
	if err != nil || !validSubtitleSidecar("srt", content) {
		return result, fmt.Errorf("OCR output for subtitle stream %d did not contain valid timed subtitle text", stream.Index)
	}
	tempASS := filepath.Join(tempDir, "ocr.ass")
	var stderr bytes.Buffer
	if err := runSubtitleCommand(ctx, "ffmpeg", subtitleCueRenderArgs(tempSRT, "ass", tempASS), nil, &stderr); err != nil {
		return result, fmt.Errorf("ASS render failed for subtitle stream %d: %s", stream.Index, fallback(strings.TrimSpace(stderr.String()), err.Error()))
	}
	assContent, err := os.ReadFile(tempASS)
	if err != nil || !validSubtitleSidecar("ass", assContent) {
		return result, fmt.Errorf("ASS render for subtitle stream %d did not contain valid timed subtitle text", stream.Index)
	}
	// Keep a logical both conversion behind one publication barrier. The
	// complete requested set is validated before either new sidecar is linked.
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	published := []string{}
	for _, pair := range []struct {
		temp string
		path string
	}{
		{temp: tempSRT, path: srtPath},
		{temp: tempASS, path: assPath},
	} {
		if _, statErr := os.Stat(pair.path); statErr == nil {
			continue
		} else if !os.IsNotExist(statErr) {
			rollbackSubtitlePublication(published)
			return result, fmt.Errorf("cannot inspect OCR subtitle destination: %w", statErr)
		}
		if linkErr := os.Link(pair.temp, pair.path); linkErr != nil {
			if os.IsExist(linkErr) {
				continue
			}
			rollbackSubtitlePublication(published)
			return result, fmt.Errorf("cannot publish OCR subtitle beside the asset: %w", linkErr)
		}
		published = append(published, pair.path)
		result.Created = append(result.Created, pair.path)
	}
	return result, nil
}

func generateBitmapSubtitleAtPath(ctx context.Context, mediaPath string, stream FFProbeStream, format string, ocrLanguage string, ocrMode string, outputPath string, segmentStart float64, segmentDuration int, onProgress OCRProgressFunc) error {
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
		var output bytes.Buffer
		runErr := runSubtitleCommand(ctx, "ffmpeg", args, &output, &output)
		if runErr != nil {
			return fmt.Errorf("cannot isolate subtitle test window for stream %d: %s", stream.Index, fallback(strings.TrimSpace(output.String()), runErr.Error()))
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
		onProgress,
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
	if normalizedOCRMode(ocrMode) != "raw" {
		rawOutputPath := strings.TrimSuffix(outputPath, "."+format) + ".raw." + format
		_ = os.Remove(rawOutputPath)
		if err := os.Rename(rawTempPath, rawOutputPath); err != nil {
			return fmt.Errorf("cannot stage raw OCR subtitle: %w", err)
		}
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

	var output bytes.Buffer
	err := runSubtitleCommand(ctx, "ffmpeg", args, &output, &output)
	if err != nil {
		return strings.TrimSpace(output.String()), fmt.Errorf(
			"cannot extract PGS subtitle stream %d: %w",
			stream.Index,
			err,
		)
	}

	return strings.TrimSpace(output.String()), nil
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
	capabilities, capabilityMessage, err := probeTesseractLanguages(ctx)
	if err != nil {
		return capabilityMessage, err
	}
	if _, available := capabilities[language]; !available {
		return capabilityMessage, fmt.Errorf("OCR language %q is unavailable in the running Tesseract runtime; install its traineddata and retry", language)
	}

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

	primary := runBitmapOCRLanguageAttempt(ctx, ocrInputPath, stream, format, language, mode, dir, "primary", useTrackNumber, onProgress)
	message := strings.TrimSpace(fmt.Sprintf("OCR effective language: %s\n%s\n%s", language, extractionMessage, primary.message))
	chosen := primary.chosenPath
	successfulLanguage := language
	if primary.zero {
		alternate := oppositeJapaneseOCRLanguage(language)
		if alternate == "" {
			return message, conciseZeroRecognitionError(language, "")
		}
		if _, available := capabilities[alternate]; !available {
			return message, fmt.Errorf("OCR recognized no subtitle text using %s; alternate Japanese orientation %s is unavailable in the running Tesseract runtime", language, alternate)
		}
		fallbackAttempt := runBitmapOCRLanguageAttempt(ctx, ocrInputPath, stream, format, alternate, mode, dir, "fallback", useTrackNumber, onProgress)
		message = strings.TrimSpace(message + "\nOCR fallback language: " + alternate + "\n" + fallbackAttempt.message)
		if fallbackAttempt.zero {
			return message, conciseZeroRecognitionError(language, alternate)
		}
		if fallbackAttempt.err != nil {
			return message, fallbackAttempt.err
		}
		chosen = fallbackAttempt.chosenPath
		successfulLanguage = alternate
	} else if primary.err != nil {
		return message, primary.err
	}

	if err := copyFileContents(chosen, rawOutputPath); err != nil {
		return message, err
	}
	if mode == "raw" {
		return message, copyFileContents(chosen, outputPath)
	}
	cleanMessage, cleanErr := runSeConvCleanup(ctx, chosen, format, successfulLanguage, outputPath)
	return strings.TrimSpace(message + "\n" + cleanMessage), cleanErr
}

func probeTesseractLanguages(ctx context.Context) (map[string]struct{}, string, error) {
	var stdout, stderr bytes.Buffer
	err := runSubtitleCommand(ctx, "tesseract", []string{"--list-langs"}, &stdout, &stderr)
	diagnostics := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
	capabilities := parseTesseractLanguages(stdout.String())
	message := "Tesseract runtime languages: " + strings.Join(sortedOCRLanguages(capabilities), ", ")
	if err != nil {
		return capabilities, conciseCommandOutput(diagnostics, 1200), fmt.Errorf("cannot inspect Tesseract runtime languages: %w", err)
	}
	if len(capabilities) == 0 {
		return capabilities, conciseCommandOutput(diagnostics, 1200), errors.New("Tesseract runtime reported no usable OCR languages")
	}
	return capabilities, message, nil
}

func parseTesseractLanguages(output string) map[string]struct{} {
	capabilities := map[string]struct{}{}
	for _, line := range strings.Split(output, "\n") {
		language := strings.TrimSpace(line)
		if language == "" || strings.HasPrefix(strings.ToLower(language), "list of available languages") {
			continue
		}
		if strings.IndexFunc(language, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_'
		}) != -1 {
			continue
		}
		capabilities[language] = struct{}{}
	}
	return capabilities
}

func sortedOCRLanguages(capabilities map[string]struct{}) []string {
	values := make([]string, 0, len(capabilities))
	for language := range capabilities {
		values = append(values, language)
	}
	sort.Strings(values)
	return values
}

func oppositeJapaneseOCRLanguage(language string) string {
	switch language {
	case "jpn":
		return "jpn_vert"
	case "jpn_vert":
		return "jpn"
	default:
		return ""
	}
}

func runBitmapOCRLanguageAttempt(
	ctx context.Context,
	inputPath string,
	stream FFProbeStream,
	format, language, mode, dir, tag string,
	useTrackNumber bool,
	onProgress OCRProgressFunc,
) bitmapOCRLanguageAttempt {
	first := filepath.Join(dir, "ocr-"+tag+"-isolated."+format)
	firstResult := runSeConvOCRPassDetailed(ctx, inputPath, stream, format, language, first, false, useTrackNumber, onProgress)
	result := bitmapOCRLanguageAttempt{message: firstResult.message}
	if firstResult.zeroRecognition {
		result.zero = true
		return result
	}
	if firstResult.err != nil {
		result.err = firstResult.err
		return result
	}
	chosen := first
	if mode == "accurate" {
		second := filepath.Join(dir, "ocr-"+tag+"-colors."+format)
		secondResult := runSeConvOCRPassDetailed(ctx, inputPath, stream, format, language, second, true, useTrackNumber, onProgress)
		result.message = strings.TrimSpace(result.message + "\n" + secondResult.message)
		if secondResult.err != nil && ctx.Err() != nil {
			result.err = secondResult.err
			return result
		}
		if secondResult.err == nil && preferSecondaryOCR(subtitleOCRFileScore(first, format), subtitleOCRFileScore(second, format)) {
			chosen = second
		}
	}
	result.chosenPath = chosen
	return result
}

func runSeConvOCRPassDetailed(
	ctx context.Context,
	inputPath string,
	stream FFProbeStream,
	format, language, outputPath string,
	preserveColors, useTrackNumber bool,
	onProgress OCRProgressFunc,
) ocrPassResult {
	message, err := runSeConvOCRPass(ctx, inputPath, stream, format, language, outputPath, preserveColors, useTrackNumber, onProgress)
	if isGenuineZeroRecognition(ctx, message, err) {
		return ocrPassResult{message: message, zeroRecognition: true}
	}
	return ocrPassResult{message: message, err: err}
}

func isGenuineZeroRecognition(ctx context.Context, diagnostics string, _ error) bool {
	if ctx.Err() != nil {
		return false
	}
	knownSignal := ocrZeroRecognitionPattern.MatchString(diagnostics) || ocrDroppedCountPattern.MatchString(diagnostics)
	return knownSignal && !ocrDisqualifyingFailurePattern.MatchString(diagnostics)
}

func conciseZeroRecognitionError(primary, alternate string) error {
	if alternate == "" {
		return fmt.Errorf("OCR recognized no subtitle text using %s; check subtitle orientation or source content", primary)
	}
	return fmt.Errorf("OCR recognized no subtitle text with Japanese orientations %s and %s; check subtitle orientation or source content", primary, alternate)
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
	var stdout, stderr bytes.Buffer
	err := runSubtitleCommand(ctx, "seconv", args, &stdout, &stderr)
	return strings.TrimSpace(strings.Join([]string{stderr.String(), stdout.String()}, "\n")), err
}

func runSeConvWithProgress(
	ctx context.Context,
	args []string,
	onProgress OCRProgressFunc,
) (string, error) {
	cmd := exec.Command("seconv", args...)
	configureSubtitleProcessGroup(cmd)

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
	var outputMu sync.Mutex
	var wg sync.WaitGroup
	appendDiagnostic := func(line string) {
		outputMu.Lock()
		defer outputMu.Unlock()
		if output.Len() >= maxOCRDiagnostics {
			return
		}
		remaining := maxOCRDiagnostics - output.Len()
		if len(line) > remaining {
			line = line[:remaining]
		}
		output.WriteString(line)
	}

	process := func(r io.Reader) {
		defer wg.Done()

		scanner := bufio.NewScanner(r)
		scanner.Split(scanLinesAndCarriageReturns)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())

			if line == "" {
				continue
			}

			appendDiagnostic(line + "\n")

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

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	waitErr := waitSubtitleProcess(ctx, cmd, waitDone)
	wg.Wait()

	outputMu.Lock()
	diagnostics := strings.TrimSpace(output.String())
	outputMu.Unlock()
	return diagnostics, waitErr
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
