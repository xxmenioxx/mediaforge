package capabilities

import "testing"

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
