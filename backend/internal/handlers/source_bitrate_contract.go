package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
	"gorm.io/gorm"
)

func sourceBitrateEvidenceForInventory(db *gorm.DB, path string, inventory MediaStreamInventory) quality.SourceBitrateEvidence {
	if evidence, ok := cachedSourceBitrateEvidence(db, path); ok {
		return evidence
	}
	input := quality.SourceBitrateInput{
		ContainerBitrate: inventory.TotalBitrate,
		DurationSeconds:  inventory.Duration,
		Video:            quality.SourceStreamFacts{Index: -1},
		Audio:            []quality.SourceStreamFacts{},
		Subtitles:        []quality.SourceStreamFacts{},
	}
	if len(inventory.Video) > 0 {
		video := inventory.Video[0]
		input.Video = quality.SourceStreamFacts{Index: video.Index, Bitrate: video.Bitrate, DurationSeconds: inventory.Duration, Tags: video.BitrateTags}
	}
	for _, stream := range inventory.Audio {
		input.Audio = append(input.Audio, quality.SourceStreamFacts{Index: stream.Index, Bitrate: stream.Bitrate, DurationSeconds: inventory.Duration, Tags: stream.BitrateTags})
	}
	for _, stream := range inventory.Subtitle {
		input.Subtitles = append(input.Subtitles, quality.SourceStreamFacts{Index: stream.Index, Bitrate: stream.Bitrate, DurationSeconds: inventory.Duration, Tags: stream.BitrateTags})
	}
	if info, err := os.Stat(path); err == nil {
		input.FileSizeBytes = info.Size()
	}
	return quality.ResolveSourceBitrate(input)
}

func sourceBitrateEvidenceForScan(scan models.ScanResult) quality.SourceBitrateEvidence {
	if evidence, ok := decodeSourceBitrateEvidence(scan.SourceBitrateEvidence); ok {
		return evidence
	}
	input := quality.SourceBitrateInput{
		ContainerBitrate: scan.Bitrate,
		FileSizeBytes:    scan.SizeBytes,
		DurationSeconds:  scan.Duration,
		Video:            quality.SourceStreamFacts{Index: -1},
		Audio:            []quality.SourceStreamFacts{},
		Subtitles:        []quality.SourceStreamFacts{},
	}
	for _, raw := range scan.VideoStreams {
		if values, ok := upscaleStreamMap(raw); ok {
			input.Video = quality.SourceStreamFacts{
				Index: workerIntValue(values["index"], -1), Bitrate: int64(workerNumberValue(values["bitrate"], 0)),
				DurationSeconds: workerNumberValue(values["duration"], scan.Duration),
			}
			break
		}
	}
	for _, raw := range scan.AudioStreams {
		if values, ok := upscaleStreamMap(raw); ok {
			input.Audio = append(input.Audio, quality.SourceStreamFacts{
				Index: workerIntValue(values["index"], -1), Bitrate: int64(workerNumberValue(values["bitrate"], 0)),
				DurationSeconds: workerNumberValue(values["duration"], scan.Duration),
			})
		}
	}
	return quality.ResolveSourceBitrate(input)
}

func decodeSourceBitrateEvidence(raw any) (quality.SourceBitrateEvidence, bool) {
	value, ok := snapshotCacheMap(raw)
	if !ok {
		return quality.SourceBitrateEvidence{}, false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return quality.SourceBitrateEvidence{}, false
	}
	var evidence quality.SourceBitrateEvidence
	if json.Unmarshal(encoded, &evidence) != nil || evidence.SchemaVersion != quality.SourceBitrateEvidenceSchemaVersion {
		return quality.SourceBitrateEvidence{}, false
	}
	return evidence, true
}

