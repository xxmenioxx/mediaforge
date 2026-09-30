package quality

import "testing"

func TestEvaluateBPPPFUsesEffectiveGeometryFrameRateAndEncoderAssumptions(t *testing.T) {
	base := BPPPFInput{
		TargetVideoBitrate: 2_500_000, OutputWidth: 1920, OutputHeight: 1080,
		EffectiveFPS: 30, CodecFamily: "hevc", Encoder: "libx265", OptimizationIntent: "balanced",
	}
	baseResult := EvaluateBPPPF(base)
	if baseResult.Status != BPPPFStatusWarning || baseResult.BPPPF >= .045 {
		t.Fatalf("unexpected base result: %#v", baseResult)
	}
	if got := EvaluateBPPPF(BPPPFInput{TargetVideoBitrate: 2_500_000, OutputWidth: 1280, OutputHeight: 720, EffectiveFPS: 30, CodecFamily: "hevc", Encoder: "libx265", OptimizationIntent: "balanced"}); got.Status != BPPPFStatusNormal {
		t.Fatalf("lower output resolution did not raise BPPPF: %#v", got)
	}
	if got := EvaluateBPPPF(BPPPFInput{TargetVideoBitrate: 2_500_000, OutputWidth: 1920, OutputHeight: 1080, EffectiveFPS: 60, CodecFamily: "hevc", Encoder: "libx265", OptimizationIntent: "balanced"}); got.Status != BPPPFStatusHighRisk {
		t.Fatalf("higher output FPS did not lower BPPPF: %#v", got)
	}
	if got := EvaluateBPPPF(BPPPFInput{TargetVideoBitrate: 2_500_000, OutputWidth: 1920, OutputHeight: 1080, EffectiveFPS: 30, CodecFamily: "h264", Encoder: "libx265", OptimizationIntent: "balanced"}); got.ReferenceBitrate <= baseResult.ReferenceBitrate {
		t.Fatalf("codec family did not affect reference: %#v", got)
	}
	if got := EvaluateBPPPF(BPPPFInput{TargetVideoBitrate: 2_500_000, OutputWidth: 1920, OutputHeight: 1080, EffectiveFPS: 30, CodecFamily: "hevc", Encoder: "hevc_qsv", OptimizationIntent: "balanced"}); got.ReferenceBitrate <= baseResult.ReferenceBitrate {
		t.Fatalf("hardware encoder class did not affect reference: %#v", got)
	}
	if got := EvaluateBPPPF(BPPPFInput{TargetVideoBitrate: 2_500_000, OutputWidth: 1920, OutputHeight: 1080, EffectiveFPS: 30, CodecFamily: "hevc", Encoder: "libx265", HDR: true, OptimizationIntent: "balanced"}); got.ReferenceBitrate <= baseResult.ReferenceBitrate {
		t.Fatalf("HDR class did not affect reference: %#v", got)
	}
	if got := EvaluateBPPPF(BPPPFInput{TargetVideoBitrate: 2_500_000, OutputWidth: 1920, OutputHeight: 1080, EffectiveFPS: 30, CodecFamily: "hevc", Encoder: "libx265", OptimizationIntent: "archive"}); got.ReferenceBitrate <= baseResult.ReferenceBitrate {
		t.Fatalf("optimization goal did not affect reference: %#v", got)
	}
}

func TestEvaluateBPPPFBoundariesAndUnknownInputs(t *testing.T) {
	base := BPPPFInput{TargetVideoBitrate: 4_000_000, OutputWidth: 1920, OutputHeight: 1080, EffectiveFPS: 30, CodecFamily: "h264", Encoder: "libx264", OptimizationIntent: "balanced"}
	base.TargetVideoBitrate = 4_200_000
	if got := EvaluateBPPPF(base); got.Status != BPPPFStatusNormal {
		t.Fatalf("expected normal rail state: %#v", got)
	}
	base.TargetVideoBitrate = 3_276_000
	if got := EvaluateBPPPF(base); got.Status != BPPPFStatusWarning {
		t.Fatalf("expected warning rail state: %#v", got)
	}
	base.TargetVideoBitrate = 3_032_000
	if got := EvaluateBPPPF(base); got.Status != BPPPFStatusHighRisk {
		t.Fatalf("expected high-risk rail state: %#v", got)
	}
	base.EffectiveFPS = 0
	if got := EvaluateBPPPF(base); got.Status != BPPPFStatusUnavailable || !got.HeuristicOnly {
		t.Fatalf("unknown FPS must be unavailable planning context: %#v", got)
	}
	base.EffectiveFPS = 30
	base.Encoder = "unknown_encoder"
	if got := EvaluateBPPPF(base); got.Status != BPPPFStatusUnavailable {
		t.Fatalf("unknown encoder must not use a guessed efficiency factor: %#v", got)
	}
}

func TestBPPPFDoesNotCreatePerceptualVerdictsAndLABRemainsAuthoritative(t *testing.T) {
	result := EvaluateBPPPF(BPPPFInput{TargetVideoBitrate: 500_000, OutputWidth: 1920, OutputHeight: 1080, EffectiveFPS: 30, CodecFamily: "h264", Encoder: "libx264", OptimizationIntent: "maximum_quality"})
	if result.Status != BPPPFStatusHighRisk || !result.HeuristicOnly || result.Explanation == "" {
		t.Fatalf("expected an explanatory heuristic warning: %#v", result)
	}
	if result.Status == "failed" || result.Status == "passed" {
		t.Fatalf("BPPPF must not become a perceptual verdict: %#v", result)
	}
	if got := ParseFrameRate("24000/1001"); got < 23.97 || got > 23.98 {
		t.Fatalf("rational FPS parse=%f", got)
	}
}
