package handlers

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	subtitleSyncAutoWindowMilliseconds = int64(100)
	subtitleSyncAutoSearchLimitMs      = int64(60_000)
	subtitleSyncAutoMinimumCorrelation = 0.20
	subtitleSyncAutoMinimumMargin      = 0.025
	subtitleSyncAutoHighConfidence     = 0.35
	maxSubtitleSyncAutoWindows         = 3 * 60 * 60 * 1000 / int(subtitleSyncAutoWindowMilliseconds)
	subtitleSyncAutoSampleRate         = 8_000
	subtitleSyncAutoSamplesPerWindow   = int(subtitleSyncAutoSampleRate * subtitleSyncAutoWindowMilliseconds / 1000)
	subtitleSyncAutoMaxAddressSpace    = 512 << 20
	subtitleSyncAutoMaxCPUSeconds      = 90
	subtitleSyncAutoMaxWallTime        = 120 * time.Second
	subtitleSyncAutoMaxAllocation      = 128 << 20
	subtitleSyncAutoMaxStderrBytes     = 32 << 10
	subtitleSyncAutoEngineVersion      = "audio-activity-correlation-v1"
	subtitleSyncAutoEngine             = "mvforge-audio-activity"
	subtitleSyncLimitedExecName        = "mvforge-subtitle-sync-limited-exec"
)

type subtitleSyncEstimate struct {
	offsetMilliseconds int64
	confidence         float64
	qualityState       string
	diagnostics        []string
}

type subtitleSyncPCMDecoder interface {
	Decode(context.Context, string, int, int) ([]float64, error)
}

type ffmpegSubtitleSyncPCMDecoder struct{}

type automaticSubtitleSyncEngine struct {
	decoder subtitleSyncPCMDecoder
}

var subtitleSyncAnalysisSlots = make(chan struct{}, 1)

func (automaticSubtitleSyncEngine) Apply(ctx context.Context, source []byte, proposal SubtitleSyncProposal) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSubtitleSyncProposalShape(proposal); err != nil {
		return nil, err
	}
	if proposal.Model != subtitleSyncModelFixedOffset || proposal.Engine != subtitleSyncAutoEngine || proposal.EngineVersion != subtitleSyncAutoEngineVersion {
		return nil, errors.New("unsupported automatic subtitle sync proposal")
	}
	if proposal.QualityState != "automatic_review" {
		return nil, errors.New("low-confidence automatic subtitle sync proposals cannot be applied")
	}
	if proposal.ProposalID == "" || proposal.ProposalID != subtitleSyncProposalID(proposal) {
		return nil, errors.New("subtitle sync proposal identity is invalid")
	}
	output, _, err := applySubtitleSyncTransform("srt", source, proposal.OffsetMilliseconds)
	return output, err
}

