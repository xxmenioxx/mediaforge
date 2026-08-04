package capabilities

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DecodeEncoderCapability reconstructs the exact capability evidence stored in
// a runtime snapshot. Keeping this conversion here lets the scheduler, LAB and
// worker use the same probe result without reinterpreting individual fields.
func DecodeEncoderCapability(raw any) (EncoderCapability, bool) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return EncoderCapability{}, false
	}
	var value struct {
		Listed                     bool              `json:"listed"`
		Usable                     bool              `json:"usable"`
		Main10                     bool              `json:"main10"`
		ICQ                        bool              `json:"icq"`
		LowPower                   bool              `json:"lowPower"`
		LookAhead                  bool              `json:"lookAhead"`
		ExtendedBRC                bool              `json:"extendedBrc"`
		AdaptiveI                  bool              `json:"adaptiveI"`
		AdaptiveB                  bool              `json:"adaptiveB"`
		QSVFullCombination         bool              `json:"qsvFullCombination"`
		QSVICQMain8                bool              `json:"qsvIcqMain8"`
		QSVICQMain10               bool              `json:"qsvIcqMain10"`
		QSVLAICQMain10             bool              `json:"qsvLaIcqMain10"`
		QSVCQPMain8                bool              `json:"qsvCqpMain8"`
		QSVCQPMain10               bool              `json:"qsvCqpMain10"`
		QSVVBRMain8                bool              `json:"qsvVbrMain8"`
		QSVVBRMain10               bool              `json:"qsvVbrMain10"`
		QSVCBRMain8                bool              `json:"qsvCbrMain8"`
		QSVCBRMain10               bool              `json:"qsvCbrMain10"`
		QSVLowPowerMain10          bool              `json:"qsvLowPowerMain10"`
		VideoToolboxMain           bool              `json:"videoToolboxMain"`
		VideoToolboxMain10         bool              `json:"videoToolboxMain10"`
		VideoToolboxBFrames        bool              `json:"videoToolboxBFrames"`
		VideoToolboxPowerEfficient bool              `json:"videoToolboxPowerEfficient"`
		Reason                     string            `json:"reason"`
		TestedModes                map[string]bool   `json:"testedModes"`
		ModeReasons                map[string]string `json:"modeReasons"`
	}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return EncoderCapability{}, false
	}
	return EncoderCapability{
		Listed: value.Listed, Usable: value.Usable, Main10: value.Main10, ICQ: value.ICQ,
		LowPower: value.LowPower, LookAhead: value.LookAhead, ExtendedBRC: value.ExtendedBRC,
		AdaptiveI: value.AdaptiveI, AdaptiveB: value.AdaptiveB, QSVFullCombination: value.QSVFullCombination,
		QSVICQMain8: value.QSVICQMain8, QSVICQMain10: value.QSVICQMain10, QSVLAICQMain10: value.QSVLAICQMain10,
		QSVCQPMain8: value.QSVCQPMain8, QSVCQPMain10: value.QSVCQPMain10,
		QSVVBRMain8: value.QSVVBRMain8, QSVVBRMain10: value.QSVVBRMain10,
		QSVCBRMain8: value.QSVCBRMain8, QSVCBRMain10: value.QSVCBRMain10,
		QSVLowPowerMain10: value.QSVLowPowerMain10,
		VideoToolboxMain:  value.VideoToolboxMain, VideoToolboxMain10: value.VideoToolboxMain10,
		VideoToolboxBFrames: value.VideoToolboxBFrames, VideoToolboxPowerEfficient: value.VideoToolboxPowerEfficient,
		Reason: value.Reason, TestedModes: value.TestedModes, ModeReasons: value.ModeReasons,
	}, true
}

