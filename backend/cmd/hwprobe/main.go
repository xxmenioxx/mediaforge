package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type testResult struct {
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	ExitCode   int    `json:"exitCode"`
	DurationMS int64  `json:"durationMs"`
	TimedOut   bool   `json:"timedOut"`
	ReasonCode string `json:"reasonCode"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
}

type component struct {
	Status       string                `json:"status"`
	Usable       bool                  `json:"usable"`
	Capabilities map[string]bool       `json:"capabilities,omitempty"`
	Tests        map[string]testResult `json:"tests"`
}

type report struct {
	RanAt  time.Time `json:"ranAt"`
	Device struct {
		Path       string `json:"path"`
		Exists     bool   `json:"exists"`
		Readable   bool   `json:"readable"`
		Writable   bool   `json:"writable"`
		ReasonCode string `json:"reasonCode"`
	} `json:"device"`
	Intel struct {
		Driver component `json:"driver"`
		VAAPI  component `json:"vaapi"`
		QSV    component `json:"qsv"`
	} `json:"intel"`
}

func run(name string, timeout time.Duration, binary string, args ...string) testResult {
	result := testResult{Name: name, ExitCode: -1}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err := cmd.Run()
	result.DurationMS = time.Since(start).Milliseconds()
	result.Stdout, result.Stderr = stdout.String(), stderr.String()
	result.Passed = err == nil
	if err == nil {
		result.ExitCode = 0
		result.ReasonCode = "ok"
		return result
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.ReasonCode = "timeout"
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else if result.Stderr == "" {
		result.Stderr = err.Error()
	}
	result.ReasonCode = classify(result.Stdout + "\n" + result.Stderr)
	return result
}

func classify(output string) string {
	text := strings.ToLower(output)
	switch {
	case strings.Contains(text, "permission denied"), strings.Contains(text, "operation not permitted"):
		return "permission_denied"
	case strings.Contains(text, "unknown encoder"):
		return "not_listed"
	case strings.Contains(text, "no such file"), strings.Contains(text, "cannot open a drm render node"):
		return "device_missing"
	case strings.Contains(text, "unsupported"), strings.Contains(text, "not supported"):
		return "feature_unsupported"
	case strings.Contains(text, "mfx"), strings.Contains(text, "qsv"), strings.Contains(text, "vpl"):
		return "runtime_incompatible"
	default:
		return "command_failed"
	}
}

func ffmpegTest(name, encoder, pixelFormat string, extra ...string) testResult {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if strings.HasSuffix(encoder, "_qsv") {
		args = append(args, "-init_hw_device", "qsv=hw,child_device=/dev/dri/renderD128")
	} else {
		args = append(args, "-vaapi_device", "/dev/dri/renderD128")
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30", "-frames:v", "30", "-an")
	if strings.HasSuffix(encoder, "_vaapi") {
		args = append(args, "-vf", "format="+pixelFormat+",hwupload")
	} else {
		args = append(args, "-pix_fmt", pixelFormat)
	}
	args = append(args, "-c:v", encoder, "-b:v", "1M")
	args = append(args, extra...)
	args = append(args, "-f", "null", "-")
	return run(name, 15*time.Second, "ffmpeg", args...)
}

func main() {
	var output report
	output.RanAt = time.Now().UTC()
	output.Device.Path = "/dev/dri/renderD128"
	output.Intel.Driver.Tests = map[string]testResult{}
	output.Intel.VAAPI.Tests = map[string]testResult{}
	output.Intel.QSV.Tests = map[string]testResult{}
	output.Intel.VAAPI.Capabilities = map[string]bool{}
	output.Intel.QSV.Capabilities = map[string]bool{}

	if info, err := os.Stat(output.Device.Path); err == nil && !info.IsDir() {
		output.Device.Exists = true
		if f, openErr := os.OpenFile(output.Device.Path, os.O_RDWR, 0); openErr == nil {
			output.Device.Readable, output.Device.Writable = true, true
			_ = f.Close()
		} else if errors.Is(openErr, syscall.EACCES) {
			output.Device.ReasonCode = "permission_denied"
		} else {
			output.Device.ReasonCode = "device_open_failed"
		}
	} else {
		output.Device.ReasonCode = "device_missing"
	}

	vainfo := run("vainfo", 10*time.Second, "vainfo", "--display", "drm", "--device", output.Device.Path)
	output.Intel.Driver.Tests["vainfo"] = vainfo
	output.Intel.VAAPI.Tests["vainfo"] = vainfo
	vpl := run("vpl_inspect", 10*time.Second, "vpl-inspect")
	output.Intel.QSV.Tests["vplInspect"] = vpl
	encoders := run("ffmpeg_encoders", 10*time.Second, "ffmpeg", "-hide_banner", "-encoders")
	output.Intel.VAAPI.Tests["ffmpegEncoders"] = encoders
	output.Intel.QSV.Tests["ffmpegEncoders"] = encoders
	vaListed := strings.Contains(encoders.Stdout+encoders.Stderr, "hevc_vaapi")
	qsvListed := strings.Contains(encoders.Stdout+encoders.Stderr, "hevc_qsv")

	vaH264 := ffmpegTest("h264_vaapi_basic", "h264_vaapi", "nv12")
	vaHEVC := ffmpegTest("hevc_vaapi_basic", "hevc_vaapi", "nv12")
	output.Intel.VAAPI.Tests["h264Basic"], output.Intel.VAAPI.Tests["hevcBasic"] = vaH264, vaHEVC
	output.Intel.VAAPI.Capabilities["h264Basic"] = vaH264.Passed
	output.Intel.VAAPI.Capabilities["hevcBasic"] = vaHEVC.Passed

	qH264 := ffmpegTest("h264_qsv_basic", "h264_qsv", "nv12")
	qHEVC := ffmpegTest("hevc_qsv_basic", "hevc_qsv", "nv12")
	qICQ := ffmpegTest("hevc_qsv_icq", "hevc_qsv", "nv12", "-global_quality", "18")
	qMain10 := ffmpegTest("hevc_qsv_main10", "hevc_qsv", "p010le", "-profile:v", "main10")
	qLP0 := ffmpegTest("hevc_qsv_low_power_0", "hevc_qsv", "nv12", "-low_power", "0")
	qLP1 := ffmpegTest("hevc_qsv_low_power_1", "hevc_qsv", "nv12", "-low_power", "1")
	for key, value := range map[string]testResult{
		"h264Basic": qH264, "hevcBasic": qHEVC, "icq": qICQ,
		"main10": qMain10, "lowPower0": qLP0, "lowPower1": qLP1,
	} {
		output.Intel.QSV.Tests[key] = value
		output.Intel.QSV.Capabilities[key] = value.Passed
	}

	switch {
	case !output.Device.Exists:
		output.Intel.Driver.Status, output.Intel.VAAPI.Status, output.Intel.QSV.Status = "device_missing", "device_missing", "device_missing"
	case !output.Device.Readable:
		output.Intel.Driver.Status, output.Intel.VAAPI.Status, output.Intel.QSV.Status = "permission_denied", "permission_denied", "permission_denied"
	case !vainfo.Passed:
		output.Intel.Driver.Status, output.Intel.VAAPI.Status = "driver_failed", "driver_failed"
	default:
		output.Intel.Driver.Status, output.Intel.Driver.Usable = "runtime_initialized", true
		output.Intel.VAAPI.Usable = vaH264.Passed || vaHEVC.Passed
		if output.Intel.VAAPI.Usable {
			output.Intel.VAAPI.Status = "basic_usable"
		} else {
			output.Intel.VAAPI.Status = "limited"
		}
	}
	if output.Intel.QSV.Status == "" {
		switch {
		case !qsvListed:
			output.Intel.QSV.Status = "not_listed"
		case !vpl.Passed:
			output.Intel.QSV.Status = "runtime_missing"
		case !qH264.Passed && !qHEVC.Passed:
			output.Intel.QSV.Status = "runtime_incompatible"
		case qICQ.Passed && qMain10.Passed && qLP0.Passed && qLP1.Passed:
			output.Intel.QSV.Status, output.Intel.QSV.Usable = "fully_usable", true
		default:
			output.Intel.QSV.Status, output.Intel.QSV.Usable = "limited", true
		}
	}
	if !vaListed && (output.Intel.VAAPI.Status == "basic_usable" || output.Intel.VAAPI.Status == "limited") {
		output.Intel.VAAPI.Status, output.Intel.VAAPI.Usable = "not_listed", false
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(output)
}
