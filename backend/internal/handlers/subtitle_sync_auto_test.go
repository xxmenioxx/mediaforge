package handlers

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

type fixedSubtitleSyncPCMDecoder struct {
	levels   []float64
	path     string
	stream   int
	duration int
}

func (decoder *fixedSubtitleSyncPCMDecoder) Decode(ctx context.Context, path string, stream, duration int) ([]float64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	decoder.path, decoder.stream, decoder.duration = path, stream, duration
	return append([]float64(nil), decoder.levels...), nil
}

func TestEstimateSubtitleSyncOffsetFindsBoundedSyntheticShift(t *testing.T) {
	const windowMs = int64(100)
	const durationMs = int64(180_000)
	rms := make([]float64, durationMs/windowMs)
	for index := range rms {
		rms[index] = 200
	}
	timings := make([]SubtitleSyncTiming, 0, 20)
	const wantOffset = int64(1_200)
	for index := 0; index < 20; index++ {
		start := int64(5_000 + index*8_000)
		end := start + 2_000
		timings = append(timings, SubtitleSyncTiming{StartMilliseconds: start, EndMilliseconds: end})
		for sample := (start + wantOffset) / windowMs; sample < (end+wantOffset)/windowMs; sample++ {
			rms[sample] = 5_000
		}
	}

	got, err := estimateSubtitleSyncOffset(context.Background(), timings, rms)
	if err != nil {
		t.Fatal(err)
	}
	if got.offsetMilliseconds != wantOffset {
		t.Fatalf("estimated offset %d ms, want %d ms", got.offsetMilliseconds, wantOffset)
	}
	if got.qualityState != "automatic_review" || got.confidence <= 0 {
		t.Fatalf("expected a reviewable high-confidence estimate, got %#v", got)
	}
}

func TestEstimateSubtitleSyncOffsetKeepsUncorrelatedEvidenceLowConfidence(t *testing.T) {
	const windowMs = int64(100)
	rms := make([]float64, 180_000/windowMs)
	for index := range rms {
		rms[index] = 200
		if index%10 < 3 {
			rms[index] = 5_000
		}
	}
	timings := make([]SubtitleSyncTiming, 0, 20)
	for index := 0; index < 20; index++ {
		start := int64(5_000 + index*8_000)
		timings = append(timings, SubtitleSyncTiming{StartMilliseconds: start, EndMilliseconds: start + 2_000})
	}

	got, err := estimateSubtitleSyncOffset(context.Background(), timings, rms)
	if err != nil {
		t.Fatal(err)
	}
	if got.qualityState != "low_confidence_review" {
		t.Fatalf("uncorrelated evidence must remain review-only, got %#v", got)
	}
}

func TestEstimateSubtitleSyncOffsetHonorsCancellationAndBounds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := estimateSubtitleSyncOffset(ctx, []SubtitleSyncTiming{{StartMilliseconds: 0, EndMilliseconds: 1_000}}, make([]float64, 1_000)); err == nil {
		t.Fatal("expected cancelled analysis to fail")
	}
	if _, err := estimateSubtitleSyncOffset(context.Background(), []SubtitleSyncTiming{{StartMilliseconds: 0, EndMilliseconds: 1_000}}, make([]float64, maxSubtitleSyncAutoWindows+1)); err == nil {
		t.Fatal("expected oversized analysis to fail closed")
	}
}

func TestCollectSubtitleSyncPCMActivityStreamsBoundedRMSWindows(t *testing.T) {
	pcm := make([]byte, 1_600*2)
	for index := 0; index < 1_600; index++ {
		level := int16(100)
		if index >= 800 {
			level = 200
		}
		binary.LittleEndian.PutUint16(pcm[index*2:], uint16(level))
	}
	got, err := collectSubtitleSyncPCMActivity(context.Background(), bytes.NewReader(pcm), int64(len(pcm)))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 100 || got[1] != 200 {
		t.Fatalf("unexpected bounded RMS windows: %#v", got)
	}
	if _, err := collectSubtitleSyncPCMActivity(context.Background(), bytes.NewReader(pcm), int64(len(pcm)-1)); err == nil {
		t.Fatal("expected audio output beyond the byte budget to fail closed")
	}
}

func TestSubtitleSyncAutomaticDurationEnforcesThreeHourLimit(t *testing.T) {
	duration, err := subtitleSyncAutoDurationSeconds([]SubtitleSyncTiming{{StartMilliseconds: 1_000, EndMilliseconds: 2_000}})
	if err != nil || duration != 62 {
		t.Fatalf("expected cue duration plus the 60-second search pad, got duration=%d err=%v", duration, err)
	}
	duration, err = subtitleSyncAutoDurationSeconds([]SubtitleSyncTiming{{StartMilliseconds: 1, EndMilliseconds: 10_740_000}})
	if err != nil || duration != 3*60*60 {
		t.Fatalf("expected bounded three-hour window, got duration=%d err=%v", duration, err)
	}
	if _, err := subtitleSyncAutoDurationSeconds([]SubtitleSyncTiming{{StartMilliseconds: 1, EndMilliseconds: 10_740_001}}); err == nil {
		t.Fatal("expected cues beyond the three-hour decode bound to be rejected")
	}
}

