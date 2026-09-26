package handlers

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"unicode"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

// resolveTrackPlan turns a Track Profile plus the stable asset snapshot into
// concrete decisions. It is intentionally independent from Queue, Lab, Test
// Encode, and FFmpeg rendering so every caller can share the same decision.
func resolveTrackPlan(scan models.ScanResult, profile map[string]any) (ResolvedTrackPlan, error) {
	return resolveTrackPlanWithSubtitleOverrides(scan, profile, SubtitleOperationOverrides{})
}

func resolveTrackPlanWithSubtitleOverrides(scan models.ScanResult, profile map[string]any, overrides SubtitleOperationOverrides) (ResolvedTrackPlan, error) {
	attachmentInventoryAvailable := normalizeScanResultAttachmentStreams(&scan)
	if err := validateSubtitleRules(profile); err != nil {
		return ResolvedTrackPlan{}, err
	}
	canonicalDisposition, err := canonicalTrackDispositionProfile(profile)
	if err != nil {
		return ResolvedTrackPlan{}, err
	}
	if overrides.Extract {
		canonicalDisposition = true
	}

	video := resolvedStreams(scan.VideoStreams, selectedProfileIndexes(profile, "keepVideoStreams"))
	selectedAudio, audioSelectionExplicit := profileIndexSet(profile, "keepAudioStreams")
	var selectedAudioPointer *map[int]bool
	if audioSelectionExplicit {
		selectedAudioPointer = &selectedAudio
	}
	audio := resolvedStreams(scan.AudioStreams, selectedAudioPointer)
	removedAudio := []ResolvedTrackStream{}
	if audioSelectionExplicit {
		allAudio := resolvedStreams(scan.AudioStreams, nil)
		for _, stream := range allAudio {
			if !selectedAudio[stream.StreamIndex] {
				removedAudio = append(removedAudio, stream)
			}
		}
	}

	defaultDisposition := SubtitleDispositionKeep
	if raw, exists := profile["subtitleDisposition"]; exists {
		parsed, err := ParseSubtitleDisposition(workerStringValue(raw))
		if err != nil {
			return ResolvedTrackPlan{}, err
		}
		defaultDisposition = parsed
	}
	selectedSubtitles, explicitSubtitleSelection := profileIndexSet(profile, "keepSubtitleStreams")
	resolvedCanonicalPathSelection :=
		canonicalDisposition &&
			storedSettingProfileScope(profile) == "path" &&
			strings.TrimSpace(workerStringValue(profile["resolvedForAsset"])) != "" &&
			explicitSubtitleSelection
	transforms := subtitleTransformsByIndex(profile["subtitleTransforms"])
	subtitles := make([]ResolvedSubtitleTrack, 0, len(scan.SubtitleStreams))
	sidecars := []ResolvedTrackSidecar{}
	warnings := []string{}
	preferredRuleSource := map[int]bool{}
	for _, raw := range scan.SubtitleStreams {
		stream := settingProfileObject(raw)
		index := streamIndexValue(stream["index"])
		if index < 0 {
			continue
		}
		if overrides.Extract && overrides.StreamIndex != nil && index != *overrides.StreamIndex {
			continue
		}
		codec := strings.ToLower(strings.TrimSpace(workerStringValue(stream["codec"])))
		language := normalizedTrackLanguage(workerStringValue(stream["language"]))
		role := classifySubtitleRole(scan, stream, profile)
		action := defaultDisposition
		if explicitSubtitleSelection &&
			(!canonicalDisposition || resolvedCanonicalPathSelection) &&
			!selectedSubtitles[index] {
			action = SubtitleDispositionRemove
		}
		if !canonicalDisposition {
			if transform, ok := transforms[index]; ok {
				if transform.RemoveEmbedded {
					action = SubtitleDispositionExtract
				} else {
					action = SubtitleDispositionKeepAndExtract
				}
			}
		}
		if !resolvedCanonicalPathSelection || selectedSubtitles[index] {
			var profileRuleMatched bool
			action, profileRuleMatched = matchingSubtitleRuleActionWithMatch(
				profile,
				index,
				language,
				role.Role,
				action,
			)
			if !profileRuleMatched {
				var preferredRuleMatched bool
				action, preferredRuleMatched = matchingPreferredSubtitleRuleAction(profile, language, action)
				if preferredRuleMatched {
					// Kept on the resolved track for the preview and downstream
					// diagnostics; this is not a second resolution path.
					preferredRuleSource[index] = true
				}
			}
		}
		if overrides.Extract {
			action = SubtitleDispositionKeepAndExtract
		}
		sourceForced := boolValue(stream["forced"], false)
		effectiveForced := sourceForced || role.Role == SubtitleRoleForced
		resolved := ResolvedSubtitleTrack{
			StreamIndex: index, Codec: codec, Language: language, Action: action,
			Role: role.Role, RoleProvenance: role.Provenance, RoleConfidence: role.Confidence,
			RoleClassifierVersion: role.ClassifierVersion,
			RoleOverrideState:     role.OverrideState, RoleSourceSnapshotFingerprint: role.SourceSnapshotFingerprint,
			RoleStreamSignature: role.StreamSignature, Forced: sourceForced, EffectiveForced: effectiveForced,
		}
		if overrides.Extract {
			resolved.ActionSource = "operation_override"
		} else if preferredRuleSource[index] {
			resolved.ActionSource = "preferred_subtitle_language"
		}
		subtitles = append(subtitles, resolved)
		if action.ExtractsSidecar() {
			formats := []string{"original"}
			formatSource := "built_in"
			if canonicalDisposition {
				defaultFormats := formats
				if overrides.Extract {
					defaultFormats = []string{"srt"}
				}
				formats = matchingSubtitleSidecarFormats(profile, index, language, role.Role, defaultFormats)
				formatSource = subtitleSidecarFormatSource(profile, index, language, role.Role)
			} else {
				if transform, ok := transforms[index]; ok &&
					strings.TrimSpace(transform.Format) != "" {
					formats = []string{strings.ToLower(strings.TrimSpace(transform.Format))}
					formatSource = "asset_profile"
				}
			}
			if overrides.Format != nil {
				formats = []string{strings.ToLower(strings.TrimSpace(*overrides.Format))}
				formatSource = "operation_override"
			}
			fallbackEnabled := subtitleSRTFallbackEnabled(profile)
			originalRequestedFormats := append([]string(nil), formats...)
			formats = effectiveSubtitleSidecarFormats(codec, formats, fallbackEnabled)
			resolvedFormats := map[string]bool{}
			for _, requested := range formats {
				requestedFormats := []string{requested}
				if strings.EqualFold(strings.TrimSpace(requested), "both") {
					requestedFormats = []string{"srt", "ass"}
				}
				for _, outputRequested := range requestedFormats {
					format, mode, formatErr := resolveSubtitleSidecarFormat(codec, outputRequested)
					if formatErr != nil {
						return ResolvedTrackPlan{}, fmt.Errorf("subtitle stream %d: %w", index, formatErr)
					}
					outputKey := format + ":" + mode + ":" + requested
					if resolvedFormats[outputKey] {
						continue
					}
					resolvedFormats[outputKey] = true
					ocrLanguage, ocrMode := "", ""
					ocrLanguageSource, ocrModeSource := "built_in", "built_in"
					sourceType := subtitleSourceType(codec)
					stages := subtitleArtifactStages(sourceType, mode)
					if mode == "converted" && isBitmapSubtitleCodecName(codec) {
						ocrLanguage, ocrMode = "auto", "accurate"
						if !canonicalDisposition {
							if transform, ok := transforms[index]; ok {
								if strings.TrimSpace(transform.OCRLanguage) != "" {
									ocrLanguage = strings.ToLower(strings.TrimSpace(transform.OCRLanguage))
									ocrLanguageSource = "asset_profile"
								}
								if strings.TrimSpace(transform.OCRMode) != "" {
									ocrMode = normalizedOCRMode(transform.OCRMode)
									ocrModeSource = "asset_profile"
								}
							}
						} else {
							ocrLanguage, ocrMode, ocrLanguageSource, ocrModeSource = matchingSubtitleOCRConfigWithSources(profile, index, language, role.Role)
						}
						if overrides.OCRLanguage != nil {
							ocrLanguage = strings.ToLower(strings.TrimSpace(*overrides.OCRLanguage))
							ocrLanguageSource = "operation_override"
						}
						if overrides.OCRMode != nil {
							ocrMode = normalizedOCRMode(*overrides.OCRMode)
							ocrModeSource = "operation_override"
						}
					}
					fallbackSource, styleSource := subtitleFallbackSettingSource(profile), subtitleStylingSettingSource(profile)
					fallbackApplied := subtitleSRTFallbackApplied(codec, originalRequestedFormats, format, fallbackEnabled)
					var stylingPolicy *SubtitleStylingSettings
					if mode == "converted" && format == "ass" {
						settings := subtitleStylePolicyFromMap(profile[subtitleStylingPolicyKey]).Settings
						stylingPolicy = &settings
					}
					sidecars = append(sidecars, ResolvedTrackSidecar{
						StreamIndex: index, Codec: codec, SourceType: sourceType, Language: language, Format: format, RequestedFormat: requested, Mode: mode,
						OCRRequired: mode == "converted" && sourceType == "bitmap", Stages: stages, Extension: "." + format,
						NamingKey: fmt.Sprintf("subtitle:%d:%s:%s", index, mode, format), LogicalTransformKey: fmt.Sprintf("subtitle:%d:%s:%s", index, mode, requested),
						Title: workerStringValue(stream["title"]), Default: boolValue(stream["default"], false), Forced: effectiveForced, SourceForced: sourceForced,
						Role: role.Role, RoleProvenance: role.Provenance, RoleConfidence: role.Confidence,
						RoleClassifierVersion: role.ClassifierVersion,
						OCRLanguage:           ocrLanguage, OCRMode: ocrMode,
						SRTFallbackApplied: fallbackApplied, StylingPolicy: stylingPolicy,
						SettingsSources: map[string]string{
							"format": formatSource, "ocrLanguage": ocrLanguageSource, "ocrMode": ocrModeSource,
							"srtFallback": fallbackSource, "styling": styleSource,
						},
					})
					if mode == "converted" && format == "srt" && (codec == "ass" || codec == "ssa") {
						warnings = append(warnings, fmt.Sprintf("Subtitle stream %d: SRT improves compatibility but does not preserve all ASS styling and positioning.", index))
					}
				}
			}
		}
	}

	attachmentPolicy := AttachmentPolicyKeep
	if raw, exists := profile["attachmentPolicy"]; exists {
		parsed, err := ParseAttachmentPolicy(workerStringValue(raw))
		if err != nil {
			return ResolvedTrackPlan{}, err
		}
		attachmentPolicy = parsed
	}
	attachmentsKept, err := ResolveAttachmentRetention(attachmentPolicy, subtitles)
	if err != nil {
		return ResolvedTrackPlan{}, err
	}
	attachments := resolvedAttachmentStreams(scan.AttachmentStreams)
	fontPolicy, err := ParseFontAttachmentExportPolicy(workerStringValue(profile["fontAttachmentExportPolicy"]))
	if err != nil {
		return ResolvedTrackPlan{}, err
	}
	fontAttachments, err := resolveFontAttachmentPlan(fontPolicy, attachmentInventoryAvailable, attachments, sidecars)
	if err != nil {
		return ResolvedTrackPlan{}, err
	}
	attachmentReason := attachmentResolutionReason(attachmentPolicy, subtitles)
	if attachmentPolicy == AttachmentPolicyRemove && embeddedASSOrSSAExists(subtitles) {
		warnings = append(warnings, "Font attachments were explicitly removed while an embedded ASS/SSA subtitle remains; rendering may differ on clients without those fonts.")
	}

	chapterPolicy := ChapterPolicyKeep
	if raw, exists := profile["chapterPolicy"]; exists {
		parsed, parseErr := ParseChapterPolicy(workerStringValue(raw))
		if parseErr != nil {
			return ResolvedTrackPlan{}, parseErr
		}
		chapterPolicy = parsed
	}
	defaultAudio := resolveDefaultAudioTrack(profile, audio)
	defaultSubtitle := resolveDefaultSubtitleTrack(profile, subtitles)
	composition, err := resolveSubtitleCompositionPlan(scan, profile, subtitles)
	if err != nil {
		return ResolvedTrackPlan{}, err
	}
	return ResolvedTrackPlan{
		RoleClassifierVersion: subtitleSemanticRoleClassifierVersion,
		VideoStreams:          video, AudioStreams: audio, RemovedAudioStreams: removedAudio, AudioSelectionExplicit: audioSelectionExplicit, SubtitleStreams: subtitles,
		AttachmentPolicy: attachmentPolicy, AttachmentsKept: attachmentsKept, AttachmentReason: attachmentReason,
		AttachmentStreams: attachments, FontAttachmentExportPolicy: fontPolicy,
		FontAttachments: fontAttachments, FontAttachmentsExported: len(fontAttachments) > 0,
		ChapterPolicy: chapterPolicy, ChaptersKept: chapterPolicy == ChapterPolicyKeep,
		SidecarOutputs: sidecars, Warnings: warnings,
		DefaultAudio:    defaultAudio,
		DefaultSubtitle: defaultSubtitle,
		Composition:     composition,
	}, nil
}

