package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCanonicalOCRIdentityIsSerializerNeutralAndPreparationScoped(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 4, ID: "0x7", CodecName: "dvd_subtitle", Tags: map[string]string{"language": "spa"}}
	full := canonicalOCRRequestFor(mediaPath, stream, "spa", "clean", 0, 0)
	ass := canonicalOCRRequestFor(mediaPath, stream, "spa", "clean", 0, 0)
	segmented := canonicalOCRRequestFor(mediaPath, stream, "spa", "clean", 42.5, 20)
	otherSegment := canonicalOCRRequestFor(mediaPath, stream, "spa", "clean", 43, 20)
	if full.Identity.key() != ass.Identity.key() {
		t.Fatal("serializer format must not participate in canonical OCR identity")
	}
	if full.Identity.key() == segmented.Identity.key() || segmented.Identity.key() == otherSegment.Identity.key() {
		t.Fatal("prepared-input scope and timing must distinguish segmented OCR evidence")
	}
	if full.Identity.PreparedRepresentationHash != "" {
		t.Fatal("normal full-media identity must not hash the whole media file")
	}
	if segmented.Identity.PreparationScope != "segmented-test-encode" || segmented.Identity.SegmentDuration != 20 {
		t.Fatalf("segmented preparation identity=%#v", segmented.Identity)
	}
}

func TestCanonicalOCRPreparedFingerprintValidationDoesNotRunOCROnCacheValidation(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("stable source"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
output=""
for argument do output="$argument"; done
case "$output" in
  *.sup) printf '%s' "$PREPARED_OCR_CONTENT" > "$output"; printf 'ffmpeg\n' >> "$MVFORGE_PREPARED_LOG" ;;
esac
`)
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
output=""
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
  esac
done
printf 'seconv\n' >> "$MVFORGE_PREPARED_LOG"
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Prepared OCR' > "$output"
`)
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng spa jpn jpn_vert; exit 0; fi\nexit 0\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "internal-cache"))
	logPath := filepath.Join(root, "prepared.log")
	t.Setenv("MVFORGE_PREPARED_LOG", logPath)
	t.Setenv("PREPARED_OCR_CONTENT", "prepared-v1")
	stream := FFProbeStream{Index: 4, CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "spa"}}
	request := canonicalOCRRequestFor(mediaPath, stream, "spa", "raw", 0, 0)
	if _, err := canonicalOCREvidenceForRequest(context.Background(), request, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := canonicalOCREvidenceForRequest(context.Background(), request, nil); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(log), "ffmpeg\n") != 2 || strings.Count(string(log), "seconv\n") != 1 {
		t.Fatalf("cache validation should prepare without OCR: %q", log)
	}
	t.Setenv("PREPARED_OCR_CONTENT", "prepared-v2")
	if _, err := canonicalOCREvidenceForRequest(context.Background(), request, nil); err != nil {
		t.Fatal(err)
	}
	log, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(log), "ffmpeg\n") != 4 || strings.Count(string(log), "seconv\n") != 2 {
		t.Fatalf("changed prepared subtitle representation reused cached OCR: %q", log)
	}
}

