package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func compatiblePreviewPersistenceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "preview-persistence.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.Library{}, &models.CompatiblePreviewRequestRecord{}, &models.CompatiblePreviewArtifactRecord{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func compatiblePreviewPersistentFixture(t *testing.T, db *gorm.DB, artifactID string) (string, compatiblePreviewRequest, *compatiblePreviewDescriptor, time.Time) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "preview-cache")
	t.Setenv("MVFORGE_COMPATIBLE_PREVIEW_CACHE_ROOT", root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source.mkv")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Library{Name: "fixture", SourcePath: filepath.Dir(source), DestinationPath: filepath.Dir(source), Type: "library"}).Error; err != nil {
		t.Fatal(err)
	}
	request := compatiblePreviewRequest{Path: source, ProfileID: 7, Start: "00:00:05", Seconds: 4, Mode: "quality"}
	normalized, err := normalizeCompatiblePreviewRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := sourceRevisionForPath(source)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := &compatiblePreviewDescriptor{
		Request: normalized, Path: source, SourceRevision: revision, Start: normalized.Start, Seconds: normalized.Seconds,
		PreviewMode: normalized.Mode, Args: []string{"-i", source, "-t", "4", "-f", "mp4", "pipe:1"},
		PreviewProfile:          &models.Profile{ID: 7, Name: "frozen-profile", VideoCodec: "x265"},
		EffectivePreviewProfile: &models.Profile{ID: 7, Name: "frozen-effective", VideoCodec: "x265"},
		RequestedVideoEncoder:   "libx265", EffectiveVideoEncoder: "libx265", RequestedQSVRateControl: "icq", EffectiveQSVRateControl: "icq",
		CanonicalEncodeSignature: "signature-" + artifactID, ArtifactID: artifactID, RetentionPolicy: compatiblePreviewPersistentRetention,
		CachePath: filepath.Join(root, artifactID+".mp4"),
	}
	now := time.Now().Truncate(time.Microsecond)
	requestID, err := compatiblePreviewRequestIdentity(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistCompatiblePreviewPreparedRequest(db, requestID, normalized, descriptor, now); err != nil {
		t.Fatal(err)
	}
	return requestID, normalized, descriptor, now
}

