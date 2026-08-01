package capabilities

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type EncoderCapability struct {
	Listed, Usable     bool
	Reason             string
	Main10             bool
	ICQ                bool
	LowPower           bool
	LookAhead          bool
	ExtendedBRC        bool
	AdaptiveI          bool
	AdaptiveB          bool
	QSVFullCombination bool
	VideoToolboxMain   bool
	VideoToolboxMain10 bool
}

var encoderCandidates = []string{
	"libx264",
	"libx265",
	"libsvtav1",
	"hevc_qsv",
	"hevc_vaapi",
	"hevc_nvenc",
	"hevc_videotoolbox",
	"hevc_amf",
}

var encoderCache = struct {
	sync.Mutex
	listedChecked bool
	listed        map[string]bool
	probed        map[string]EncoderCapability
}{listed: map[string]bool{}, probed: map[string]EncoderCapability{}}

// ResetEncoderCache forces the next capability check to query FFmpeg and run
// hardware smoke tests again. It is used by explicit runtime refreshes; normal
// scheduler checks remain cached to avoid spawning FFmpeg for every plan.
func ResetEncoderCache() {
	encoderCache.Lock()
	defer encoderCache.Unlock()
	encoderCache.listedChecked = false
	encoderCache.listed = map[string]bool{}
	encoderCache.probed = map[string]EncoderCapability{}
}

func CheckEncoder(encoder string) EncoderCapability {
	encoder = strings.TrimSpace(encoder)
	if encoder == "" {
		return EncoderCapability{Reason: "encoder is empty"}
	}
	if encoder == "copy" {
		return EncoderCapability{Listed: true, Usable: true}
	}
	encoderCache.Lock()
	defer encoderCache.Unlock()
	if cached, ok := encoderCache.probed[encoder]; ok {
		return cached
	}
	if !encoderCache.listedChecked {
		encoderCache.listedChecked = true
		if output, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").CombinedOutput(); err == nil {
			text := string(output)
			for _, candidate := range encoderCandidates {
				encoderCache.listed[candidate] = strings.Contains(text, candidate)
			}
		}
	}
	if !encoderCache.listed[encoder] {
		result := EncoderCapability{Reason: "encoder is not listed by FFmpeg"}
		encoderCache.probed[encoder] = result
		return result
	}
	result := EncoderCapability{Listed: true, Usable: true}
	if isHardwareEncoder(encoder) {
		mainPixelFormat := "yuv420p"
		if strings.HasSuffix(encoder, "_qsv") || strings.HasSuffix(encoder, "_vaapi") {
			mainPixelFormat = "nv12"
		}
		var mainReason, main10Reason string
		result.Usable, mainReason = hardwareEncoderSmokeTest(encoder, mainPixelFormat)
		result.Main10, main10Reason = hardwareEncoderSmokeTest(encoder, "p010le")
		if !result.Usable && result.Main10 {
			result.Usable = true
		}
		if !result.Usable {
			reason := mainReason
			if reason == "" {
				reason = main10Reason
			}
			result.Reason = encoder + " is listed but could not complete a hardware encoding smoke test"
			if reason != "" {
				result.Reason += ": " + reason
			}
		} else if !result.Main10 {
			result.Reason = encoder + " is usable for HEVC Main but Main10 is unavailable"
		}
		if encoder == "hevc_qsv" && result.Usable {
			result.ICQ = qsvFeatureSmokeTest(mainPixelFormat, "-global_quality", "18")
			result.LowPower = qsvFeatureSmokeTest(mainPixelFormat, "-low_power", "1")
			result.LookAhead = result.ICQ && qsvFeatureSmokeTest(mainPixelFormat, "-global_quality", "18", "-look_ahead", "1")
			result.ExtendedBRC = result.ICQ && qsvFeatureSmokeTest(mainPixelFormat, "-global_quality", "18", "-extbrc", "1", "-look_ahead_depth", "40")
			result.AdaptiveI = result.ICQ && qsvFeatureSmokeTest(mainPixelFormat, "-global_quality", "18", "-adaptive_i", "1")
			result.AdaptiveB = result.ICQ && qsvFeatureSmokeTest(mainPixelFormat, "-global_quality", "18", "-adaptive_b", "1")
			result.QSVFullCombination = result.Main10 && qsvFeatureSmokeTest("p010le", "-global_quality", "25", "-look_ahead", "1", "-look_ahead_depth", "40", "-extbrc", "1", "-adaptive_i", "1", "-adaptive_b", "1")
		}
		if encoder == "hevc_videotoolbox" {
			result.VideoToolboxMain = result.Usable
			result.VideoToolboxMain10 = result.Main10
		}
	}
	encoderCache.probed[encoder] = result
	return result
}

func isHardwareEncoder(encoder string) bool {
	return strings.HasSuffix(encoder, "_qsv") ||
		strings.HasSuffix(encoder, "_vaapi") ||
		strings.HasSuffix(encoder, "_nvenc") ||
		strings.HasSuffix(encoder, "_videotoolbox") ||
		strings.HasSuffix(encoder, "_amf")
}

func hardwareEncoderSmokeTest(encoder, pixelFormat string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ffmpeg", hardwareEncoderSmokeArgs(encoder, pixelFormat)...).CombinedOutput()
	if err == nil {
		return true, ""
	}
	reason := strings.Join(strings.Fields(string(output)), " ")
	if len(reason) > 500 {
		reason = reason[:500] + "…"
	}
	return false, reason
}

func hardwareEncoderSmokeArgs(encoder, pixelFormat string) []string {
	args := []string{
		"-hide_banner", "-loglevel", "error",
	}
	if strings.HasSuffix(encoder, "_qsv") {
		args = append(args, "-init_hw_device", "qsv=hw,child_device=/dev/dri/renderD128")
	} else if strings.HasSuffix(encoder, "_vaapi") {
		args = append(args, "-vaapi_device", "/dev/dri/renderD128")
	}
	args = append(args,
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30",
		"-frames:v", "30", "-an",
	)
	if strings.HasSuffix(encoder, "_vaapi") {
		args = append(args, "-vf", "format="+pixelFormat+",hwupload")
	} else {
		args = append(args, "-pix_fmt", pixelFormat)
	}
	args = append(args, "-c:v", encoder, "-b:v", "1M")
	if pixelFormat == "p010le" && (strings.HasSuffix(encoder, "_qsv") || strings.HasSuffix(encoder, "_vaapi") || encoder == "hevc_videotoolbox") {
		args = append(args, "-profile:v", "main10")
	}
	args = append(args, "-f", "null", "-")
	return args
}

func qsvFeatureSmokeTest(pixelFormat string, featureArgs ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-init_hw_device", "qsv=hw,child_device=/dev/dri/renderD128",
		"-f", "lavfi", "-i", "testsrc2=size=128x128:rate=30",
		"-frames:v", "30", "-an",
		"-c:v", "hevc_qsv",
		"-b:v", "1M",
		"-pix_fmt", pixelFormat,
	}
	args = append(args, featureArgs...)
	args = append(args, "-f", "null", "-")
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	_ = output
	return err == nil
}
