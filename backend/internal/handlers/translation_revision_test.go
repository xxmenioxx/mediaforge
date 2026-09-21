package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTranslationExternalSourceRevisionUsesExistingBoundedRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.srt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	revision, err := captureTranslationExternalSourceRevision(path, "translation:source:srt")
	if err != nil {
		t.Fatal(err)
	}
	if revision.External == nil || revision.External.ArtifactPath != filepath.Clean(path) || revision.External.ArtifactIdentity != "path:"+filepath.Clean(path) || revision.External.ContentHash == "" {
		t.Fatalf("unexpected external revision: %#v", revision)
	}
	if err := recheckTranslationExternalSourceRevision(revision); err != nil {
		t.Fatalf("unchanged external source rejected: %v", err)
	}
}

func TestTranslationExternalSourceRevisionRejectsContentAndPathPreservingChanges(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, path string, revision translationSourceRevision)
	}{
		{
			name: "content edit same size",
			mutate: func(t *testing.T, path string, revision translationSourceRevision) {
				if err := os.WriteFile(path, []byte("world"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "same path replacement",
			mutate: func(t *testing.T, path string, revision translationSourceRevision) {
				replacement := filepath.Join(filepath.Dir(path), "replacement.srt")
				if err := os.WriteFile(replacement, []byte("world"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "mtime mismatch",
			mutate: func(t *testing.T, path string, revision translationSourceRevision) {
				modified := time.Unix(0, revision.External.ModifiedAtUnixNano+int64(time.Second))
				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "missing source",
			mutate: func(t *testing.T, path string, revision translationSourceRevision) {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.srt")
			if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
				t.Fatal(err)
			}
			revision, err := captureTranslationExternalSourceRevision(path, "translation:source:srt")
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, path, revision)
			err = recheckTranslationExternalSourceRevision(revision)
			if !errors.Is(err, errTranslationSourceStale) {
				t.Fatalf("error=%v, want stale-source error", err)
			}
		})
	}
}

func TestTranslationEmbeddedSourceRevisionBindsSourceAndExactStream(t *testing.T) {
	base := translationEmbeddedSourceSnapshot{
		SourcePath:            "/media/title.mkv",
		SourceSize:            1000,
		SourceModTimeUnixNano: 2000,
		SourceSnapshotVersion: "stat-v1",
		StreamIndex:           4,
		StreamID:              "0x7",
		StreamSignature:       "signature-a",
		StreamCodec:           "hdmv_pgs_subtitle",
		SourceRepresentation:  "pgs",
	}
	revision, err := captureTranslationEmbeddedSourceRevision(base)
	if err != nil {
		t.Fatal(err)
	}
	if revision.Embedded == nil || revision.Embedded.StreamIndex != 4 || revision.Embedded.StreamID != "0x7" || revision.Embedded.StreamSignature != "signature-a" {
		t.Fatalf("unexpected embedded revision: %#v", revision)
	}
	if revision.key() == "" {
		t.Fatal("valid embedded revision has no key")
	}
	encoded, err := json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("contentHash")) {
		t.Fatal("embedded revision unexpectedly contains a full-media content hash")
	}
	if err := recheckTranslationEmbeddedSourceRevision(revision, base); err != nil {
		t.Fatalf("unchanged embedded source rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*translationEmbeddedSourceSnapshot)
	}{
		{name: "source size", mutate: func(snapshot *translationEmbeddedSourceSnapshot) { snapshot.SourceSize++ }},
		{name: "source mtime", mutate: func(snapshot *translationEmbeddedSourceSnapshot) { snapshot.SourceModTimeUnixNano++ }},
		{name: "snapshot version", mutate: func(snapshot *translationEmbeddedSourceSnapshot) { snapshot.SourceSnapshotVersion = "stat-v2" }},
		{name: "stream index", mutate: func(snapshot *translationEmbeddedSourceSnapshot) { snapshot.StreamIndex = 5 }},
		{name: "stream ID", mutate: func(snapshot *translationEmbeddedSourceSnapshot) { snapshot.StreamID = "0x8" }},
		{name: "stream signature", mutate: func(snapshot *translationEmbeddedSourceSnapshot) { snapshot.StreamSignature = "signature-b" }},
		{name: "codec", mutate: func(snapshot *translationEmbeddedSourceSnapshot) { snapshot.StreamCodec = "dvd_subtitle" }},
		{name: "representation", mutate: func(snapshot *translationEmbeddedSourceSnapshot) { snapshot.SourceRepresentation = "vobsub" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			current := base
			test.mutate(&current)
			if err := recheckTranslationEmbeddedSourceRevision(revision, current); !errors.Is(err, errTranslationSourceStale) {
				t.Fatalf("error=%v, want stale-source error", err)
			}
		})
	}
}

func TestTranslationSourceRevisionChangesTranslationIdentity(t *testing.T) {
	base := translationEmbeddedSourceSnapshot{
		SourcePath:            "/media/title.mkv",
		SourceSize:            1000,
		SourceModTimeUnixNano: 2000,
		SourceSnapshotVersion: "stat-v1",
		StreamIndex:           4,
		StreamID:              "0x7",
		StreamSignature:       "signature-a",
		StreamCodec:           "hdmv_pgs_subtitle",
		SourceRepresentation:  "pgs",
	}
	firstRevision, err := captureTranslationEmbeddedSourceRevision(base)
	if err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.StreamSignature = "signature-b"
	secondRevision, err := captureTranslationEmbeddedSourceRevision(changed)
	if err != nil {
		t.Fatal(err)
	}

	firstInput := translationIdentityTestInput()
	firstInput.SourceRevision = firstRevision.key()
	secondInput := translationIdentityTestInput()
	secondInput.SourceRevision = secondRevision.key()
	firstIdentity, err := newTranslationIdentity(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	secondIdentity, err := newTranslationIdentity(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if firstIdentity.key() == secondIdentity.key() {
		t.Fatal("changed embedded source revision reused translation identity")
	}
}
