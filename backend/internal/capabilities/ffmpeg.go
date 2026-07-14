package capabilities

import (
	"os/exec"
	"strings"
	"sync"
)

type EncoderCapability struct {
	Listed, Usable bool
	Reason         string
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
			for _, candidate := range []string{"libx264", "libx265", "libsvtav1", "hevc_qsv", "hevc_nvenc", "hevc_videotoolbox", "hevc_amf"} {
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
	if encoder == "hevc_videotoolbox" && !videoToolboxSmokeTest() {
		result.Usable = false
		result.Reason = "VideoToolbox is listed but could not open an HEVC encoding session"
	}
	encoderCache.probed[encoder] = result
	return result
}

func videoToolboxSmokeTest() bool {
	return exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=size=64x64:rate=1", "-frames:v", "1", "-an", "-c:v", "hevc_videotoolbox", "-b:v", "1M", "-pix_fmt", "yuv420p", "-f", "null", "-").Run() == nil
}
