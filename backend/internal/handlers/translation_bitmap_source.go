package handlers

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// bitmapTranslationSource is an operation-local view of canonical bitmap OCR
// evidence. OCR identity, execution, and quality policy remain owned by the
// canonical OCR domains; translation only receives the final cue view.
type bitmapTranslationSource struct {
	EffectiveLanguage string
	Evidence          *canonicalOCREvidence
	Envelopes         []translationCueEnvelope
	ReviewTarget      *canonicalOCRReviewTarget
	ArtifactRevision  string
	LogicalTransform  string
}

func bitmapTranslationSourceFor(
	ctx context.Context,
	mediaPath string,
	stream FFProbeStream,
	requestedLanguage string,
	ocrMode string,
	segmentStart float64,
	segmentDuration int,
	sourceIdentity string,
	sourceRepresentation string,
	logicalTransformKey string,
	onProgress OCRProgressFunc,
) (*bitmapTranslationSource, error) {
	if ctx == nil {
		return nil, fmt.Errorf("bitmap translation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	codec := strings.ToLower(strings.TrimSpace(stream.CodecName))
	switch codec {
	case "hdmv_pgs_subtitle", "pgssub", "dvd_subtitle":
	default:
		return nil, fmt.Errorf("bitmap translation does not support subtitle codec %q", stream.CodecName)
	}
	request := canonicalOCRRequestFor(mediaPath, stream, requestedLanguage, ocrMode, segmentStart, segmentDuration)
	selection, foundPublished, selectionErr := canonicalOCRTranslationSelectionForRequest(request)
	if selectionErr != nil {
		target := &canonicalOCRReviewTarget{State: "conflict", Message: "Published canonical OCR evidence is ambiguous for this request.", Identity: request.Identity}
		return nil, subtitleOCRReviewRequiredErrorWithTarget(fmt.Errorf("published canonical OCR evidence is ambiguous"), target)
	}
	var evidence *canonicalOCREvidence
	var reviewTarget *canonicalOCRReviewTarget
	artifactRevision, logicalTransform := "", ""
	if selection != nil {
		reviewTarget = selection.ReviewTarget
	}
	if foundPublished {
		evidence = selection.Evidence
		artifactRevision = selection.ArtifactRevision
		logicalTransform = selection.LogicalTransform
	}
	if foundPublished {
		if onProgress != nil {
			onProgress(1, 1)
		}
	} else {
		var err error
		evidence, err = canonicalOCREvidenceForRequest(ctx, request, onProgress)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, subtitleOCRFailedError(fmt.Errorf("canonical bitmap OCR failed: %w", err))
		}
	}
	if evidence == nil || !evidence.Complete || len(evidence.Cues) == 0 || strings.TrimSpace(evidence.EffectiveLanguage) == "" {
		return nil, subtitleOCRFailedError(fmt.Errorf("canonical bitmap OCR returned incomplete evidence"))
	}
	finalEvidence, err := correctedCanonicalOCREvidenceForTranslation(evidence)
	if err != nil {
		return nil, subtitleOCRFailedError(fmt.Errorf("canonical bitmap OCR correction failed: %w", err))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sourceIdentity) == "" {
		sourceIdentity = finalEvidence.Identity.key()
	}
	if strings.TrimSpace(sourceRepresentation) == "" {
		sourceRepresentation = codec
	}
	envelopes := translationCueEnvelopesForBitmapCues(sourceIdentity, sourceRepresentation, logicalTransformKey, finalEvidence.Cues)
	if len(envelopes) != len(finalEvidence.Cues) || len(envelopes) == 0 {
		return nil, subtitleOCRFailedError(fmt.Errorf("canonical bitmap OCR produced no usable translation envelopes"))
	}
	return &bitmapTranslationSource{
		EffectiveLanguage: finalEvidence.EffectiveLanguage,
		Evidence:          finalEvidence,
		Envelopes:         envelopes,
		ReviewTarget:      reviewTarget,
		ArtifactRevision:  artifactRevision,
		LogicalTransform:  logicalTransform,
	}, nil
}

func publishedCanonicalOCREvidenceForTranslation(request canonicalOCRRequest) (*canonicalOCREvidence, bool, error) {
	selection, found, err := canonicalOCRTranslationSelectionForRequest(request)
	if err != nil || !found {
		return nil, found, err
	}
	return cloneCanonicalOCREvidence(selection.Evidence), true, nil
}

type canonicalOCRTranslationSelection struct {
	Evidence         *canonicalOCREvidence
	ReviewTarget     *canonicalOCRReviewTarget
	ArtifactRevision string
	LogicalTransform string
	Artifacts        []canonicalOCRArtifactDescriptor
}

func canonicalOCRTranslationSelectionForRequest(request canonicalOCRRequest) (*canonicalOCRTranslationSelection, bool, error) {
	artifacts := findPublishedCanonicalOCRArtifactsForEmbeddedSource(request.MediaPath, request.Stream)
	compatible := make([]canonicalOCRArtifactDescriptor, 0, len(artifacts))
	for _, artifact := range artifacts {
		snapshot := artifact.Snapshot
		if snapshot == nil {
			continue
		}
		evidence := &snapshot.Evidence
		if !canonicalOCREvidenceMatchesTranslationRequest(evidence, request) || !evidence.Complete || len(evidence.Cues) == 0 ||
			strings.TrimSpace(evidence.EffectiveLanguage) == "" {
			continue
		}
		compatible = append(compatible, artifact)
	}
	if len(compatible) == 0 {
		if staleTarget := canonicalOCRReviewTargetForStaleRequest(request); staleTarget != nil {
			return &canonicalOCRTranslationSelection{ReviewTarget: staleTarget}, false, nil
		}
		return nil, false, nil
	}
	logicalTransform := strings.TrimSpace(compatible[0].LogicalTransform)
	for i := 1; i < len(compatible); i++ {
		if strings.TrimSpace(compatible[i].LogicalTransform) != logicalTransform ||
			!canonicalOCRTranslationEvidenceEquivalent(&compatible[0].Snapshot.Evidence, &compatible[i].Snapshot.Evidence) {
			return nil, false, fmt.Errorf("multiple compatible canonical OCR artifacts have conflicting transforms or evidence")
		}
	}
	sort.SliceStable(compatible, func(i, j int) bool {
		left, right := compatible[i], compatible[j]
		if left.Format != right.Format {
			return left.Format == "srt"
		}
		return left.Path < right.Path
	})
	formats := make(map[string]bool, len(compatible))
	for _, artifact := range compatible {
		if formats[artifact.Format] {
			return nil, false, fmt.Errorf("multiple canonical OCR artifacts claim the same logical transform and format")
		}
		formats[artifact.Format] = true
	}
	revisions := make([]publishedSubtitleRevision, 0, len(compatible))
	for _, artifact := range compatible {
		revisions = append(revisions, artifact.Snapshot.Revision)
	}
	format := compatible[0].Format
	if len(compatible) > 1 {
		format = "both"
	}
	target := &canonicalOCRReviewTarget{
		State: "exact", Identity: compatible[0].Snapshot.Evidence.Identity, LogicalTransformKey: logicalTransform,
		Format: format, Artifacts: make([]canonicalOCRReviewArtifact, 0, len(compatible)),
		SidecarPaths: make([]string, 0, len(compatible)), Revisions: revisions,
		BaselineRevision: subtitleRevisionSetKey(revisions),
	}
	for _, artifact := range compatible {
		target.Artifacts = append(target.Artifacts, canonicalOCRReviewArtifact{
			OwnerIdentity: artifact.Identity, Path: artifact.Path, Format: artifact.Format,
			LogicalTransform: artifact.LogicalTransform, Revision: artifact.Snapshot.Revision,
		})
		target.SidecarPaths = append(target.SidecarPaths, artifact.Path)
	}
	return &canonicalOCRTranslationSelection{
		Evidence:     cloneCanonicalOCREvidence(&compatible[0].Snapshot.Evidence),
		ReviewTarget: target, ArtifactRevision: target.BaselineRevision,
		LogicalTransform: logicalTransform, Artifacts: compatible,
	}, true, nil
}

func canonicalOCREvidenceMatchesTranslationRequest(evidence *canonicalOCREvidence, request canonicalOCRRequest) bool {
	if evidence == nil {
		return false
	}
	identity := evidence.Identity
	requested := request.Identity
	return identity.Version == requested.Version &&
		identity.SourcePath == requested.SourcePath &&
		identity.SourceSize == requested.SourceSize &&
		identity.SourceModTimeUnixNano == requested.SourceModTimeUnixNano &&
		identity.SourceSnapshotVersion == requested.SourceSnapshotVersion &&
		identity.StreamIndex == requested.StreamIndex &&
		identity.StreamID == requested.StreamID &&
		identity.StreamCodec == requested.StreamCodec &&
		identity.RequestedLanguage == requested.RequestedLanguage &&
		identity.OCRMode == requested.OCRMode &&
		identity.PreparationScope == requested.PreparationScope &&
		identity.SegmentStart == requested.SegmentStart &&
		identity.SegmentDuration == requested.SegmentDuration &&
		identity.PreparedStreamMapping == requested.PreparedStreamMapping &&
		identity.PreparedRepresentation == requested.PreparedRepresentation &&
		identity.PipelineProvenance == requested.PipelineProvenance &&
		evidence.RequestedLanguage == request.Language && evidence.OCRMode == request.Mode
}

func canonicalOCRTranslationEvidenceEquivalent(left, right *canonicalOCREvidence) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Identity.key() == right.Identity.key() &&
		left.EffectiveLanguage == right.EffectiveLanguage &&
		left.RequestedLanguage == right.RequestedLanguage &&
		left.OCRMode == right.OCRMode &&
		reflect.DeepEqual(left.Cues, right.Cues) &&
		reflect.DeepEqual(left.CorrectionProvenance, right.CorrectionProvenance)
}
