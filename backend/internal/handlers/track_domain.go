package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

// Track disposition data flow (current, before the end-to-end migration):
//
//	AppSetting(trackProfiles) -> inheritance/resolveTrackProfileForAsset
//	-> QueueJob.TrackProfileSnapshot -> worker override merge
//	-> MediaJobPlan/ResolvedStreamPlan -> FFmpegCommandBuilder
//
// Track profiles currently own selected stream indexes and subtitle transforms,
// while the legacy video Profile still owns PreserveSubtitles and
// PreserveChapters, and WorkerConfig may own removeAttachments. ResolvedTrackPlan
// is the canonical boundary being introduced to remove that split authority;
// command rendering must consume it rather than reinterpret profile semantics.

type SubtitleDisposition string

const (
	SubtitleDispositionKeep           SubtitleDisposition = "keep"
	SubtitleDispositionRemove         SubtitleDisposition = "remove"
	SubtitleDispositionExtract        SubtitleDisposition = "extract"
	SubtitleDispositionKeepAndExtract SubtitleDisposition = "keep_and_extract"
)

func ParseSubtitleDisposition(value string) (SubtitleDisposition, error) {
	disposition := SubtitleDisposition(strings.ToLower(strings.TrimSpace(value)))
	switch disposition {
	case SubtitleDispositionKeep, SubtitleDispositionRemove, SubtitleDispositionExtract, SubtitleDispositionKeepAndExtract:
		return disposition, nil
	default:
		return "", fmt.Errorf("subtitle disposition must be keep, remove, extract, or keep_and_extract")
	}
}

func (value SubtitleDisposition) KeepsEmbedded() bool {
	return value == SubtitleDispositionKeep || value == SubtitleDispositionKeepAndExtract
}

func (value SubtitleDisposition) ExtractsSidecar() bool {
	return value == SubtitleDispositionExtract || value == SubtitleDispositionKeepAndExtract
}

func (value SubtitleDisposition) MarshalJSON() ([]byte, error) {
	parsed, err := ParseSubtitleDisposition(string(value))
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(parsed))
}

func (value *SubtitleDisposition) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("subtitle disposition: %w", err)
	}
	parsed, err := ParseSubtitleDisposition(raw)
	if err != nil {
		return err
	}
	*value = parsed
	return nil
}

type AttachmentPolicy string

const (
	AttachmentPolicyAuto   AttachmentPolicy = "auto"
	AttachmentPolicyKeep   AttachmentPolicy = "keep"
	AttachmentPolicyRemove AttachmentPolicy = "remove"
)

func ParseAttachmentPolicy(value string) (AttachmentPolicy, error) {
	policy := AttachmentPolicy(strings.ToLower(strings.TrimSpace(value)))
	switch policy {
	case AttachmentPolicyAuto, AttachmentPolicyKeep, AttachmentPolicyRemove:
		return policy, nil
	default:
		return "", fmt.Errorf("attachment policy must be auto, keep, or remove")
	}
}

// ResolveAttachmentRetention implements the conservative auto policy: font
// attachments survive when any embedded ASS/SSA subtitle may depend on them.
// Extract-only ASS/SSA does not require fonts in the final media container.
func ResolveAttachmentRetention(policy AttachmentPolicy, subtitles []ResolvedSubtitleTrack) (bool, error) {
	parsed, err := ParseAttachmentPolicy(string(policy))
	if err != nil {
		return false, err
	}
	if parsed == AttachmentPolicyKeep {
		return true, nil
	}
	if parsed == AttachmentPolicyRemove {
		return false, nil
	}
	for _, subtitle := range subtitles {
		codec := strings.ToLower(strings.TrimSpace(subtitle.Codec))
		if subtitle.Action.KeepsEmbedded() && (codec == "ass" || codec == "ssa") {
			return true, nil
		}
	}
	return false, nil
}

func (value AttachmentPolicy) MarshalJSON() ([]byte, error) {
	parsed, err := ParseAttachmentPolicy(string(value))
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(parsed))
}

func (value *AttachmentPolicy) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("attachment policy: %w", err)
	}
	parsed, err := ParseAttachmentPolicy(raw)
	if err != nil {
		return err
	}
	*value = parsed
	return nil
}

type FontAttachmentExportPolicy string

const (
	FontAttachmentExportNone FontAttachmentExportPolicy = "none"
	FontAttachmentExportAll  FontAttachmentExportPolicy = "all"
)

func ParseFontAttachmentExportPolicy(value string) (FontAttachmentExportPolicy, error) {
	policy := FontAttachmentExportPolicy(strings.ToLower(strings.TrimSpace(value)))
	switch policy {
	case "", FontAttachmentExportNone:
		return FontAttachmentExportNone, nil
	case FontAttachmentExportAll:
		return FontAttachmentExportAll, nil
	default:
		return "", fmt.Errorf("invalid ASS/SSA font attachment export policy %q", value)
	}
}

func (value FontAttachmentExportPolicy) MarshalJSON() ([]byte, error) {
	parsed, err := ParseFontAttachmentExportPolicy(string(value))
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(parsed))
}

func (value *FontAttachmentExportPolicy) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("ASS/SSA font attachment export policy: %w", err)
	}
	parsed, err := ParseFontAttachmentExportPolicy(raw)
	if err != nil {
		return err
	}
	*value = parsed
	return nil
}

