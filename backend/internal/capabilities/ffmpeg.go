package capabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
		Listed                bool `json:"listed"`
		Usable                bool `json:"usable"`
		Main10                bool `json:"main10"`
		ICQ                   bool `json:"icq"`
		LowPower              bool `json:"lowPower"`
		LookAhead             bool `json:"lookAhead"`
		ExtendedBRC           bool `json:"extendedBrc"`
		AdaptiveI             bool `json:"adaptiveI"`
		AdaptiveB             bool `json:"adaptiveB"`
		QSVFullCombination    bool `json:"qsvFullCombination"`
		QSVICQMain8           bool `json:"qsvIcqMain8"`
		QSVICQMain10          bool `json:"qsvIcqMain10"`
		QSVLAICQMain10        bool `json:"qsvLaIcqMain10"`
		QSVLAICQMain8         bool `json:"qsvLaIcqMain8"`
		QSVCQPMain8           bool `json:"qsvCqpMain8"`
		QSVCQPMain10          bool `json:"qsvCqpMain10"`
		QSVVBRMain8           bool `json:"qsvVbrMain8"`
		QSVVBRMain10          bool `json:"qsvVbrMain10"`
		QSVCBRMain8           bool `json:"qsvCbrMain8"`
		QSVCBRMain10          bool `json:"qsvCbrMain10"`
		QSVLowPowerMain10     bool `json:"qsvLowPowerMain10"`
		QSVVBRExtBRCMain10    bool `json:"qsvVbrExtBrcMain10"`
		QSVVBRLookAheadMain10 bool `json:"qsvVbrLookAheadMain10"`
		QSVVBRExtBRCMain8     bool `json:"qsvVbrExtBrcMain8"`
		QSVVBRLookAheadMain8  bool `json:"qsvVbrLookAheadMain8"`

		QSVCBRExtBRCMain10    bool `json:"qsvCbrExtBrcMain10"`
		QSVCBRLookAheadMain10 bool `json:"qsvCbrLookAheadMain10"`
		QSVCBRExtBRCMain8     bool `json:"qsvCbrExtBrcMain8"`
		QSVCBRLookAheadMain8  bool `json:"qsvCbrLookAheadMain8"`

		QSVAdaptiveIMain10 bool `json:"qsvAdaptiveIMain10"`
		QSVAdaptiveBMain10 bool `json:"qsvAdaptiveBMain10"`
		QSVAdaptiveIMain8  bool `json:"qsvAdaptiveIMain8"`
		QSVAdaptiveBMain8  bool `json:"qsvAdaptiveBMain8"`

		QSVVBRAdvancedMain10        bool              `json:"qsvVbrAdvancedMain10"`
		QSVVBRAdvancedMain8         bool              `json:"qsvVbrAdvancedMain8"`
		VideoToolboxMain            bool              `json:"videoToolboxMain"`
		VideoToolboxMain10          bool              `json:"videoToolboxMain10"`
		VideoToolboxBFrames         bool              `json:"videoToolboxBFrames"`
		VideoToolboxBFramesVerified bool              `json:"videoToolboxBFramesVerified"`
		VideoToolboxBFramesDisabled bool              `json:"videoToolboxBFramesDisabled"`
		VideoToolboxObservedBFrames int               `json:"videoToolboxObservedBFrames"`
		VideoToolboxPowerEfficient  bool              `json:"videoToolboxPowerEfficient"`
		Reason                      string            `json:"reason"`
		TestedModes                 map[string]bool   `json:"testedModes"`
		ModeReasons                 map[string]string `json:"modeReasons"`
	}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return EncoderCapability{}, false
	}
	return EncoderCapability{
		Listed: value.Listed, Usable: value.Usable, Main10: value.Main10, ICQ: value.ICQ,
		LowPower: value.LowPower, LookAhead: value.LookAhead, ExtendedBRC: value.ExtendedBRC,
		AdaptiveI: value.AdaptiveI, AdaptiveB: value.AdaptiveB, QSVFullCombination: value.QSVFullCombination,

		QSVICQMain8: value.QSVICQMain8, QSVICQMain10: value.QSVICQMain10, QSVLAICQMain8: value.QSVLAICQMain8, QSVLAICQMain10: value.QSVLAICQMain10,
		QSVCQPMain8: value.QSVCQPMain8, QSVCQPMain10: value.QSVCQPMain10,
		QSVVBRMain8: value.QSVVBRMain8, QSVVBRMain10: value.QSVVBRMain10,
		QSVVBRExtBRCMain10:    value.QSVVBRExtBRCMain10,
		QSVVBRLookAheadMain10: value.QSVVBRLookAheadMain10,
		QSVVBRExtBRCMain8:     value.QSVVBRExtBRCMain8,
		QSVVBRLookAheadMain8:  value.QSVVBRLookAheadMain8,
		QSVCBRMain8:           value.QSVCBRMain8, QSVCBRMain10: value.QSVCBRMain10,
		QSVCBRExtBRCMain10:    value.QSVCBRExtBRCMain10,
		QSVCBRLookAheadMain10: value.QSVCBRLookAheadMain10,
		QSVCBRExtBRCMain8:     value.QSVCBRExtBRCMain8,
		QSVCBRLookAheadMain8:  value.QSVCBRLookAheadMain8,
		QSVAdaptiveIMain10:    value.QSVAdaptiveIMain10,
		QSVAdaptiveBMain10:    value.QSVAdaptiveBMain10,
		QSVAdaptiveIMain8:     value.QSVAdaptiveIMain8,
		QSVAdaptiveBMain8:     value.QSVAdaptiveBMain8,
		QSVVBRAdvancedMain10:  value.QSVVBRAdvancedMain10,
		QSVVBRAdvancedMain8:   value.QSVVBRAdvancedMain8,

		QSVLowPowerMain10: value.QSVLowPowerMain10,

		VideoToolboxMain: value.VideoToolboxMain, VideoToolboxMain10: value.VideoToolboxMain10,
		VideoToolboxBFrames: value.VideoToolboxBFrames, VideoToolboxBFramesVerified: value.VideoToolboxBFramesVerified,
		VideoToolboxBFramesDisabled: value.VideoToolboxBFramesDisabled, VideoToolboxObservedBFrames: value.VideoToolboxObservedBFrames,
		VideoToolboxPowerEfficient: value.VideoToolboxPowerEfficient,

		Reason: value.Reason, TestedModes: value.TestedModes, ModeReasons: value.ModeReasons,
	}, true
}

