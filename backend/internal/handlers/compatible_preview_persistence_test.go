package handlers

import (
	"context"
	"encoding/json"
	"errors"
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
	_, _, descriptor, _ := compatiblePreviewPersistentFixture(t, db, "artifact-generation-race")
	markCompatiblePreviewArtifact(t, db, descriptor.ArtifactID, string(compatiblePreviewArtifactGenerating), "generation-18", 18, descriptor.CachePath)
	if err := persistCompatiblePreviewArtifactTransition(db, descriptor.ArtifactID, "generation-17", 17, compatiblePreviewArtifactReady, "late completion", time.Now()); err != nil {
		t.Fatal(err)
	}
	var artifact models.CompatiblePreviewArtifactRecord
	if err := db.First(&artifact, "artifact_id = ?", descriptor.ArtifactID).Error; err != nil {
		t.Fatal(err)
	}
	if artifact.State != string(compatiblePreviewArtifactGenerating) || artifact.GenerationToken != "generation-18" {
		t.Fatalf("late generation overwrote replacement: state=%q token=%q", artifact.State, artifact.GenerationToken)
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