func (engine automaticSubtitleSyncEngine) Analyze(ctx context.Context, input SubtitleSyncAnalysisInput) (SubtitleSyncProposal, error) {
	if err := ctx.Err(); err != nil {
		return SubtitleSyncProposal{}, err
	}
	if strings.ToLower(strings.TrimSpace(input.Representation)) != "srt" {
		return SubtitleSyncProposal{}, errors.New("automatic subtitle alignment currently supports SRT only")
	}
	if strings.TrimSpace(input.MediaPath) == "" || strings.TrimSpace(input.SourceIdentity) == "" || strings.TrimSpace(input.SourceRevision) == "" {
		return SubtitleSyncProposal{}, errors.New("subtitle source, revision, and media path are required for automatic alignment")
	}
	if input.ReferenceAudio.StreamIndex < 0 || strings.TrimSpace(input.ReferenceAudio.Codec) == "" {
		return SubtitleSyncProposal{}, errors.New("an explicit resolved reference audio stream is required")
	}
	if engine.decoder == nil {
		return SubtitleSyncProposal{}, errors.New("bounded subtitle audio decoder is unavailable")
	}
	analysisCtx, cancel := context.WithTimeout(ctx, subtitleSyncAutoMaxWallTime)
	defer cancel()
	select {
	case subtitleSyncAnalysisSlots <- struct{}{}:
		defer func() { <-subtitleSyncAnalysisSlots }()
	case <-analysisCtx.Done():
		return SubtitleSyncProposal{}, analysisCtx.Err()
	}

	_, sourceTimings, err := applySubtitleSyncTransform("srt", input.Source, 0)
	if err != nil {
		return SubtitleSyncProposal{}, err
	}
	durationSeconds, err := subtitleSyncAutoDurationSeconds(sourceTimings)
	if err != nil {
		return SubtitleSyncProposal{}, err
	}
	audioRMS, err := engine.decoder.Decode(analysisCtx, input.MediaPath, input.ReferenceAudio.StreamIndex, durationSeconds)
	if err != nil {
		return SubtitleSyncProposal{}, err
	}
	estimate, err := estimateSubtitleSyncOffset(analysisCtx, sourceTimings, audioRMS)
	if err != nil {
		return SubtitleSyncProposal{}, err
	}
	_, proposedTimings, err := applySubtitleSyncTransform("srt", input.Source, estimate.offsetMilliseconds)
	if err != nil {
		return SubtitleSyncProposal{}, err
	}
	proposal := SubtitleSyncProposal{
		SubtitlePath: input.SubtitlePath, SourceIdentity: input.SourceIdentity, SourceRevision: input.SourceRevision,
		Representation: "srt", ReferenceAudio: input.ReferenceAudio,
		Engine: subtitleSyncAutoEngine, EngineVersion: subtitleSyncAutoEngineVersion, Model: subtitleSyncModelFixedOffset,
		OffsetMilliseconds: estimate.offsetMilliseconds, Confidence: estimate.confidence, Scale: 1,
		Segments: []SubtitleSyncSegment{}, QualityState: estimate.qualityState, AnalyzedAt: time.Now().UTC(),
		Diagnostics: estimate.diagnostics, Preview: subtitleSyncPreview(sourceTimings, proposedTimings),
	}
	if err := validateSubtitleSyncProposalShape(proposal); err != nil {
		return SubtitleSyncProposal{}, err
	}
	proposal.ProposalID = subtitleSyncProposalID(proposal)
	return proposal, nil
}

func subtitleSyncAutoDurationSeconds(timings []SubtitleSyncTiming) (int, error) {
	if len(timings) == 0 {
		return 0, errors.New("automatic subtitle alignment requires at least one cue")
	}
	var lastCueEnd int64
	for _, cue := range timings {
		if cue.StartMilliseconds < 0 || cue.EndMilliseconds <= cue.StartMilliseconds {
			return 0, errors.New("subtitle cue timing is invalid for automatic alignment")
		}
		if cue.EndMilliseconds > lastCueEnd {
			lastCueEnd = cue.EndMilliseconds
		}
	}
	const searchPaddingMs = subtitleSyncAutoSearchLimitMs
	const maximumDurationSeconds = 3 * 60 * 60
	maximumDurationMs := int64(maximumDurationSeconds * 1000)
	if lastCueEnd > maximumDurationMs-searchPaddingMs {
		return 0, errors.New("subtitle cues exceed the three-hour automatic analysis limit; manual fixed offset remains available")
	}
	durationMs := lastCueEnd + searchPaddingMs
	if durationMs < 60_000 {
		durationMs = 60_000
	}
	return int((durationMs + 999) / 1000), nil
}