func TestCanonicalOCRRuntimeProvenanceIsStableAndCapabilitySensitive(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("runtime source"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "spa"}}
	binA := installCanonicalRuntimeTools(t, "seconv-a", "eng spa jpn jpn_vert")
	t.Setenv("PATH", binA+string(os.PathListSeparator)+os.Getenv("PATH"))
	request := canonicalOCRRequestFor(mediaPath, stream, "spa", "raw", 0, 0)
	withRuntimeA, err := canonicalOCRRequestWithRuntime(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	withRuntimeAAgain, err := canonicalOCRRequestWithRuntime(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if withRuntimeA.Identity.RuntimeProvenance != withRuntimeAAgain.Identity.RuntimeProvenance || withRuntimeA.Identity.key() != withRuntimeAAgain.Identity.key() {
		t.Fatal("unchanged OCR runtime provenance was not deterministic")
	}
	if !strings.Contains(withRuntimeA.Identity.RuntimeProvenance, `"jpnVert":true`) {
		t.Fatalf("Japanese orientation capability was not recorded: %s", withRuntimeA.Identity.RuntimeProvenance)
	}
	binB := installCanonicalRuntimeTools(t, "seconv-b", "eng spa jpn")
	t.Setenv("PATH", binB+string(os.PathListSeparator)+os.Getenv("PATH"))
	withRuntimeB, err := canonicalOCRRequestWithRuntime(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if withRuntimeA.Identity.key() == withRuntimeB.Identity.key() || withRuntimeA.Identity.RuntimeProvenance == withRuntimeB.Identity.RuntimeProvenance {
		t.Fatal("runtime/tool or OCR capability change did not invalidate identity")
	}
	if !strings.Contains(withRuntimeB.Identity.RuntimeProvenance, `"jpnVert":false`) {
		t.Fatalf("changed Japanese orientation capability was not recorded: %s", withRuntimeB.Identity.RuntimeProvenance)
	}
}

func TestCanonicalOCREquivalentSRTAndASSShareEvidenceAndProgress(t *testing.T) {
	root := t.TempDir()
	installCanonicalOCRTestTools(t, `#!/bin/sh
output=""
kind=ocr
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
    --fix-common-errors-rules:*) kind=clean ;;
    assa) printf '%s\n' 'format-assa' >> "$MVFORGE_OCR_LIFECYCLE_LOG" ;;
  esac
done
printf '%s\n' 'Running tesseract OCR on 3 VobSub image(s)...' >&2
printf '%s\n' "$kind" >> "$MVFORGE_OCR_LIFECYCLE_LOG"
sleep 0.2
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Shared canonical OCR' > "$output"
`)
	logPath := filepath.Join(root, "ocr.log")
	t.Setenv("MVFORGE_OCR_LIFECYCLE_LOG", logPath)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "internal-cache"))
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "spa"}}
	var progressMu sync.Mutex
	progress := map[string][][2]int{"srt": nil, "ass": nil}
	var wait sync.WaitGroup
	wait.Add(2)
	results := make(chan error, 2)
	go func() {
		defer wait.Done()
		_, err := generateBitmapSubtitleAtPathWithResult(context.Background(), mediaPath, stream, "srt", "spa", "raw", filepath.Join(root, "movie.srt"), 0, 0, func(processed, total int) {
			progressMu.Lock()
			progress["srt"] = append(progress["srt"], [2]int{processed, total})
			progressMu.Unlock()
		})
		results <- err
	}()
	go func() {
		defer wait.Done()
		_, err := generateBitmapSubtitleAtPathWithResult(context.Background(), mediaPath, stream, "ass", "spa", "raw", filepath.Join(root, "movie.ass"), 0, 0, func(processed, total int) {
			progressMu.Lock()
			progress["ass"] = append(progress["ass"], [2]int{processed, total})
			progressMu.Unlock()
		})
		results <- err
	}()
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	srt, err := os.ReadFile(filepath.Join(root, "movie.srt"))
	if err != nil || !strings.Contains(string(srt), "Shared canonical OCR") {
		t.Fatalf("SRT=%q err=%v", srt, err)
	}
	ass, err := os.ReadFile(filepath.Join(root, "movie.ass"))
	if err != nil || !strings.Contains(string(ass), "Shared canonical OCR") {
		t.Fatalf("ASS=%q err=%v", ass, err)
	}
	lines, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(lines), "ocr\n"); got != 1 || strings.Contains(string(lines), "format-assa") {
		t.Fatalf("equivalent SRT/ASS requests ran OCR %d times: %q", got, lines)
	}
	progressMu.Lock()
	for format, updates := range progress {
		if len(updates) == 0 || updates[0][1] != 3 {
			t.Fatalf("%s subscriber lost shared OCR progress: %#v", format, updates)
		}
	}
	progressMu.Unlock()

	request := canonicalOCRRequestFor(mediaPath, stream, "spa", "raw", 0, 0)
	evidence, err := canonicalOCREvidenceForRequest(context.Background(), request, nil)
	if err != nil || evidence == nil || !strings.Contains(evidence.Message, "cache hit") {
		t.Fatalf("expected reusable canonical evidence cache hit: evidence=%#v err=%v", evidence, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "internal-cache"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".mvforge-ocr-flight-") {
			t.Fatalf("flight workspace survived terminal completion: %s", entry.Name())
		}
	}
}