func subtitleSourceType(codec string) string {
	if isBitmapSubtitleCodecName(codec) {
		return "bitmap"
	}
	return "text"
}

func subtitleArtifactStages(sourceType, mode string) []string {
	if strings.EqualFold(mode, "original") {
		return []string{"extract"}
	}
	if strings.EqualFold(sourceType, "bitmap") {
		return []string{"ocr", "normalize", "render"}
	}
	return []string{"extract", "normalize", "render"}
}

func extractsOriginalASSOrSSA(sidecars []ResolvedTrackSidecar) bool {
	for _, sidecar := range sidecars {
		if !strings.EqualFold(strings.TrimSpace(sidecar.Mode), "original") {
			continue
		}
		format := strings.ToLower(strings.TrimSpace(sidecar.Format))
		if format == "ass" || format == "ssa" {
			return true
		}
	}
	return false
}

func resolveFontAttachmentPlan(policy FontAttachmentExportPolicy, inventoryAvailable bool, attachments []ResolvedAttachmentStream, sidecars []ResolvedTrackSidecar) ([]ResolvedFontAttachment, error) {
	if policy != FontAttachmentExportAll || !extractsOriginalASSOrSSA(sidecars) {
		return []ResolvedFontAttachment{}, nil
	}
	if !inventoryAvailable {
		return nil, fmt.Errorf("ASS/SSA font attachment export requires an available canonical attachment inventory; refresh the asset snapshot")
	}
	result := []ResolvedFontAttachment{}
	used := map[string]struct{}{}
	for ordinal, attachment := range attachments {
		format := strings.ToUpper(strings.TrimSpace(attachment.FontFormat))
		if !strings.EqualFold(strings.TrimSpace(attachment.AttachmentKind), "FONT") || !supportedFontAttachmentFormat(format) {
			continue
		}
		safeName := uniqueFontAttachmentFilename(safeFontAttachmentFilename(attachment.Filename, attachment.StreamIndex, format), attachment.StreamIndex, used)
		result = append(result, ResolvedFontAttachment{
			ArtifactID: fontAttachmentArtifactID(attachment.StreamIndex), StreamIndex: attachment.StreamIndex, AttachmentOrdinal: ordinal,
			Codec: attachment.Codec, OriginalName: attachment.Filename, MIMEType: attachment.MIMEType, FontFormat: format, SafeFilename: safeName,
		})
	}
	return result, nil
}

