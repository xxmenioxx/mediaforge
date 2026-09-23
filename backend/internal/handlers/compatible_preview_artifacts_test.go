package handlers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resetCompatiblePreviewArtifactsForTest() {
	compatiblePreviewArtifacts.Lock()
	for _, entry := range compatiblePreviewArtifacts.items {
		if entry.cancel != nil {
			entry.cancel()
		}
	}
	compatiblePreviewArtifacts.items = map[string]*compatiblePreviewArtifact{}
	compatiblePreviewArtifacts.Unlock()
}

func testCompatiblePreviewDescriptor(t *testing.T, artifactID string) *compatiblePreviewDescriptor {
	t.Helper()
	return &compatiblePreviewDescriptor{
		Path:                     filepath.Join(t.TempDir(), "source.mkv"),
		Start:                    "00:00:00",
		Seconds:                  2,
		PreviewMode:              "quality",
		Args:                     []string{"-hide_banner", "-loglevel", "verbose", "-c:v", "libx264", "-f", "mp4", "pipe:1"},
		ArtifactID:               artifactID,
		RetentionPolicy:          compatiblePreviewPersistentRetention,
		CanonicalEncodeSignature: artifactID,
		CachePath:                filepath.Join(t.TempDir(), artifactID+".mp4"),
	}
}

func installFakeFFmpeg(t *testing.T, script string) {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "ffmpeg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCompatiblePreviewArtifactIdentitySeparatesConsumerDiagnostics(t *testing.T) {
	base := &compatiblePreviewDescriptor{
		Path:           "/media/raw/movie.mkv",
		SourceRevision: compatiblePreviewSourceRevision{Size: 10, ModifiedAtNS: 20, Mode: 0o600},
		Start:          "00:00:05",
		Seconds:        20,
		PreviewMode:    "quality",
		Args:           []string{"-loglevel", "error", "-c:v", "hevc_qsv", "-global_quality", "22"},
	}
	verbose := *base
	verbose.Args = []string{"-loglevel", "verbose", "-c:v", "hevc_qsv", "-global_quality", "22"}
	if compatiblePreviewEncodeSignature(base) != compatiblePreviewEncodeSignature(&verbose) {
		t.Fatal("consumer diagnostic verbosity changed canonical encode signature")
	}
	if compatiblePreviewArtifactID(compatiblePreviewEncodeSignature(base), compatiblePreviewPersistentRetention, "") == "" {
		t.Fatal("canonical artifact identity must not be empty")
	}
	changed := *base
	changed.Args = append([]string(nil), base.Args...)
	changed.Args[len(changed.Args)-1] = "23"
	if compatiblePreviewEncodeSignature(base) == compatiblePreviewEncodeSignature(&changed) {
		t.Fatal("byte-affecting codec input did not change canonical encode signature")
	}
	requestID, err := compatiblePreviewRequestIdentity(compatiblePreviewRequest{Path: base.Path})
	if err != nil {
		t.Fatal(err)
	}
	artifactID := compatiblePreviewArtifactID(compatiblePreviewEncodeSignature(base), compatiblePreviewPersistentRetention, "")
	if requestID == artifactID {
		t.Fatal("request identity must remain distinct from artifact identity")
	}
}

func TestCompatiblePreviewEphemeralRequestIdentityIsolated(t *testing.T) {
	input := compatiblePreviewRequest{Path: "/media/raw/movie.mkv", Ephemeral: true}
	first, _, err := storeCompatiblePreviewRequest(input, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := storeCompatiblePreviewRequest(input, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("independent ephemeral requests must not share request identity")
	}
}

func TestCompatiblePreviewSourceRevisionInvalidatesCanonicalIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.mkv")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstRevision, err := sourceRevisionForPath(path)
	if err != nil {
		t.Fatal(err)
	}
	first := &compatiblePreviewDescriptor{Path: path, SourceRevision: firstRevision, Start: "0", Seconds: 2, Args: []string{"-c:v", "libx264"}}
	if err := os.WriteFile(path, []byte("source revision changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if firstRevision.matches(path) {
		t.Fatal("source revision check reused a changed source")
	}
	secondRevision, err := sourceRevisionForPath(path)
	if err != nil {
		t.Fatal(err)
	}
	second := *first
	second.SourceRevision = secondRevision
	if compatiblePreviewEncodeSignature(first) == compatiblePreviewEncodeSignature(&second) {
		t.Fatal("source revision change did not invalidate canonical artifact identity")
	}
}

func TestCompatiblePreviewArtifactSingleFlightAndIndependentCancellation(t *testing.T) {
	resetCompatiblePreviewArtifactsForTest()
	t.Cleanup(resetCompatiblePreviewArtifactsForTest)
	counter := filepath.Join(t.TempDir(), "ffmpeg-count")
	t.Setenv("FFMPEG_COUNTER", counter)
	installFakeFFmpeg(t, `echo encode >> "$FFMPEG_COUNTER"; sleep 0.2; printf artifact`)
	descriptor := testCompatiblePreviewDescriptor(t, "single-flight")

	firstContext, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, lease, err := acquireCompatiblePreviewArtifact(firstContext, descriptor)
		if lease != nil {
			lease.release()
		}
		firstResult <- err
	}()
	time.Sleep(35 * time.Millisecond)
	secondResult := make(chan struct {
		hit   bool
		lease *compatiblePreviewArtifactLease
		err   error
	}, 1)
	go func() {
		hit, lease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
		secondResult <- struct {
			hit   bool
			lease *compatiblePreviewArtifactLease
			err   error
		}{hit, lease, err}
	}()
	cancelFirst()
	if err := <-firstResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("first canceled waiter error=%v, want context cancellation", err)
	}
	second := <-secondResult
	if second.err != nil || second.lease == nil {
		t.Fatalf("remaining waiter did not receive shared artifact: hit=%t err=%v", second.hit, second.err)
	}
	second.lease.release()
	if data, err := os.ReadFile(counter); err != nil || strings.Count(string(data), "encode") != 1 {
		t.Fatalf("shared generation count=%q err=%v", string(data), err)
	}
	thirdHit, thirdLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || !thirdHit || thirdLease == nil {
		t.Fatalf("ready artifact was not reused: hit=%t err=%v", thirdHit, err)
	}
	thirdLease.release()
}

