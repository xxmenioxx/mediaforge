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
	attachmentInventoryAvailable := normalizeScanResultAttachmentStreams(&scan)
	if err := validateSubtitleRules(profile); err != nil {
		return ResolvedTrackPlan{}, err
	}
	canonicalDisposition, err := canonicalTrackDispositionProfile(profile)
	if err != nil {
		return ResolvedTrackPlan{}, err
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
	transforms := subtitleTransformsByIndex(profile["subtitleTransforms"])
	subtitles := make([]ResolvedSubtitleTrack, 0, len(scan.SubtitleStreams))
	sidecars := []ResolvedTrackSidecar{}
	warnings := []string{}
	for _, raw := range scan.SubtitleStreams {
		stream := settingProfileObject(raw)
		index := streamIndexValue(stream["index"])
		if index < 0 {
			continue
		}
		codec := strings.ToLower(strings.TrimSpace(workerStringValue(stream["codec"])))
		language := normalizedTrackLanguage(workerStringValue(stream["language"]))
		action := defaultDisposition
		if !canonicalDisposition &&
			explicitSubtitleSelection &&
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
		action = matchingSubtitleRuleAction(profile, index, language, action)
		resolved := ResolvedSubtitleTrack{StreamIndex: index, Codec: codec, Language: language, Action: action}
		subtitles = append(subtitles, resolved)
		if action.ExtractsSidecar() {
			formats := []string{"original"}
			if canonicalDisposition {
				formats = matchingSubtitleSidecarFormats(profile, index, language, formats)
			} else {
				if transform, ok := transforms[index]; ok &&
					strings.TrimSpace(transform.Format) != "" {
					formats = []string{strings.ToLower(strings.TrimSpace(transform.Format))}
				}
			}
			resolvedFormats := map[string]bool{}
			for _, requested := range formats {
				format, mode, formatErr := resolveSubtitleSidecarFormat(codec, requested)
				if formatErr != nil {
					return ResolvedTrackPlan{}, fmt.Errorf("subtitle stream %d: %w", index, formatErr)
				}
				outputKey := format + ":" + mode
				if resolvedFormats[outputKey] {
					continue
				}
				resolvedFormats[outputKey] = true
				sidecars = append(sidecars, ResolvedTrackSidecar{
					StreamIndex: index, Codec: codec, Language: language, Format: format, Mode: mode,
					Title: workerStringValue(stream["title"]), Default: boolValue(stream["default"], false), Forced: boolValue(stream["forced"], false),
				})
				if mode == "converted" && format == "srt" && (codec == "ass" || codec == "ssa") {
					warnings = append(warnings, fmt.Sprintf("Subtitle stream %d: SRT improves compatibility but does not preserve all ASS styling and positioning.", index))
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

	return ResolvedTrackPlan{
		VideoStreams: video, AudioStreams: audio, RemovedAudioStreams: removedAudio, AudioSelectionExplicit: audioSelectionExplicit, SubtitleStreams: subtitles,
		AttachmentPolicy: attachmentPolicy, AttachmentsKept: attachmentsKept, AttachmentReason: attachmentReason,
		AttachmentStreams: attachments, FontAttachmentExportPolicy: fontPolicy,
		FontAttachments: fontAttachments, FontAttachmentsExported: len(fontAttachments) > 0,
		ChapterPolicy: chapterPolicy, ChaptersKept: chapterPolicy == ChapterPolicyKeep,
		SidecarOutputs: sidecars, Warnings: warnings,
	}, nil
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

func matchingSubtitleSidecarFormats(profile map[string]any, streamIndex int, language string, fallback []string) []string {
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
		if rawIndex, ok := rule["streamIndex"]; ok && streamIndexValue(rawIndex) >= 0 {
			if streamIndexValue(rawIndex) == streamIndex {
				formats, streamSpecific = candidate, true
			}
			continue
		}
		if !streamSpecific && normalizedTrackLanguage(workerStringValue(rule["language"])) == language {
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
		if (format == "original" || format == "srt") && !seen[format] {
			seen[format] = true
			result = append(result, format)
		}
	}
	return result
}

func resolveSubtitleSidecarFormat(codec, requested string) (format, mode string, err error) {
	codec = strings.ToLower(strings.TrimSpace(codec))
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "original" {
		format = subtitleSidecarFormat(codec)
		if format == "" {
			return "", "", fmt.Errorf("codec %s does not support original sidecar extraction", codec)
		}
		return format, "original", nil
	}
	if requested == "srt" {
		switch codec {
		case "subrip", "srt":
			return "srt", "original", nil
		case "ass", "ssa":
			return "srt", "converted", nil
		default:
			return "", "", fmt.Errorf("codec %s cannot be converted to SRT without OCR", codec)
		}
	}
	return "", "", fmt.Errorf("unsupported sidecar format %q", requested)
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
		}
	}
	return result
}

// subtitleRules is the semantic, path-safe rule list. A rule may match a
// language, a concrete streamIndex (asset scope), or both. Rules are applied in
// persisted order; a stream-specific match wins over language-only rules.
func matchingSubtitleRuleAction(profile map[string]any, streamIndex int, language string, fallback SubtitleDisposition) SubtitleDisposition {
	action := fallback
	streamSpecific := false
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
		rawLanguage, hasLanguage := rule["language"]
		ruleLanguage := ""
		if hasLanguage && strings.TrimSpace(workerStringValue(rawLanguage)) != "" {
			ruleLanguage = normalizedTrackLanguage(workerStringValue(rawLanguage))
		}
		rawIndex, hasIndex := rule["streamIndex"]
		index := streamIndexValue(rawIndex)
		if hasIndex && index >= 0 {
			if index == streamIndex {
				action, streamSpecific = parsed, true
			}
			continue
		}
		if !streamSpecific && ruleLanguage != "" && ruleLanguage == language {
			action = parsed
		}
	}
	return action
}

func normalizedTrackLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "und"
	}
	return value
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
			if format != "original" && format != "srt" {
				return fmt.Errorf("%s must contain only original or srt", label)
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