type ChapterPolicy string

const (
	ChapterPolicyKeep   ChapterPolicy = "keep"
	ChapterPolicyRemove ChapterPolicy = "remove"
)

func ParseChapterPolicy(value string) (ChapterPolicy, error) {
	policy := ChapterPolicy(strings.ToLower(strings.TrimSpace(value)))
	switch policy {
	case ChapterPolicyKeep, ChapterPolicyRemove:
		return policy, nil
	default:
		return "", fmt.Errorf("chapter policy must be keep or remove")
	}
}

func (value ChapterPolicy) MarshalJSON() ([]byte, error) {
	parsed, err := ParseChapterPolicy(string(value))
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(parsed))
}

func (value *ChapterPolicy) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("chapter policy: %w", err)
	}
	parsed, err := ParseChapterPolicy(raw)
	if err != nil {
		return err
	}
	*value = parsed
	return nil
}

type ResolvedTrackStream struct {
	StreamIndex int    `json:"streamIndex"`
	Codec       string `json:"codec,omitempty"`
	Language    string `json:"language,omitempty"`
}

type ResolvedAttachmentStream struct {
	StreamIndex    int    `json:"streamIndex"`
	Codec          string `json:"codec,omitempty"`
	Filename       string `json:"filename,omitempty"`
	MIMEType       string `json:"mimeType,omitempty"`
	Title          string `json:"title,omitempty"`
	AttachmentKind string `json:"attachmentKind"`
	FontFormat     string `json:"fontFormat,omitempty"`
}

type ResolvedFontAttachment struct {
	ArtifactID        string `json:"artifactId"`
	StreamIndex       int    `json:"streamIndex"`
	AttachmentOrdinal int    `json:"attachmentOrdinal"`
	Codec             string `json:"codec,omitempty"`
	OriginalName      string `json:"originalName,omitempty"`
	MIMEType          string `json:"mimeType,omitempty"`
	FontFormat        string `json:"fontFormat"`
	SafeFilename      string `json:"safeFilename"`
}

type ResolvedSubtitleTrack struct {
	StreamIndex int                 `json:"streamIndex"`
	Codec       string              `json:"codec,omitempty"`
	Language    string              `json:"language,omitempty"`
	Action      SubtitleDisposition `json:"action"`
}

type ResolvedTrackSidecar struct {
	StreamIndex int    `json:"streamIndex"`
	Codec       string `json:"codec,omitempty"`
	Language    string `json:"language,omitempty"`
	Format      string `json:"format,omitempty"`
	Mode        string `json:"mode"`
	Title       string `json:"title,omitempty"`
	Default     bool   `json:"default"`
	Forced      bool   `json:"forced"`
	OCRLanguage string `json:"ocrLanguage,omitempty"`
	OCRMode     string `json:"ocrMode,omitempty"`
}

// ResolvedTrackPlan freezes semantic Track Profile rules into per-asset stream
// decisions. Task 2 wires this model into Queue snapshots and execution plans.
type ResolvedTrackPlan struct {
	VideoStreams               []ResolvedTrackStream      `json:"videoStreams"`
	AudioStreams               []ResolvedTrackStream      `json:"audioStreams"`
	RemovedAudioStreams        []ResolvedTrackStream      `json:"removedAudioStreams"`
	AudioSelectionExplicit     bool                       `json:"audioSelectionExplicit"`
	SubtitleStreams            []ResolvedSubtitleTrack    `json:"subtitleStreams"`
	AttachmentPolicy           AttachmentPolicy           `json:"attachmentPolicy"`
	AttachmentsKept            bool                       `json:"attachmentsKept"`
	AttachmentReason           string                     `json:"attachmentReason"`
	AttachmentStreams          []ResolvedAttachmentStream `json:"attachmentStreams"`
	FontAttachmentExportPolicy FontAttachmentExportPolicy `json:"fontAttachmentExportPolicy"`
	FontAttachments            []ResolvedFontAttachment   `json:"fontAttachments"`
	FontAttachmentsExported    bool                       `json:"fontAttachmentsExported"`
	ChapterPolicy              ChapterPolicy              `json:"chapterPolicy"`
	ChaptersKept               bool                       `json:"chaptersKept"`
	SidecarOutputs             []ResolvedTrackSidecar     `json:"sidecarOutputs"`
	Warnings                   []string                   `json:"warnings,omitempty"`
}

const resolvedTrackPlanSnapshotKey = "resolvedTrackPlan"

// ResolvedTrackPlanFromSnapshot reads the canonical decision frozen by Queue.
// A missing plan is expected for jobs created before the track-plan migration.
func ResolvedTrackPlanFromSnapshot(snapshot models.JSONMap) (*ResolvedTrackPlan, bool) {
	raw, ok := snapshot[resolvedTrackPlanSnapshotKey]
	if !ok || raw == nil {
		return nil, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var plan ResolvedTrackPlan
	if err := json.Unmarshal(encoded, &plan); err != nil {
		return nil, false
	}
	policy, err := ParseFontAttachmentExportPolicy(string(plan.FontAttachmentExportPolicy))
	if err != nil {
		return nil, false
	}
	plan.FontAttachmentExportPolicy = policy
	if plan.FontAttachments == nil {
		plan.FontAttachments = []ResolvedFontAttachment{}
	}
	return &plan, true
}