func TestCanonicalOCRCacheInvalidatesOnSourceProvenanceChange(t *testing.T) {
	root := t.TempDir()
	installCanonicalOCRTestTools(t, `#!/bin/sh
output=""
for argument do case "$argument" in --output-filename:*) output="${argument#--output-filename:}" ;; esac; done
printf 'ocr\n' >> "$MVFORGE_OCR_LIFECYCLE_LOG"
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Provenance OCR' > "$output"
`)
	logPath := filepath.Join(root, "ocr.log")
	t.Setenv("MVFORGE_OCR_LIFECYCLE_LOG", logPath)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "internal-cache"))
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "spa"}}
	request := canonicalOCRRequestFor(mediaPath, stream, "spa", "raw", 0, 0)
	if _, err := canonicalOCREvidenceForRequest(context.Background(), request, nil); err != nil {
		t.Fatal(err)
	}
	cacheEntries, err := filepath.Glob(filepath.Join(root, "internal-cache", "evidence-*.json"))
	if err != nil || len(cacheEntries) != 1 {
		t.Fatalf("expected one cache entry after first OCR: %v %#v", err, cacheEntries)
	}
	if err := os.WriteFile(cacheEntries[0], []byte("partial cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := canonicalOCREvidenceForRequest(context.Background(), request, nil); err != nil {
		t.Fatal(err)
	}
	cacheEntries, err = filepath.Glob(filepath.Join(root, "internal-cache", "evidence-*.json"))
	if err != nil || len(cacheEntries) != 1 {
		t.Fatalf("expected cache entry after integrity miss: %v %#v", err, cacheEntries)
	}
	if err := os.Chtimes(cacheEntries[0], time.Now().Add(-canonicalOCRCacheMaxAge-time.Hour), time.Now().Add(-canonicalOCRCacheMaxAge-time.Hour)); err != nil {
		t.Fatal(err)
	}
	pruneCanonicalOCREvidenceCache(filepath.Join(root, "internal-cache"))
	if _, err := canonicalOCREvidenceForRequest(context.Background(), request, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("source changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed := canonicalOCRRequestFor(mediaPath, stream, "spa", "raw", 0, 0)
	if request.Identity.key() == changed.Identity.key() {
		t.Fatal("source snapshot change did not invalidate OCR identity")
	}
	if _, err := canonicalOCREvidenceForRequest(context.Background(), changed, nil); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(log), "ocr\n"); got != 4 {
		t.Fatalf("expected integrity, stale, and provenance misses to rerun OCR, got %d: %q", got, log)
	}
}

func TestCanonicalOCRModesPublishOnlyRequestedCanonicalOutput(t *testing.T) {
	root := t.TempDir()
	installCanonicalOCRTestTools(t, `#!/bin/sh
output=""
kind=ocr
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
    --fix-common-errors-rules:*) kind=cleanup ;;
  esac
done
printf '%s\n' "$kind" >> "$MVFORGE_OCR_LIFECYCLE_LOG"
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' "$kind canonical cue" > "$output"
`)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_LOG", filepath.Join(root, "ocr.log"))
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "internal-cache"))
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("mode media"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "spa"}}
	for _, mode := range []string{"raw", "clean", "accurate"} {
		output := filepath.Join(root, mode+".srt")
		if _, err := generateBitmapSubtitleAtPathWithResult(context.Background(), mediaPath, stream, "srt", "spa", mode, output, 0, 0, nil); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
		if _, err := os.Stat(filepath.Join(root, mode+".raw.srt")); !os.IsNotExist(err) {
			t.Fatalf("mode %s published raw intermediate: %v", mode, err)
		}
	}
	log, err := os.ReadFile(filepath.Join(root, "ocr.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(log), "cleanup\n") != 2 || strings.Count(string(log), "ocr\n") != 4 {
		t.Fatalf("unexpected raw/clean/accurate lifecycle calls: %q", log)
	}
}

