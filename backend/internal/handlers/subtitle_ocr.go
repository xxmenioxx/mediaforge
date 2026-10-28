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
	ocrDictionaryCacheMu           sync.Mutex
	ocrDictionaryCache             = map[string]map[string]struct{}{}
	ocrDictionaryFolderOverride    string
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

type bitmapOCRProcessError struct {
	language    string
	diagnostics string
	err         error
}

type bitmapNoEligibleCueError struct {
	format   string
	evidence pgsSUPCueEvidence
}

func (e *bitmapNoEligibleCueError) Error() string {
	return fmt.Sprintf(
		"no eligible PGS bitmap cues (display sets=%d, presentation composition events=%d, bitmap object references=%d, geometry cue records=%d); required %s artifact was not created and OCR was not attempted",
		e.evidence.DisplaySets,
		e.evidence.PresentationCompositionEvents,
		e.evidence.BitmapObjectReferences,
		e.evidence.GeometryCueRecords,
		strings.ToUpper(e.format),
	)
}

func (e *bitmapOCRProcessError) Error() string {
	diagnostics := conciseCommandOutput(e.diagnostics, 1200)
	if diagnostics == "" {
		return fmt.Sprintf("OCR failed using %s: %v", e.language, e.err)
	}
	return fmt.Sprintf("OCR failed using %s: %s (%v)", e.language, diagnostics, e.err)
}

func (e *bitmapOCRProcessError) Unwrap() error {
	return e.err
}

type OCRProgressFunc func(processed, total int)

type ocrProcessOutput struct {
	mu                    sync.Mutex
	pending               []byte
	discardUntilDelimiter bool
	onLine                func(string)
}

func (o *ocrProcessOutput) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	written := len(data)

	lines := make([]string, 0, 1)
	o.mu.Lock()
	for len(data) > 0 {
		if o.discardUntilDelimiter {
			index := bytes.IndexAny(data, "\r\n")
			if index < 0 {
				break
			}
			data = data[index+1:]
			o.discardUntilDelimiter = false
			continue
		}

		index := bytes.IndexAny(data, "\r\n")
		if index < 0 {
			remaining := maxOCRDiagnostics - len(o.pending)
			if remaining <= 0 {
				o.discardUntilDelimiter = true
				break
			}
			if len(data) > remaining {
				o.pending = append(o.pending, data[:remaining]...)
				o.discardUntilDelimiter = true
				break
			}
			o.pending = append(o.pending, data...)
			break
		}

		line := data[:index]
		remaining := maxOCRDiagnostics - len(o.pending)
		if remaining > 0 {
			if len(line) > remaining {
				line = line[:remaining]
			}
			o.pending = append(o.pending, line...)
		}
		lines = append(lines, string(o.pending))
		o.pending = o.pending[:0]
		data = data[index+1:]
	}
	o.mu.Unlock()

	for _, line := range lines {
		if o.onLine != nil {
			o.onLine(line)
		}
	}
	return written, nil
}

func (o *ocrProcessOutput) flush() {
	o.mu.Lock()
	line := string(o.pending)
	o.pending = o.pending[:0]
	o.mu.Unlock()
	if line != "" && o.onLine != nil {
		o.onLine(line)
	}
}

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
	tempSRT := filepath.Join(tempDir, "ocr.srt")
	languageValue := normalizedOCRLanguage(input.OCRLanguage, stream.Tags["language"])
	ocrResult, err := generateBitmapSubtitleAtPathWithResult(ctx, sourceMediaPath, stream, "srt", languageValue, input.OCRMode, tempSRT, 0, 0, onProgress)
	if err != nil {
		return result, fmt.Errorf("OCR failed for subtitle stream %d (%s): %w", stream.Index, stream.CodecName, err)
	}
	content, err := os.ReadFile(tempSRT)
	if err != nil || !validSubtitleSidecar("srt", content) {
		return result, fmt.Errorf("OCR output for subtitle stream %d did not contain valid timed subtitle text", stream.Index)
	}
	tempASS := filepath.Join(tempDir, "ocr.ass")
	var stderr bytes.Buffer
	if len(ocrResult.Geometry) > 0 && len(ocrResult.Cues) > 0 {
		if err := os.WriteFile(tempASS, renderBitmapASSCues(ocrResult.Cues), 0o644); err != nil {
			return result, err
		}
	} else if err := runSubtitleCommand(ctx, "ffmpeg", subtitleCueRenderArgs(tempSRT, "ass", tempASS), nil, &stderr); err != nil {
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
	_, err := generateBitmapSubtitleAtPathWithResult(ctx, mediaPath, stream, format, ocrLanguage, ocrMode, outputPath, segmentStart, segmentDuration, onProgress)
	return err
}