func (ffmpegSubtitleSyncPCMDecoder) Decode(ctx context.Context, mediaPath string, streamIndex, durationSeconds int) ([]float64, error) {
	if runtime.GOOS != "linux" || streamIndex < 0 || durationSeconds <= 0 || durationSeconds > 3*60*60 {
		return nil, errors.New("bounded FFmpeg audio analysis is unavailable for this runtime or duration")
	}
	limiterPath, err := exec.LookPath(subtitleSyncLimitedExecName)
	if err != nil {
		return nil, errors.New("bounded FFmpeg analysis helper is unavailable")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("FFmpeg is unavailable for automatic subtitle alignment")
	}
	decodeCtx, cancel := context.WithTimeout(ctx, subtitleSyncAutoMaxWallTime)
	defer cancel()
	ffmpegArgs := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error", "-threads", "1", "-max_alloc", strconv.Itoa(subtitleSyncAutoMaxAllocation),
		"-i", mediaPath, "-map", fmt.Sprintf("0:%d", streamIndex), "-vn", "-sn", "-dn", "-t", strconv.Itoa(durationSeconds),
		"-threads", "1", "-filter_threads", "1", "-ac", "1", "-ar", strconv.Itoa(subtitleSyncAutoSampleRate),
		"-c:a", "pcm_s16le", "-f", "s16le", "pipe:1",
	}
	args := []string{"--address-space-bytes", strconv.Itoa(subtitleSyncAutoMaxAddressSpace), "--cpu-seconds", strconv.Itoa(subtitleSyncAutoMaxCPUSeconds), "--", ffmpegPath}
	args = append(args, ffmpegArgs...)
	command := exec.CommandContext(decodeCtx, limiterPath, args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &subtitleSyncBoundedStderr{limit: subtitleSyncAutoMaxStderrBytes}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return nil, errors.New("could not start bounded FFmpeg audio analysis")
	}
	maxBytes := int64(durationSeconds * subtitleSyncAutoSampleRate * 2)
	levels, readErr := collectSubtitleSyncPCMActivity(decodeCtx, stdout, maxBytes)
	if readErr != nil {
		cancel()
	}
	waitErr := command.Wait()
	if readErr != nil {
		if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
			return nil, readErr
		}
		return nil, fmt.Errorf("bounded FFmpeg audio analysis failed: %w", readErr)
	}
	if waitErr != nil {
		if decodeCtx.Err() != nil {
			return nil, decodeCtx.Err()
		}
		return nil, errors.New("bounded FFmpeg audio analysis exited unsuccessfully")
	}
	if len(levels) < 10 {
		return nil, errors.New("selected audio stream is too short for automatic alignment")
	}
	return levels, nil
}

type subtitleSyncBoundedStderr struct {
	limit int
	data  []byte
}

func (buffer *subtitleSyncBoundedStderr) Write(data []byte) (int, error) {
	inputLength := len(data)
	remaining := buffer.limit - len(buffer.data)
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		buffer.data = append(buffer.data, data...)
	}
	return inputLength, nil
}