func TestCanonicalOCRSubscriberCancellationKeepsFlightForRemainingSubscriber(t *testing.T) {
	root := t.TempDir()
	installCanonicalOCRTestTools(t, `#!/bin/sh
output=""
for argument do case "$argument" in --output-filename:*) output="${argument#--output-filename:}" ;; esac; done
touch "$MVFORGE_OCR_LIFECYCLE_MARKER"
printf 'ocr\n' >> "$MVFORGE_OCR_LIFECYCLE_LOG"
sleep 0.4
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Remaining subscriber' > "$output"
`)
	marker := filepath.Join(root, "started")
	t.Setenv("MVFORGE_OCR_LIFECYCLE_MARKER", marker)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_LOG", filepath.Join(root, "ocr.log"))
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "internal-cache"))
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("shared cancel media"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "spa"}}
	request := canonicalOCRRequestFor(mediaPath, stream, "spa", "raw", 0, 0)
	ctxA, cancelA := context.WithCancel(context.Background())
	resultA := make(chan error, 1)
	go func() {
		_, err := canonicalOCREvidenceForRequest(ctxA, request, nil)
		resultA <- err
	}()
	waitForFile(t, marker)
	resultB := make(chan error, 1)
	go func() {
		_, err := canonicalOCREvidenceForRequest(context.Background(), request, nil)
		resultB <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancelA()
	if err := <-resultA; err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("cancelled subscriber error=%v", err)
	}
	if err := <-resultB; err != nil {
		t.Fatalf("remaining subscriber lost shared flight: %v", err)
	}
	log, err := os.ReadFile(filepath.Join(root, "ocr.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(log), "ocr\n"); got != 1 {
		t.Fatalf("remaining subscriber caused duplicate OCR: %q", log)
	}
	entries, err := os.ReadDir(filepath.Join(root, "internal-cache"))
	if err != nil {
		t.Fatal(err)
	}
	cacheEntries := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "evidence-") {
			cacheEntries++
		}
	}
	if cacheEntries != 1 {
		t.Fatalf("successful remaining subscriber did not commit reusable evidence: %#v", entries)
	}
}

func TestCanonicalOCRLastSubscriberCancellationCommitsNoEvidenceAndRetriesFresh(t *testing.T) {
	root := t.TempDir()
	installCanonicalOCRTestTools(t, `#!/bin/sh
output=""
for argument do case "$argument" in --output-filename:*) output="${argument#--output-filename:}" ;; esac; done
touch "$MVFORGE_OCR_LIFECYCLE_MARKER"
printf 'ocr\n' >> "$MVFORGE_OCR_LIFECYCLE_LOG"
sleep 0.4
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Retry after cancellation' > "$output"
`)
	marker := filepath.Join(root, "started")
	logPath := filepath.Join(root, "ocr.log")
	t.Setenv("MVFORGE_OCR_LIFECYCLE_MARKER", marker)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_LOG", logPath)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "internal-cache"))
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("cancel media"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "spa"}}
	request := canonicalOCRRequestFor(mediaPath, stream, "spa", "raw", 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, err := canonicalOCREvidenceForRequest(ctx, request, nil)
		first <- err
	}()
	waitForFile(t, marker)
	cancel()
	if err := <-first; err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("last subscriber cancellation error=%v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "internal-cache"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "evidence-") {
			t.Fatalf("cancelled flight committed evidence: %s", entry.Name())
		}
	}
	if _, err := canonicalOCREvidenceForRequest(context.Background(), request, nil); err != nil {
		t.Fatalf("fresh retry failed after cancelled flight: %v", err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(log), "ocr\n"); got != 2 {
		t.Fatalf("retry reused cancelled OCR flight or evidence: %q", log)
	}
}

func TestCanonicalOCRFailedFlightDoesNotCommitAndRetriesFresh(t *testing.T) {
	root := t.TempDir()
	installCanonicalOCRTestTools(t, `#!/bin/sh
printf '%s\n' 'fatal OCR failure' >&2
printf 'failed\n' >> "$MVFORGE_OCR_LIFECYCLE_LOG"
exit 1
`)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_LOG", filepath.Join(root, "ocr.log"))
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "internal-cache"))
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("failed media"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "spa"}}
	request := canonicalOCRRequestFor(mediaPath, stream, "spa", "raw", 0, 0)
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := canonicalOCREvidenceForRequest(context.Background(), request, nil); err == nil || !strings.Contains(err.Error(), "fatal OCR failure") {
			t.Fatalf("attempt %d did not preserve failed-flight diagnostics: %v", attempt, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "internal-cache"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "evidence-") {
			t.Fatalf("failed flight committed evidence: %s", entry.Name())
		}
	}
	log, err := os.ReadFile(filepath.Join(root, "ocr.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(log), "failed\n"); got != 2 {
		t.Fatalf("failed retry reused evidence: %q", log)
	}
}

func installCanonicalOCRTestTools(t *testing.T, seconv string) string {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "seconv"), seconv)
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng spa jpn jpn_vert; exit 0; fi\nexit 0\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
}

func installCanonicalRuntimeTools(t *testing.T, seconvLabel, languages string) string {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "seconv"), "#!/bin/sh\nprintf '%s\\n' '"+seconvLabel+"' >&2\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' "+languages+"; exit 0; fi\nprintf '%s\\n' 'tesseract-runtime' >&2\nexit 0\n")
	return bin
}