func supportedFontAttachmentFormat(format string) bool {
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case "TTF", "OTF", "TTC", "OTC":
		return true
	default:
		return false
	}
}

func safeFontAttachmentFilename(original string, streamIndex int, fontFormat string) string {
	base := path.Base(strings.ReplaceAll(strings.TrimSpace(original), `\`, "/"))
	extension := "." + strings.ToLower(strings.TrimSpace(fontFormat))
	base = strings.TrimSuffix(base, path.Ext(base))
	base = strings.Map(func(value rune) rune {
		if unicode.IsControl(value) || strings.ContainsRune(`<>:"/\|?*`, value) {
			return -1
		}
		return value
	}, base)
	base = strings.Trim(strings.TrimSpace(base), ".")
	if base == "" {
		base = fmt.Sprintf("attachment-%d", streamIndex)
	}
	return base + extension
}

func uniqueFontAttachmentFilename(candidate string, streamIndex int, used map[string]struct{}) string {
	key := strings.ToLower(candidate)
	if _, exists := used[key]; !exists {
		used[key] = struct{}{}
		return candidate
	}
	extension := path.Ext(candidate)
	base := strings.TrimSuffix(candidate, extension)
	candidate = fmt.Sprintf("%s.stream-%d%s", base, streamIndex, extension)
	for discriminator := 2; ; discriminator++ {
		key = strings.ToLower(candidate)
		if _, exists := used[key]; !exists {
			used[key] = struct{}{}
			return candidate
		}
		candidate = fmt.Sprintf("%s.stream-%d.%d%s", base, streamIndex, discriminator, extension)
	}
}

