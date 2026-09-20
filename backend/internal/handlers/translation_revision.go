package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	translationSourceRevisionVersion  = 1
	translationSourceRevisionExternal = "external-subtitle"
	translationSourceRevisionEmbedded = "embedded-subtitle"
)

var errTranslationSourceStale = errors.New("translation source is stale")

// translationEmbeddedSourceSnapshot is the bounded source/stream snapshot
// supplied by the existing media probe/source inventory path. It contains no
// full-media content hash.
type translationEmbeddedSourceSnapshot struct {
	SourcePath            string `json:"sourcePath"`
	SourceSize            int64  `json:"sourceSize"`
	SourceModTimeUnixNano int64  `json:"sourceModTimeUnixNano"`
	SourceSnapshotVersion string `json:"sourceSnapshotVersion"`
	StreamIndex           int    `json:"streamIndex"`
	StreamID              string `json:"streamId"`
	StreamSignature       string `json:"streamSignature"`
	StreamCodec           string `json:"streamCodec"`
	SourceRepresentation  string `json:"sourceRepresentation"`
}

// translationSourceRevision is a tagged union. Exactly one bounded source
// representation is populated for a translation operation.
type translationSourceRevision struct {
	Version  int                                `json:"version"`
	Kind     string                             `json:"kind"`
	External *publishedSubtitleRevision         `json:"external,omitempty"`
	Embedded *translationEmbeddedSourceSnapshot `json:"embedded,omitempty"`
}

func captureTranslationExternalSourceRevision(path, logicalTransform string) (translationSourceRevision, error) {
	revision, _, err := publishedSubtitleRevisionFor(path, logicalTransform)
	if err != nil {
		return translationSourceRevision{}, err
	}
	result := translationSourceRevision{
		Version:  translationSourceRevisionVersion,
		Kind:     translationSourceRevisionExternal,
		External: &revision,
	}
	if err := validateTranslationSourceRevision(result); err != nil {
		return translationSourceRevision{}, err
	}
	return result, nil
}

func captureTranslationEmbeddedSourceRevision(snapshot translationEmbeddedSourceSnapshot) (translationSourceRevision, error) {
	snapshot.SourcePath = cleanTranslationSourcePath(snapshot.SourcePath)
	snapshot.SourceRepresentation = normalizeTranslationRepresentation(snapshot.SourceRepresentation)
	snapshot.StreamID = strings.TrimSpace(snapshot.StreamID)
	snapshot.StreamSignature = strings.TrimSpace(snapshot.StreamSignature)
	snapshot.StreamCodec = strings.ToLower(strings.TrimSpace(snapshot.StreamCodec))
	snapshot.SourceSnapshotVersion = strings.TrimSpace(snapshot.SourceSnapshotVersion)
	result := translationSourceRevision{
		Version:  translationSourceRevisionVersion,
		Kind:     translationSourceRevisionEmbedded,
		Embedded: &snapshot,
	}
	if err := validateTranslationSourceRevision(result); err != nil {
		return translationSourceRevision{}, err
	}
	return result, nil
}

// recheckTranslationExternalSourceRevision is the final pre-publication
// guard for an external subtitle. It deliberately reuses the existing full
// bounded subtitle revision helper instead of a second implementation.
func recheckTranslationExternalSourceRevision(expected translationSourceRevision) error {
	if err := validateTranslationSourceRevision(expected); err != nil || expected.Kind != translationSourceRevisionExternal {
		return staleTranslationSourceError("invalid external source revision")
	}
	if !publishedSubtitleRevisionMatches(*expected.External) {
		return staleTranslationSourceError("external subtitle revision changed")
	}
	return nil
}

// recheckTranslationEmbeddedSourceRevision compares the current bounded
// source/stream snapshot with the captured revision immediately before a
// later publication boundary.
func recheckTranslationEmbeddedSourceRevision(expected translationSourceRevision, current translationEmbeddedSourceSnapshot) error {
	actual, err := captureTranslationEmbeddedSourceRevision(current)
	if err != nil {
		return staleTranslationSourceError("current embedded subtitle snapshot is unavailable: %v", err)
	}
	if err := validateTranslationSourceRevision(expected); err != nil || expected.Kind != translationSourceRevisionEmbedded || expected.key() != actual.key() {
		return staleTranslationSourceError("embedded subtitle source or stream changed")
	}
	return nil
}

func (revision translationSourceRevision) key() string {
	if validateTranslationSourceRevision(revision) != nil {
		return ""
	}
	encoded, _ := json.Marshal(revision)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func validateTranslationSourceRevision(revision translationSourceRevision) error {
	if revision.Version != translationSourceRevisionVersion {
		return fmt.Errorf("unsupported translation source revision version")
	}
	switch revision.Kind {
	case translationSourceRevisionExternal:
		if revision.External == nil || revision.Embedded != nil {
			return fmt.Errorf("external translation source revision is incomplete")
		}
		if cleanTranslationSourcePath(revision.External.ArtifactPath) == "." || revision.External.ArtifactPath == "" || revision.External.SizeBytes < 0 || revision.External.ModifiedAtUnixNano < 0 || strings.TrimSpace(revision.External.ContentHash) == "" {
			return fmt.Errorf("external translation source revision is invalid")
		}
	case translationSourceRevisionEmbedded:
		if revision.Embedded == nil || revision.External != nil {
			return fmt.Errorf("embedded translation source revision is incomplete")
		}
		if cleanTranslationSourcePath(revision.Embedded.SourcePath) == "." || revision.Embedded.SourcePath == "" || revision.Embedded.SourceSize < 0 || revision.Embedded.SourceModTimeUnixNano < 0 || strings.TrimSpace(revision.Embedded.SourceSnapshotVersion) == "" || revision.Embedded.StreamIndex < 0 || strings.TrimSpace(revision.Embedded.StreamID) == "" || strings.TrimSpace(revision.Embedded.StreamSignature) == "" || strings.TrimSpace(revision.Embedded.StreamCodec) == "" || strings.TrimSpace(revision.Embedded.SourceRepresentation) == "" {
			return fmt.Errorf("embedded translation source revision is invalid")
		}
	default:
		return fmt.Errorf("unsupported translation source revision kind %q", revision.Kind)
	}
	return nil
}

func cleanTranslationSourcePath(path string) string {
	return filepath.Clean(strings.TrimSpace(path))
}

func staleTranslationSourceError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errTranslationSourceStale, fmt.Sprintf(format, args...))
}