type EncoderCapability struct {
	Listed, Usable              bool
	Reason                      string
	Main10                      bool
	ICQ                         bool
	LowPower                    bool
	LookAhead                   bool
	ExtendedBRC                 bool
	AdaptiveI                   bool
	AdaptiveB                   bool
	QSVFullCombination          bool
	QSVICQMain8                 bool
	QSVICQMain10                bool
	QSVLAICQMain10              bool
	QSVLAICQMain8               bool
	QSVCQPMain8                 bool
	QSVCQPMain10                bool
	QSVVBRMain8                 bool
	QSVVBRMain10                bool
	QSVCBRMain8                 bool
	QSVCBRMain10                bool
	QSVLowPowerMain10           bool
	QSVVBRExtBRCMain10          bool
	QSVVBRLookAheadMain10       bool
	QSVVBRExtBRCMain8           bool
	QSVVBRLookAheadMain8        bool
	QSVCBRExtBRCMain10          bool
	QSVCBRLookAheadMain10       bool
	QSVCBRExtBRCMain8           bool
	QSVCBRLookAheadMain8        bool
	QSVVBRAdvancedMain10        bool
	QSVVBRAdvancedMain8         bool
	QSVAdaptiveIMain10          bool
	QSVAdaptiveBMain10          bool
	QSVAdaptiveIMain8           bool
	QSVAdaptiveBMain8           bool
	VideoToolboxMain            bool
	VideoToolboxMain10          bool
	VideoToolboxBFrames         bool
	VideoToolboxBFramesVerified bool
	VideoToolboxBFramesDisabled bool
	VideoToolboxObservedBFrames int
	VideoToolboxPowerEfficient  bool
	TestedModes                 map[string]bool
	ModeReasons                 map[string]string
}

