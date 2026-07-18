package scheduler

import (
	"encoding/json"
	"strings"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/gorm"
)

const WaitingDirectPlayReview = "WAITING_DIRECTPLAY_REVIEW"

type DirectPlayConfig struct {
	Enabled       bool     `json:"enabled"`
	Strategy      string   `json:"strategy"`
	TargetClients []string `json:"targetClients"`
	MinimumScore  int      `json:"minimumScore"`
	Enforcement   string   `json:"enforcement"`
}

type DirectPlayClientResult struct {
	Client   string   `json:"client"`
	Score    int      `json:"score"`
	Risk     string   `json:"risk"`
	Warnings []string `json:"warnings"`
}

type DirectPlayReport struct {
	Enabled      bool                     `json:"enabled"`
	Estimated    bool                     `json:"estimated"`
	Strategy     string                   `json:"strategy"`
	MinimumScore int                      `json:"minimumScore"`
	LowestScore  int                      `json:"lowestScore"`
	Risk         string                   `json:"risk"`
	Blocked      bool                     `json:"blocked"`
	Clients      []DirectPlayClientResult `json:"clients"`
}

type directPlayFacts struct {
	Container, VideoCodec, PixelFormat string
	HasAACStereo, AACStereoDefault     bool
	SubtitleCodecs                     []string
}

func DefaultDirectPlayConfig() DirectPlayConfig {
	return DirectPlayConfig{Enabled: true, Strategy: "balanced", TargetClients: []string{"jellyfin_web", "jellyfin_android_tv", "jellyfin_roku", "jellyfin_webos", "apple_tv"}, MinimumScore: 70, Enforcement: "warn"}
}

func LoadDirectPlayConfig(db *gorm.DB) (DirectPlayConfig, error) {
	config := DefaultDirectPlayConfig()
	var setting models.AppSetting
	result := db.Where("key = ?", "directPlay").Limit(1).Find(&setting)
	if result.Error != nil || result.RowsAffected == 0 {
		return config, result.Error
	}
	data, err := json.Marshal(setting.Value)
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	if config.MinimumScore <= 0 {
		config.MinimumScore = 70
	}
	if len(config.TargetClients) == 0 {
		config.TargetClients = DefaultDirectPlayConfig().TargetClients
	}
	return config, nil
}

func EvaluatePlannedDirectPlay(db *gorm.DB, profile models.Profile) (DirectPlayReport, error) {
	config, err := LoadDirectPlayConfig(db)
	if err != nil {
		return DirectPlayReport{}, err
	}
	report := DirectPlayReport{Enabled: config.Enabled, Estimated: true, Strategy: config.Strategy, MinimumScore: config.MinimumScore, LowestScore: 100, Risk: "low"}
	if !config.Enabled {
		return report, nil
	}
	for _, client := range config.TargetClients {
		result := evaluateDirectPlayClient(client, profile)
		report.Clients = append(report.Clients, result)
		if result.Score < report.LowestScore {
			report.LowestScore = result.Score
		}
	}
	report.Risk = directPlayRisk(report.LowestScore)
	report.Blocked = config.Enforcement == "block" && report.LowestScore < config.MinimumScore
	return report, nil
}

func EvaluateActualDirectPlay(db *gorm.DB, probe models.JSONMap) (DirectPlayReport, error) {
	config, err := LoadDirectPlayConfig(db)
	if err != nil {
		return DirectPlayReport{}, err
	}
	report := DirectPlayReport{Enabled: config.Enabled, Estimated: false, Strategy: config.Strategy, MinimumScore: config.MinimumScore, LowestScore: 100, Risk: "low"}
	if !config.Enabled {
		return report, nil
	}
	facts := directPlayFactsFromProbe(probe)
	for _, client := range config.TargetClients {
		result := evaluateActualDirectPlayClient(client, facts)
		report.Clients = append(report.Clients, result)
		if result.Score < report.LowestScore {
			report.LowestScore = result.Score
		}
	}
	report.Risk = directPlayRisk(report.LowestScore)
	report.Blocked = config.Enforcement == "block" && report.LowestScore < config.MinimumScore
	return report, nil
}

