package capabilities

import (
	"slices"
	"strings"
	"testing"
)

func TestHardwareEncoderClassification(t *testing.T) {
	for _, encoder := range []string{"hevc_qsv", "hevc_vaapi", "hevc_nvenc", "hevc_videotoolbox", "h264_videotoolbox", "hevc_amf"} {
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

func TestQSVRateControlMethod(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "icq",
			output: "[hevc_qsv @ 0x123] TargetUsage: 4; RateControlMethod: ICQ\n",
			want:   "ICQ",
		},
		{
			name:   "vbr",
			output: "[hevc_qsv @ 0x123] TargetUsage: 4; RateControlMethod: VBR\n",
			want:   "VBR",
		},
		{
			name:   "missing",
			output: "[hevc_qsv @ 0x123] ExtBRC: OFF\n",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := qsvRateControlMethod([]byte(tt.output))
			if got != tt.want {
				t.Fatalf("qsvRateControlMethod() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseQSVGPBContext(t *testing.T) {
	gpbOn, refDistOne, bRefOff := parseQSVGPBContext("GopPicSize: 75\nGopRefDist: 1\nBRefType: off\nGPB: ON\n")
	if !gpbOn || !refDistOne || !bRefOff {
		t.Fatalf("expected observed NAS GPB context, got gpb=%t refDistOne=%t bRefOff=%t", gpbOn, refDistOne, bRefOff)
	}
	gpbOn, refDistOne, bRefOff = parseQSVGPBContext("GopRefDist: 4\nBRefType: pyramid\nGPB: OFF\n")
	if gpbOn || refDistOne || bRefOff {
		t.Fatalf("must not infer GPB context from unrelated QSV settings")
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

func TestHardwareFeatureProbesUseRepresentativeFrameSize(t *testing.T) {
	qsvArgs := qsvFeatureSmokeArgs("nv12")
	if !strings.Contains(strings.Join(qsvArgs, " "), "testsrc2=size=640x360:rate=30") {
		t.Fatalf("QSV feature probe uses an unreliable tiny frame: %v", qsvArgs)
	}
	videoToolboxArgs := videoToolboxFeatureSmokeArgs("yuv420p")
	if !strings.Contains(strings.Join(videoToolboxArgs, " "), "testsrc2=size=640x360:rate=30") {
		t.Fatalf("VideoToolbox feature probe uses an unreliable tiny frame: %v", videoToolboxArgs)
	}
}

func TestVideoToolboxBFrameProbeRequiresObservedFrames(t *testing.T) {
	if observed, respected, reason := evaluateVideoToolboxBFrameProbe(3, "I\nP\nP\n"); observed != 0 || respected || !strings.Contains(reason, "produced no B-frames") {
		t.Fatalf("accepted-but-ineffective probe was not rejected: observed=%d respected=%t reason=%q", observed, respected, reason)
	}
	if observed, respected, reason := evaluateVideoToolboxBFrameProbe(3, "I\nB\nB\nP\n"); observed != 2 || !respected || reason != "" {
		t.Fatalf("effective B-frame probe was not accepted: observed=%d respected=%t reason=%q", observed, respected, reason)
	}
}

func TestVideoToolboxDisabledProbeDetectsIgnoredBFZero(t *testing.T) {
	if observed, respected, reason := evaluateVideoToolboxBFrameProbe(0, "I\nB\nP\n"); observed != 1 || respected || !strings.Contains(reason, "still produced B-frames") {
		t.Fatalf("ignored -bf 0 was not detected: observed=%d respected=%t reason=%q", observed, respected, reason)
	}
	if observed, respected, reason := evaluateVideoToolboxBFrameProbe(0, "I\nP\nP\n"); observed != 0 || !respected || reason != "" {
		t.Fatalf("effective -bf 0 was not accepted: observed=%d respected=%t reason=%q", observed, respected, reason)
	}
}
