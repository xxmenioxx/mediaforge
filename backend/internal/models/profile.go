package models

import (
	"time"

	"gorm.io/gorm"
)

type Profile struct {
	ID                 uint           `json:"id" gorm:"primaryKey"`
	Name               string         `json:"name" gorm:"not null;uniqueIndex"`
	Scope              string         `json:"scope" gorm:"not null;default:'';index"`
	Description        string         `json:"description"`
	TemplateKey        string         `json:"templateKey,omitempty" gorm:"not null;default:'';index"`
	TemplateVersion    int            `json:"templateVersion,omitempty" gorm:"not null;default:0"`
	Container          string         `json:"container" gorm:"not null"`
	VideoCodec         string         `json:"videoCodec" gorm:"not null"`
	CodecFamily        string         `json:"codecFamily" gorm:"not null;default:''"`
	EncoderPolicy      string         `json:"encoderPolicy" gorm:"not null;default:''"`
	PreferredEncoder   string         `json:"preferredEncoder" gorm:"not null;default:''"`
	AllowedEncoders    StringList     `json:"allowedEncoders" gorm:"type:json"`
	FallbackPolicy     string         `json:"fallbackPolicy" gorm:"not null;default:''"`
	BitDepth           int            `json:"bitDepth" gorm:"not null;default:0"`
	PixelFormat        string         `json:"pixelFormat" gorm:"not null;default:''"`
	QualityStrategy    string         `json:"qualityStrategy" gorm:"not null;default:''"`
	OptimizationIntent string         `json:"optimizationIntent" gorm:"not null;default:'';index"`
	ProfileVersion     int            `json:"profileVersion" gorm:"not null;default:1"`
	AudioCodec         string         `json:"audioCodec" gorm:"not null"`
	QualityMode        string         `json:"qualityMode" gorm:"not null"`
	QualityValue       int            `json:"qualityValue" gorm:"not null"`
	PreserveHDR        bool           `json:"preserveHdr"`
	PreserveSubtitles  bool           `json:"preserveSubtitles"`
	PreserveChapters   bool           `json:"preserveChapters"`
	WorkerConfig       JSONMap        `json:"workerConfig" gorm:"type:json"`
	Disabled           bool           `json:"disabled"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `json:"deletedAt" gorm:"index"`
}

type ProfileDefinition struct {
	Container          string     `json:"container"`
	VideoCodec         string     `json:"videoCodec"`
	CodecFamily        string     `json:"codecFamily"`
	EncoderPolicy      string     `json:"encoderPolicy"`
	PreferredEncoder   string     `json:"preferredEncoder"`
	AllowedEncoders    StringList `json:"allowedEncoders"`
	FallbackPolicy     string     `json:"fallbackPolicy"`
	BitDepth           int        `json:"bitDepth"`
	PixelFormat        string     `json:"pixelFormat"`
	QualityStrategy    string     `json:"qualityStrategy"`
	OptimizationIntent string     `json:"optimizationIntent"`
	AudioCodec         string     `json:"audioCodec"`
	QualityMode        string     `json:"qualityMode"`
	QualityValue       int        `json:"qualityValue"`
	PreserveHDR        bool       `json:"preserveHdr"`
	PreserveSubtitles  bool       `json:"preserveSubtitles"`
	PreserveChapters   bool       `json:"preserveChapters"`
	WorkerConfig       JSONMap    `json:"workerConfig"`
}

func (definition ProfileDefinition) Clone() ProfileDefinition {
	definition.AllowedEncoders = append(StringList(nil), definition.AllowedEncoders...)
	definition.WorkerConfig = cloneProfileJSONMap(definition.WorkerConfig)
	return definition
}

func (profile Profile) Definition() ProfileDefinition {
	return ProfileDefinition{
		Container:          profile.Container,
		VideoCodec:         profile.VideoCodec,
		CodecFamily:        profile.CodecFamily,
		EncoderPolicy:      profile.EncoderPolicy,
		PreferredEncoder:   profile.PreferredEncoder,
		AllowedEncoders:    append(StringList(nil), profile.AllowedEncoders...),
		FallbackPolicy:     profile.FallbackPolicy,
		BitDepth:           profile.BitDepth,
		PixelFormat:        profile.PixelFormat,
		QualityStrategy:    profile.QualityStrategy,
		OptimizationIntent: profile.OptimizationIntent,
		AudioCodec:         profile.AudioCodec,
		QualityMode:        profile.QualityMode,
		QualityValue:       profile.QualityValue,
		PreserveHDR:        profile.PreserveHDR,
		PreserveSubtitles:  profile.PreserveSubtitles,
		PreserveChapters:   profile.PreserveChapters,
		WorkerConfig:       cloneProfileJSONMap(profile.WorkerConfig),
	}
}

func (definition ProfileDefinition) NewProfile(name, scope, description string) Profile {
	return Profile{
		Name:               name,
		Scope:              scope,
		Description:        description,
		Container:          definition.Container,
		VideoCodec:         definition.VideoCodec,
		CodecFamily:        definition.CodecFamily,
		EncoderPolicy:      definition.EncoderPolicy,
		PreferredEncoder:   definition.PreferredEncoder,
		AllowedEncoders:    append(StringList(nil), definition.AllowedEncoders...),
		FallbackPolicy:     definition.FallbackPolicy,
		BitDepth:           definition.BitDepth,
		PixelFormat:        definition.PixelFormat,
		QualityStrategy:    definition.QualityStrategy,
		OptimizationIntent: definition.OptimizationIntent,
		ProfileVersion:     1,
		AudioCodec:         definition.AudioCodec,
		QualityMode:        definition.QualityMode,
		QualityValue:       definition.QualityValue,
		PreserveHDR:        definition.PreserveHDR,
		PreserveSubtitles:  definition.PreserveSubtitles,
		PreserveChapters:   definition.PreserveChapters,
		WorkerConfig:       cloneProfileJSONMap(definition.WorkerConfig),
	}
}

func cloneProfileJSONMap(value JSONMap) JSONMap {
	if value == nil {
		return JSONMap{}
	}
	cloned := make(JSONMap, len(value))
	for key, item := range value {
		cloned[key] = cloneProfileJSONValue(item)
	}
	return cloned
}

func cloneProfileJSONValue(value any) any {
	switch typed := value.(type) {
	case JSONMap:
		return cloneProfileJSONMap(typed)
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, item := range typed {
			cloned[key] = cloneProfileJSONValue(item)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneProfileJSONValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}
