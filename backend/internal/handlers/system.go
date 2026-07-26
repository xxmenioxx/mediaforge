package handlers

import (
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
)

type SoftwareComponent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
}

func Versions(c *gin.Context) {
	components := []SoftwareComponent{
		{Name: "MVForge API", Version: "1.0.10", Source: "backend"},
		{Name: "Go", Version: runtime.Version(), Source: "runtime"},
		{Name: "FFmpeg", Version: commandVersion("ffmpeg", "-version"), Source: "container"},
		{Name: "FFprobe", Version: commandVersion("ffprobe", "-version"), Source: "container"},
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		components = appendModuleVersion(components, info, "github.com/gin-gonic/gin", "Gin")
		components = appendModuleVersion(components, info, "gorm.io/gorm", "GORM")
		components = appendModuleVersion(components, info, "gorm.io/driver/sqlite", "GORM SQLite Driver")
	}

	c.JSON(200, gin.H{"components": components})
}

func appendModuleVersion(components []SoftwareComponent, info *debug.BuildInfo, modulePath string, name string) []SoftwareComponent {
	for _, dep := range info.Deps {
		if dep.Path == modulePath {
			return append(components, SoftwareComponent{Name: name, Version: dep.Version, Source: "go module"})
		}
	}

	return append(components, SoftwareComponent{Name: name, Version: "unknown", Source: "go module"})
}

func commandVersion(name string, args ...string) string {
	output, err := exec.Command(name, args...).Output()
	if err != nil {
		return "unavailable"
	}

	line := strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0])
	if line == "" {
		return "unknown"
	}

	return line
}