type EncoderCapability struct {
	Listed, Usable             bool
	Reason                     string
	Main10                     bool
	ICQ                        bool
	LowPower                   bool
	LookAhead                  bool
	ExtendedBRC                bool
	AdaptiveI                  bool
	AdaptiveB                  bool
	QSVFullCombination         bool
	QSVICQMain8                bool
	QSVICQMain10               bool
	QSVLAICQMain10             bool
	QSVCQPMain8                bool
	QSVCQPMain10               bool
	QSVVBRMain8                bool
	QSVVBRMain10               bool
	QSVCBRMain8                bool
	QSVCBRMain10               bool
	QSVLowPowerMain10          bool
	VideoToolboxMain           bool
	VideoToolboxMain10         bool
	VideoToolboxBFrames        bool
	VideoToolboxPowerEfficient bool
	TestedModes                map[string]bool
	ModeReasons                map[string]string
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
	result := EncoderCapability{Listed: true, Usable: true, TestedModes: map[string]bool{}, ModeReasons: map[string]string{}}
	if isHardwareEncoder(encoder) {
		mainPixelFormat := "yuv420p"
		if strings.HasSuffix(encoder, "_qsv") || strings.HasSuffix(encoder, "_vaapi") {
			mainPixelFormat = "nv12"
		}
		var mainReason, main10Reason string
		result.Usable, mainReason = hardwareEncoderSmokeTest(encoder, mainPixelFormat)
		result.Main10, main10Reason = hardwareEncoderSmokeTest(encoder, "p010le")
		result.TestedModes["main"] = result.Usable
		result.TestedModes["main10"] = result.Main10
		if !result.Usable {
			result.ModeReasons["main"] = mainReason
		}
		if !result.Main10 {
			result.ModeReasons["main10"] = main10Reason
		}
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
			probe := func(name, format string, args ...string) bool {
				passed, reason := qsvFeatureSmokeProbe(format, args...)
				result.TestedModes[name] = passed
				if !passed {
					result.ModeReasons[name] = reason
				}
				return passed
			}
			skip := func(name, reason string) bool {
				result.TestedModes[name] = false
				result.ModeReasons[name] = reason
				return false
			}
			result.QSVICQMain8 = probe("qsvIcqMain8", "nv12", "-profile:v", "main", "-global_quality", "25")
			result.QSVCQPMain8 = probe("qsvCqpMain8", "nv12", "-profile:v", "main", "-global_quality", "25", "-flags", "+qscale")
			result.QSVVBRMain8 = probe("qsvVbrMain8", "nv12", "-profile:v", "main", "-b:v", "2M", "-maxrate", "3M", "-bufsize", "4M")
			result.QSVCBRMain8 = probe("qsvCbrMain8", "nv12", "-profile:v", "main", "-b:v", "2M", "-maxrate", "2M", "-bufsize", "4M")
			if result.Main10 {
				result.QSVICQMain10 = probe("qsvIcqMain10", "p010le", "-profile:v", "main10", "-global_quality", "25")
				result.QSVCQPMain10 = probe("qsvCqpMain10", "p010le", "-profile:v", "main10", "-global_quality", "25", "-flags", "+qscale")
				result.QSVVBRMain10 = probe("qsvVbrMain10", "p010le", "-profile:v", "main10", "-b:v", "2M", "-maxrate", "3M", "-bufsize", "4M")
				result.QSVCBRMain10 = probe("qsvCbrMain10", "p010le", "-profile:v", "main10", "-b:v", "2M", "-maxrate", "2M", "-bufsize", "4M")
			} else {
				result.QSVICQMain10 = skip("qsvIcqMain10", "skipped because the QSV Main10 base probe failed")
				result.QSVCQPMain10 = skip("qsvCqpMain10", "skipped because the QSV Main10 base probe failed")
				result.QSVVBRMain10 = skip("qsvVbrMain10", "skipped because the QSV Main10 base probe failed")
				result.QSVCBRMain10 = skip("qsvCbrMain10", "skipped because the QSV Main10 base probe failed")
			}
			result.ICQ = result.QSVICQMain8 || result.QSVICQMain10
			result.LowPower = probe("qsvLowPowerMain8", "nv12", "-profile:v", "main", "-global_quality", "25", "-low_power", "1")
			if result.QSVICQMain10 {
				result.QSVLowPowerMain10 = probe("qsvLowPowerMain10", "p010le", "-profile:v", "main10", "-global_quality", "25", "-low_power", "1")
				result.QSVLAICQMain10 = probe("qsvLaIcqMain10", "p010le", "-profile:v", "main10", "-global_quality", "25", "-look_ahead", "1", "-look_ahead_depth", "40")
			} else {
				result.QSVLowPowerMain10 = skip("qsvLowPowerMain10", "skipped because QSV ICQ Main10 is unavailable")
				result.QSVLAICQMain10 = skip("qsvLaIcqMain10", "skipped because QSV ICQ Main10 is unavailable")
			}
			result.LookAhead = result.QSVLAICQMain10
			if result.QSVLAICQMain10 {
				result.ExtendedBRC = probe("qsvExtendedBrcMain10", "p010le", "-profile:v", "main10", "-global_quality", "25", "-look_ahead", "1", "-look_ahead_depth", "40", "-extbrc", "1")
			} else {
				result.ExtendedBRC = skip("qsvExtendedBrcMain10", "skipped because QSV LA-ICQ Main10 is unavailable")
			}
			if result.QSVICQMain10 {
				result.AdaptiveI = probe("qsvAdaptiveIMain10", "p010le", "-profile:v", "main10", "-global_quality", "25", "-adaptive_i", "1")
				result.AdaptiveB = probe("qsvAdaptiveBMain10", "p010le", "-profile:v", "main10", "-global_quality", "25", "-adaptive_b", "1")
			} else {
				result.AdaptiveI = skip("qsvAdaptiveIMain10", "skipped because QSV ICQ Main10 is unavailable")
				result.AdaptiveB = skip("qsvAdaptiveBMain10", "skipped because QSV ICQ Main10 is unavailable")
			}
			if result.QSVLAICQMain10 {
				result.QSVFullCombination = probe("qsvFullCombination", "p010le", "-profile:v", "main10", "-global_quality", "25", "-look_ahead", "1", "-look_ahead_depth", "40", "-extbrc", "1", "-adaptive_i", "1", "-adaptive_b", "1")
			} else {
				result.QSVFullCombination = skip("qsvFullCombination", "skipped because QSV LA-ICQ Main10 is unavailable")
			}
		}
		if encoder == "hevc_videotoolbox" {
			result.VideoToolboxMain = result.Usable
			result.VideoToolboxMain10 = result.Main10
			result.TestedModes["videoToolboxMain"] = result.Usable
			result.TestedModes["videoToolboxMain10"] = result.Main10
			if !result.Usable {
				result.ModeReasons["videoToolboxMain"] = mainReason
			}
			if !result.Main10 {
				result.ModeReasons["videoToolboxMain10"] = main10Reason
			}
			vtProbe := func(name, format string, args ...string) bool {
				passed, reason := videoToolboxFeatureSmokeProbe(format, args...)
				result.TestedModes[name] = passed
				if !passed {
					result.ModeReasons[name] = reason
				}
				return passed
			}
			result.VideoToolboxBFrames = result.Usable && vtProbe("videoToolboxBFramesMain", "yuv420p", "-profile:v", "main", "-bf", "3")
			result.VideoToolboxPowerEfficient = result.Usable && vtProbe("videoToolboxPowerEfficientMain", "yuv420p", "-profile:v", "main", "-power_efficient", "1")
			if result.Main10 {
				result.TestedModes["videoToolboxBFramesMain10"] = vtProbe("videoToolboxBFramesMain10", "p010le", "-profile:v", "main10", "-bf", "3")
				result.TestedModes["videoToolboxPowerEfficientMain10"] = vtProbe("videoToolboxPowerEfficientMain10", "p010le", "-profile:v", "main10", "-power_efficient", "1")
			} else {
				result.TestedModes["videoToolboxBFramesMain10"] = false
				result.TestedModes["videoToolboxPowerEfficientMain10"] = false
				result.ModeReasons["videoToolboxBFramesMain10"] = "skipped because the VideoToolbox Main10 base probe failed"
				result.ModeReasons["videoToolboxPowerEfficientMain10"] = "skipped because the VideoToolbox Main10 base probe failed"
			}
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
	passed, _ := qsvFeatureSmokeProbe(pixelFormat, featureArgs...)
	return passed
}

func qsvFeatureSmokeProbe(pixelFormat string, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := qsvFeatureSmokeArgs(pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	return err == nil, summarizedProbeReason(output, err)
}

func qsvFeatureSmokeArgs(pixelFormat string, featureArgs ...string) []string {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-init_hw_device", "qsv=hw,child_device=/dev/dri/renderD128",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30",
		"-frames:v", "30", "-an",
		"-c:v", "hevc_qsv",
		"-b:v", "1M",
		"-pix_fmt", pixelFormat,
	}
	args = append(args, featureArgs...)
	args = append(args, "-f", "null", "-")
	return args
}

func videoToolboxFeatureSmokeProbe(pixelFormat string, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := videoToolboxFeatureSmokeArgs(pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	return err == nil, summarizedProbeReason(output, err)
}

func videoToolboxFeatureSmokeArgs(pixelFormat string, featureArgs ...string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30", "-frames:v", "30", "-an", "-c:v", "hevc_videotoolbox", "-b:v", "1M", "-pix_fmt", pixelFormat}
	args = append(args, featureArgs...)
	args = append(args, "-f", "null", "-")
	return args
}

func summarizedProbeReason(output []byte, err error) string {
	if err == nil {
		return ""
	}
	reason := strings.Join(strings.Fields(string(output)), " ")
	if reason == "" {
		reason = err.Error()
	}
	if len(reason) > 500 {
		reason = reason[:500] + "…"
	}
	return reason
}
