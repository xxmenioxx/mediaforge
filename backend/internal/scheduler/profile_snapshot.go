package scheduler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
)

const ProfileSnapshotSchemaVersion = 1

type profileSnapshot struct {
	SchemaVersion     int                  `json:"schemaVersion"`
	CaptureSource     string               `json:"captureSource"`
	CapturedAt        time.Time            `json:"capturedAt"`
	ProfileID         uint                 `json:"profileId"`
	ProfileVersion    int                  `json:"profileVersion"`
	ProfileName       string               `json:"profileName"`
	Constraints       ExecutionConstraints `json:"constraints"`
	Container         string               `json:"container"`
	VideoCodec        string               `json:"videoCodec"`
	AudioCodec        string               `json:"audioCodec"`
	PreserveHDR       bool                 `json:"preserveHdr"`
	PreserveSubtitles bool                 `json:"preserveSubtitles"`
	PreserveChapters  bool                 `json:"preserveChapters"`
	WorkerConfig      models.JSONMap       `json:"workerConfig"`
}

func CaptureProfileSnapshot(profile models.Profile, capturedAt time.Time, source string) (models.JSONMap, error) {
	constraints, err := ResolveExecutionConstraints(profile)
	if err != nil {
		return nil, err
	}

	value := profileSnapshot{
		SchemaVersion:     ProfileSnapshotSchemaVersion,
		CaptureSource:     source,
		CapturedAt:        capturedAt,
		ProfileID:         profile.ID,
		ProfileVersion:    constraints.SourceProfileVersion,
		ProfileName:       profile.Name,
		Constraints:       constraints,
		Container:         profile.Container,
		VideoCodec:        profile.VideoCodec,
		AudioCodec:        profile.AudioCodec,
		PreserveHDR:       profile.PreserveHDR,
		PreserveSubtitles: profile.PreserveSubtitles,
		PreserveChapters:  profile.PreserveChapters,
		WorkerConfig:      profile.WorkerConfig,
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var snapshot models.JSONMap
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func RestoreProfileSnapshot(snapshot models.JSONMap) (models.Profile, error) {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return models.Profile{}, err
	}
	var value profileSnapshot
	if err := json.Unmarshal(encoded, &value); err != nil {
		return models.Profile{}, err
	}
	if value.SchemaVersion != ProfileSnapshotSchemaVersion {
		return models.Profile{}, fmt.Errorf("unsupported profile snapshot schema version %d", value.SchemaVersion)
	}
	if err := ValidateExecutionConstraints(value.Constraints); err != nil {
		return models.Profile{}, err
	}
	return models.Profile{
		ID:                value.ProfileID,
		Name:              value.ProfileName,
		Container:         value.Container,
		VideoCodec:        value.VideoCodec,
		CodecFamily:       value.Constraints.CodecFamily,
		EncoderPolicy:     value.Constraints.EncoderPolicy,
		PreferredEncoder:  value.Constraints.PreferredEncoder,
		AllowedEncoders:   models.StringList(value.Constraints.AllowedEncoders),
		FallbackPolicy:    value.Constraints.FallbackPolicy,
		BitDepth:          value.Constraints.BitDepth,
		PixelFormat:       value.Constraints.PixelFormat,
		QualityStrategy:   value.Constraints.QualityStrategy,
		ProfileVersion:    value.ProfileVersion,
		AudioCodec:        value.AudioCodec,
		QualityMode:       value.Constraints.QualityMode,
		QualityValue:      value.Constraints.QualityValue,
		PreserveHDR:       value.PreserveHDR,
		PreserveSubtitles: value.PreserveSubtitles,
		PreserveChapters:  value.PreserveChapters,
		WorkerConfig:      value.WorkerConfig,
	}, nil
}