func matchingSubtitleSidecarFormats(profile map[string]any, streamIndex int, language string, role SubtitleSemanticRole, fallback []string) []string {
	formats := normalizedSubtitleSidecarFormats(profile["subtitleSidecarFormats"])
	if len(formats) == 0 {
		formats = append([]string(nil), fallback...)
	}
	streamSpecific := false
	for _, raw := range workerSliceValue(profile["subtitleRules"]) {
		rule := settingProfileObject(raw)
		candidate := normalizedSubtitleSidecarFormats(rule["sidecarFormats"])
		if len(candidate) == 0 {
			continue
		}
		matched, specific := subtitleRuleMatches(rule, streamIndex, language, role)
		if specific {
			if matched {
				formats, streamSpecific = candidate, true
			}
			continue
		}
		if matched && !streamSpecific {
			formats = candidate
		}
	}
	if byStream := settingProfileObject(profile["subtitleSidecarFormatsByStream"]); byStream != nil {
		if candidate := normalizedSubtitleSidecarFormats(byStream[fmt.Sprintf("%d", streamIndex)]); len(candidate) > 0 {
			formats = candidate
		}
	} else if byStream, ok := profile["subtitleSidecarFormatsByStream"].(map[int][]string); ok {
		if candidate := normalizedSubtitleSidecarFormats(byStream[streamIndex]); len(candidate) > 0 {
			formats = candidate
		}
	}
	return formats
}

const (
	subtitleSettingSourcesKey = "resolvedSubtitleSettingSources"
	subtitleTrackSourceKey    = "resolvedTrackSettingsSource"
)

func setSubtitleSettingSource(profile map[string]any, field, source string) {
	if profile == nil {
		return
	}
	sources := settingProfileObject(profile[subtitleSettingSourcesKey])
	copy := map[string]any{}
	for key, value := range sources {
		copy[key] = value
	}
	copy[field] = source
	profile[subtitleSettingSourcesKey] = copy
}

func markSubtitleTrackSources(profile map[string]any, source string) {
	if profile == nil {
		return
	}
	sources := settingProfileObject(profile[subtitleSettingSourcesKey])
	copy := map[string]any{}
	for key, value := range sources {
		copy[key] = value
	}
	for key := range map[string]bool{
		"subtitleSidecarFormats": true, "subtitleSidecarFormatsByStream": true,
		"subtitleRules": true, "ocrLanguage": true, "ocrMode": true,
		"subtitleDisposition": true, "defaultSubtitleLanguage": true,
		"preferredSubtitleRules": true,
	} {
		if _, present := profile[key]; present {
			copy[key] = source
		}
	}
	if rules := workerSliceValue(profile["subtitleRules"]); len(rules) > 0 {
		copy[subtitleRuleSourcesKey] = repeatedSubtitleRuleSources(len(rules), source)
	}
	profile[subtitleSettingSourcesKey] = copy
}

