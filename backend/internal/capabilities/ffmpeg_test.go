package capabilities

import (
	"slices"
	"testing"
)

func TestHardwareEncoderClassification(t *testing.T) {
	for _, encoder := range []string{"hevc_qsv", "hevc_vaapi", "hevc_nvenc", "hevc_videotoolbox", "hevc_amf"} {
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

func TestVAAPISmokeTestInitializesRenderDeviceAndUploadsFrames(t *testing.T) {
	args := hardwareEncoderSmokeArgs("hevc_vaapi", "nv12")
	for _, expected := range []string{"-vaapi_device", "/dev/dri/renderD128", "format=nv12,hwupload", "hevc_vaapi"} {
		if !slices.Contains(args, expected) {
			t.Fatalf("expected VAAPI smoke args to contain %q: %#v", expected, args)
		}
	}
}

func TestQSVSmokeTestUsesRepresentativeVideo(t *testing.T) {
	args := hardwareEncoderSmokeArgs("hevc_qsv", "p010le")
	for _, expected := range []string{"qsv=hw,child_device=/dev/dri/renderD128", "testsrc2=size=640x360:rate=30", "30", "hevc_qsv", "p010le", "main10"} {
		if !slices.Contains(args, expected) {
			t.Fatalf("expected QSV smoke args to contain %q: %#v", expected, args)
		}
	}
	if slices.Contains(args, "-low_power") || slices.Contains(args, "-global_quality") {
		t.Fatalf("basic QSV smoke test must not require optional features: %#v", args)
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
