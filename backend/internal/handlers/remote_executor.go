package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type RemoteExecutor struct {
	Name           string `json:"name"`
	Host           string `json:"host"`
	User           string `json:"user"`
	SSHKeyPath     string `json:"sshKeyPath"`
	KnownHostsPath string `json:"knownHostsPath"`
	FFmpegPath     string `json:"ffmpegPath"`
	FFprobePath    string `json:"ffprobePath"`
	StorageRoot    string `json:"storageRoot"`
}

type RemoteExecutorProbe struct {
	Name              string `json:"name"`
	Reachable         bool   `json:"reachable"`
	Hostname          string `json:"hostname,omitempty"`
	FFmpegAvailable   bool   `json:"ffmpegAvailable"`
	FFmpegVersion     string `json:"ffmpegVersion,omitempty"`
	FFprobeAvailable  bool   `json:"ffprobeAvailable"`
	FFprobeVersion    string `json:"ffprobeVersion,omitempty"`
	VideoToolbox      bool   `json:"videoToolbox"`
	StorageAccessible bool   `json:"storageAccessible"`
	Error             string `json:"error,omitempty"`
}

func (executor RemoteExecutor) Probe(ctx context.Context) RemoteExecutorProbe {
	result := RemoteExecutorProbe{
		Name: executor.Name,
	}

	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	hostname, err := executor.runSSH(probeCtx, "hostname")
	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Reachable = true
	result.Hostname = strings.TrimSpace(hostname)

	ffmpegOutput, err := executor.runSSH(
		probeCtx,
		fmt.Sprintf("%s -version | head -1", shellQuote(executor.FFmpegPath)),
	)
	if err == nil {
		result.FFmpegAvailable = true
		result.FFmpegVersion = strings.TrimSpace(ffmpegOutput)
	}

	ffprobeOutput, err := executor.runSSH(
		probeCtx,
		fmt.Sprintf("%s -version | head -1", shellQuote(executor.FFprobePath)),
	)
	if err == nil {
		result.FFprobeAvailable = true
		result.FFprobeVersion = strings.TrimSpace(ffprobeOutput)
	}

	videoToolboxOutput, err := executor.runSSH(
		probeCtx,
		fmt.Sprintf(
			"%s -hide_banner -encoders 2>/dev/null | grep -E 'h264_videotoolbox|hevc_videotoolbox'",
			shellQuote(executor.FFmpegPath),
		),
	)
	if err == nil && strings.Contains(videoToolboxOutput, "hevc_videotoolbox") {
		result.VideoToolbox = true
	}

	_, err = executor.runSSH(
		probeCtx,
		fmt.Sprintf("test -d %s", shellQuote(executor.StorageRoot)),
	)
	result.StorageAccessible = err == nil

	return result
}

func (executor RemoteExecutor) runSSH(
	ctx context.Context,
	remoteCommand string,
) (string, error) {
	target := fmt.Sprintf("%s@%s", executor.User, executor.Host)

	args := []string{
		"-i", executor.SSHKeyPath,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
	}

	if executor.KnownHostsPath != "" {
		args = append(
			args,
			"-o", "StrictHostKeyChecking=yes",
			"-o", "UserKnownHostsFile="+executor.KnownHostsPath,
		)
	}

	args = append(args, target, remoteCommand)

	command := exec.CommandContext(ctx, "ssh", args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}

		return "", fmt.Errorf(
			"remote executor %q ssh command failed: %s",
			executor.Name,
			message,
		)
	}

	return stdout.String(), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

type RemoteExecutorProbeRequest struct {
	Name           string `json:"name"`
	Host           string `json:"host"`
	User           string `json:"user"`
	SSHKeyPath     string `json:"sshKeyPath"`
	KnownHostsPath string `json:"knownHostsPath"`
	FFmpegPath     string `json:"ffmpegPath"`
	FFprobePath    string `json:"ffprobePath"`
	StorageRoot    string `json:"storageRoot"`
}

func (h *AssetHandler) ProbeRemoteExecutor(c *gin.Context) {
	var input RemoteExecutorProbeRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	executor := RemoteExecutor{
		Name:           strings.TrimSpace(input.Name),
		Host:           strings.TrimSpace(input.Host),
		User:           strings.TrimSpace(input.User),
		SSHKeyPath:     strings.TrimSpace(input.SSHKeyPath),
		KnownHostsPath: strings.TrimSpace(input.KnownHostsPath),
		FFmpegPath:     strings.TrimSpace(input.FFmpegPath),
		FFprobePath:    strings.TrimSpace(input.FFprobePath),
		StorageRoot:    strings.TrimSpace(input.StorageRoot),
	}

	if executor.Name == "" ||
		executor.Host == "" ||
		executor.User == "" ||
		executor.SSHKeyPath == "" ||
		executor.FFmpegPath == "" ||
		executor.FFprobePath == "" ||
		executor.StorageRoot == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "name, host, user, sshKeyPath, ffmpegPath, ffprobePath, and storageRoot are required",
		})
		return
	}

	result := executor.Probe(c.Request.Context())

	status := http.StatusOK
	if !result.Reachable {
		status = http.StatusBadGateway
	}

	c.JSON(status, result)
}