func mergeSubtitleTrackProfiles(base, overlay map[string]any) map[string]any {
	if base == nil && overlay == nil {
		return nil
	}
	result := map[string]any{}
	for key, value := range base {
		result[key] = value
	}
	baseRules := workerSliceValue(result["subtitleRules"])
	for key, value := range overlay {
		if key == subtitleSettingSourcesKey || key == subtitleTrackSourceKey {
			continue
		}
		if key == "subtitleRules" {
			combined := append([]any(nil), baseRules...)
			combined = append(combined, workerSliceValue(value)...)
			result[key] = combined
			continue
		}
		if key == "keepVideoStreams" || key == "keepAudioStreams" || key == "keepSubtitleStreams" ||
			key == "videoMetadata" || key == "audioMetadata" || key == "subtitleMetadata" || key == "subtitleTransforms" {
			result[key] = value
			continue
		}
		result[key] = value
	}
	sources := map[string]any{}
	for key, value := range settingProfileObject(base[subtitleSettingSourcesKey]) {
		sources[key] = value
	}
	for key, value := range settingProfileObject(overlay[subtitleSettingSourcesKey]) {
		sources[key] = value
	}
	baseRuleSources := subtitleRuleSources(base)
	overlayRuleSources := subtitleRuleSources(overlay)
	if len(baseRuleSources)+len(overlayRuleSources) > 0 {
		sources[subtitleRuleSourcesKey] = append(baseRuleSources, overlayRuleSources...)
	}
	result[subtitleSettingSourcesKey] = sources
	return result
}

func subtitleProfileSettingSource(profile map[string]any, field string, present bool) string {
	if !present {
		return "built_in"
	}
	if sources := settingProfileObject(profile[subtitleSettingSourcesKey]); sources != nil {
		if source := strings.TrimSpace(workerStringValue(sources[field])); source != "" {
			return source
		}
	}
	if source := strings.TrimSpace(workerStringValue(profile[subtitleTrackSourceKey])); source != "" {
		return source
	}
	return "asset_profile"
}

func subtitleSidecarFormatSource(profile map[string]any, streamIndex int, language string, role SubtitleSemanticRole) string {
	source := "built_in"
	if len(normalizedSubtitleSidecarFormats(profile["subtitleSidecarFormats"])) > 0 {
		source = subtitleProfileSettingSource(profile, "subtitleSidecarFormats", true)
	}
	streamSpecific := false
	for index, raw := range workerSliceValue(profile["subtitleRules"]) {
		rule := settingProfileObject(raw)
		if len(normalizedSubtitleSidecarFormats(rule["sidecarFormats"])) == 0 {
			continue
		}
		matched, specific := subtitleRuleMatches(rule, streamIndex, language, role)
		if specific {
			if matched {
				source = subtitleRuleSettingSource(profile, index)
				streamSpecific = true
			}
			continue
		}
		if matched && !streamSpecific {
			source = subtitleRuleSettingSource(profile, index)
		}
	}
	if byStream := settingProfileObject(profile["subtitleSidecarFormatsByStream"]); byStream != nil {
		if len(normalizedSubtitleSidecarFormats(byStream[fmt.Sprintf("%d", streamIndex)])) > 0 {
			return subtitleProfileSettingSource(profile, "subtitleSidecarFormatsByStream", true)
		}
	}
	if byStream, ok := profile["subtitleSidecarFormatsByStream"].(map[int][]string); ok {
		if len(normalizedSubtitleSidecarFormats(byStream[streamIndex])) > 0 {
			return subtitleProfileSettingSource(profile, "subtitleSidecarFormatsByStream", true)
		}
	}
	return source
}

const subtitleRuleSourcesKey = "subtitleRuleSources"

func repeatedSubtitleRuleSources(count int, source string) []any {
	result := make([]any, count)
	for index := range result {
		result[index] = source
	}
	return result
}

func subtitleRuleSources(profile map[string]any) []any {
	sources := settingProfileObject(profile[subtitleSettingSourcesKey])
	ruleSources := workerSliceValue(sources[subtitleRuleSourcesKey])
	rules := workerSliceValue(profile["subtitleRules"])
	result := make([]any, len(rules))
	for index := range rules {
		if index < len(ruleSources) {
			result[index] = workerStringValue(ruleSources[index])
		} else {
			result[index] = subtitleProfileSettingSource(profile, "subtitleRules", len(rules) > 0)
		}
	}
	return result
}

func subtitleRuleSettingSource(profile map[string]any, ruleIndex int) string {
	ruleSources := subtitleRuleSources(profile)
	if ruleIndex >= 0 && ruleIndex < len(ruleSources) {
		if source := strings.TrimSpace(workerStringValue(ruleSources[ruleIndex])); source != "" {
			return source
		}
	}
	return subtitleProfileSettingSource(profile, "subtitleRules", len(workerSliceValue(profile["subtitleRules"])) > 0)
}