func collectSubtitleSyncPCMActivity(ctx context.Context, reader io.Reader, maxBytes int64) ([]float64, error) {
	buffer := make([]byte, 64<<10)
	levels := make([]float64, 0, maxSubtitleSyncAutoWindows)
	var totalBytes int64
	var sumSquares float64
	var samples int
	var carry byte
	hasCarry := false
	process := func(value int16) {
		sample := float64(value)
		sumSquares += sample * sample
		samples++
		if samples == subtitleSyncAutoSamplesPerWindow {
			levels = append(levels, math.Sqrt(sumSquares/float64(samples)))
			sumSquares, samples = 0, 0
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, readErr := reader.Read(buffer)
		if count > 0 {
			totalBytes += int64(count)
			if totalBytes > maxBytes {
				return nil, errors.New("bounded FFmpeg audio output exceeded its duration limit")
			}
			data := buffer[:count]
			index := 0
			if hasCarry {
				process(int16(uint16(carry) | uint16(data[0])<<8))
				hasCarry = false
				index = 1
			}
			for ; index+1 < len(data); index += 2 {
				process(int16(binary.LittleEndian.Uint16(data[index : index+2])))
			}
			if index < len(data) {
				carry, hasCarry = data[index], true
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, readErr
		}
		if count == 0 {
			continue
		}
	}
	if hasCarry {
		return nil, errors.New("bounded FFmpeg audio output ended mid-sample")
	}
	if samples > 0 {
		levels = append(levels, math.Sqrt(sumSquares/float64(samples)))
	}
	return levels, nil
}

func subtitleSyncAutomaticEngineAvailable() bool {
	return subtitleSyncAutomaticUnavailableReason() == ""
}

func subtitleSyncAutomaticUnavailableReason() string {
	if runtime.GOOS != "linux" {
		return "Automatic alignment is unavailable because bounded FFmpeg analysis requires Linux. Manual fixed offset remains available."
	}
	if _, err := exec.LookPath(subtitleSyncLimitedExecName); err != nil {
		return "Automatic alignment is unavailable because the bounded FFmpeg analysis helper is missing. Manual fixed offset remains available."
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "Automatic alignment is unavailable because FFmpeg is missing from this runtime. Manual fixed offset remains available."
	}
	return ""
}

// estimateSubtitleSyncOffset compares cue occupancy against bounded audio
// activity windows. It returns an estimate for human review only; callers must
// reject low-confidence proposals during apply.
func estimateSubtitleSyncOffset(ctx context.Context, timings []SubtitleSyncTiming, audioRMS []float64) (subtitleSyncEstimate, error) {
	if len(timings) == 0 || len(audioRMS) < 10 || len(audioRMS) > maxSubtitleSyncAutoWindows {
		return subtitleSyncEstimate{}, errors.New("automatic subtitle synchronization requires bounded cue and audio evidence")
	}
	if err := ctx.Err(); err != nil {
		return subtitleSyncEstimate{}, err
	}
	threshold, err := subtitleSyncActivityThreshold(audioRMS)
	if err != nil {
		return subtitleSyncEstimate{}, err
	}
	audioActive := make([]bool, len(audioRMS))
	audioActiveCount := 0
	for index, level := range audioRMS {
		if !math.IsNaN(level) && !math.IsInf(level, 0) && level >= threshold {
			audioActive[index] = true
			audioActiveCount++
		}
	}
	if audioActiveCount < 10 || audioActiveCount*100/len(audioActive) > 95 {
		return subtitleSyncEstimate{}, errors.New("reference audio has insufficient silence/activity contrast for a safe estimate")
	}

	cueActive := make([]bool, len(audioRMS))
	for _, cue := range timings {
		if cue.StartMilliseconds < 0 || cue.EndMilliseconds <= cue.StartMilliseconds {
			return subtitleSyncEstimate{}, errors.New("subtitle cue timing is invalid for automatic alignment")
		}
		start := int(cue.StartMilliseconds / subtitleSyncAutoWindowMilliseconds)
		end := int((cue.EndMilliseconds + subtitleSyncAutoWindowMilliseconds - 1) / subtitleSyncAutoWindowMilliseconds)
		if end > len(cueActive) {
			return subtitleSyncEstimate{}, errors.New("subtitle cues exceed the bounded automatic audio window")
		}
		for index := start; index < end; index++ {
			cueActive[index] = true
		}
	}
	cueActiveCount := 0
	for _, active := range cueActive {
		if active {
			cueActiveCount++
		}
	}
	if cueActiveCount < 10 || cueActiveCount*100/len(cueActive) > 90 {
		return subtitleSyncEstimate{}, errors.New("subtitle cues have insufficient timing variation for a safe estimate")
	}

	bestOffset := int64(0)
	bestScore := math.Inf(-1)
	type candidateScore struct {
		offset int64
		score  float64
	}
	candidates := make([]candidateScore, 0, int(2*subtitleSyncAutoSearchLimitMs/subtitleSyncAutoWindowMilliseconds+1))
	windowCount := len(audioActive)
	for offset := -subtitleSyncAutoSearchLimitMs; offset <= subtitleSyncAutoSearchLimitMs; offset += subtitleSyncAutoWindowMilliseconds {
		if offset%500 == 0 {
			if err := ctx.Err(); err != nil {
				return subtitleSyncEstimate{}, err
			}
		}
		shift := int(offset / subtitleSyncAutoWindowMilliseconds)
		if !subtitleSyncShiftFits(timings, offset, int64(windowCount)*subtitleSyncAutoWindowMilliseconds) {
			continue
		}
		tp, fp, fn, tn := 0, 0, 0, 0
		for index, audio := range audioActive {
			cueIndex := index - shift
			cue := cueIndex >= 0 && cueIndex < len(cueActive) && cueActive[cueIndex]
			switch {
			case cue && audio:
				tp++
			case cue:
				fn++
			case audio:
				fp++
			default:
				tn++
			}
		}
		score := subtitleSyncPhiCorrelation(tp, fp, fn, tn)
		candidates = append(candidates, candidateScore{offset: offset, score: score})
		if score > bestScore {
			bestScore, bestOffset = score, offset
		}
	}
	if math.IsInf(bestScore, -1) {
		return subtitleSyncEstimate{}, errors.New("no valid subtitle offset fits the bounded audio evidence")
	}
	secondScore := math.Inf(-1)
	secondOffset := int64(0)
	for _, candidate := range candidates {
		if absInt64(candidate.offset-bestOffset) >= 1_000 && candidate.score > secondScore {
			secondScore, secondOffset = candidate.score, candidate.offset
		}
	}
	if math.IsInf(secondScore, -1) {
		secondScore = -1
	}
	margin := bestScore - secondScore
	confidence := math.Max(0, math.Min(1, (bestScore+1)/2))
	quality := "low_confidence_review"
	diagnostics := []string{fmt.Sprintf("Audio activity correlation %.3f; competing offset %+d ms scored %.3f.", bestScore, secondOffset, secondScore)}
	if bestScore >= subtitleSyncAutoHighConfidence && margin >= subtitleSyncAutoMinimumMargin {
		quality = "automatic_review"
		diagnostics = append(diagnostics, "This is an automatic fixed-offset estimate. Review the timing preview before applying; no media or subtitle was changed.")
	} else {
		diagnostics = append(diagnostics, "Confidence is low or ambiguous. Applying is disabled; use a manual fixed offset after reviewing the media.")
	}
	if bestScore < subtitleSyncAutoMinimumCorrelation {
		diagnostics = append(diagnostics, "Audio activity did not correlate strongly with subtitle cue timing.")
	}
	return subtitleSyncEstimate{offsetMilliseconds: bestOffset, confidence: confidence, qualityState: quality, diagnostics: diagnostics}, nil
}

func subtitleSyncActivityThreshold(levels []float64) (float64, error) {
	finite := make([]float64, 0, len(levels))
	for _, level := range levels {
		if !math.IsNaN(level) && !math.IsInf(level, 0) && level >= 0 {
			finite = append(finite, level)
		}
	}
	if len(finite) < 10 {
		return 0, errors.New("reference audio has insufficient activity evidence")
	}
	sort.Float64s(finite)
	p20 := finite[(len(finite)-1)*20/100]
	p85 := finite[(len(finite)-1)*85/100]
	if p85 < 150 || p85-p20 < 100 {
		return 0, errors.New("reference audio has insufficient level variation for a safe estimate")
	}
	threshold := p20 + (p85-p20)*0.35
	if threshold < 120 {
		threshold = 120
	}
	return threshold, nil
}

func subtitleSyncPhiCorrelation(tp, fp, fn, tn int) float64 {
	denominator := math.Sqrt(float64(tp+fp) * float64(tp+fn) * float64(tn+fp) * float64(tn+fn))
	if denominator == 0 {
		return -1
	}
	return float64(tp*tn-fp*fn) / denominator
}

func subtitleSyncShiftFits(timings []SubtitleSyncTiming, offset, duration int64) bool {
	for _, cue := range timings {
		start := cue.StartMilliseconds + offset
		end := cue.EndMilliseconds + offset
		if start < 0 || end > duration {
			return false
		}
	}
	return true
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
