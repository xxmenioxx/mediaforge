//go:build darwin || linux

package handlers

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func ffmpegProcessTreeContainmentSupported() bool { return true }

func terminateFFmpegProcessGroup(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return false
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	return err == nil || errors.Is(err, syscall.ESRCH)
}

func waitFFmpegProcessGroupExit(processGroupID int) error {
	if processGroupID <= 0 {
		return errors.New("ffmpeg process group identity is unavailable")
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := syscall.Kill(-processGroupID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return fmt.Errorf("inspect ffmpeg process group: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("ffmpeg process group %d did not exit within the safety window", processGroupID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