func TestAutomaticSubtitleSyncAnalyzeBuildsReviewableCanonicalSRTProposal(t *testing.T) {
	const windowMs = int64(100)
	const wantOffset = int64(1_200)
	var source bytes.Buffer
	rms := make([]float64, 2_200)
	for index := range rms {
		rms[index] = 200
	}
	for index := 0; index < 20; index++ {
		start := int64(5_000 + index*8_000)
		end := start + 2_000
		fmt.Fprintf(&source, "%d\n%s --> %s\nspoken cue %d\n\n", index+1, formatSRTTime(start), formatSRTTime(end), index+1)
		for sample := (start + wantOffset) / windowMs; sample < (end+wantOffset)/windowMs; sample++ {
			rms[sample] = 5_000
		}
	}
	decoder := &fixedSubtitleSyncPCMDecoder{levels: rms}
	inputSource := append([]byte(nil), source.Bytes()...)
	proposal, err := (automaticSubtitleSyncEngine{decoder: decoder}).Analyze(context.Background(), SubtitleSyncAnalysisInput{
		SubtitlePath: "/media/episode.srt", SourceIdentity: "external:/media/episode.srt", SourceRevision: "sha256:source",
		Representation: "srt", MediaPath: "/media/episode.mkv", Source: inputSource,
		ReferenceAudio: SubtitleSyncAudioReference{StreamIndex: 2, StreamID: "0x2", Codec: "aac"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Model != subtitleSyncModelFixedOffset || proposal.Engine != subtitleSyncAutoEngine || proposal.EngineVersion != subtitleSyncAutoEngineVersion || proposal.QualityState != "automatic_review" || proposal.OffsetMilliseconds != wantOffset || proposal.Confidence <= 0 || proposal.ProposalID == "" {
		t.Fatalf("expected reviewable fixed-offset proposal, got %#v", proposal)
	}
	if proposal.Representation != "srt" || proposal.Scale != 1 || len(proposal.Segments) != 0 || len(proposal.Preview) == 0 || proposal.Preview[0].CurrentStartMs != 5_000 || proposal.Preview[0].ProposedStartMs != proposal.Preview[0].CurrentStartMs+wantOffset {
		t.Fatalf("unexpected canonical preview proposal: %#v", proposal)
	}
	if !bytes.Equal(source.Bytes(), inputSource) || decoder.path != "/media/episode.mkv" || decoder.stream != 2 || decoder.duration > 3*60*60 {
		t.Fatalf("analysis changed source or escaped selected-audio bounds: sourceChanged=%t decoder=%#v", !bytes.Equal(source.Bytes(), inputSource), decoder)
	}
	output, err := (automaticSubtitleSyncEngine{}).Apply(context.Background(), inputSource, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), "00:00:05,000 --> 00:00:07,000") || !strings.Contains(string(output), "00:00:06,200 --> 00:00:08,200") {
		t.Fatalf("apply did not use reviewed automatic offset: %s", output[:min(len(output), 300)])
	}
}

func TestAutomaticSubtitleSyncApplyRejectsLowConfidenceAndForgedProposal(t *testing.T) {
	engine := automaticSubtitleSyncEngine{}
	proposal := SubtitleSyncProposal{Engine: subtitleSyncAutoEngine, EngineVersion: subtitleSyncAutoEngineVersion, Model: subtitleSyncModelFixedOffset, OffsetMilliseconds: 500, Scale: 1, QualityState: "low_confidence_review"}
	proposal.ProposalID = subtitleSyncProposalID(proposal)
	if _, err := engine.Apply(context.Background(), []byte("1\n00:00:01,000 --> 00:00:02,000\ncue\n"), proposal); err == nil {
		t.Fatal("expected low-confidence automatic proposal to remain review-only")
	}
	proposal.QualityState = "automatic_review"
	if _, err := engine.Apply(context.Background(), []byte("1\n00:00:01,000 --> 00:00:02,000\ncue\n"), proposal); err == nil {
		t.Fatal("expected quality-state mutation to invalidate automatic proposal")
	}
}

func TestAutomaticSubtitleSyncRejectsASSBeforeDecoding(t *testing.T) {
	_, err := (automaticSubtitleSyncEngine{}).Analyze(context.Background(), SubtitleSyncAnalysisInput{Representation: "ass"})
	if err == nil || !strings.Contains(err.Error(), "supports SRT only") {
		t.Fatalf("expected ASS automatic analysis to be rejected explicitly, got %v", err)
	}
}

func formatSRTTime(milliseconds int64) string {
	hours := milliseconds / 3_600_000
	minutes := milliseconds % 3_600_000 / 60_000
	seconds := milliseconds % 60_000 / 1_000
	fraction := milliseconds % 1_000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, fraction)
}