func markCompatiblePreviewArtifact(t *testing.T, db *gorm.DB, artifactID, state, token string, version int64, cachePath string) {
	t.Helper()
	if err := db.Model(&models.CompatiblePreviewArtifactRecord{}).Where("artifact_id = ?", artifactID).Updates(map[string]any{
		"state": state, "generation_token": token, "generation_version": version, "cache_path": cachePath,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestCompatiblePreviewReadyArtifactReconcilesWithoutEncode(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	requestID, _, descriptor, _ := compatiblePreviewPersistentFixture(t, db, "artifact-ready")
	if err := os.WriteFile(descriptor.CachePath, []byte("published"), 0o644); err != nil {
		t.Fatal(err)
	}
	markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(compatiblePreviewArtifactReady), "generation-1", 1, descriptor.CachePath)
	if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
		t.Fatal(err)
	}
	entry, tombstone, ok := compatiblePreviewRequestLookup(requestID, time.Now())
	if !ok || tombstone != nil || entry == nil {
		t.Fatalf("restored request missing: ok=%t tombstone=%v entry=%v", ok, tombstone, entry)
	}
	entry.mu.RLock()
	restored := entry.Descriptor
	entry.mu.RUnlock()
	if restored == nil || restored.ArtifactID != descriptor.ArtifactID || !restored.frozen {
		t.Fatalf("frozen relationship was not restored: %#v", restored)
	}
	hit, lease, err := acquireCompatiblePreviewArtifact(context.Background(), restored)
	if err != nil || !hit || lease == nil {
		t.Fatalf("ready artifact was not reused: hit=%t lease=%v err=%v", hit, lease, err)
	}
	lease.release()
}

func TestCompatiblePreviewRegistrationReResolvesSamePathSourceReplacement(t *testing.T) {
	resetCompatiblePreviewRuntimeForReconcile()
	t.Cleanup(resetCompatiblePreviewRuntimeForReconcile)
	db := compatiblePreviewPersistenceDB(t)
	root := filepath.Join(t.TempDir(), "preview-cache")
	t.Setenv("MVFORGE_COMPATIBLE_PREVIEW_CACHE_ROOT", root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(source, []byte("revision-a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Library{Name: "fixture", SourcePath: filepath.Dir(source), DestinationPath: filepath.Dir(source), Type: "library"}).Error; err != nil {
		t.Fatal(err)
	}
	installFakeFFmpeg(t, `echo encode >> "$FFMPEG_COUNTER"; printf artifact`)
	counter := filepath.Join(t.TempDir(), "ffmpeg-count")
	t.Setenv("FFMPEG_COUNTER", counter)

	request := compatiblePreviewRequest{Path: source, Start: "00:00:00", Seconds: 2, Mode: "quality", PreviewNormalization: "normalize_bt709"}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/requests", AssetHandler{db: db}.CreateCompatiblePreviewRequest)
	create := func() struct {
		RequestID     string `json:"requestId"`
		CacheIdentity string `json:"cacheIdentity"`
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/requests", strings.NewReader(string(body))))
		if response.Code != http.StatusCreated {
			t.Fatalf("registration status=%d body=%s", response.Code, response.Body.String())
		}
		var created struct {
			RequestID     string `json:"requestId"`
			CacheIdentity string `json:"cacheIdentity"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		return created
	}

	first := create()
	entry, _, ok := compatiblePreviewRequestLookup(first.RequestID, time.Now())
	if !ok || entry == nil {
		t.Fatal("first canonical request was not registered")
	}
	entry.mu.RLock()
	firstDescriptor := entry.Descriptor
	entry.mu.RUnlock()
	if firstDescriptor == nil || firstDescriptor.ArtifactID != first.CacheIdentity {
		t.Fatalf("first descriptor/cache identity mismatch: descriptor=%#v response=%#v", firstDescriptor, first)
	}
	if _, lease, err := acquireCompatiblePreviewArtifact(context.Background(), firstDescriptor); err != nil || lease == nil {
		t.Fatalf("first reference artifact did not become ready: lease=%v err=%v", lease, err)
	} else {
		lease.release()
	}

	if err := os.WriteFile(source, []byte("revision-b-replaced-at-same-path"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := create()
	if second.RequestID != first.RequestID {
		t.Fatalf("canonical request identity changed for same logical request: first=%q second=%q", first.RequestID, second.RequestID)
	}
	if second.CacheIdentity == first.CacheIdentity {
		t.Fatalf("stale Sample A artifact was returned after source replacement: identity=%q", second.CacheIdentity)
	}
	entry, _, ok = compatiblePreviewRequestLookup(second.RequestID, time.Now())
	if !ok || entry == nil {
		t.Fatal("replacement canonical request was not registered")
	}
	entry.mu.RLock()
	secondDescriptor := entry.Descriptor
	entry.mu.RUnlock()
	if secondDescriptor == nil || secondDescriptor.ArtifactID != second.CacheIdentity || secondDescriptor.SourceRevision.matches(source) == false {
		t.Fatalf("replacement descriptor did not resolve the current source: %#v", secondDescriptor)
	}
	if encoded, err := os.ReadFile(counter); err != nil || strings.Count(string(encoded), "encode") != 1 {
		t.Fatalf("registration unexpectedly generated another reference artifact: count=%q err=%v", string(encoded), err)
	}
}

func TestCompatiblePreviewSharedArtifactRestoresOnce(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	requestA, normalized, descriptor, now := compatiblePreviewPersistentFixture(t, db, "artifact-shared")
	requestB := requestA + "-second"
	if err := persistCompatiblePreviewPreparedRequest(db, requestB, normalized, descriptor, now); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(descriptor.CachePath, []byte("published"), 0o644); err != nil {
		t.Fatal(err)
	}
	markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(compatiblePreviewArtifactReady), "generation-1", 1, descriptor.CachePath)
	if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
		t.Fatal(err)
	}
	if len(compatiblePreviewArtifacts.items) != 1 {
		t.Fatalf("restored %d runtime artifacts, want one shared artifact", len(compatiblePreviewArtifacts.items))
	}
	if _, _, ok := compatiblePreviewRequestLookup(requestA, time.Now()); !ok {
		t.Fatal("request A was not restored")
	}
	if _, _, ok := compatiblePreviewRequestLookup(requestB, time.Now()); !ok {
		t.Fatal("request B was not restored")
	}
}

func TestCompatiblePreviewExpiredRequestDoesNotDeleteSharedArtifact(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	requestA, normalized, descriptor, now := compatiblePreviewPersistentFixture(t, db, "artifact-shared-retention")
	requestB := requestA + "-active"
	if err := persistCompatiblePreviewPreparedRequest(db, requestB, normalized, descriptor, now); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(descriptor.CachePath, []byte("published"), 0o644); err != nil {
		t.Fatal(err)
	}
	markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(compatiblePreviewArtifactReady), "generation-1", 1, descriptor.CachePath)
	if err := db.Model(&models.CompatiblePreviewRequestRecord{}).Where("request_id = ?", requestA).Updates(map[string]any{"expires_at": now.Add(-time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.CompatiblePreviewArtifactRecord{}).Where("artifact_id = ?", descriptor.ArtifactID).Update("expires_at", now.Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
		t.Fatal(err)
	}
	var artifact models.CompatiblePreviewArtifactRecord
	if err := db.First(&artifact, "artifact_id = ?", descriptor.ArtifactID).Error; err != nil {
		t.Fatal(err)
	}
	if artifact.ArtifactID == "" {
		t.Fatal("shared artifact row was removed while request B remained active")
	}
}

func TestCompatiblePreviewGenerationReconciliationPromotesOrInterrupts(t *testing.T) {
	t.Run("complete final is promoted", func(t *testing.T) {
		db := compatiblePreviewPersistenceDB(t)
		requestID, _, descriptor, _ := compatiblePreviewPersistentFixture(t, db, "artifact-complete")
		if err := os.WriteFile(descriptor.CachePath, []byte("published"), 0o644); err != nil {
			t.Fatal(err)
		}
		markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(compatiblePreviewArtifactGenerating), "generation-17", 17, descriptor.CachePath)
		if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
			t.Fatal(err)
		}
		var artifact models.CompatiblePreviewArtifactRecord
		if err := db.First(&artifact, "artifact_id = ?", descriptor.ArtifactID).Error; err != nil {
			t.Fatal(err)
		}
		if artifact.State != string(compatiblePreviewArtifactReady) {
			t.Fatalf("state=%q, want ready", artifact.State)
		}
		if entry, _, ok := compatiblePreviewRequestLookup(requestID, time.Now()); !ok || entry == nil {
			t.Fatal("request was not restored after promotion")
		}
	})

	t.Run("missing final is interrupted", func(t *testing.T) {
		db := compatiblePreviewPersistenceDB(t)
		requestID, _, descriptor, _ := compatiblePreviewPersistentFixture(t, db, "artifact-interrupted")
		markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(compatiblePreviewArtifactGenerating), "generation-17", 17, descriptor.CachePath)
		if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
			t.Fatal(err)
		}
		var artifact models.CompatiblePreviewArtifactRecord
		if err := db.First(&artifact, "artifact_id = ?", descriptor.ArtifactID).Error; err != nil {
			t.Fatal(err)
		}
		if artifact.State != string(compatiblePreviewArtifactInterrupted) {
			t.Fatalf("state=%q, want interrupted", artifact.State)
		}
		entry, _, ok := compatiblePreviewRequestLookup(requestID, time.Now())
		if !ok || entry == nil {
			t.Fatal("interrupted request was not restored")
		}
		entry.mu.RLock()
		state := entry.recoveryState
		entry.mu.RUnlock()
		if state != string(compatiblePreviewArtifactInterrupted) {
			t.Fatalf("recovery state=%q, want interrupted", state)
		}
	})
}

func TestCompatiblePreviewFailedAndCanceledStatesSurviveRestart(t *testing.T) {
	for _, state := range []compatiblePreviewArtifactState{compatiblePreviewArtifactFailed, compatiblePreviewArtifactCanceled} {
		t.Run(string(state), func(t *testing.T) {
			db := compatiblePreviewPersistenceDB(t)
			requestID, _, descriptor, _ := compatiblePreviewPersistentFixture(t, db, "artifact-"+string(state))
			markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(state), "generation-terminal", 3, descriptor.CachePath)
			if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
				t.Fatal(err)
			}
			entry, _, ok := compatiblePreviewRequestLookup(requestID, time.Now())
			if !ok || entry == nil {
				t.Fatal("terminal request was not restored")
			}
			entry.mu.RLock()
			recoveryState := entry.recoveryState
			entry.mu.RUnlock()
			if recoveryState != string(state) {
				t.Fatalf("recovery state=%q, want %q", recoveryState, state)
			}
			if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
				t.Fatal(err)
			}
			entry, _, ok = compatiblePreviewRequestLookup(requestID, time.Now())
			if !ok || entry == nil {
				t.Fatal("terminal request was not restored after second restart")
			}
			entry.mu.RLock()
			recoveryState = entry.recoveryState
			entry.mu.RUnlock()
			if recoveryState != string(state) {
				t.Fatalf("recovery state after restart=%q, want %q", recoveryState, state)
			}
		})
	}
}

func TestCompatiblePreviewReadyMissingOrUnsafeArtifactBecomesStale(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	requestID, _, descriptor, _ := compatiblePreviewPersistentFixture(t, db, "artifact-missing")
	markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(compatiblePreviewArtifactReady), "generation-1", 1, descriptor.CachePath)
	if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
		t.Fatal(err)
	}
	var artifact models.CompatiblePreviewArtifactRecord
	if err := db.First(&artifact, "artifact_id = ?", descriptor.ArtifactID).Error; err != nil {
		t.Fatal(err)
	}
	if artifact.State != string(compatiblePreviewArtifactStale) {
		t.Fatalf("missing artifact state=%q, want stale", artifact.State)
	}
	entry, _, ok := compatiblePreviewRequestLookup(requestID, time.Now())
	if !ok || entry == nil {
		t.Fatal("stale request was not restored")
	}
	entry.mu.RLock()
	state := entry.recoveryState
	entry.mu.RUnlock()
	if state != string(compatiblePreviewArtifactStale) {
		t.Fatalf("recovery state=%q, want stale", state)
	}
}

func TestCompatiblePreviewUnsafeAndPartialFinalFilesAreNeverReady(t *testing.T) {
	tests := []struct {
		name string
		path func(root, artifactID string) string
	}{
		{name: "zero byte", path: func(root, artifactID string) string { return filepath.Join(root, artifactID+".mp4") }},
		{name: "partial", path: func(root, artifactID string) string { return filepath.Join(root, artifactID+".mp4.partial") }},
		{name: "outside root", path: func(root, artifactID string) string { return filepath.Join(filepath.Dir(root), artifactID+".mp4") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := compatiblePreviewPersistenceDB(t)
			requestID, _, descriptor, _ := compatiblePreviewPersistentFixture(t, db, "artifact-"+strings.ReplaceAll(test.name, " ", "-"))
			candidate := test.path(filepath.Dir(descriptor.CachePath), descriptor.ArtifactID)
			if test.name == "zero byte" {
				if err := os.WriteFile(candidate, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(candidate, []byte("partial"), 0o644); err != nil {
				t.Fatal(err)
			}
			if test.name == "outside root" {
				if err := db.Model(&models.CompatiblePreviewArtifactRecord{}).Where("artifact_id = ?", descriptor.ArtifactID).Update("cache_path", candidate).Error; err != nil {
					t.Fatal(err)
				}
			}
			markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(compatiblePreviewArtifactReady), "generation-1", 1, candidate)
			if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
				t.Fatal(err)
			}
			entry, _, ok := compatiblePreviewRequestLookup(requestID, time.Now())
			if !ok || entry == nil {
				t.Fatal("request was not retained for deterministic stale response")
			}
			entry.mu.RLock()
			state := entry.recoveryState
			entry.mu.RUnlock()
			if state != string(compatiblePreviewArtifactStale) {
				t.Fatalf("recovery state=%q, want stale", state)
			}
		})
	}
}

func TestCompatiblePreviewFrozenProfileAndSourceRevisionAreNotReResolved(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	requestID, _, descriptor, _ := compatiblePreviewPersistentFixture(t, db, "artifact-frozen")
	if err := os.WriteFile(descriptor.CachePath, []byte("published"), 0o644); err != nil {
		t.Fatal(err)
	}
	markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(compatiblePreviewArtifactReady), "generation-1", 1, descriptor.CachePath)
	if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
		t.Fatal(err)
	}
	entry, _, ok := compatiblePreviewRequestLookup(requestID, time.Now())
	if !ok || entry == nil {
		t.Fatal("request was not restored")
	}
	entry.mu.RLock()
	restored := entry.Descriptor
	entry.mu.RUnlock()
	if restored == nil || restored.ArtifactID != descriptor.ArtifactID || restored.PreviewProfile == nil || restored.PreviewProfile.Name != "frozen-profile" {
		t.Fatalf("profile snapshot was re-resolved or lost: %#v", restored)
	}
	if err := os.WriteFile(descriptor.Path, []byte("source changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(descriptor.Path); err != nil {
		t.Fatal(err)
	}
	entry.mu.Lock()
	entry.recoveryState = ""
	entry.mu.Unlock()
	_, err := (AssetHandler{db: db}).compatiblePreviewDescriptorForEntry(entry, requestID)
	var lifecycleErr *compatiblePreviewLifecycleError
	if !errors.As(err, &lifecycleErr) || lifecycleErr.Code != "stale_artifact" {
		t.Fatalf("source revision mismatch error=%v, want stale_artifact", err)
	}
}

func TestCompatiblePreviewExpiredTombstoneBecomesUnknownAndCanBeRecreated(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	requestID, request, _, now := compatiblePreviewPersistentFixture(t, db, "artifact-expiring")
	if err := db.Model(&models.CompatiblePreviewRequestRecord{}).Where("request_id = ?", requestID).Updates(map[string]any{"expires_at": now.Add(-time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
		t.Fatal(err)
	}
	if _, tombstone, ok := compatiblePreviewRequestLookup(requestID, time.Now()); !ok || tombstone == nil {
		t.Fatal("expired request did not become a tombstone")
	}
	if id, _, err := storeCompatiblePreviewRequest(request, time.Now()); err != nil || id != requestID {
		t.Fatalf("explicit deterministic recreate id=%q err=%v", id, err)
	}
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{}
	compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{}
	compatiblePreviewRequests.Unlock()
	if err := db.Model(&models.CompatiblePreviewRequestRecord{}).Where("request_id = ?", requestID).Updates(map[string]any{"state": compatiblePreviewRequestStateExpiredTombstone, "tombstone_expires_at": time.Now().Add(-time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ReconcileCompatiblePreviewPersistence(db); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := compatiblePreviewRequestLookup(requestID, time.Now()); ok {
		t.Fatal("purged tombstone remained addressable")
	}
}

func TestCompatiblePreviewLateGenerationTransitionCannotOverwriteReplacement(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	requestID, request, descriptor, now := compatiblePreviewPersistentFixture(t, db, "artifact-generation-race")
	resetCompatiblePreviewRuntimeForReconcile()
	t.Cleanup(resetCompatiblePreviewRuntimeForReconcile)
	requestEntry := newCompatiblePreviewRequestEntry(request, now, db)
	descriptorCopy := *descriptor
	descriptorCopy.frozen = true
	descriptorCopy.persistenceDB = db
	descriptorCopy.requestID = requestID
	requestEntry.Descriptor = &descriptorCopy
	oldGeneration := newCompatiblePreviewArtifactEntry(&descriptorCopy, now, compatiblePreviewArtifactReady)
	oldGeneration.generationToken = "generation-17"
	oldGeneration.generationVersion = 17
	currentGeneration := newCompatiblePreviewArtifactEntry(&descriptorCopy, now, compatiblePreviewArtifactReady)
	currentGeneration.generationToken = "generation-18"
	currentGeneration.generationVersion = 18
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items[requestID] = requestEntry
	compatiblePreviewRequests.Unlock()
	compatiblePreviewArtifacts.Lock()
	compatiblePreviewArtifacts.items[descriptor.ArtifactID] = currentGeneration
	compatiblePreviewArtifacts.Unlock()
	markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(compatiblePreviewArtifactGenerating), "generation-18", 18, descriptor.CachePath)
	if updated, err := persistCompatiblePreviewGenerationCompletion(oldGeneration, descriptor.ArtifactID, "generation-17", 17, compatiblePreviewArtifactCanceled, "late cancellation", time.Now()); err != nil {
		t.Fatal(err)
	} else if updated {
		t.Fatal("stale generation unexpectedly owned replacement transition")
	}
	requestEntry.mu.RLock()
	if requestEntry.recoveryState != "" {
		t.Fatalf("stale generation poisoned request recovery state: %q", requestEntry.recoveryState)
	}
	requestEntry.mu.RUnlock()
	updated, err := persistCompatiblePreviewGenerationCompletion(currentGeneration, descriptor.ArtifactID, "generation-18", 18, compatiblePreviewArtifactReady, "", time.Now())
	if err != nil || !updated {
		t.Fatalf("replacement generation did not complete: updated=%t err=%v", updated, err)
	}
	var artifact models.CompatiblePreviewArtifactRecord
	if err := db.First(&artifact, "artifact_id = ?", descriptor.ArtifactID).Error; err != nil {
		t.Fatal(err)
	}
	if artifact.State != string(compatiblePreviewArtifactReady) || artifact.GenerationToken != "generation-18" {
		t.Fatalf("late generation overwrote replacement: state=%q token=%q", artifact.State, artifact.GenerationToken)
	}
	requestEntry.mu.RLock()
	if requestEntry.recoveryState != "" {
		t.Fatalf("replacement generation left request unusable: %q", requestEntry.recoveryState)
	}
	requestEntry.mu.RUnlock()
}

func TestCompatiblePreviewStaleRecreateFailurePreservesLifecycle(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	requestID, request, descriptor, now := compatiblePreviewPersistentFixture(t, db, "artifact-stale-recreate")
	resetCompatiblePreviewRuntimeForReconcile()
	t.Cleanup(resetCompatiblePreviewRuntimeForReconcile)
	entry := newCompatiblePreviewRequestEntry(request, now, db)
	entry.Descriptor = descriptor
	entry.recoveryState = string(compatiblePreviewArtifactStale)
	entry.recoveryReason = "artifact is stale"
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items[requestID] = entry
	compatiblePreviewRequests.Unlock()
	persistCompatiblePreviewRequestStale(db, requestID, entry.recoveryReason)
	if err := os.Remove(descriptor.Path); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.POST("/requests", AssetHandler{db: db}.CreateCompatiblePreviewRequest)
	router.GET("/requests/:requestId", AssetHandler{db: db}.CompatiblePreviewRequestVideo)
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/requests", strings.NewReader(string(body))))
	if createResponse.Code != http.StatusNotFound {
		t.Fatalf("stale recreate status=%d body=%s, want preparation failure", createResponse.Code, createResponse.Body.String())
	}
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/requests/"+requestID, nil))
	if getResponse.Code != http.StatusConflict {
		t.Fatalf("stale request after failed recreate status=%d body=%s, want 409", getResponse.Code, getResponse.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(getResponse.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "stale_artifact" {
		t.Fatalf("stale request after failed recreate code=%v", payload["code"])
	}
}

func TestCompatiblePreviewExpiredRecreateFailurePreservesTombstone(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	request := compatiblePreviewRequest{Path: filepath.Join(t.TempDir(), "missing.mkv")}
	normalized, err := normalizeCompatiblePreviewRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	requestID, err := compatiblePreviewRequestIdentity(normalized)
	if err != nil {
		t.Fatal(err)
	}
	resetCompatiblePreviewRuntimeForReconcile()
	t.Cleanup(resetCompatiblePreviewRuntimeForReconcile)
	tombstone := compatiblePreviewRequestTombstone{RequestID: requestID, ExpiresAt: time.Now().Add(-time.Hour), TombstoneExpiresAt: time.Now().Add(time.Hour), Reason: "request expired"}
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.tombstones[requestID] = tombstone
	compatiblePreviewRequests.Unlock()

	router := gin.New()
	router.POST("/requests", AssetHandler{db: db}.CreateCompatiblePreviewRequest)
	router.GET("/requests/:requestId", AssetHandler{db: db}.CompatiblePreviewRequestVideo)
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/requests", strings.NewReader(string(body))))
	if createResponse.Code != http.StatusForbidden {
		t.Fatalf("expired recreate status=%d body=%s, want preparation failure", createResponse.Code, createResponse.Body.String())
	}
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/requests/"+requestID, nil))
	if getResponse.Code != http.StatusGone {
		t.Fatalf("expired request after failed recreate status=%d body=%s, want 410", getResponse.Code, getResponse.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(getResponse.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "expired" {
		t.Fatalf("expired request after failed recreate code=%v", payload["code"])
	}
}

func TestCompatiblePreviewExpiredTombstoneSuccessfulCreateReactivatesID(t *testing.T) {
	request := compatiblePreviewRequest{Path: "/media/raw/recreate.mkv"}
	normalized, err := normalizeCompatiblePreviewRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	requestID, err := compatiblePreviewRequestIdentity(normalized)
	if err != nil {
		t.Fatal(err)
	}
	resetCompatiblePreviewRuntimeForReconcile()
	t.Cleanup(resetCompatiblePreviewRuntimeForReconcile)
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.tombstones[requestID] = compatiblePreviewRequestTombstone{
		RequestID: requestID, ExpiresAt: time.Now().Add(-time.Hour), TombstoneExpiresAt: time.Now().Add(time.Hour), Reason: "request expired",
	}
	compatiblePreviewRequests.Unlock()

	router := gin.New()
	router.POST("/requests", AssetHandler{}.CreateCompatiblePreviewRequest)
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/requests", strings.NewReader(string(body))))
	if response.Code != http.StatusCreated {
		t.Fatalf("successful expired recreate status=%d body=%s", response.Code, response.Body.String())
	}
	entry, tombstone, ok := compatiblePreviewRequestLookup(requestID, time.Now())
	if !ok || entry == nil || tombstone != nil {
		t.Fatalf("successful recreate did not reactivate deterministic ID: entry=%v tombstone=%v ok=%v", entry != nil, tombstone != nil, ok)
	}
}

func TestCompatiblePreviewArtifactStartupLoadIsBoundedToReferencedRows(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	if err := db.Create(&models.CompatiblePreviewArtifactRecord{ArtifactID: "referenced-artifact", State: string(compatiblePreviewArtifactReady)}).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < compatiblePreviewRequestMaxItems+25; index++ {
		record := models.CompatiblePreviewArtifactRecord{ArtifactID: fmt.Sprintf("unreferenced-artifact-%d", index), State: string(compatiblePreviewArtifactReady)}
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	}
	records, err := compatiblePreviewArtifactRecordsByID(db, []models.CompatiblePreviewRequestRecord{{ArtifactID: "referenced-artifact"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("startup artifact recovery loaded %d rows, want only referenced row", len(records))
	}
	if _, ok := records["referenced-artifact"]; !ok {
		t.Fatal("referenced artifact was not loaded")
	}
}

func TestCompatiblePreviewLifecycleHTTPContractDistinguishesExpiredAndUnknown(t *testing.T) {
	t.Cleanup(func() {
		compatiblePreviewRequests.Lock()
		compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{}
		compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{}
		compatiblePreviewRequests.Unlock()
	})
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{}
	compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{
		"expired-request": {RequestID: "expired-request", TombstoneExpiresAt: time.Now().Add(time.Hour), Reason: "request expired"},
	}
	compatiblePreviewRequests.Unlock()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/preview/:requestId", AssetHandler{}.CompatiblePreviewRequestVideo)
	for _, test := range []struct {
		id     string
		status int
		code   string
	}{
		{id: "expired-request", status: http.StatusGone, code: "expired"},
		{id: "unknown-request", status: http.StatusNotFound, code: "unknown_request"},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/preview/"+test.id, nil)
		router.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("id=%s status=%d, want %d body=%s", test.id, response.Code, test.status, response.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["code"] != test.code {
			t.Fatalf("id=%s code=%v, want %s", test.id, payload["code"], test.code)
		}
	}
}

func TestCompatiblePreviewGenerationStartGetsNewTokenAndVersion(t *testing.T) {
	db := compatiblePreviewPersistenceDB(t)
	_, _, descriptor, now := compatiblePreviewPersistentFixture(t, db, "artifact-attempts")
	firstToken, firstVersion, err := persistCompatiblePreviewArtifactGenerationStart(db, descriptor, now)
	if err != nil {
		t.Fatal(err)
	}
	secondToken, secondVersion, err := persistCompatiblePreviewArtifactGenerationStart(db, descriptor, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if firstToken == secondToken || firstVersion != 1 || secondVersion != 2 {
		t.Fatalf("generation attempts token/version first=%s/%d second=%s/%d", firstToken, firstVersion, secondToken, secondVersion)
	}
}