func subtitleSRTFallbackApplied(codec string, requestedFormats []string, output string, enabled bool) bool {
	if !enabled || output != "srt" {
		return false
	}
	hasOriginal, hasASS, hasSRT := false, false, false
	for _, format := range requestedFormats {
		switch strings.ToLower(strings.TrimSpace(format)) {
		case "original":
			hasOriginal = true
		case "ass":
			hasASS = true
		case "srt", "both":
			hasSRT = true
		}
	}
	return !hasSRT && ((hasOriginal && subtitleFallbackEligible(codec, "original", true)) || (hasASS && subtitleFallbackEligible(codec, "ass", true)))
}

func subtitleFallbackSettingSource(profile map[string]any) string {
	settings := settingProfileObject(profile[subtitleSettingsKey])
	_, present := settings[generateSRTFallbackForASSKey]
	return subtitleProfileSettingSource(profile, "srtFallback", present)
}

func subtitleStylingSettingSource(profile map[string]any) string {
	_, present := profile[subtitleStylingPolicyKey]
	return subtitleProfileSettingSource(profile, "styling", present)
}

func normalizedSubtitleSidecarFormats(raw any) []string {
	result := []string{}
	seen := map[string]bool{}
	values := workerSliceValue(raw)
	if strings, ok := raw.([]string); ok {
		values = make([]any, len(strings))
		for index := range strings {
			values[index] = strings[index]
		}
	}
	for _, value := range values {
		format := strings.ToLower(strings.TrimSpace(workerStringValue(value)))
		if (format == "original" || format == "srt" || format == "ass" || format == "both") && !seen[format] {
			seen[format] = true
			result = append(result, format)
		}
	}
	return result
}

func subtitleSRTFallbackEnabled(profile map[string]any) bool {
	settings := settingProfileObject(profile[subtitleSettingsKey])
	if settings == nil {
		return false
	}
	return generateSRTFallbackForASSValue(models.JSONMap(settings))
}

func subtitleFallbackEligible(codec, requested string, enabled bool) bool {
	if !enabled {
		return false
	}
	codec = strings.ToLower(strings.TrimSpace(codec))
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "original" {
		return codec == "ass" || codec == "ssa"
	}
	if requested != "ass" && requested != "both" {
		return false
	}
	_, mode, err := resolveSubtitleSidecarFormat(codec, "ass")
	return err == nil && mode == "converted"
}

func effectiveSubtitleSidecarFormats(codec string, formats []string, fallbackEnabled bool) []string {
	normalized := normalizedSubtitleSidecarFormats(formats)
	hasConvertedASS := false
	hasConvertedSRT := false
	for _, requested := range normalized {
		switch requested {
		case "both":
			hasConvertedASS = true
			hasConvertedSRT = true
		case "ass":
			if _, mode, err := resolveSubtitleSidecarFormat(codec, "ass"); err == nil && mode == "converted" {
				hasConvertedASS = true
			}
		case "srt":
			if _, mode, err := resolveSubtitleSidecarFormat(codec, "srt"); err == nil && mode == "converted" {
				hasConvertedSRT = true
			}
		}
	}
	combineConverted := hasConvertedASS && (hasConvertedSRT || subtitleFallbackEligible(codec, "ass", fallbackEnabled))
	result := make([]string, 0, len(normalized)+1)
	convertedAdded := false
	for _, requested := range normalized {
		switch requested {
		case "original":
			result = append(result, requested)
		case "ass", "srt", "both":
			if combineConverted {
				if !convertedAdded {
					result = append(result, "both")
					convertedAdded = true
				}
				continue
			}
			result = append(result, requested)
		}
	}
	if subtitleFallbackEligible(codec, "original", fallbackEnabled) && !hasConvertedSRT && !combineConverted {
		result = append(result, "srt")
	}
	return result
}

func resolveSubtitleSidecarFormat(codec, requested string) (format, mode string, err error) {
	codec = strings.ToLower(strings.TrimSpace(codec))
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" {
		requested = "srt"
	}
	if requested == "original" {
		format = subtitleSidecarFormat(codec)
		if format == "" {
			return "", "", fmt.Errorf("codec %s does not support original sidecar extraction", codec)
		}
		return format, "original", nil
	}
	if requested == "srt" || requested == "ass" {
		if isBitmapSubtitleCodecName(codec) || subtitleCanConvertText(codec) {
			return requested, "converted", nil
		}
		return "", "", fmt.Errorf("codec %s cannot be converted to %s", codec, strings.ToUpper(requested))
	}
	return "", "", fmt.Errorf("unsupported sidecar format %q", requested)
}

func matchingSubtitleOCRConfig(profile map[string]any, streamIndex int, language string, role SubtitleSemanticRole) (string, string) {
	ocrLanguage, ocrMode, _, _ := matchingSubtitleOCRConfigWithSources(profile, streamIndex, language, role)
	return ocrLanguage, ocrMode
}

