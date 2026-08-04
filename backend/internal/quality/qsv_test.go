package quality

import "testing"

func TestQSVTranslatorUsesLAICQAndAssetComplexity(t *testing.T) {
	intent := NewIntent(IntentInput{
		Preset: "recommended", SourcePath: "/media/raw/anime/Akira.mkv",
		SourceWidth: 720, SourceHeight: 460, SourceVideoBitrate: 6_000_000, DurationSeconds: 100,
		GrainScore: .5, GrainScoreKnown: true, RequestedRateControl: "la_icq", RequestedBitDepth: 10,
	})
	recommendation, err := (QSVTranslator{}).Translate(intent, WorkerCapabilities{Main: true, Main10: true, ICQ: true, LAICQ: true})
	if err != nil || recommendation.EffectiveRateControl != "la_icq" || recommendation.GlobalQuality == nil || *recommendation.GlobalQuality != 25 || !recommendation.LookAhead {
		t.Fatalf("unexpected QSV recommendation: %#v err=%v", recommendation, err)
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
