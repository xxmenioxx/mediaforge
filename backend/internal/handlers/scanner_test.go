package handlers

import "testing"

func TestIsHDRDoesNotTreatTenBitSDRAsHDR(t *testing.T) {
	stream := FFProbeStream{
		Profile: "Main 10", PixFmt: "yuv420p10le",
		ColorTransfer: "smpte170m", ColorPrimaries: "smpte170m",
	}
	if isHDR(stream) {
		t.Fatal("10-bit SDR must not be classified as HDR")
	}
}

func TestIsHDRDetectsPQAndHDRMetadata(t *testing.T) {
	if !isHDR(FFProbeStream{ColorTransfer: "smpte2084", ColorPrimaries: "bt2020"}) {
		t.Fatal("PQ transfer must be classified as HDR")
	}
	if !isHDR(FFProbeStream{SideDataList: []map[string]any{{"side_data_type": "Mastering display metadata"}}}) {
		t.Fatal("mastering display metadata must be classified as HDR")
	}
}

func TestStreamSizeBytesUsesMatroskaStatistics(t *testing.T) {
	stream := FFProbeStream{Tags: map[string]string{"NUMBER_OF_BYTES-eng": "46764973", "BPS-eng": "252532"}}
	if got := streamSizeBytes(stream); got != 46764973 {
		t.Fatalf("size=%d", got)
	}
	if got := streamBitrate(stream); got != 252532 {
		t.Fatalf("bitrate=%d", got)
	}
}
