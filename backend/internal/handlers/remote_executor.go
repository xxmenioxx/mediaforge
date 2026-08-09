package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type RemoteStorageMapping struct {
	LocalRoot  string `json:"localRoot"`
	RemoteRoot string `json:"remoteRoot"`
}

type RemoteExecutor struct {
	Name            string                 `json:"name"`
	Host            string                 `json:"host"`
	User            string                 `json:"user"`
	SSHKeyPath      string                 `json:"sshKeyPath"`
	KnownHostsPath  string                 `json:"knownHostsPath"`
	FFmpegPath      string                 `json:"ffmpegPath"`
	FFprobePath     string                 `json:"ffprobePath"`
	StorageMappings []RemoteStorageMapping `json:"storageMappings"`
}

type RemoteExecutorTestConversionRequest struct {
	Executor   RemoteExecutor `json:"executor"`
	SourcePath string         `json:"sourcePath"`
	OutputPath string         `json:"outputPath"`
	Start      float64        `json:"start"`
	Seconds    float64        `json:"seconds"`
}

type RemoteExecutorTestConversionResult struct {
	Executor         string `json:"executor"`
	SourcePath       string `json:"sourcePath"`
	RemoteSourcePath string `json:"remoteSourcePath"`
	OutputPath       string `json:"outputPath"`
	RemoteOutputPath string `json:"remoteOutputPath"`
	Command          string `json:"command"`
	FFmpegOutput     string `json:"ffmpegOutput,omitempty"`
	ProbeOutput      string `json:"probeOutput,omitempty"`
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

func (executor RemoteExecutor) MapPath(localPath string) (string, error) {
	cleanPath := filepath.Clean(localPath)

	for _, mapping := range executor.StorageMappings {
		localRoot := filepath.Clean(mapping.LocalRoot)
		remoteRoot := filepath.Clean(mapping.RemoteRoot)

		relative, err := filepath.Rel(localRoot, cleanPath)
		if err != nil {
			continue
		}

		if relative == ".." ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}

		return filepath.Join(remoteRoot, relative), nil
	}

	return "", fmt.Errorf(
		"no remote storage mapping found for path %q",
		localPath,
	)
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

	result.StorageAccessible = len(executor.StorageMappings) > 0

	for _, mapping := range executor.StorageMappings {
		_, err = executor.runSSH(
			probeCtx,
			fmt.Sprintf(
				"test -d %s",
				shellQuote(mapping.RemoteRoot),
			),
		)

		if err != nil {
			result.StorageAccessible = false
			break
		}
	}
	return result
}