var encoderCandidates = []string{
	"libx264",
	"libx265",
	"libsvtav1",
	"hevc_qsv",
	"hevc_vaapi",
	"hevc_nvenc",
	"hevc_videotoolbox",
	"h264_videotoolbox",
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
			rateControlProbe := func(name, expected, format string, args ...string) bool {
				passed, reason := qsvRateControlSmokeProbe(expected, format, args...)
				result.TestedModes[name] = passed
				if !passed {
					result.ModeReasons[name] = reason
				}
				return passed
			}
			vbrLookAheadProbe := func(name, format string, expectedDepth int, args ...string) bool {
				passed, reason := qsvVBRLookAheadSmokeProbe(format, expectedDepth, args...)
				result.TestedModes[name] = passed
				if !passed {
					result.ModeReasons[name] = reason
				}
				return passed
			}
			vbrExtBRCProbe := func(name, format string, args ...string) bool {
				passed, reason := qsvVBRExtBRCSmokeProbe(format, args...)
				result.TestedModes[name] = passed
				if !passed {
					result.ModeReasons[name] = reason
				}
				return passed
			}
			cbrExtBRCProbe := func(name, format string, args ...string) bool {
				passed, reason := qsvCBRExtBRCSmokeProbe(format, args...)
				result.TestedModes[name] = passed
				if !passed {
					result.ModeReasons[name] = reason
				}
				return passed
			}

			cbrLookAheadProbe := func(name, format string, expectedDepth int, args ...string) bool {
				passed, reason := qsvCBRLookAheadSmokeProbe(format, expectedDepth, args...)
				result.TestedModes[name] = passed
				if !passed {
					result.ModeReasons[name] = reason
				}
				return passed
			}
			adaptiveIProbe := func(name, format string, args ...string) bool {
				passed, reason := qsvAdaptiveISmokeProbe(format, args...)
				result.TestedModes[name] = passed
				if !passed {
					result.ModeReasons[name] = reason
				}
				return passed
			}
			adaptiveBProbe := func(name, format string, args ...string) bool {
				passed, reason := qsvAdaptiveBSmokeProbe(format, args...)
				result.TestedModes[name] = passed
				if !passed {
					result.ModeReasons[name] = reason
				}
				return passed
			}
			vbrAdvancedProbe := func(name, format string, expectedDepth int, args ...string) bool {
				passed, reason := qsvVBRAdvancedSmokeProbe(format, expectedDepth, args...)
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
			result.QSVLAICQMain8 = rateControlProbe("qsvLaIcqMain8", "LA_ICQ", "nv12", "-profile:v", "main", "-global_quality", "25", "-look_ahead", "1", "-look_ahead_depth", "40")
			result.QSVVBRExtBRCMain8 = vbrExtBRCProbe("qsvVbrExtBrcMain8", "nv12", "-profile:v", "main", "-b:v", "2M", "-maxrate", "3M", "-bufsize", "4M", "-extbrc", "1")
			result.QSVVBRLookAheadMain8 = vbrLookAheadProbe("qsvVbrLookAheadMain8", "nv12", 40, "-profile:v", "main", "-b:v", "2M", "-maxrate", "3M", "-bufsize", "4M", "-extbrc", "1", "-look_ahead_depth", "40")
			result.QSVCBRExtBRCMain8 = cbrExtBRCProbe("qsvCbrExtBrcMain8", "nv12", "-profile:v", "main", "-b:v", "2M", "-maxrate", "2M", "-bufsize", "4M", "-extbrc", "1")
			result.QSVCBRLookAheadMain8 = cbrLookAheadProbe("qsvCbrLookAheadMain8", "nv12", 40, "-profile:v", "main", "-b:v", "2M", "-maxrate", "2M", "-bufsize", "4M", "-extbrc", "1", "-look_ahead_depth", "40")
			result.QSVVBRAdvancedMain8 = vbrAdvancedProbe("qsvVbrAdvancedMain8", "nv12", 40, "-profile:v", "main", "-b:v", "2M", "-maxrate", "3M", "-bufsize", "4M", "-extbrc", "1", "-look_ahead_depth", "40", "-adaptive_i", "1", "-adaptive_b", "1")
			if result.QSVICQMain8 {
				result.QSVAdaptiveIMain8 = adaptiveIProbe("qsvAdaptiveIMain8", "nv12", "-profile:v", "main", "-global_quality", "25", "-adaptive_i", "1")
				result.QSVAdaptiveBMain8 = adaptiveBProbe("qsvAdaptiveBMain8", "nv12", "-profile:v", "main", "-global_quality", "25", "-adaptive_b", "1")
			} else {
				result.QSVAdaptiveIMain8 = skip("qsvAdaptiveIMain8", "skipped because QSV ICQ Main8 is unavailable")
				result.QSVAdaptiveBMain8 = skip("qsvAdaptiveBMain8", "skipped because QSV ICQ Main8 is unavailable")
			}
			probe("qsvPStrategySimpleMain8", "nv12", "-profile:v", "main", "-global_quality", "25", "-bf", "0", "-p_strategy", "1")
			probe("qsvPStrategyPyramidMain8", "nv12", "-profile:v", "main", "-global_quality", "25", "-bf", "0", "-p_strategy", "2")
			recordQSVGPBContext(&result, "Main8", "nv12", "main")
			if result.Main10 {
				result.QSVICQMain10 = rateControlProbe("qsvIcqMain10", "ICQ", "p010le", "-profile:v", "main10", "-global_quality", "25")
				result.QSVVBRExtBRCMain10 = vbrExtBRCProbe(
					"qsvVbrExtBrcMain10",
					"p010le",
					"-profile:v", "main10",
					"-b:v", "2M",
					"-maxrate", "3M",
					"-bufsize", "4M",
					"-extbrc", "1",
				)
				result.QSVVBRLookAheadMain10 = vbrLookAheadProbe(
					"qsvVbrLookAheadMain10",
					"p010le",
					40,
					"-profile:v", "main10",
					"-b:v", "2M",
					"-maxrate", "3M",
					"-bufsize", "4M",
					"-extbrc", "1",
					"-look_ahead_depth", "40",
				)
				result.QSVCBRExtBRCMain10 = cbrExtBRCProbe(
					"qsvCbrExtBrcMain10",
					"p010le",
					"-profile:v", "main10",
					"-b:v", "2M",
					"-maxrate", "2M",
					"-bufsize", "4M",
					"-extbrc", "1",
				)
				result.QSVCBRLookAheadMain10 = cbrLookAheadProbe(
					"qsvCbrLookAheadMain10",
					"p010le",
					40,
					"-profile:v", "main10",
					"-b:v", "2M",
					"-maxrate", "2M",
					"-bufsize", "4M",
					"-extbrc", "1",
					"-look_ahead_depth", "40",
				)
				result.QSVVBRAdvancedMain10 = vbrAdvancedProbe(
					"qsvVbrAdvancedMain10",
					"p010le",
					40,
					"-profile:v", "main10",
					"-b:v", "2M",
					"-maxrate", "3M",
					"-bufsize", "4M",
					"-extbrc", "1",
					"-look_ahead_depth", "40",
					"-adaptive_i", "1",
					"-adaptive_b", "1",
				)
				result.QSVCQPMain10 = probe("qsvCqpMain10", "p010le", "-profile:v", "main10", "-global_quality", "25", "-flags", "+qscale")
				result.QSVVBRMain10 = rateControlProbe("qsvVbrMain10", "VBR", "p010le", "-profile:v", "main10", "-b:v", "2M", "-maxrate", "3M", "-bufsize", "4M")
				result.QSVCBRMain10 = rateControlProbe("qsvCbrMain10", "CBR", "p010le", "-profile:v", "main10", "-b:v", "2M", "-maxrate", "2M", "-bufsize", "4M")
				probe("qsvPStrategySimpleMain10", "p010le", "-profile:v", "main10", "-global_quality", "25", "-bf", "0", "-p_strategy", "1")
				probe("qsvPStrategyPyramidMain10", "p010le", "-profile:v", "main10", "-global_quality", "25", "-bf", "0", "-p_strategy", "2")
				recordQSVGPBContext(&result, "Main10", "p010le", "main10")
			} else {
				result.QSVICQMain10 = skip("qsvIcqMain10", "skipped because the QSV Main10 base probe failed")
				result.QSVCQPMain10 = skip("qsvCqpMain10", "skipped because the QSV Main10 base probe failed")
				result.QSVVBRMain10 = skip("qsvVbrMain10", "skipped because the QSV Main10 base probe failed")
				result.QSVCBRMain10 = skip("qsvCbrMain10", "skipped because the QSV Main10 base probe failed")
				skip("qsvPStrategySimpleMain10", "skipped because the QSV Main10 base probe failed")
				skip("qsvPStrategyPyramidMain10", "skipped because the QSV Main10 base probe failed")
				for _, mode := range []string{"qsvGpbEffective", "qsvGpbRefDistOne", "qsvGpbBRefOff", "qsvCanDisableGpb"} {
					skip(mode+"Main10", "skipped because the QSV Main10 base probe failed")
				}
			}
			result.ICQ = result.QSVICQMain8 || result.QSVICQMain10
			result.LowPower = probe("qsvLowPowerMain8", "nv12", "-profile:v", "main", "-global_quality", "25", "-low_power", "1")
			if result.QSVICQMain10 {
				result.QSVLowPowerMain10 = probe("qsvLowPowerMain10", "p010le", "-profile:v", "main10", "-global_quality", "25", "-low_power", "1")
				result.QSVLAICQMain10 = rateControlProbe("qsvLaIcqMain10", "LA_ICQ", "p010le", "-profile:v", "main10", "-global_quality", "25", "-look_ahead", "1", "-look_ahead_depth", "40")
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
				result.QSVAdaptiveIMain10 = adaptiveIProbe(
					"qsvAdaptiveIMain10",
					"p010le",
					"-profile:v", "main10",
					"-global_quality", "25",
					"-adaptive_i", "1",
				)

				result.QSVAdaptiveBMain10 = adaptiveBProbe(
					"qsvAdaptiveBMain10",
					"p010le",
					"-profile:v", "main10",
					"-global_quality", "25",
					"-adaptive_b", "1",
				)

			} else {
				result.QSVAdaptiveIMain10 = skip("qsvAdaptiveIMain10", "skipped because QSV ICQ Main10 is unavailable")
				result.QSVAdaptiveBMain10 = skip("qsvAdaptiveBMain10", "skipped because QSV ICQ Main10 is unavailable")
			}
			result.AdaptiveI = result.QSVAdaptiveIMain8 || result.QSVAdaptiveIMain10
			result.AdaptiveB = result.QSVAdaptiveBMain8 || result.QSVAdaptiveBMain10
			if result.QSVLAICQMain10 {
				result.QSVFullCombination = probe("qsvFullCombination", "p010le", "-profile:v", "main10", "-global_quality", "25", "-look_ahead", "1", "-look_ahead_depth", "40", "-extbrc", "1", "-adaptive_i", "1", "-adaptive_b", "1")
			} else {
				result.QSVFullCombination = skip("qsvFullCombination", "skipped because QSV LA-ICQ Main10 is unavailable")
			}
			result.LookAhead = result.QSVLAICQMain8 || result.QSVLAICQMain10 || result.QSVVBRLookAheadMain8 || result.QSVVBRLookAheadMain10 || result.QSVCBRLookAheadMain8 || result.QSVCBRLookAheadMain10
			result.ExtendedBRC = result.QSVVBRExtBRCMain8 || result.QSVVBRExtBRCMain10 || result.QSVCBRExtBRCMain8 || result.QSVCBRExtBRCMain10
		}
		if encoder == "hevc_videotoolbox" || encoder == "h264_videotoolbox" {
			baseProfile := "main"
			if encoder == "h264_videotoolbox" {
				baseProfile = "high"
				result.Main10 = false
			}
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
				passed, reason := videoToolboxEncoderFeatureSmokeProbe(encoder, format, args...)
				result.TestedModes[name] = passed
				if !passed {
					result.ModeReasons[name] = reason
				}
				return passed
			}
			if result.Usable {
				autoAccepted, autoObserved, autoReason := videoToolboxBFrameProbe(encoder, "yuv420p", baseProfile, -1)
				result.TestedModes["videoToolboxAutoBFramesMain"] = autoAccepted && autoObserved > 0
				if !result.TestedModes["videoToolboxAutoBFramesMain"] {
					result.ModeReasons["videoToolboxAutoBFramesMain"] = autoReason
				}
				accepted, observed, reason := videoToolboxBFrameProbe(encoder, "yuv420p", baseProfile, 3)
				result.VideoToolboxBFramesVerified = accepted
				result.VideoToolboxBFrames = accepted && observed > 0
				result.VideoToolboxObservedBFrames = observed
				result.TestedModes["videoToolboxBFramesMain"] = result.VideoToolboxBFrames
				if !result.VideoToolboxBFrames {
					result.ModeReasons["videoToolboxBFramesMain"] = reason
				}
				disabledAccepted, disabledObserved, disabledReason := videoToolboxBFrameProbe(encoder, "yuv420p", baseProfile, 0)
				result.VideoToolboxBFramesDisabled = disabledAccepted && disabledObserved == 0
				result.TestedModes["videoToolboxBFramesDisabledMain"] = result.VideoToolboxBFramesDisabled
				if !result.VideoToolboxBFramesDisabled {
					result.ModeReasons["videoToolboxBFramesDisabledMain"] = disabledReason
				}
			}
			result.VideoToolboxPowerEfficient = result.Usable && vtProbe("videoToolboxPowerEfficientMain", "yuv420p", "-profile:v", baseProfile, "-power_efficient", "1")
			if result.Main10 && encoder == "hevc_videotoolbox" {
				autoAccepted, autoObserved, autoReason := videoToolboxBFrameProbe("hevc_videotoolbox", "p010le", "main10", -1)
				result.TestedModes["videoToolboxAutoBFramesMain10"] = autoAccepted && autoObserved > 0
				if !result.TestedModes["videoToolboxAutoBFramesMain10"] {
					result.ModeReasons["videoToolboxAutoBFramesMain10"] = autoReason
				}
				accepted, observed, reason := videoToolboxBFrameProbe("hevc_videotoolbox", "p010le", "main10", 3)
				result.VideoToolboxBFramesVerified = result.VideoToolboxBFramesVerified || accepted
				if observed > result.VideoToolboxObservedBFrames {
					result.VideoToolboxObservedBFrames = observed
				}
				result.TestedModes["videoToolboxBFramesMain10"] = accepted && observed > 0
				if !result.TestedModes["videoToolboxBFramesMain10"] {
					result.ModeReasons["videoToolboxBFramesMain10"] = reason
				}
				disabledAccepted, disabledObserved, disabledReason := videoToolboxBFrameProbe("hevc_videotoolbox", "p010le", "main10", 0)
				result.TestedModes["videoToolboxBFramesDisabledMain10"] = disabledAccepted && disabledObserved == 0
				if !result.TestedModes["videoToolboxBFramesDisabledMain10"] {
					result.ModeReasons["videoToolboxBFramesDisabledMain10"] = disabledReason
				}
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

func qsvRateControlMethod(output []byte) string {
	text := string(output)

	const marker = "RateControlMethod:"
	index := strings.Index(text, marker)
	if index == -1 {
		return ""
	}

	value := text[index+len(marker):]
	value = strings.TrimSpace(value)

	if end := strings.IndexByte(value, '\n'); end >= 0 {
		value = value[:end]
	}

	if separator := strings.IndexByte(value, ';'); separator >= 0 {
		value = value[:separator]
	}

	return strings.TrimSpace(value)
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
	return err == nil, summarizedQSVProbeReason(output, err)
}

func qsvAdaptiveIEnabled(output []byte) bool {
	return strings.Contains(string(output), "AdaptiveI: ON")
}

func qsvExtBRCEnabled(output []byte) bool {
	text := string(output)
	return strings.Contains(text, "ExtBRC: ON")
}

func qsvAdaptiveBEnabled(output []byte) bool {
	return strings.Contains(string(output), "AdaptiveB: ON")
}

func qsvLookAheadDepth(output []byte) int {
	text := string(output)

	const marker = "LookAheadDepth:"
	index := strings.Index(text, marker)
	if index == -1 {
		return 0
	}

	value := strings.TrimSpace(text[index+len(marker):])

	if end := strings.IndexByte(value, '\n'); end >= 0 {
		value = value[:end]
	}

	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0
	}

	depth, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0
	}

	return depth
}