type bitmapSubtitleOCRResult struct {
	EffectiveLanguage string
	Cues              []canonicalBitmapSubtitleCue
	Geometry          []bitmapCueGeometry
	Evidence          *canonicalOCREvidence
}

func generateBitmapSubtitleAtPathWithResult(ctx context.Context, mediaPath string, stream FFProbeStream, format string, ocrLanguage string, ocrMode string, outputPath string, segmentStart float64, segmentDuration int, onProgress OCRProgressFunc, policies ...SubtitleStylePolicy) (bitmapSubtitleOCRResult, error) {
	result := bitmapSubtitleOCRResult{}
	format = strings.ToLower(strings.TrimSpace(format))
	if format != "srt" && format != "ass" {
		return result, fmt.Errorf("OCR output format must be srt or ass")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return result, fmt.Errorf("cannot create OCR output directory: %w", err)
	}
	language := normalizedOCRLanguage(ocrLanguage, stream.Tags["language"])
	request := canonicalOCRRequestFor(mediaPath, stream, language, ocrMode, segmentStart, segmentDuration)
	evidence, err := canonicalOCREvidenceForRequest(ctx, request, onProgress)
	if err != nil {
		var noEligibleCues *bitmapNoEligibleCueError
		if errors.As(err, &noEligibleCues) {
			return result, fmt.Errorf("subtitle stream %d (%s): %w", stream.Index, stream.CodecName, err)
		}
		return result, fmt.Errorf("OCR failed for subtitle stream %d (%s): %w", stream.Index, stream.CodecName, err)
	}
	result.EffectiveLanguage = evidence.EffectiveLanguage
	result.Cues = cloneCanonicalCues(evidence.Cues)
	result.Geometry = append([]bitmapCueGeometry(nil), evidence.Geometry...)
	result.Evidence = cloneCanonicalOCREvidence(evidence)
	stylePolicy := defaultSubtitleStylePolicy()
	if len(policies) > 0 {
		stylePolicy = policies[0]
	}
	role := stylePolicy.Role
	var content []byte
	if format == "ass" {
		content = renderBitmapASSCuesWithPolicyAndRole(evidence.Cues, stylePolicy, role)
	} else {
		content = renderBitmapSRTCues(evidence.Cues)
	}
	if !validSubtitleSidecar(format, content) {
		return result, fmt.Errorf("OCR output for subtitle stream %d did not contain valid timed subtitle text", stream.Index)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(outputPath), ".mvforge-subtitle-stage-*")
	if err != nil {
		return result, fmt.Errorf("cannot create OCR serializer staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return result, fmt.Errorf("cannot stage OCR subtitle: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return result, fmt.Errorf("cannot stage OCR subtitle: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	_ = os.Remove(outputPath)
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return result, fmt.Errorf("cannot stage OCR subtitle: %w", err)
	}
	return result, nil
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

func isSupportedOCRMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "raw", "clean", "accurate":
		return true
	default:
		return false
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

type bitmapOCRResult struct {
	Message           string
	EffectiveLanguage string
	Cues              []canonicalBitmapSubtitleCue
	Geometry          []bitmapCueGeometry
}

func runBitmapOCR(
	ctx context.Context,
	mediaPath string,
	stream FFProbeStream,
	format, language, mode, outputPath, rawOutputPath string,
	onProgress OCRProgressFunc,
) (string, error) {
	result, err := runBitmapOCRWithResult(ctx, mediaPath, stream, format, language, mode, outputPath, rawOutputPath, onProgress)
	return result.Message, err
}

func runBitmapOCRWithResult(
	ctx context.Context,
	mediaPath string,
	stream FFProbeStream,
	format, language, mode, outputPath, rawOutputPath string,
	onProgress OCRProgressFunc,
) (bitmapOCRResult, error) {
	return runBitmapOCRWithTiming(ctx, mediaPath, stream, format, language, mode, outputPath, rawOutputPath, onProgress, false, 0)
}

func runBitmapOCRWithTiming(
	ctx context.Context,
	mediaPath string,
	stream FFProbeStream,
	format, language, mode, outputPath, rawOutputPath string,
	onProgress OCRProgressFunc,
	preparedLocal bool,
	preparationStart float64,
) (bitmapOCRResult, error) {
	result := bitmapOCRResult{}
	mode = normalizedOCRMode(mode)
	dir := filepath.Dir(outputPath)

	ocrInputPath := mediaPath
	useTrackNumber := true
	extractionMessage := ""
	geometry := []bitmapCueGeometry{}
	pgsEvidence := pgsSUPCueEvidence{}
	pgsEvidenceKnown := false

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
			return bitmapOCRResult{Message: message}, err
		}

		extractionMessage = message
		ocrInputPath = pgsPath
		useTrackNumber = false
		geometry, pgsEvidence, pgsEvidenceKnown = bitmapSubtitleGeometryEvidenceForPath(pgsPath, stream.CodecName, preparedLocal, preparationStart)
		if pgsEvidenceKnown && pgsEvidence.BitmapObjectReferences == 0 {
			return bitmapOCRResult{Message: pgsCueEvidenceSummary(stream.CodecName, pgsEvidence, true)}, &bitmapNoEligibleCueError{
				format: format, evidence: pgsEvidence,
			}
		}
	default:
		geometry, _, _ = bitmapSubtitleGeometryEvidenceForPath(ocrInputPath, stream.CodecName, preparedLocal, preparationStart)
	}
	capabilities, capabilityMessage, err := probeTesseractLanguages(ctx)
	if err != nil {
		return bitmapOCRResult{Message: capabilityMessage}, err
	}
	if err := requireOCRLanguageAvailable(capabilities, language); err != nil {
		return bitmapOCRResult{Message: capabilityMessage}, err
	}
	ocrTimingProvenance := originalSourceAbsolute
	if preparedLocal {
		ocrTimingProvenance = preparedInputLocal
	}

	primary := runBitmapOCRLanguageAttempt(ctx, ocrInputPath, stream, format, language, mode, dir, "primary", useTrackNumber, onProgress)
	message := strings.TrimSpace(fmt.Sprintf("OCR requested language: %s\nOCR effective language: %s\nOCR mode: %s\n%s\n%s\n%s", language, language, mode, extractionMessage, pgsCueEvidenceSummary(stream.CodecName, pgsEvidence, pgsEvidenceKnown), primary.message))
	chosen := primary.chosenPath
	successfulLanguage := language
	if primary.zero {
		alternate := oppositeJapaneseOCRLanguage(language)
		if alternate == "" {
			message = appendZeroRecognitionDiagnostic(message, stream, []string{language}, mode, pgsEvidence, pgsEvidenceKnown)
			return bitmapOCRResult{Message: message, EffectiveLanguage: language}, conciseZeroRecognitionError(language, "")
		}
		if _, available := capabilities[alternate]; !available {
			message = appendZeroRecognitionDiagnostic(message, stream, []string{language}, mode, pgsEvidence, pgsEvidenceKnown)
			return bitmapOCRResult{Message: message}, fmt.Errorf("OCR recognized no subtitle text using %s; alternate Japanese orientation %s is unavailable in the running Tesseract runtime", language, alternate)
		}
		fallbackAttempt := runBitmapOCRLanguageAttempt(ctx, ocrInputPath, stream, format, alternate, mode, dir, "fallback", useTrackNumber, onProgress)
		message = strings.TrimSpace(message + "\nOCR fallback language: " + alternate + "\n" + fallbackAttempt.message)
		if fallbackAttempt.zero {
			message = appendZeroRecognitionDiagnostic(message, stream, []string{language, alternate}, mode, pgsEvidence, pgsEvidenceKnown)
			return bitmapOCRResult{Message: message}, conciseZeroRecognitionError(language, alternate)
		}
		if fallbackAttempt.err != nil {
			return bitmapOCRResult{Message: message}, fallbackAttempt.err
		}
		chosen = fallbackAttempt.chosenPath
		successfulLanguage = alternate
		message = strings.Replace(message, "OCR effective language: "+language, "OCR effective language: "+alternate, 1)
	} else if primary.err != nil {
		return bitmapOCRResult{Message: message}, primary.err
	}

	rawContent, rawReadErr := os.ReadFile(chosen)
	if rawReadErr != nil {
		return bitmapOCRResult{Message: message}, rawReadErr
	}
	rawCues := correlateBitmapSubtitleCuesWithTiming(parseCanonicalBitmapSubtitleCues(format, rawContent), geometry, ocrTimingProvenance, preparationStart)
	result.Message = message
	result.EffectiveLanguage = successfulLanguage
	result.Cues = rawCues
	result.Geometry = geometry
	if format == "ass" && len(geometry) > 0 && len(rawCues) > 0 {
		rawContent = renderBitmapASSCues(rawCues)
	}
	if err := os.WriteFile(rawOutputPath, rawContent, 0o644); err != nil {
		return bitmapOCRResult{Message: message}, err
	}
	if mode == "raw" {
		if format == "ass" && len(geometry) > 0 && len(rawCues) > 0 {
			return result, os.WriteFile(outputPath, rawContent, 0o644)
		}
		return result, copyFileContents(chosen, outputPath)
	}
	cleanMessage, cleanErr := runSeConvCleanup(ctx, chosen, format, successfulLanguage, outputPath)
	result.Message = strings.TrimSpace(message + "\n" + cleanMessage)
	if cleanErr != nil {
		return result, cleanErr
	}
	cleanContent, cleanReadErr := os.ReadFile(outputPath)
	if cleanReadErr != nil {
		return result, cleanReadErr
	}
	cleanCues := correlateBitmapSubtitleCuesWithTiming(parseCanonicalBitmapSubtitleCues(format, cleanContent), geometry, ocrTimingProvenance, preparationStart)
	result.Cues = cleanCues
	if format == "ass" && len(geometry) > 0 && len(cleanCues) > 0 {
		if err := os.WriteFile(outputPath, renderBitmapASSCues(cleanCues), 0o644); err != nil {
			return result, err
		}
	}
	return result, nil
}

func probeTesseractLanguages(ctx context.Context) (map[string]struct{}, string, error) {
	snapshot, err := resolveCanonicalOCRRuntimeProvenance(ctx)
	if err != nil {
		return nil, "", err
	}
	return cloneOCRCapabilities(snapshot.capabilities), snapshot.message, nil
}

func probeTesseractLanguagesUncached(ctx context.Context) (map[string]struct{}, string, error) {
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

func requireOCRLanguageAvailable(capabilities map[string]struct{}, language string) error {
	if _, available := capabilities[language]; available {
		return nil
	}
	return fmt.Errorf("OCR language %q is unavailable in the running Tesseract runtime; install its traineddata and retry", language)
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
		result.err = &bitmapOCRProcessError{language: language, diagnostics: firstResult.message, err: firstResult.err}
		return result
	}
	chosen := first
	if mode == "accurate" {
		second := filepath.Join(dir, "ocr-"+tag+"-colors."+format)
		secondResult := runSeConvOCRPassDetailed(ctx, inputPath, stream, format, language, second, true, useTrackNumber, onProgress)
		result.message = strings.TrimSpace(result.message + "\n" + secondResult.message)
		if secondResult.err != nil && ctx.Err() != nil {
			result.err = &bitmapOCRProcessError{language: language, diagnostics: secondResult.message, err: secondResult.err}
			return result
		}
		if secondResult.err == nil && preferSecondaryOCR(subtitleOCRFileScoreForLanguage(first, format, language), subtitleOCRFileScoreForLanguage(second, format, language)) {
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
	configureManagedProcessGroup(cmd)

	var output bytes.Buffer
	var outputMu sync.Mutex
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

	processLine := func(rawLine string) {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			return
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

	stdout := &ocrProcessOutput{onLine: processLine}
	stderr := &ocrProcessOutput{onLine: processLine}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	waitErr := waitSubtitleProcess(ctx, cmd, waitDone)
	stdout.flush()
	stderr.flush()

	outputMu.Lock()
	diagnostics := strings.TrimSpace(output.String())
	outputMu.Unlock()
	return diagnostics, waitErr
}

func ocrDictionaryFolder() string {
	if value := strings.TrimSpace(ocrDictionaryFolderOverride); value != "" {
		return value
	}
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
	return subtitleOCRFileScoreForLanguage(path, format, "")
}

func subtitleOCRFileScoreForLanguage(path, format, language string) float64 {
	content, err := os.ReadFile(path)
	if err != nil || !validSubtitleSidecar(format, content) {
		return math.Inf(-1)
	}
	return subtitleOCRScoreForLanguage(string(content), language)
}

func subtitleOCRScore(content string) float64 {
	return subtitleOCRScoreForLanguage(content, "")
}

func subtitleOCRScoreForLanguage(content, language string) float64 {
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
	score := float64(letters) + float64(digits)*0.1 - float64(suspicious)*6 - float64(replacement)*25
	score -= float64(ocrConfusionCount(content, language)) * 4
	if dictionary := ocrKnownWords(language); len(dictionary) > 0 {
		words := ocrWords(content)
		for _, word := range words {
			if _, ok := dictionary[strings.ToLower(word)]; ok {
				score += 8
				continue
			}
			// Unknown words are weak evidence only. Preserve likely proper nouns,
			// long names, and tokens that the supported dictionary does not know.
			if isLikelyProperNoun(word) || len([]rune(word)) > 12 {
				continue
			}
			score -= 2
		}
	}
	return score
}

var ocrWordPattern = regexp.MustCompile(`(?i)[[:alpha:]][[:alpha:]'’-]*`)
var ocrConfusionTokenPattern = regexp.MustCompile(`(?i)[[:alpha:]][[:alpha:][:digit:]|]*`)

func ocrWords(content string) []string {
	raw := ocrWordPattern.FindAllString(content, -1)
	words := make([]string, 0, len(raw))
	for _, word := range raw {
		if word != "" {
			words = append(words, word)
		}
	}
	return words
}

func isLikelyProperNoun(word string) bool {
	for _, r := range word {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func hunspellLanguageForOCR(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "eng", "en", "en_us", "english":
		return "en"
	case "spa", "es", "es_es", "spanish":
		return "es"
	default:
		return ""
	}
}

func ocrKnownWords(language string) map[string]struct{} {
	language = hunspellLanguageForOCR(language)
	if language == "" {
		return nil
	}
	folder := ocrDictionaryFolder()
	if folder == "" {
		return nil
	}
	cacheKey := folder + "\x00" + language
	ocrDictionaryCacheMu.Lock()
	if dictionary, ok := ocrDictionaryCache[cacheKey]; ok {
		ocrDictionaryCacheMu.Unlock()
		return dictionary
	}
	ocrDictionaryCacheMu.Unlock()
	entries, err := os.ReadDir(folder)
	if err != nil || len(entries) > 4096 {
		return nil
	}
	dictionaryPath := ""
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".dic") {
			continue
		}
		base := strings.TrimSuffix(strings.ToLower(entry.Name()), filepath.Ext(entry.Name()))
		if base == language {
			dictionaryPath = filepath.Join(folder, entry.Name())
			break
		}
		if dictionaryPath == "" && strings.HasPrefix(base, language+"_") {
			dictionaryPath = filepath.Join(folder, entry.Name())
		}
	}
	if dictionaryPath == "" {
		return nil
	}
	file, err := os.Open(dictionaryPath)
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, 8*1024*1024))
	scanner.Buffer(make([]byte, 1024), 64*1024)
	dictionary := make(map[string]struct{})
	for scanner.Scan() && len(dictionary) < 100000 {
		word := strings.TrimSpace(scanner.Text())
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		if slash := strings.IndexByte(word, '/'); slash >= 0 {
			word = word[:slash]
		}
		word = strings.ToLower(strings.TrimSpace(word))
		if word != "" && strings.IndexFunc(word, func(r rune) bool { return unicode.IsLetter(r) }) >= 0 {
			dictionary[word] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil || len(dictionary) == 0 {
		return nil
	}
	ocrDictionaryCacheMu.Lock()
	ocrDictionaryCache[cacheKey] = dictionary
	ocrDictionaryCacheMu.Unlock()
	return dictionary
}

func ocrDictionaryCandidate(token, language string) (string, bool) {
	dictionary := ocrKnownWords(language)
	if len(dictionary) == 0 || !strings.ContainsAny(token, "01") {
		return "", false
	}
	candidate := strings.NewReplacer("0", "o", "1", "l").Replace(token)
	if candidate == token {
		return "", false
	}
	if _, exists := dictionary[strings.ToLower(token)]; exists {
		return "", false
	}
	if _, exists := dictionary[strings.ToLower(candidate)]; !exists {
		return "", false
	}
	return candidate, true
}

func ocrConfusionCount(content, language string) int {
	count := 0
	for _, token := range ocrConfusionTokenPattern.FindAllString(content, -1) {
		if _, ok := ocrDictionaryCandidate(token, language); ok {
			count++
		}
	}
	return count
}

func preferSecondaryOCR(primary, secondary float64) bool {
	// Colour isolation is the reliable default for bitmap subtitles. Select the
	// alternate pass only when its structural score is materially better; tiny
	// differences usually indicate different OCR mistakes, not an improvement.
	margin := math.Max(25, math.Abs(primary)*0.05)
	return secondary > primary+margin
}

func pgsCueEvidenceSummary(codec string, evidence pgsSUPCueEvidence, known bool) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec != "hdmv_pgs_subtitle" && codec != "pgssub" {
		return fmt.Sprintf("Bitmap event/cue counts for codec %s: unknown (no bounded source counter is available)", codec)
	}
	if !known {
		return "PGS event/object/cue counts: unknown (SUP evidence was unavailable or incomplete)"
	}
	return fmt.Sprintf(
		"PGS parsed evidence: display sets=%d; presentation composition events=%d; bitmap object references=%d; geometry-backed cue records=%d",
		evidence.DisplaySets, evidence.PresentationCompositionEvents, evidence.BitmapObjectReferences, evidence.GeometryCueRecords,
	)
}

func appendZeroRecognitionDiagnostic(message string, stream FFProbeStream, languages []string, mode string, evidence pgsSUPCueEvidence, evidenceKnown bool) string {
	effectiveLanguage := "unknown"
	if len(languages) > 0 {
		effectiveLanguage = languages[0]
	}
	observedCounts := func(pattern *regexp.Regexp) string {
		matches := pattern.FindAllStringSubmatch(message, -1)
		values := make([]string, 0, len(matches))
		for _, match := range matches {
			if len(match) == 2 {
				values = append(values, match[1])
			}
		}
		if len(values) == 0 {
			return "unknown"
		}
		return strings.Join(values, ",")
	}
	diagnostic := fmt.Sprintf(
		"OCR zero-recognition evidence: stream=%d; effective input language=%s; successful OCR language=none; attempted languages=[%s]; mode=%s; %s; SeConv bitmap image counts=[%s]; SeConv no-text image counts=[%s]; candidate glyph presence=unknown; decision=required sidecar failed; no empty artifact was created.",
		stream.Index,
		effectiveLanguage,
		strings.Join(languages, ","),
		mode,
		pgsCueEvidenceSummary(stream.CodecName, evidence, evidenceKnown),
		observedCounts(ocrBitmapCountPattern),
		observedCounts(ocrDroppedCountPattern),
	)
	// canonicalOCRFailure bounds displayed diagnostics from the front, so put
	// the structured decision first and keep raw tool output behind it.
	return strings.TrimSpace(diagnostic + "\n" + message)
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
	value := strings.TrimSpace(requested)
	if value == "" {
		value = detected
	}
	value = normalizedTrackLanguageValue(value)
	switch value {
	case "es", "spa", "esp", "spanish":
		return "spa"
	case "ko", "kor", "korean":
		return "kor"
	case "fr", "fra", "fre", "french":
		return "fra"
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