func (h *AssetHandler) TestRemoteExecutorConversion(c *gin.Context) {
	var input RemoteExecutorTestConversionRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	executor := input.Executor

	if strings.TrimSpace(executor.Name) == "" ||
		strings.TrimSpace(executor.Host) == "" ||
		strings.TrimSpace(executor.User) == "" ||
		strings.TrimSpace(executor.SSHKeyPath) == "" ||
		strings.TrimSpace(executor.FFmpegPath) == "" ||
		strings.TrimSpace(executor.FFprobePath) == "" ||
		len(executor.StorageMappings) == 0 {

		c.JSON(http.StatusBadRequest, gin.H{
			"error": "executor name, host, user, sshKeyPath, ffmpegPath, ffprobePath, and storageMappings are required",
		})
		return
	}

	for _, mapping := range executor.StorageMappings {
		if strings.TrimSpace(mapping.LocalRoot) == "" ||
			strings.TrimSpace(mapping.RemoteRoot) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "each storage mapping requires localRoot and remoteRoot",
			})
			return
		}
	}

	sourcePath := strings.TrimSpace(input.SourcePath)
	outputPath := strings.TrimSpace(input.OutputPath)

	if sourcePath == "" || outputPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "sourcePath and outputPath are required",
		})
		return
	}

	if filepath.Clean(sourcePath) == filepath.Clean(outputPath) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "sourcePath and outputPath must be different",
		})
		return
	}

	seconds := input.Seconds
	if seconds <= 0 {
		seconds = 60
	}

	if seconds > 120 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "test conversion is limited to 120 seconds",
		})
		return
	}

	start := input.Start
	if start < 0 {
		start = 0
	}

	remoteSourcePath, err := executor.MapPath(sourcePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	remoteOutputPath, err := executor.MapPath(outputPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Validate that the remote source really exists.
	if _, err := executor.runSSH(
		c.Request.Context(),
		fmt.Sprintf("test -f %s", shellQuote(remoteSourcePath)),
	); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf(
				"remote source is not accessible: %v",
				err,
			),
		})
		return
	}

	// Ensure the output directory exists.
	remoteOutputDir := filepath.Dir(remoteOutputPath)

	if _, err := executor.runSSH(
		c.Request.Context(),
		fmt.Sprintf(
			"mkdir -p %s",
			shellQuote(remoteOutputDir),
		),
	); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf(
				"could not create remote output directory: %v",
				err,
			),
		})
		return
	}

	ffmpegCommand := strings.Join([]string{
		shellQuote(executor.FFmpegPath),
		"-y",
		"-hide_banner",
		"-loglevel", "warning",
		"-ss", fmt.Sprintf("%.3f", start),
		"-i", shellQuote(remoteSourcePath),
		"-t", fmt.Sprintf("%.3f", seconds),
		"-map", "0:v:0",
		"-an",
		"-c:v", "hevc_videotoolbox",
		"-profile:v", "main10",
		"-pix_fmt", "p010le",
		"-b:v", "4M",
		"-realtime", "0",
		shellQuote(remoteOutputPath),
	}, " ")

	timeout := time.Duration(seconds+60) * time.Second

	ctx, cancel := context.WithTimeout(
		c.Request.Context(),
		timeout,
	)
	defer cancel()

	output, err := executor.runSSH(
		ctx,
		ffmpegCommand,
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   err.Error(),
			"command": ffmpegCommand,
		})
		return
	}

	probeCommand := strings.Join([]string{
		shellQuote(executor.FFprobePath),
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,profile,pix_fmt,width,height",
		"-of", "json",
		shellQuote(remoteOutputPath),
	}, " ")

	probeOutput, err := executor.runSSH(
		c.Request.Context(),
		probeCommand,
	)

	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":      fmt.Sprintf("remote output validation failed: %v", err),
			"command":    ffmpegCommand,
			"outputPath": remoteOutputPath,
		})
		return
	}

	c.JSON(http.StatusOK, RemoteExecutorTestConversionResult{
		Executor:         executor.Name,
		SourcePath:       sourcePath,
		RemoteSourcePath: remoteSourcePath,
		OutputPath:       outputPath,
		RemoteOutputPath: remoteOutputPath,
		Command:          ffmpegCommand,
		FFmpegOutput:     strings.TrimSpace(output),
		ProbeOutput:      strings.TrimSpace(probeOutput),
	})
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
	Name            string                 `json:"name"`
	Host            string                 `json:"host"`
	User            string                 `json:"user"`
	SSHKeyPath      string                 `json:"sshKeyPath"`
	KnownHostsPath  string                 `json:"knownHostsPath"`
	FFmpegPath      string                 `json:"ffmpegPath"`
	FFprobePath     string                 `json:"ffprobePath"`
	StorageMappings []RemoteStorageMapping `json:"storageMappings"`
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
		Name:            strings.TrimSpace(input.Name),
		Host:            strings.TrimSpace(input.Host),
		User:            strings.TrimSpace(input.User),
		SSHKeyPath:      strings.TrimSpace(input.SSHKeyPath),
		KnownHostsPath:  strings.TrimSpace(input.KnownHostsPath),
		FFmpegPath:      strings.TrimSpace(input.FFmpegPath),
		FFprobePath:     strings.TrimSpace(input.FFprobePath),
		StorageMappings: input.StorageMappings,
	}

	if executor.Name == "" ||
		executor.Host == "" ||
		executor.User == "" ||
		executor.SSHKeyPath == "" ||
		executor.FFmpegPath == "" ||
		executor.FFprobePath == "" ||
		len(executor.StorageMappings) == 0 {

		c.JSON(http.StatusBadRequest, gin.H{
			"error": "name, host, user, sshKeyPath, ffmpegPath, ffprobePath, and storageMappings are required",
		})
		return
	}

	for _, mapping := range executor.StorageMappings {
		if strings.TrimSpace(mapping.LocalRoot) == "" ||
			strings.TrimSpace(mapping.RemoteRoot) == "" {

			c.JSON(http.StatusBadRequest, gin.H{
				"error": "each storage mapping requires localRoot and remoteRoot",
			})
			return
		}
	}

	result := executor.Probe(c.Request.Context())

	status := http.StatusOK
	if !result.Reachable {
		status = http.StatusBadGateway
	}

	c.JSON(status, result)
}