func qsvAdaptiveISmokeProbe(pixelFormat string, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := qsvFeatureSmokeArgs(pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()

	if err != nil {
		return false, summarizedProbeReason(output, err)
	}

	if !qsvAdaptiveIEnabled(output) {
		return false, "QSV encoder did not enable Adaptive I"
	}

	return true, ""
}

func qsvRateControlSmokeProbe(expected, pixelFormat string, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := qsvFeatureSmokeArgs(pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()

	if err != nil {
		return false, summarizedProbeReason(output, err)
	}

	effective := qsvRateControlMethod(output)
	if effective == "" {
		return false, "QSV encoding succeeded but RateControlMethod was not reported"
	}

	if effective != expected {
		return false, "requested QSV rate control " + expected + " but encoder used " + effective
	}

	return true, ""
}

func qsvVBRLookAheadSmokeProbe(pixelFormat string, expectedDepth int, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := qsvFeatureSmokeArgs(pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()

	if err != nil {
		return false, summarizedQSVProbeReason(output, err)
	}

	if qsvRateControlMethod(output) != "VBR" {
		return false, "QSV encoder did not use VBR"
	}

	if !qsvExtBRCEnabled(output) {
		return false, "QSV encoder did not enable ExtBRC"
	}

	if depth := qsvLookAheadDepth(output); depth != expectedDepth {
		return false, fmt.Sprintf("QSV encoder used LookAheadDepth %d; expected %d", depth, expectedDepth)
	}

	return true, ""
}

func qsvVBRExtBRCSmokeProbe(pixelFormat string, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := qsvFeatureSmokeArgs(pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()

	if err != nil {
		return false, summarizedProbeReason(output, err)
	}

	if qsvRateControlMethod(output) != "VBR" {
		return false, "QSV encoder did not use VBR"
	}

	if !qsvExtBRCEnabled(output) {
		return false, "QSV encoder did not enable ExtBRC"
	}

	return true, ""
}

func qsvCBRExtBRCSmokeProbe(pixelFormat string, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := qsvFeatureSmokeArgs(pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()

	if err != nil {
		return false, summarizedProbeReason(output, err)
	}

	if qsvRateControlMethod(output) != "CBR" {
		return false, "QSV encoder did not use CBR"
	}

	if !qsvExtBRCEnabled(output) {
		return false, "QSV encoder did not enable ExtBRC"
	}

	return true, ""
}

func qsvCBRLookAheadSmokeProbe(pixelFormat string, expectedDepth int, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := qsvFeatureSmokeArgs(pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()

	if err != nil {
		return false, summarizedQSVProbeReason(output, err)
	}

	if qsvRateControlMethod(output) != "CBR" {
		return false, "QSV encoder did not use CBR"
	}

	if !qsvExtBRCEnabled(output) {
		return false, "QSV encoder did not enable ExtBRC"
	}

	if depth := qsvLookAheadDepth(output); depth != expectedDepth {
		return false, fmt.Sprintf(
			"QSV encoder used LookAheadDepth %d; expected %d",
			depth,
			expectedDepth,
		)
	}

	return true, ""
}

func qsvAdaptiveBSmokeProbe(pixelFormat string, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := qsvFeatureSmokeArgs(pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()

	if err != nil {
		return false, summarizedProbeReason(output, err)
	}

	if !qsvAdaptiveBEnabled(output) {
		return false, "QSV encoder did not enable Adaptive B"
	}

	return true, ""
}

func qsvVBRAdvancedSmokeProbe(pixelFormat string, expectedDepth int, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := qsvFeatureSmokeArgs(pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()

	if err != nil {
		return false, summarizedQSVProbeReason(output, err)
	}

	if qsvRateControlMethod(output) != "VBR" {
		return false, "QSV encoder did not use VBR"
	}

	if !qsvExtBRCEnabled(output) {
		return false, "QSV encoder did not enable ExtBRC"
	}

	if depth := qsvLookAheadDepth(output); depth != expectedDepth {
		return false, fmt.Sprintf(
			"QSV encoder used LookAheadDepth %d; expected %d",
			depth,
			expectedDepth,
		)
	}

	if !qsvAdaptiveIEnabled(output) {
		return false, "QSV encoder did not enable Adaptive I"
	}

	if !qsvAdaptiveBEnabled(output) {
		return false, "QSV encoder did not enable Adaptive B"
	}

	return true, ""
}

func qsvFeatureSmokeArgs(pixelFormat string, featureArgs ...string) []string {
	args := []string{
		"-hide_banner", "-loglevel", "verbose",
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

func recordQSVGPBContext(result *EncoderCapability, suffix, pixelFormat, profile string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := qsvFeatureSmokeArgs(pixelFormat, "-profile:v", profile, "-global_quality", "25", "-g", "75", "-bf", "0", "-gpb", "0")
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	if err != nil {
		reason := summarizedQSVProbeReason(output, err)
		for _, mode := range []string{"qsvGpbEffective", "qsvGpbRefDistOne", "qsvGpbBRefOff", "qsvCanDisableGpb"} {
			result.TestedModes[mode+suffix] = false
			result.ModeReasons[mode+suffix] = reason
		}
		return
	}
	gpbOn, refDistOne, bRefOff := parseQSVGPBContext(string(output))
	result.TestedModes["qsvGpbEffective"+suffix] = gpbOn
	result.TestedModes["qsvGpbRefDistOne"+suffix] = refDistOne
	result.TestedModes["qsvGpbBRefOff"+suffix] = bRefOff
	result.TestedModes["qsvCanDisableGpb"+suffix] = !gpbOn
	if gpbOn {
		result.ModeReasons["qsvCanDisableGpb"+suffix] = "QSV accepted -gpb 0 but reported GPB: ON"
	}
}

func parseQSVGPBContext(output string) (gpbOn, refDistOne, bRefOff bool) {
	return strings.Contains(output, "GPB: ON"),
		strings.Contains(output, "GopRefDist: 1"),
		strings.Contains(output, "BRefType: off")
}

func videoToolboxFeatureSmokeProbe(pixelFormat string, featureArgs ...string) (bool, string) {
	return videoToolboxEncoderFeatureSmokeProbe("hevc_videotoolbox", pixelFormat, featureArgs...)
}

func videoToolboxEncoderFeatureSmokeProbe(encoder, pixelFormat string, featureArgs ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := videoToolboxEncoderFeatureSmokeArgs(encoder, pixelFormat, featureArgs...)
	output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	return err == nil, summarizedProbeReason(output, err)
}

func videoToolboxFeatureSmokeArgs(pixelFormat string, featureArgs ...string) []string {
	return videoToolboxEncoderFeatureSmokeArgs("hevc_videotoolbox", pixelFormat, featureArgs...)
}

func videoToolboxEncoderFeatureSmokeArgs(encoder, pixelFormat string, featureArgs ...string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30", "-frames:v", "30", "-an", "-c:v", encoder, "-b:v", "1M", "-pix_fmt", pixelFormat}
	args = append(args, featureArgs...)
	args = append(args, "-f", "null", "-")
	return args
}

func videoToolboxBFrameProbe(encoder, pixelFormat, profile string, requested int) (bool, int, string) {
	file, err := os.CreateTemp("", "mvforge-videotoolbox-bframes-*.mp4")
	if err != nil {
		return false, 0, err.Error()
	}
	path := file.Name()
	_ = file.Close()
	_ = os.Remove(path)
	defer os.Remove(path)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30", "-frames:v", "90", "-an", "-c:v", encoder, "-b:v", "3M", "-realtime", "0", "-profile:v", profile, "-pix_fmt", pixelFormat}
	if requested >= 0 {
		args = append(args, "-bf", strconv.Itoa(requested))
	}
	args = append(args, path)
	output, runErr := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	if runErr != nil {
		return false, 0, summarizedProbeReason(output, runErr)
	}
	probeOutput, probeErr := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "frame=pict_type", "-of", "csv=p=0", path).CombinedOutput()
	if probeErr != nil {
		return false, 0, "VideoToolbox accepted the option but frame-type validation failed: " + summarizedProbeReason(probeOutput, probeErr)
	}
	observed, _, reason := evaluateVideoToolboxBFrameProbe(requested, string(probeOutput))
	return true, observed, reason
}

func evaluateVideoToolboxBFrameProbe(requested int, frameTypes string) (int, bool, string) {
	observed := 0
	for _, frameType := range strings.Fields(frameTypes) {
		if strings.TrimSpace(frameType) == "B" {
			observed++
		}
	}
	if requested < 0 {
		if observed == 0 {
			return 0, true, "VideoToolbox Auto produced no observed B-frames"
		}
		return observed, true, ""
	}
	if requested > 0 && observed == 0 {
		return 0, false, "VideoToolbox accepted -bf but produced no B-frames"
	}
	if requested == 0 && observed > 0 {
		return observed, false, "VideoToolbox accepted -bf 0 but still produced B-frames"
	}
	return observed, true, ""
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

func summarizedQSVProbeReason(output []byte, err error) string {
	if err == nil {
		return ""
	}

	interesting := []string{
		"RateControlMethod:",
		"ExtBRC:",
		"LookAheadDepth:",
		"AdaptiveI:",
		"AdaptiveB:",
		"invalid video parameters",
		"unsupported",
		"not been used",
		"warning",
		"error",
	}

	var matches []string

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)

		for _, marker := range interesting {
			if strings.Contains(lower, strings.ToLower(marker)) {
				matches = append(matches, line)
				break
			}
		}
	}

	if len(matches) == 0 {
		return summarizedProbeReason(output, err)
	}

	reason := strings.Join(matches, " | ")

	if len(reason) > 500 {
		reason = reason[:500] + "…"
	}

	return reason
}
