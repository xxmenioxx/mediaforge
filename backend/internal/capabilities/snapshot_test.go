package capabilities

import "testing"

func TestDecodeEncoderCapabilityPreservesQSVModeEvidence(t *testing.T) {
	raw := map[string]any{
		"listed": true, "usable": true, "main10": true,
		"qsvIcqMain8": true, "qsvLaIcqMain10": false, "qsvCqpMain10": true,
		"qsvFullCombination": false, "reason": "look ahead rejected",
		"testedModes": map[string]bool{"qsvCqpMain10": true},
		"modeReasons": map[string]string{"qsvLaIcqMain10": "unsupported"},
	}
	capability, ok := DecodeEncoderCapability(raw)
	if !ok {
		t.Fatal("expected runtime snapshot capability to decode")
	}
	if !capability.QSVICQMain8 || !capability.QSVCQPMain10 || capability.QSVLAICQMain10 {
		t.Fatalf("QSV evidence was not preserved: %#v", capability)
	}
	if capability.ModeReasons["qsvLaIcqMain10"] != "unsupported" {
		t.Fatalf("mode reason missing: %#v", capability.ModeReasons)
	}
}