func TestCompatiblePreviewEphemeralArtifactReusesWithinRequestScope(t *testing.T) {
	resetCompatiblePreviewArtifactsForTest()
	t.Cleanup(resetCompatiblePreviewArtifactsForTest)
	counter := filepath.Join(t.TempDir(), "ffmpeg-count")
	t.Setenv("FFMPEG_COUNTER", counter)
	installFakeFFmpeg(t, `echo encode >> "$FFMPEG_COUNTER"; printf ephemeral`)
	descriptor := testCompatiblePreviewDescriptor(t, "ephemeral-scope")
	descriptor.RetentionPolicy = compatiblePreviewEphemeralRetention
	firstHit, firstLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || firstHit || firstLease == nil {
		t.Fatalf("first ephemeral acquisition did not generate: hit=%t err=%v", firstHit, err)
	}
	firstLease.release()
	secondHit, secondLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || !secondHit || secondLease == nil {
		t.Fatalf("sequential ephemeral consumer did not reuse artifact: hit=%t err=%v", secondHit, err)
	}
	secondLease.release()
	if data, err := os.ReadFile(counter); err != nil || strings.Count(string(data), "encode") != 1 {
		t.Fatalf("ephemeral artifact was encoded more than once: %q err=%v", string(data), err)
	}
	if compatiblePreviewArtifactID("same", compatiblePreviewEphemeralRetention, "request-a") == compatiblePreviewArtifactID("same", compatiblePreviewEphemeralRetention, "request-b") {
		t.Fatal("independent ephemeral request scopes collided")
	}
}

func TestCompatiblePreviewZeroConsumerCancellationAllowsRetry(t *testing.T) {
	resetCompatiblePreviewArtifactsForTest()
	t.Cleanup(resetCompatiblePreviewArtifactsForTest)
	installFakeFFmpeg(t, `sleep 0.4; printf retryable`)
	descriptor := testCompatiblePreviewDescriptor(t, "cancel-retry")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, lease, err := acquireCompatiblePreviewArtifact(ctx, descriptor)
		if lease != nil {
			lease.release()
		}
		result <- err
	}()
	time.Sleep(25 * time.Millisecond)
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("detached consumer error=%v, want cancellation", err)
	}
	time.Sleep(250 * time.Millisecond)
	if _, err := os.Stat(descriptor.CachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled generation published artifact: err=%v", err)
	}
	hit, lease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || hit || lease == nil {
		t.Fatalf("retry did not start a fresh generation: hit=%t err=%v", hit, err)
	}
	lease.release()
	if _, err := os.Stat(descriptor.CachePath); err != nil {
		t.Fatalf("retry artifact missing: %v", err)
	}
}

func TestCompatiblePreviewFailureDoesNotPublishPartialOrPoisonRetry(t *testing.T) {
	resetCompatiblePreviewArtifactsForTest()
	t.Cleanup(resetCompatiblePreviewArtifactsForTest)
	t.Setenv("FFMPEG_FAIL", "1")
	installFakeFFmpeg(t, `if [ "$FFMPEG_FAIL" = "1" ]; then dd if=/dev/zero bs=300000 count=1 >&2 2>/dev/null; exit 1; fi; printf recovered`)
	descriptor := testCompatiblePreviewDescriptor(t, "failure-retry")
	_, lease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if lease != nil {
		lease.release()
	}
	if err == nil || len(err.Error()) > compatiblePreviewDiagnosticLimit+32 {
		t.Fatalf("real FFmpeg failure was not preserved: %v", err)
	}
	if _, statErr := os.Stat(descriptor.CachePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed generation left a published artifact: %v", statErr)
	}
	entries, readErr := os.ReadDir(filepath.Dir(descriptor.CachePath))
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".partial") {
			t.Fatalf("failed generation left staging file %q", entry.Name())
		}
	}
	t.Setenv("FFMPEG_FAIL", "0")
	hit, lease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || hit || lease == nil {
		t.Fatalf("retry after failure did not generate: hit=%t err=%v", hit, err)
	}
	lease.release()
}

func TestCompatiblePreviewPreparationUsesPerEntrySynchronization(t *testing.T) {
	handler := AssetHandler{}
	first := &compatiblePreviewRequestEntry{Request: compatiblePreviewRequest{Path: "/tmp/first.mkv"}}
	second := &compatiblePreviewRequestEntry{Request: compatiblePreviewRequest{Path: "/tmp/second.mkv"}}
	first.prepareMu.Lock()
	firstDone := make(chan struct{})
	go func() {
		_, _ = handler.compatiblePreviewDescriptorForEntry(first, "first")
		close(firstDone)
	}()
	secondDone := make(chan struct{})
	go func() {
		_, _ = handler.compatiblePreviewDescriptorForEntry(second, "second")
		close(secondDone)
	}()
	select {
	case <-secondDone:
	case <-time.After(200 * time.Millisecond):
		first.prepareMu.Unlock()
		t.Fatal("unrelated preview preparation was blocked by another entry")
	}
	first.prepareMu.Unlock()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("blocked preview preparation did not resume")
	}
}
