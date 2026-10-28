//go:build !darwin && !linux

package handlers

import (
	"errors"
	"os/exec"
)

func ffmpegProcessTreeContainmentSupported() bool { return false }

func terminateFFmpegProcessGroup(_ *exec.Cmd) bool { return false }

func waitFFmpegProcessGroupExit(_ int) error {
	return errors.New("ffmpeg process-tree exit cannot be verified on this operating system")
}