func matchingSubtitleOCRConfigWithSources(profile map[string]any, streamIndex int, language string, role SubtitleSemanticRole) (string, string, string, string) {
	ocrLanguage, ocrMode := "auto", "accurate"
	ocrLanguageSource, ocrModeSource := "built_in", "built_in"
	if value := strings.TrimSpace(workerStringValue(profile["ocrLanguage"])); value != "" {
		ocrLanguage = strings.ToLower(value)
		ocrLanguageSource = subtitleProfileSettingSource(profile, "ocrLanguage", true)
	}
	if value := strings.TrimSpace(workerStringValue(profile["ocrMode"])); value != "" {
		ocrMode = normalizedOCRMode(value)
		ocrModeSource = subtitleProfileSettingSource(profile, "ocrMode", true)
	}
	streamSpecific := false
	for index, raw := range workerSliceValue(profile["subtitleRules"]) {
		rule := settingProfileObject(raw)
		matched, specific := subtitleRuleMatches(rule, streamIndex, language, role)
		if specific {
			if !matched {
				continue
			}
			streamSpecific = true
		} else if streamSpecific || !matched {
			continue
		}
		if value := strings.TrimSpace(workerStringValue(rule["ocrLanguage"])); value != "" {
			ocrLanguage = strings.ToLower(value)
			ocrLanguageSource = subtitleRuleSettingSource(profile, index)
		}
		if value := strings.TrimSpace(workerStringValue(rule["ocrMode"])); value != "" {
			ocrMode = normalizedOCRMode(value)
			ocrModeSource = subtitleRuleSettingSource(profile, index)
		}
	}
	return ocrLanguage, ocrMode, ocrLanguageSource, ocrModeSource
}

func embeddedASSOrSSAExists(subtitles []ResolvedSubtitleTrack) bool {
	for _, subtitle := range subtitles {
		codec := strings.ToLower(strings.TrimSpace(subtitle.Codec))
		if subtitle.Action.KeepsEmbedded() && (codec == "ass" || codec == "ssa") {
			return true
		}
	}
	return false
}

func attachmentResolutionReason(policy AttachmentPolicy, subtitles []ResolvedSubtitleTrack) string {
	switch policy {
	case AttachmentPolicyKeep:
		return "attachments explicitly kept"
	case AttachmentPolicyRemove:
		return "attachments explicitly removed"
	default:
		if embeddedASSOrSSAExists(subtitles) {
			return "embedded ASS/SSA subtitle may require font attachments"
		}
		return "no embedded ASS/SSA subtitles remain"
	}
}

func selectedProfileIndexes(profile map[string]any, key string) *map[int]bool {
	values, exists := profileIndexSet(profile, key)
	if !exists {
		return nil
	}
	return &values
}

func profileIndexSet(profile map[string]any, key string) (map[int]bool, bool) {
	raw, exists := profile[key]
	if !exists {
		return nil, false
	}
	result := map[int]bool{}
	for _, value := range workerSliceValue(raw) {
		if index := streamIndexValue(value); index >= 0 {
			result[index] = true
		}
	}
	return result, true
}

func resolvedStreams(streams models.JSONList, selected *map[int]bool) []ResolvedTrackStream {
	result := []ResolvedTrackStream{}
	for _, raw := range streams {
		stream := settingProfileObject(raw)
		index := streamIndexValue(stream["index"])
		if index < 0 || (selected != nil && !(*selected)[index]) {
			continue
		}
		result = append(result, ResolvedTrackStream{
			StreamIndex: index,
			Codec:       strings.ToLower(strings.TrimSpace(workerStringValue(stream["codec"]))),
			Language:    normalizedTrackLanguage(workerStringValue(stream["language"])),
		})
	}
	return result
}

func resolvedAttachmentStreams(streams models.JSONList) []ResolvedAttachmentStream {
	result := []ResolvedAttachmentStream{}
	for _, raw := range streams {
		stream := settingProfileObject(raw)
		index := streamIndexValue(stream["index"])
		if index < 0 {
			continue
		}
		result = append(result, ResolvedAttachmentStream{
			StreamIndex:    index,
			Codec:          strings.ToLower(strings.TrimSpace(workerStringValue(stream["codec"]))),
			Filename:       workerStringValue(stream["filename"]),
			MIMEType:       workerStringValue(stream["mimeType"]),
			Title:          workerStringValue(stream["title"]),
			AttachmentKind: workerStringValue(stream["attachmentKind"]),
			FontFormat:     workerStringValue(stream["fontFormat"]),
		})
	}
	return result
}

func subtitleTransformsByIndex(raw any) map[int]SubtitleTransform {
	result := map[int]SubtitleTransform{}
	for _, value := range workerSliceValue(raw) {
		item := settingProfileObject(value)
		index := streamIndexValue(item["streamIndex"])
		if index < 0 {
			continue
		}
		result[index] = SubtitleTransform{
			StreamIndex: index, Format: workerStringValue(item["format"]),
			RemoveEmbedded: boolValue(item["removeEmbedded"], true),
			MakeDefault:    boolValue(item["makeDefault"], false), Language: workerStringValue(item["language"]),
			OCRLanguage: workerStringValue(item["ocrLanguage"]), OCRMode: workerStringValue(item["ocrMode"]), Title: workerStringValue(item["title"]),
		}
	}
	return result
}

// subtitleRules is the semantic, path-safe rule list. A rule may match a
// language, a concrete streamIndex (asset scope), or both. Rules are applied in
// persisted order; a stream-specific match wins over language-only rules.
func matchingSubtitleRuleAction(profile map[string]any, streamIndex int, language string, role SubtitleSemanticRole, fallback SubtitleDisposition) SubtitleDisposition {
	action, _ := matchingSubtitleRuleActionWithMatch(profile, streamIndex, language, role, fallback)
	return action
}