func cachedSourceBitrateEvidence(db *gorm.DB, path string) (quality.SourceBitrateEvidence, bool) {
	if db == nil || strings.TrimSpace(path) == "" || !db.Migrator().HasTable(&models.ScanResult{}) {
		return quality.SourceBitrateEvidence{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return quality.SourceBitrateEvidence{}, false
	}
	var scan models.ScanResult
	if err := db.Where("path = ?", path).Order("updated_at desc, id desc").First(&scan).Error; err != nil {
		return quality.SourceBitrateEvidence{}, false
	}
	return cachedSourceBitrateEvidenceFromScan(path, info, scan)
}

func cachedSourceBitrateEvidenceFromScan(path string, info os.FileInfo, scan models.ScanResult) (quality.SourceBitrateEvidence, bool) {
	if info == nil || filepath.Clean(scan.Path) != filepath.Clean(path) {
		return quality.SourceBitrateEvidence{}, false
	}
	cache, cacheOK := snapshotCacheMap(scan.RawProbe["snapshotCache"])
	if !cacheOK || workerIntValue(cache["schemaVersion"], 0) != snapshotCacheSchemaVersion {
		return quality.SourceBitrateEvidence{}, false
	}
	fingerprint, fingerprintOK := snapshotCacheMap(cache["fingerprint"])
	if !fingerprintOK || filepath.Clean(stringFromUnknown(fingerprint["path"])) != filepath.Clean(path) ||
		jsonInt64(fingerprint["sizeBytes"]) != info.Size() || jsonInt64(fingerprint["mtimeNs"]) != info.ModTime().UnixNano() {
		return quality.SourceBitrateEvidence{}, false
	}
	components, componentsOK := snapshotCacheMap(cache["components"])
	if !componentsOK {
		return quality.SourceBitrateEvidence{}, false
	}
	component, componentOK := snapshotCacheMap(components["sourceBitrate"])
	if !componentOK || workerIntValue(component["version"], 0) != quality.SourceBitrateEvidenceSchemaVersion {
		return quality.SourceBitrateEvidence{}, false
	}
	evidenceMap := scan.SourceBitrateEvidence
	if len(evidenceMap) == 0 {
		evidenceMap = sourceBitrateEvidenceFromRaw(scan.RawProbe)
	}
	encoded, err := json.Marshal(evidenceMap)
	if err != nil {
		return quality.SourceBitrateEvidence{}, false
	}
	var evidence quality.SourceBitrateEvidence
	if json.Unmarshal(encoded, &evidence) != nil || evidence.SchemaVersion != quality.SourceBitrateEvidenceSchemaVersion {
		return quality.SourceBitrateEvidence{}, false
	}
	return evidence, true
}

func bpppfAssessmentForRecommendation(
	profile models.Profile,
	encoder string,
	intent quality.QualityIntent,
	recommendation quality.EncoderRecommendation,
	streams MediaStreamInventory,
) quality.BPPPFAssessment {
	bitrate := int64(0)
	basis := ""
	if recommendation.TargetBitrate != nil && *recommendation.TargetBitrate > 0 {
		bitrate, basis = *recommendation.TargetBitrate, "configured_target"
	} else if recommendation.EstimatedVideoBitrate != nil && *recommendation.EstimatedVideoBitrate > 0 {
		bitrate, basis = *recommendation.EstimatedVideoBitrate, "recommendation_estimate"
	}
	width := workerIntValue(profile.WorkerConfig["effectiveOutputWidth"], intent.OutputWidth)
	height := workerIntValue(profile.WorkerConfig["effectiveOutputHeight"], intent.OutputHeight)
	fps := effectiveQualityOutputFPS(profile, streams)
	codec := strings.TrimSpace(profile.CodecFamily)
	if codec == "" {
		codec = profile.VideoCodec
	}
	return quality.EvaluateBPPPF(quality.BPPPFInput{
		TargetVideoBitrate: bitrate, BitrateBasis: basis,
		OutputWidth: width, OutputHeight: height, EffectiveFPS: fps,
		CodecFamily: codec, Encoder: encoder,
		HDR:                profile.PreserveHDR && intent.HDR,
		OptimizationIntent: profile.OptimizationIntent,
	})
}

func bpppfAssessmentForQueueProfile(
	profile models.Profile,
	encoder string,
	scan models.ScanResult,
	evidence quality.SourceBitrateEvidence,
) quality.BPPPFAssessment {
	streams := mediaStreamInventoryFromScan(scan)
	if len(streams.Video) == 0 {
		return quality.EvaluateBPPPF(quality.BPPPFInput{})
	}
	streams.Video[0].Bitrate = evidence.Bitrate
	profile = applySelectedEncoder(profile, encoder)
	intent := qualityIntentForMedia(profile, scan.Path, streams)
	recommendation := genericEncoderRecommendation(profile, intent, encoder)
	if target := int64(workerNumberValue(profile.WorkerConfig["qsvTargetBitrate"], 0)); target > 0 {
		recommendation.TargetBitrate = &target
	}
	return bpppfAssessmentForRecommendation(profile, encoder, intent, recommendation, streams)
}

func effectiveQualityOutputFPS(profile models.Profile, streams MediaStreamInventory) float64 {
	if profileWorkerBool(profile, "effectiveOutputFrameRateUnknown", false) {
		return 0
	}
	if value := quality.ParseFrameRate(workerStringValue(profile.WorkerConfig["effectiveOutputFrameRate"])); value > 0 {
		return value
	}
	filters := strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoFilters"]))
	for _, filter := range strings.Split(filters, ",") {
		filter = strings.TrimSpace(filter)
		if !strings.HasPrefix(strings.ToLower(filter), "fps=") {
			continue
		}
		if value := quality.ParseFrameRate(strings.TrimSpace(filter[len("fps="):])); value > 0 {
			return value
		}
		return 0
	}
	if len(streams.Video) == 0 {
		return 0
	}
	if value := quality.ParseFrameRate(streams.Video[0].FrameRate); value > 0 {
		return value
	}
	return quality.ParseFrameRate(streams.Video[0].RealFrameRate)
}