func directPlayFactsFromProbe(probe models.JSONMap) directPlayFacts {
	facts := directPlayFacts{}
	if format, ok := probe["format"].(map[string]any); ok {
		facts.Container, _ = format["format_name"].(string)
	}
	streams, _ := probe["streams"].([]any)
	for _, raw := range streams {
		stream, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := stream["codec_type"].(string)
		codec, _ := stream["codec_name"].(string)
		switch kind {
		case "video":
			if facts.VideoCodec == "" {
				facts.VideoCodec, _ = stream["codec_name"].(string)
				facts.PixelFormat, _ = stream["pix_fmt"].(string)
			}
		case "audio":
			channels := numberFromUnknown(stream["channels"])
			if codec == "aac" && channels > 0 && channels <= 2 {
				facts.HasAACStereo = true
				if disposition, ok := stream["disposition"].(map[string]any); ok && numberFromUnknown(disposition["default"]) == 1 {
					facts.AACStereoDefault = true
				}
			}
		case "subtitle":
			facts.SubtitleCodecs = append(facts.SubtitleCodecs, codec)
		}
	}
	return facts
}

func evaluateActualDirectPlayClient(client string, facts directPlayFacts) DirectPlayClientResult {
	score := 100
	warnings := []string{}
	penalize := func(points int, warning string) { score -= points; warnings = append(warnings, warning) }
	codec := strings.ToLower(facts.VideoCodec)
	if codec == "hevc" {
		if client == "jellyfin_web" {
			penalize(20, "HEVC support varies by browser")
		}
		if strings.Contains(strings.ToLower(facts.PixelFormat), "10") && (client == "jellyfin_web" || client == "jellyfin_roku") {
			penalize(15, "HEVC Main10 support varies on this client")
		}
	} else if codec == "av1" {
		if client != "jellyfin_android_tv" {
			penalize(20, "AV1 hardware decoding is device-dependent")
		}
	} else if codec != "h264" {
		penalize(25, "Video codec support is uncertain")
	}
	if strings.Contains(strings.ToLower(facts.Container), "matroska") && client == "apple_tv" {
		penalize(10, "MKV support depends on the Apple TV player")
	}
	if !facts.HasAACStereo {
		penalize(15, "Final file has no AAC stereo compatibility track")
	} else if !facts.AACStereoDefault {
		penalize(5, "AAC stereo exists but is not the default audio track")
	}
	for _, subtitle := range facts.SubtitleCodecs {
		switch strings.ToLower(subtitle) {
		case "ass", "ssa":
			penalize(10, "ASS/SSA subtitles may require burn-in")
		case "hdmv_pgs_subtitle", "dvd_subtitle":
			penalize(15, "Image subtitles may require transcoding")
		}
	}
	if score < 0 {
		score = 0
	}
	return DirectPlayClientResult{Client: client, Score: score, Risk: directPlayRisk(score), Warnings: warnings}
}

func numberFromUnknown(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	}
	return 0
}

func evaluateDirectPlayClient(client string, profile models.Profile) DirectPlayClientResult {
	score := 100
	warnings := []string{}
	penalize := func(points int, warning string) { score -= points; warnings = append(warnings, warning) }
	codec := strings.ToLower(profile.CodecFamily)
	if codec == "" {
		codec = strings.ToLower(profile.VideoCodec)
	}
	if codec == "hevc" || codec == "h265" || codec == "x265" {
		if client == "jellyfin_web" {
			penalize(20, "HEVC support varies by browser")
		}
		if profile.BitDepth >= 10 && (client == "jellyfin_web" || client == "jellyfin_roku") {
			penalize(15, "HEVC Main10 support varies on this client")
		}
	} else if codec == "av1" {
		if client != "jellyfin_android_tv" {
			penalize(20, "AV1 hardware decoding is device-dependent")
		}
	} else if codec != "h264" && codec != "avc" {
		penalize(25, "Video codec support is uncertain")
	}
	if strings.ToLower(profile.Container) == "mkv" && client == "apple_tv" {
		penalize(10, "MKV support depends on the Apple TV player")
	}
	addAAC, _ := profile.WorkerConfig["addAacStereoDefault"].(bool)
	if !addAAC && strings.ToLower(profile.AudioCodec) != "aac" {
		penalize(15, "No AAC stereo compatibility track is guaranteed")
	}
	preferSRT, _ := profile.WorkerConfig["preferSrtSubtitles"].(bool)
	if profile.PreserveSubtitles && !preferSRT {
		penalize(10, "Preserved ASS or image subtitles may require transcoding")
	}
	if score < 0 {
		score = 0
	}
	return DirectPlayClientResult{Client: client, Score: score, Risk: directPlayRisk(score), Warnings: warnings}
}

func directPlayRisk(score int) string {
	if score >= 85 {
		return "low"
	}
	if score >= 70 {
		return "medium"
	}
	return "high"
}
