package handlers

import (
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestPlaybackAnalysisExplainsJellyfinBitmapSubtitleTranscode(t *testing.T) {
	scan := models.ScanResult{
		Width: 720, Height: 480, VideoCodec: "hevc",
		VideoStreams: models.JSONList{models.JSONMap{
			"codec": "hevc", "sampleAspectRatio": "32:27", "displayAspectRatio": "16:9",
			"fieldOrder": "tt", "colorSpace": "smpte170m", "colorTransfer": "smpte170m",
		}},
		AudioStreams:      models.JSONList{models.JSONMap{"codec": "ac3", "channels": 6}},
		SubtitleStreams:   models.JSONList{models.JSONMap{"codec": "dvd_subtitle"}},
		InterlaceAnalysis: models.JSONMap{"status": "interlaced"},
	}
	result := buildPlaybackCompatibilityAnalysis(scan)
	if result["overall"] != "transcode_likely" || result["subtitles"] != "burn_in_likely" {
		t.Fatalf("unexpected compatibility result: %#v", result)
	}
	reasons := result["reasons"].(models.JSONList)
	joined := fmtStringList(reasons)
	for _, expected := range []string{"burn-in", "AAC stereo", "anamorphic", "deinterlace"} {
		if !strings.Contains(strings.ToLower(joined), strings.ToLower(expected)) {
			t.Fatalf("missing %q in reasons: %s", expected, joined)
		}
	}
	warnings := fmtStringList(result["warnings"].(models.JSONList))
	if !strings.Contains(warnings, "BT.709") {
		t.Fatalf("missing SD color conversion warning: %s", warnings)
	}
}

func fmtStringList(values models.JSONList) string {
	result := ""
	for _, value := range values {
		if text, ok := value.(string); ok {
			result += text + " "
		}
	}
	return result
}
