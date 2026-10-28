//go:build darwin || linux

package handlers

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestFFmpegProcessGroupParentHelper(t *testing.T) {
	if os.Getenv("MVFORGE_PROCESS_GROUP_HELPER") != "parent" {
		return
	}
	child := exec.Command(os.Args[0], "-test.run=^TestFFmpegProcessGroupChildHelper$")
	child.Env = append(os.Environ(), "MVFORGE_PROCESS_GROUP_HELPER=child")
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
	_, _ = fmt.Fprintln(os.Stdout, "descendant-started")
	for {
		time.Sleep(time.Second)
	}
}

func TestFFmpegProcessGroupChildHelper(t *testing.T) {
	if os.Getenv("MVFORGE_PROCESS_GROUP_HELPER") != "child" {
		return
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestFFmpegProcessGroupTerminationWaitsForDescendants(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestFFmpegProcessGroupParentHelper$")
	cmd.Env = append(os.Environ(), "MVFORGE_PROCESS_GROUP_HELPER=parent")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	configureManagedProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	processGroupID := cmd.Process.Pid
	defer func() { _ = terminateFFmpegProcessGroup(cmd) }()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "descendant-started" {
		t.Fatalf("synthetic descendant did not start: line=%q err=%v", line, err)
	}
	if !terminateFFmpegProcessGroup(cmd) {
		t.Fatal("process group termination was not delivered")
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("parent process unexpectedly exited successfully after SIGKILL")
	}
	if err := waitFFmpegProcessGroupExit(processGroupID); err != nil {
		t.Fatalf("process group still has descendants after cancellation: %v", err)
	}
}
