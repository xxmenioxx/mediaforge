package capabilities

import (
	"slices"
	"testing"
)

func TestHardwareEncoderClassification(t *testing.T) {
	for _, encoder := range []string{"hevc_qsv", "hevc_nvenc", "hevc_videotoolbox", "hevc_amf"} {
		if !isHardwareEncoder(encoder) {
			t.Fatalf("expected %s to require a smoke test", encoder)
		}
	}
	for _, encoder := range []string{"libx265", "libx264", "copy"} {
		if isHardwareEncoder(encoder) {
			t.Fatalf("did not expect %s to require a hardware smoke test", encoder)
		}
	}
}

func TestQSVSmokeTestUsesRepresentativeVideo(t *testing.T) {
	args := hardwareEncoderSmokeArgs("hevc_qsv", "p010le")
	for _, expected := range []string{"testsrc2=size=640x360:rate=30", "30", "hevc_qsv", "p010le", "main10", "0"} {
		if !slices.Contains(args, expected) {
			t.Fatalf("expected QSV smoke args to contain %q: %#v", expected, args)
		}
	}
}

func TestEncoderCapabilityCanReportMainWithoutMain10(t *testing.T) {
	capability := EncoderCapability{Listed: true, Usable: true, Main10: false, Reason: "Main10 is unavailable"}
	if !capability.Usable {
		t.Fatal("expected Main-only hardware encoder to remain usable")
	}
	if capability.Main10 {
		t.Fatal("did not expect Main10 support")
	}
}
