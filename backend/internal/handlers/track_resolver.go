package handlers

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

// resolveTrackPlan turns a Track Profile plus the stable asset snapshot into
// concrete decisions. It is intentionally independent from Queue, Lab, Test
// Encode, and FFmpeg rendering so every caller can share the same decision.
func resolveTrackPlan(scan models.ScanResult, profile map[string]any) (ResolvedTrackPlan, error) {
	if err := validateSubtitleRules(profile); err != nil {
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
	for _, raw := range scan.SubtitleStreams {
		stream := settingProfileObject(raw)
		index := streamIndexValue(stream["index"])
		if index < 0 {
			continue
		}
		codec := strings.ToLower(strings.TrimSpace(workerStringValue(stream["codec"])))
		language := normalizedTrackLanguage(workerStringValue(stream["language"]))
		action := defaultDisposition
		if explicitSubtitleSelection && !selectedSubtitles[index] {
			action = SubtitleDispositionRemove
		}
		if transform, ok := transforms[index]; ok {
			if transform.RemoveEmbedded {
				action = SubtitleDispositionExtract
			} else {
				action = SubtitleDispositionKeepAndExtract
			}
		}
		action = matchingSubtitleRuleAction(profile, index, language, action)
		resolved := ResolvedSubtitleTrack{StreamIndex: index, Codec: codec, Language: language, Action: action}
		subtitles = append(subtitles, resolved)
		if action.ExtractsSidecar() {
			format := subtitleSidecarFormat(codec)
			if transform, ok := transforms[index]; ok && strings.TrimSpace(transform.Format) != "" {
				format = strings.ToLower(strings.TrimSpace(transform.Format))
			}
			sidecars = append(sidecars, ResolvedTrackSidecar{
				StreamIndex: index, Codec: codec, Language: language, Format: format,
				Title: workerStringValue(stream["title"]), Default: boolValue(stream["default"], false), Forced: boolValue(stream["forced"], false),
			})
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
	attachments := resolvedRawProbeStreams(scan.RawProbe, "attachment")
	if !attachmentsKept {
		attachments = []ResolvedTrackStream{}
	}
	attachmentReason := attachmentResolutionReason(attachmentPolicy, subtitles)
	warnings := []string{}
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
		AttachmentStreams: attachments, FontAttachmentsExported: false,
		ChapterPolicy: chapterPolicy, ChaptersKept: chapterPolicy == ChapterPolicyKeep,
		SidecarOutputs: sidecars, Warnings: warnings,
	}, nil
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

func resolvedRawProbeStreams(raw models.JSONMap, codecType string) []ResolvedTrackStream {
	result := []ResolvedTrackStream{}
	for _, value := range workerSliceValue(raw["streams"]) {
		stream := settingProfileObject(value)
		if !strings.EqualFold(workerStringValue(stream["codec_type"]), codecType) {
			continue
		}
		language := "und"
		if tags := settingProfileObject(stream["tags"]); tags != nil {
			language = normalizedTrackLanguage(workerStringValue(tags["language"]))
		}
		result = append(result, ResolvedTrackStream{
			StreamIndex: streamIndexValue(stream["index"]),
			Codec:       strings.ToLower(strings.TrimSpace(workerStringValue(stream["codec_name"]))),
			Language:    language,
		})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].StreamIndex < result[j].StreamIndex })
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