func matchingSubtitleRuleActionWithMatch(profile map[string]any, streamIndex int, language string, role SubtitleSemanticRole, fallback SubtitleDisposition) (SubtitleDisposition, bool) {
	action := fallback
	streamSpecific := false
	matchedAny := false
	for _, raw := range workerSliceValue(profile["subtitleRules"]) {
		rule := settingProfileObject(raw)
		rawAction := workerStringValue(rule["action"])
		if rawAction == "" {
			rawAction = workerStringValue(rule["disposition"])
		}
		parsed, err := ParseSubtitleDisposition(rawAction)
		if err != nil {
			continue
		}
		matched, specific := subtitleRuleMatches(rule, streamIndex, language, role)
		if specific {
			if matched {
				action, streamSpecific, matchedAny = parsed, true, true
			}
			continue
		}
		if matched && !streamSpecific {
			action, matchedAny = parsed, true
		}
	}
	return action, matchedAny
}

func normalizedTrackLanguage(value string) string {
	return normalizedTrackLanguageValue(value)
}

func matchingPreferredSubtitleRuleAction(profile map[string]any, language string, fallback SubtitleDisposition) (SubtitleDisposition, bool) {
	if language == "" || language == "und" {
		return fallback, false
	}
	for _, raw := range workerSliceValue(profile["preferredSubtitleRules"]) {
		rule := settingProfileObject(raw)
		if rule == nil {
			continue
		}
		preferredLanguage, ok := canonicalSubtitleLanguage(workerStringValue(rule["language"]))
		if !ok || preferredLanguage != language {
			continue
		}
		action, err := ParseSubtitleDisposition(workerStringValue(rule["action"]))
		if err != nil || action == SubtitleDispositionRemove {
			continue
		}
		return action, true
	}
	return fallback, false
}

func subtitleSidecarFormat(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "ass":
		return "ass"
	case "ssa":
		return "ssa"
	case "subrip", "srt":
		return "srt"
	case "hdmv_pgs_subtitle", "pgs":
		return "sup"
	default:
		return ""
	}
}

func validateSubtitleRules(profile map[string]any) error {
	strictSelectors, err := canonicalTrackDispositionProfile(profile)
	if err != nil {
		return err
	}
	for index, raw := range workerSliceValue(profile["subtitleRules"]) {
		rule := settingProfileObject(raw)
		if rule == nil {
			return fmt.Errorf("subtitle rule %d must be an object", index+1)
		}
		action := workerStringValue(rule["action"])
		if action == "" {
			action = workerStringValue(rule["disposition"])
		}
		if _, err := ParseSubtitleDisposition(action); err != nil {
			return fmt.Errorf("subtitle rule %d: %w", index+1, err)
		}
		if rawIndex, exists := rule["streamIndex"]; exists && streamIndexValue(rawIndex) < 0 {
			return fmt.Errorf("subtitle rule %d streamIndex must be a non-negative integer", index+1)
		}
		if rawRoles, exists := rule["roles"]; exists {
			roles, configured := normalizedSubtitleRoleList(rawRoles)
			if !configured || len(roles) == 0 {
				return fmt.Errorf("subtitle rule %d roles must contain at least one supported semantic role", index+1)
			}
		}
		rawLanguage, hasLanguage := rule["language"]
		rawIndex, hasIndex := rule["streamIndex"]
		if strictSelectors &&
			(!hasLanguage || strings.TrimSpace(workerStringValue(rawLanguage)) == "") &&
			(!hasIndex || streamIndexValue(rawIndex) < 0) {
			return fmt.Errorf(
				"subtitle rule %d requires a non-empty language or streamIndex selector; use subtitleDisposition for the default action",
				index+1,
			)
		}
	}
	return nil
}

func validateSubtitleSidecarFormats(profile map[string]any) error {
	validate := func(label string, raw any) error {
		for _, value := range workerSliceValue(raw) {
			format := strings.ToLower(strings.TrimSpace(workerStringValue(value)))
			if format != "original" && format != "srt" && format != "ass" && format != "both" {
				return fmt.Errorf("%s must contain only original, srt, ass, or both", label)
			}
		}
		return nil
	}
	if err := validate("subtitleSidecarFormats", profile["subtitleSidecarFormats"]); err != nil {
		return err
	}
	for index, raw := range workerSliceValue(profile["subtitleRules"]) {
		rule := settingProfileObject(raw)
		if err := validate(fmt.Sprintf("subtitle rule %d sidecarFormats", index+1), rule["sidecarFormats"]); err != nil {
			return err
		}
	}
	return nil
}

func resolvedTrackPlanMap(plan ResolvedTrackPlan) (models.JSONMap, error) {
	if plan.AttachmentPolicy == "" {
		plan.AttachmentPolicy = AttachmentPolicyKeep
	}
	if plan.ChapterPolicy == "" {
		plan.ChapterPolicy = ChapterPolicyKeep
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	var result models.JSONMap
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}
