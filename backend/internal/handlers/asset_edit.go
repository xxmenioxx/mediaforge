package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// assetOverrideMutationMu serializes read-modify-write operations over the two
// AppSetting maps. The current MVForge deployment has one backend process, so
// a process-local guard is sufficient and avoids introducing a distributed lock.
var assetOverrideMutationMu sync.Mutex

type AssetEditInput struct {
	Metadata   AssetMetadataUpdateInput   `json:"metadata"`
	Conversion AssetConversionUpdateInput `json:"conversion"`
}

type AssetEditResponse struct {
	Path       string                       `json:"path"`
	Metadata   AssetMetadataState           `json:"metadata"`
	Conversion AssetConversionOverrideState `json:"conversion"`
}

func (h AssetHandler) Edit(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var input AssetEditInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cleanPath := filepath.Clean(resolvedPath)
	var response AssetEditResponse
	assetOverrideMutationMu.Lock()
	defer assetOverrideMutationMu.Unlock()
	allowed, err := h.pathBelongsToLibrary(cleanPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}
	if _, err := os.Stat(cleanPath); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "asset path changed; reload Asset Info before saving"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		metadataEntries := assetMetadataOverrides(tx)
		conversionEntries := assetConversionOverrides(tx)

		metadata, removeMetadata := normalizedAssetMetadata(input.Metadata)
		if existing, ok := metadataEntries[cleanPath]; ok &&
			stringSlicesEqual(existing.Categories, metadata.Categories) &&
			stringSlicesEqual(existing.Tags, metadata.Tags) {
			metadata.InheritedFrom = existing.InheritedFrom
		}
		if removeMetadata {
			delete(metadataEntries, cleanPath)
		} else {
			metadata.UpdatedAt = time.Now()
			metadataEntries[cleanPath] = metadata
		}

		conversion, err := normalizeAssetConversionUpdate(
			tx,
			cleanPath,
			input.Conversion,
			conversionEntries[cleanPath],
		)
		if err != nil {
			return err
		}
		if assetConversionOverrideEmpty(conversion) {
			delete(conversionEntries, cleanPath)
		} else {
			now := time.Now()
			conversion.UpdatedAt = &now
			conversionEntries[cleanPath] = conversion
		}

		if err := saveAssetMetadataOverrides(tx, metadataEntries); err != nil {
			return err
		}
		if err := saveAssetConversionOverrides(tx, conversionEntries); err != nil {
			return err
		}

		response = AssetEditResponse{
			Path:       cleanPath,
			Metadata:   metadataForPath(cleanPath, metadataEntries),
			Conversion: conversionOverrideForPath(cleanPath, conversionEntries),
		}
		return nil
	})
	if err != nil {
		if isAssetConversionValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

func normalizedAssetMetadata(input AssetMetadataUpdateInput) (AssetMetadataState, bool) {
	metadata := AssetMetadataState{
		Categories: normalizedTags(input.Categories),
		Tags:       normalizedTags(input.Tags),
	}
	return metadata, len(metadata.Categories) == 0 && len(metadata.Tags) == 0
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func normalizeAssetConversionUpdate(db *gorm.DB, path string, input AssetConversionUpdateInput, existing AssetConversionOverrideState) (AssetConversionOverrideState, error) {
	if mode := strings.TrimSpace(input.UpscaleMode); mode != "" && normalizedUpscaleMode(mode) == "" {
		return AssetConversionOverrideState{}, fmt.Errorf("upscaleMode must be disabled, auto, 720p, 1080p, custom, or omitted to inherit")
	}
	if sharpen := strings.TrimSpace(input.UpscaleSharpen); sharpen != "" && normalizedUpscaleSharpen(sharpen) == "" {
		return AssetConversionOverrideState{}, fmt.Errorf("upscaleSharpen must be off, light, medium, custom, or omitted to inherit")
	}
	if normalizedUpscaleSharpen(input.UpscaleSharpen) == string(UpscaleSharpenCustom) && (input.UpscaleSharpenCustomStrength == nil || *input.UpscaleSharpenCustomStrength < 0 || *input.UpscaleSharpenCustomStrength > .4) {
		return AssetConversionOverrideState{}, fmt.Errorf("upscaleSharpenCustomStrength must be between 0.00 and 0.40 for custom sharpen")
	}
	if fieldOrder := strings.TrimSpace(input.DeinterlaceFieldOrder); fieldOrder != "" && normalizedCadenceFieldOrder(fieldOrder) == "" {
		return AssetConversionOverrideState{}, fmt.Errorf("deinterlaceFieldOrder must be auto, tff, bff, or omitted to inherit")
	}
	if normalizedUpscaleMode(input.UpscaleMode) == string(UpscaleModeCustom) && normalizedUpscaleCustomHeight(input.UpscaleMode, input.UpscaleCustomHeight) == 0 {
		return AssetConversionOverrideState{}, fmt.Errorf("upscaleCustomHeight must be a positive even height for custom upscale")
	}
	if input.CropAspectPolicy != "" && input.CropAspectPolicy != "source_sar" && input.CropAspectPolicy != "preserve_dar" {
		return AssetConversionOverrideState{}, fmt.Errorf("cropAspectPolicy must be source_sar, preserve_dar, or omitted to inherit")
	}
	if input.HEVCLevelMode != "" && normalizedHEVCLevelMode(input.HEVCLevelMode) == "" {
		return AssetConversionOverrideState{}, fmt.Errorf("hevcLevelMode must be auto, recommended, custom, or omitted to inherit")
	}
	if input.HEVCLevel != "" && normalizedHEVCLevel(input.HEVCLevel) == "" {
		return AssetConversionOverrideState{}, fmt.Errorf("hevcLevel is not a supported HEVC level")
	}

	roleOverrides := existing.SubtitleRoleOverrides
	if input.SubtitleRoleOverrides != nil {
		var persistedScan models.ScanResult
		if err := db.Where("path = ?", path).Order("updated_at desc, id desc").First(&persistedScan).Error; err == nil {
			roleOverrides = subtitleRoleOverridesForConversionUpdate(persistedScan, existing.SubtitleRoleOverrides, input.SubtitleRoleOverrides, true)
		} else {
			roleOverrides = subtitleRoleOverridesForConversionUpdate(models.ScanResult{}, existing.SubtitleRoleOverrides, input.SubtitleRoleOverrides, true)
		}
	}

	return AssetConversionOverrideState{
		TrackProfileKey:                  strings.TrimSpace(input.TrackProfileKey),
		KeepVideoStreams:                 normalizedStreamIndexes(input.KeepVideoStreams),
		KeepAudioStreams:                 normalizedStreamIndexes(input.KeepAudioStreams),
		KeepSubtitleStreams:              normalizedStreamIndexes(input.KeepSubtitleStreams),
		VideoMetadata:                    normalizedStreamMetadata(input.VideoMetadata),
		AudioMetadata:                    normalizedStreamMetadata(input.AudioMetadata),
		SubtitleMetadata:                 normalizedStreamMetadata(input.SubtitleMetadata),
		SubtitleRoleOverrides:            roleOverrides,
		SubtitleTransforms:               normalizedSubtitleTransforms(input.SubtitleTransforms),
		SubtitleSidecarFormatsByStream:   normalizedSubtitleSidecarFormatsByStream(input.SubtitleSidecarFormatsByStream),
		VideoCodec:                       strings.TrimSpace(input.VideoCodec),
		AudioCodec:                       strings.TrimSpace(input.AudioCodec),
		QualityMode:                      strings.TrimSpace(input.QualityMode),
		QualityValue:                     input.QualityValue,
		OptimizationIntent:               normalizedOptimizationIntent(input.OptimizationIntent),
		VideoPreset:                      strings.TrimSpace(input.VideoPreset),
		PixFmt:                           strings.TrimSpace(input.PixFmt),
		VideoFilters:                     strings.TrimSpace(input.VideoFilters),
		CropAspectPolicy:                 strings.TrimSpace(input.CropAspectPolicy),
		UpscaleMode:                      normalizedUpscaleMode(input.UpscaleMode),
		UpscaleSharpen:                   normalizedUpscaleSharpen(input.UpscaleSharpen),
		UpscaleSharpenCustomStrength:     normalizedUpscaleSharpenStrength(input.UpscaleSharpen, input.UpscaleSharpenCustomStrength),
		UpscaleCustomHeight:              normalizedUpscaleCustomHeight(input.UpscaleMode, input.UpscaleCustomHeight),
		DeinterlaceMode:                  strings.TrimSpace(input.DeinterlaceMode),
		DeinterlaceFieldOrder:            normalizedCadenceFieldOrder(input.DeinterlaceFieldOrder),
		FieldStructureMode:               normalizedFieldStructureMode(input.FieldStructureMode),
		CadenceMode:                      normalizedCadenceMode(input.CadenceMode),
		CadenceFieldOrder:                normalizedCadenceFieldOrder(input.CadenceFieldOrder),
		X265Params:                       strings.TrimSpace(input.X265Params),
		FrameStructureMode:               normalizedFrameStructureMode(input.FrameStructureMode),
		FrameStructureGOPMode:            normalizedFrameStructureGOPMode(input.FrameStructureGOPMode),
		FrameStructureGOPStrategy:        normalizedFrameStructureGOPStrategy(input.FrameStructureGOPStrategy),
		FrameStructureGOPFrames:          min(1000, max(0, input.FrameStructureGOPFrames)),
		FrameStructureBFrameMode:         normalizedFrameStructureBFrameMode(input.FrameStructureBFrameMode),
		FrameStructureMaxBFrames:         min(16, max(0, input.FrameStructureMaxBFrames)),
		HEVCLevelMode:                    normalizedHEVCLevelMode(input.HEVCLevelMode),
		HEVCLevel:                        normalizedHEVCLevel(input.HEVCLevel),
		ProcessingMode:                   strings.TrimSpace(input.ProcessingMode),
		PreserveHDR:                      input.PreserveHDR,
		PreserveSubtitles:                input.PreserveSubtitles,
		PreserveChapters:                 input.PreserveChapters,
		ExternalSubtitleFormat:           strings.TrimSpace(input.ExternalSubtitleFormat),
		FinalColorPolicy:                 strings.TrimSpace(input.FinalColorPolicy),
		AddAACStereoTrack:                input.AddAACStereoTrack,
		AACStereoDefault:                 input.AACStereoDefault,
		EnhancedAudioSourceStreamIndex:   normalizedOptionalStreamIndex(input.EnhancedAudioSourceStreamIndex),
		UseHardwareIfAvailable:           input.UseHardwareIfAvailable,
		VideoEncoder:                     strings.TrimSpace(input.VideoEncoder),
		PreferredEncoder:                 strings.TrimSpace(input.PreferredEncoder),
		TargetWorkerName:                 strings.TrimSpace(input.TargetWorkerName),
		GlobalQuality:                    input.GlobalQuality,
		QSVRateControl:                   strings.TrimSpace(input.QSVRateControl),
		QSVLookAheadDepth:                input.QSVLookAheadDepth,
		QSVExtendedBRC:                   input.QSVExtendedBRC,
		QSVAdaptiveI:                     input.QSVAdaptiveI,
		QSVAdaptiveB:                     input.QSVAdaptiveB,
		QSVMBBRCMode:                     normalizedOptionalQSVMBBRCMode(input.QSVMBBRCMode),
		QSVRDOMode:                       normalizedOptionalQSVRDOMode(input.QSVRDOMode),
		QSVPStrategy:                     normalizedOptionalQSVPStrategy(input.QSVPStrategy),
		VideoToolboxBitrateMbps:          input.VideoToolboxBitrateMbps,
		VideoToolboxMaxrateMbps:          input.VideoToolboxMaxrateMbps,
		VideoToolboxBufferMbps:           input.VideoToolboxBufferMbps,
		VideoToolboxQualityProfile:       input.VideoToolboxQualityProfile,
		VideoToolboxProfile:              strings.TrimSpace(input.VideoToolboxProfile),
		VideoToolboxGOP:                  input.VideoToolboxGOP,
		VideoToolboxRealtime:             input.VideoToolboxRealtime,
		VideoToolboxBFramePolicy:         strings.TrimSpace(input.VideoToolboxBFramePolicy),
		VideoToolboxBFrames:              input.VideoToolboxBFrames,
		VideoToolboxAutoAdjustBitrate:    input.VideoToolboxAutoAdjustBitrate,
		VideoToolboxAllowFrameReordering: input.VideoToolboxAllowFrameReordering,
		VideoToolboxPowerEfficiency:      input.VideoToolboxPowerEfficiency,
		HardwareQualityPreset:            strings.TrimSpace(input.HardwareQualityPreset),
	}, nil
}

func isAssetConversionValidationError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "must be ") || strings.Contains(message, "is not a supported")
}
