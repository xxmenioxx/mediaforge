package quality

import "testing"

func TestQSVTranslatorUsesLAICQAndAssetComplexity(t *testing.T) {
	intent := NewIntent(IntentInput{
		Preset: "high_quality", SourcePath: "/media/raw/anime/Akira.mkv",
		SourceWidth: 720, SourceHeight: 460, SourceVideoBitrate: 6_000_000, DurationSeconds: 100,
		GrainScore: .5, GrainScoreKnown: true, RequestedRateControl: "la_icq", RequestedBitDepth: 10,
	})
	recommendation, err := (QSVTranslator{}).Translate(intent, WorkerCapabilities{Main: true, Main10: true, ICQ: true, LAICQ: true})
	if err != nil || recommendation.EffectiveRateControl != "la_icq" || recommendation.GlobalQuality == nil || *recommendation.GlobalQuality != 19 || !recommendation.LookAhead {
		t.Fatalf("unexpected QSV recommendation: %#v err=%v", recommendation, err)
	}
}

func TestQSVTranslatorAdvancedFeaturesRespectPresetAndIndependentCapabilities(t *testing.T) {
	capability := WorkerCapabilities{
		ICQ: true, Main10: true, AdaptiveI: true, AdaptiveB: true,
		VBRExtendedBRC: true, VBRLookAhead: true, VBR: true,
	}
	lowPreset, err := (QSVTranslator{}).Translate(NewIntent(IntentInput{
		Preset: "recommended", RequestedBitDepth: 10, RequestedRateControl: "icq",
	}), capability)
	if err != nil || lowPreset.AdaptiveI || lowPreset.AdaptiveB || lowPreset.ExtendedBRC || lowPreset.LookAhead {
		t.Fatalf("advanced QSV features must remain off below High Quality: %#v err=%v", lowPreset, err)
	}
	highPreset, err := (QSVTranslator{}).Translate(NewIntent(IntentInput{
		Preset: "high_quality", RequestedBitDepth: 10, RequestedRateControl: "icq",
	}), capability)
	if err != nil || !highPreset.AdaptiveI || !highPreset.AdaptiveB {
		t.Fatalf("independent Adaptive I/B capabilities were not applied: %#v err=%v", highPreset, err)
	}
	vbrPreset, err := (QSVTranslator{}).Translate(NewIntent(IntentInput{
		Preset: "high_quality", RequestedBitDepth: 10, RequestedRateControl: "vbr", SourceVideoBitrate: 4_000_000,
	}), capability)
	if err != nil || !vbrPreset.ExtendedBRC || !vbrPreset.LookAhead {
		t.Fatalf("contextual VBR features were not applied: %#v err=%v", vbrPreset, err)
	}
}

func TestQSVTranslatorFallsBackInCapabilityOrder(t *testing.T) {
	intent := NewIntent(IntentInput{Preset: "recommended", SourceVideoBitrate: 4_000_000, RequestedRateControl: "la_icq"})
	for _, test := range []struct {
		name       string
		capability WorkerCapabilities
		want       string
	}{
		{"icq", WorkerCapabilities{ICQ: true, CQP: true, VBR: true, CBR: true}, "icq"},
		{"cqp", WorkerCapabilities{CQP: true, VBR: true, CBR: true}, "cqp"},
		{"vbr", WorkerCapabilities{VBR: true, CBR: true}, "vbr"},
		{"cbr", WorkerCapabilities{CBR: true}, "cbr"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recommendation, err := (QSVTranslator{}).Translate(intent, test.capability)
			if err != nil || recommendation.EffectiveRateControl != test.want || recommendation.RateControlFallback == "" {
				t.Fatalf("unexpected fallback: %#v err=%v", recommendation, err)
			}
		})
	}
}

func TestQSVTranslatorEstimateConfidence(t *testing.T) {
	base := IntentInput{Preset: "recommended", SourceVideoBitrate: 4_000_000, DurationSeconds: 100, RequestedRateControl: "icq"}
	capability := WorkerCapabilities{ICQ: true}
	low, _ := (QSVTranslator{}).Translate(NewIntent(base), capability)
	if low.EstimateConfidence != "low" {
		t.Fatalf("expected low estimate: %#v", low)
	}
	base.HistoricalRatioMin, base.HistoricalRatioMax, base.HistoricalSamples = .2, .3, 4
	medium, _ := (QSVTranslator{}).Translate(NewIntent(base), capability)
	if medium.EstimateConfidence != "medium" {
		t.Fatalf("expected medium estimate: %#v", medium)
	}
	base.MeasuredVideoBitrate = 900_000
	high, _ := (QSVTranslator{}).Translate(NewIntent(base), capability)
	if high.EstimateConfidence != "high" || high.EstimatedVideoBitrate == nil || *high.EstimatedVideoBitrate != 900_000 {
		t.Fatalf("expected high estimate: %#v", high)
	}
}

func TestQSVEstimatedBitratesRoundToTwoDecimalMbps(t *testing.T) {
	recommendation, err := (QSVTranslator{}).Translate(NewIntent(IntentInput{
		Preset: "recommended", SourceVideoBitrate: 3_333_333, RequestedRateControl: "icq",
	}), WorkerCapabilities{ICQ: true})
	if err != nil || recommendation.EstimatedVideoBitrateMin == nil || recommendation.EstimatedVideoBitrateMax == nil {
		t.Fatalf("missing QSV estimate: %#v err=%v", recommendation, err)
	}
	if *recommendation.EstimatedVideoBitrateMin%10_000 != 0 || *recommendation.EstimatedVideoBitrateMax%10_000 != 0 {
		t.Fatalf("QSV Mbps estimates must have at most two decimals: %#v", recommendation)
	}
}
