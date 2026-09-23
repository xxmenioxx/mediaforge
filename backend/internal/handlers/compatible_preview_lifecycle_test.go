package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func lifecycleTestArtifact(t *testing.T, artifactID string) *compatiblePreviewArtifact {
	t.Helper()
	descriptor := testCompatiblePreviewDescriptor(t, artifactID)
	descriptor.requestID = "request-" + artifactID
	return newCompatiblePreviewArtifactEntry(descriptor, time.Now(), compatiblePreviewArtifactGenerating)
}

func TestCompatiblePreviewLogicalLeaseHandoffNeverDropsConsumer(t *testing.T) {
	resetCompatiblePreviewLeases()
	t.Cleanup(resetCompatiblePreviewLeases)
	entry := lifecycleTestArtifact(t, "handoff")
	httpLease := &compatiblePreviewArtifactLease{entry: entry}
	lease, duplicate, err := createCompatiblePreviewLogicalLease("request-handoff", "owner-a", entry, httpLease, time.Now())
	if err != nil || lease == nil || duplicate {
		t.Fatalf("handoff lease creation failed: lease=%#v duplicate=%t err=%v", lease, duplicate, err)
	}
	entry.mu.Lock()
	waiters := entry.waiters
	entry.mu.Unlock()
	if waiters != 1 {
		t.Fatalf("HTTP-to-logical handoff changed consumer count: got %d, want 1", waiters)
	}
	if !releaseCompatiblePreviewLogicalLease(lease, time.Now()) {
		t.Fatal("active lease was not released")
	}
	if releaseCompatiblePreviewLogicalLease(lease, time.Now()) {
		t.Fatal("releasing the same lease twice decremented ownership twice")
	}
	entry.mu.Lock()
	waiters = entry.waiters
	entry.mu.Unlock()
	if waiters != 0 {
		t.Fatalf("released lease left %d artifact consumers", waiters)
	}
}

func TestCompatiblePreviewLogicalLeaseStartIsIdempotentForOwner(t *testing.T) {
	resetCompatiblePreviewLeases()
	t.Cleanup(resetCompatiblePreviewLeases)
	entry := lifecycleTestArtifact(t, "start-idempotent")
	first, duplicate, err := createCompatiblePreviewLogicalLease("request-start", "owner-a", entry, &compatiblePreviewArtifactLease{entry: entry}, time.Now())
	if err != nil || first == nil || duplicate {
		t.Fatalf("initial lease creation failed: duplicate=%t err=%v", duplicate, err)
	}
	entry.mu.Lock()
	entry.waiters++
	entry.mu.Unlock()
	second, duplicate, err := createCompatiblePreviewLogicalLease("request-start", "owner-a", entry, &compatiblePreviewArtifactLease{entry: entry}, time.Now())
	if err != nil || second != first || !duplicate {
		t.Fatalf("same owner did not reuse its lease: first=%p second=%p duplicate=%t err=%v", first, second, duplicate, err)
	}
	entry.mu.Lock()
	waiters := entry.waiters
	entry.mu.Unlock()
	if waiters != 1 {
		t.Fatalf("idempotent retry left duplicate consumers: got %d, want 1", waiters)
	}
	releaseCompatiblePreviewLogicalLease(first, time.Now())
}

func TestCompatiblePreviewSharedLogicalLeasesCancelIndependently(t *testing.T) {
	resetCompatiblePreviewLeases()
	t.Cleanup(resetCompatiblePreviewLeases)
	entry := lifecycleTestArtifact(t, "shared")
	first, _, err := createCompatiblePreviewLogicalLease("request-a", "owner-a", entry, &compatiblePreviewArtifactLease{entry: entry}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	entry.mu.Lock()
	entry.waiters++
	entry.mu.Unlock()
	second, _, err := createCompatiblePreviewLogicalLease("request-b", "owner-b", entry, &compatiblePreviewArtifactLease{entry: entry}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !releaseCompatiblePreviewLogicalLease(first, time.Now()) || releaseCompatiblePreviewLogicalLease(first, time.Now()) {
		t.Fatal("owner A cancellation was not exactly-once")
	}
	entry.mu.Lock()
	waiters, state := entry.waiters, entry.state
	entry.mu.Unlock()
	if waiters != 1 || state != compatiblePreviewArtifactGenerating {
		t.Fatalf("owner B was disturbed by owner A cancellation: waiters=%d state=%s", waiters, state)
	}
	if !releaseCompatiblePreviewLogicalLease(second, time.Now()) {
		t.Fatal("owner B was not releasable")
	}
}

func TestCompatiblePreviewLeaseInvalidAndExpiredClassification(t *testing.T) {
	resetCompatiblePreviewLeases()
	t.Cleanup(resetCompatiblePreviewLeases)
	entry := lifecycleTestArtifact(t, "classification")
	now := time.Now()
	lease, _, err := createCompatiblePreviewLogicalLease("request-classification", "owner-a", entry, &compatiblePreviewArtifactLease{entry: entry}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := findCompatiblePreviewLease("request-classification", "owner-a", "not-a-token", now); err == nil || err.(*compatiblePreviewLifecycleError).Code != "invalid_lease" {
		t.Fatalf("random token was not classified as invalid_lease: %v", err)
	}
	cleanupCompatiblePreviewLeases(now.Add(compatiblePreviewLogicalLeaseLifetime + time.Second))
	if _, err := findCompatiblePreviewLease("request-classification", "owner-a", lease.token, now.Add(compatiblePreviewLogicalLeaseLifetime+time.Second)); err == nil || err.(*compatiblePreviewLifecycleError).Code != "lease_expired" {
		t.Fatalf("expired token was not classified as lease_expired: %v", err)
	}
	if _, err := findCompatiblePreviewLease("other-request", "owner-a", lease.token, now); err == nil || err.(*compatiblePreviewLifecycleError).Code != "invalid_lease" {
		t.Fatalf("wrong request binding was not classified as invalid_lease: %v", err)
	}
}

func TestCompatiblePreviewLegacyNonFollowableHandoffIsExplicit(t *testing.T) {
	descriptor := testCompatiblePreviewDescriptor(t, "legacy-non-followable")
	entry := lifecycleTestArtifact(t, "legacy-non-followable")
	httpLease := &compatiblePreviewArtifactLease{entry: entry}
	_, err := promoteCompatiblePreviewLogicalLease(testGinContext(http.MethodGet, "/preview"), descriptor, entry, httpLease)
	if err == nil {
		t.Fatal("ephemeral legacy handoff unexpectedly created a followable lease")
	}
	lifecycleErr, ok := err.(*compatiblePreviewLifecycleError)
	if !ok || lifecycleErr.Code != "legacy_non_followable" || lifecycleErr.Status != http.StatusConflict || !lifecycleErr.Retryable {
		t.Fatalf("legacy limitation=%#v", err)
	}
}

func TestCompatiblePreviewExpiredRequestPrecedesLeaseClassification(t *testing.T) {
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{}
	compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{
		"expired-request": {RequestID: "expired-request", Reason: "request expired", TombstoneExpiresAt: time.Now().Add(time.Minute)},
	}
	compatiblePreviewRequests.Unlock()
	t.Cleanup(func() {
		compatiblePreviewRequests.Lock()
		compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{}
		compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{}
		compatiblePreviewRequests.Unlock()
	})

	entry, tombstone, _, known := (AssetHandler{}).compatiblePreviewLifecycleEntry(testGinContext(http.MethodPost, "/requests/expired-request/cancel"))
	if entry != nil || tombstone == nil || !known {
		t.Fatalf("expired request was not found as a tombstone: entry=%p tombstone=%#v known=%t", entry, tombstone, known)
	}
}

func TestCompatiblePreviewCancelEndpointIsIdempotent(t *testing.T) {
	resetCompatiblePreviewLeases()
	t.Cleanup(resetCompatiblePreviewLeases)
	descriptor := testCompatiblePreviewDescriptor(t, "cancel-endpoint")
	descriptor.requestID = "cancel-request"
	artifact := newCompatiblePreviewArtifactEntry(descriptor, time.Now(), compatiblePreviewArtifactGenerating)
	requestEntry := &compatiblePreviewRequestEntry{Descriptor: descriptor, CreatedAt: time.Now(), AccessedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{"cancel-request": requestEntry}
	compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{}
	compatiblePreviewRequests.Unlock()
	t.Cleanup(func() {
		compatiblePreviewRequests.Lock()
		compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{}
		compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{}
		compatiblePreviewRequests.Unlock()
	})
	lease, _, err := createCompatiblePreviewLogicalLease("cancel-request", "owner-a", artifact, &compatiblePreviewArtifactLease{entry: artifact}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/requests/:requestId/cancel", AssetHandler{}.CompatiblePreviewRequestCancel)
	body := `{"ownerId":"owner-a","leaseToken":"` + lease.token + `"}`
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/requests/cancel-request/cancel", bytes.NewBufferString(body)))
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"leaseReleased":true`) {
		t.Fatalf("first cancel response=%d %s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/requests/cancel-request/cancel", bytes.NewBufferString(body)))
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"leaseReleased":false`) {
		t.Fatalf("repeated cancel response=%d %s", second.Code, second.Body.String())
	}
}

func TestCompatiblePreviewStatusProjectionMatrix(t *testing.T) {
	cases := []struct {
		name       string
		state      string
		wantStatus int
		wantCode   string
		wantState  string
	}{
		{name: "idle", wantStatus: http.StatusOK, wantCode: "idle", wantState: "idle"},
		{name: "generating", state: string(compatiblePreviewArtifactGenerating), wantStatus: http.StatusOK, wantCode: "generating", wantState: "generating"},
		{name: "ready", state: string(compatiblePreviewArtifactReady), wantStatus: http.StatusOK, wantCode: "ready", wantState: "ready"},
		{name: "failed", state: string(compatiblePreviewArtifactFailed), wantStatus: http.StatusOK, wantCode: "failed", wantState: "failed"},
		{name: "canceled", state: string(compatiblePreviewArtifactCanceled), wantStatus: http.StatusOK, wantCode: "canceled", wantState: "canceled"},
		{name: "interrupted", state: string(compatiblePreviewArtifactInterrupted), wantStatus: http.StatusOK, wantCode: "interrupted", wantState: "interrupted"},
		{name: "stale", state: string(compatiblePreviewArtifactStale), wantStatus: http.StatusOK, wantCode: "stale_artifact", wantState: "stale"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			resetCompatiblePreviewArtifactsForTest()
			compatiblePreviewRequests.Lock()
			compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{}
			compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{}
			compatiblePreviewRequests.Unlock()
			t.Cleanup(func() { resetCompatiblePreviewArtifactsForTest() })
			descriptor := testCompatiblePreviewDescriptor(t, "status-"+test.name)
			descriptor.requestID = "status-" + test.name
			entry := &compatiblePreviewRequestEntry{Descriptor: descriptor, CreatedAt: time.Now(), AccessedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
			if test.state == string(compatiblePreviewArtifactStale) || test.state == string(compatiblePreviewArtifactFailed) || test.state == string(compatiblePreviewArtifactCanceled) || test.state == string(compatiblePreviewArtifactInterrupted) {
				entry.recoveryState = test.state
				entry.recoveryReason = "controlled test state"
			}
			compatiblePreviewRequests.Lock()
			compatiblePreviewRequests.items[descriptor.requestID] = entry
			compatiblePreviewRequests.Unlock()
			if test.state == string(compatiblePreviewArtifactGenerating) || test.state == string(compatiblePreviewArtifactReady) {
				artifact := newCompatiblePreviewArtifactEntry(descriptor, time.Now(), compatiblePreviewArtifactState(test.state))
				if test.state == string(compatiblePreviewArtifactReady) {
					if err := os.WriteFile(descriptor.CachePath, []byte("ready"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				compatiblePreviewArtifacts.Lock()
				compatiblePreviewArtifacts.items[descriptor.ArtifactID] = artifact
				compatiblePreviewArtifacts.Unlock()
			}
			router := gin.New()
			router.GET("/requests/:requestId/status", AssetHandler{}.CompatiblePreviewRequestStatus)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/requests/"+descriptor.requestID+"/status", nil))
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["code"] != test.wantCode || body["state"] != test.wantState {
				t.Fatalf("projection=%#v, want code=%q state=%q", body, test.wantCode, test.wantState)
			}
		})
	}

	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{}
	compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{"expired": {RequestID: "expired", Reason: "request expired", TombstoneExpiresAt: time.Now().Add(time.Minute)}}
	compatiblePreviewRequests.Unlock()
	router := gin.New()
	router.GET("/requests/:requestId/status", AssetHandler{}.CompatiblePreviewRequestStatus)
	for _, test := range []struct {
		id     string
		status int
		code   string
	}{
		{id: "expired", status: http.StatusGone, code: "expired"},
		{id: "unknown", status: http.StatusNotFound, code: "unknown_request"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/requests/"+test.id+"/status", nil))
		if response.Code != test.status || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("%s status=%d body=%s", test.id, response.Code, response.Body.String())
		}
	}
}

func TestCompatiblePreviewResourceActionMatrixAndGeneratingHandoff(t *testing.T) {
	oldBudget := compatiblePreviewHTTPWaitBudget
	compatiblePreviewHTTPWaitBudget = time.Millisecond
	t.Cleanup(func() { compatiblePreviewHTTPWaitBudget = oldBudget })
	resetCompatiblePreviewLeases()
	resetCompatiblePreviewArtifactsForTest()
	t.Cleanup(resetCompatiblePreviewLeases)
	t.Cleanup(resetCompatiblePreviewArtifactsForTest)
	descriptor := testCompatiblePreviewDescriptor(t, "resource-generating")
	descriptor.requestID = "resource-generating"
	descriptor.frozen = true
	if err := os.WriteFile(descriptor.Path, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	descriptor.SourceRevision, _ = sourceRevisionForPath(descriptor.Path)
	requestEntry := &compatiblePreviewRequestEntry{Request: descriptor.Request, Descriptor: descriptor, CreatedAt: time.Now(), AccessedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	artifact := newCompatiblePreviewArtifactEntry(descriptor, time.Now(), compatiblePreviewArtifactGenerating)
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{descriptor.requestID: requestEntry}
	compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{}
	compatiblePreviewRequests.Unlock()
	compatiblePreviewArtifacts.Lock()
	compatiblePreviewArtifacts.items[descriptor.ArtifactID] = artifact
	compatiblePreviewArtifacts.Unlock()
	router := gin.New()
	router.GET("/requests/:requestId/video", AssetHandler{}.CompatiblePreviewRequestVideo)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/requests/"+descriptor.requestID+"/video", nil))
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"code":"generating"`) || !strings.Contains(response.Body.String(), `"leaseToken"`) {
		t.Fatalf("generating resource response=%d body=%s", response.Code, response.Body.String())
	}

	for _, test := range []struct {
		name   string
		state  string
		status int
	}{
		{name: "failed", state: string(compatiblePreviewArtifactFailed), status: http.StatusConflict},
		{name: "canceled", state: string(compatiblePreviewArtifactCanceled), status: http.StatusConflict},
		{name: "interrupted", state: string(compatiblePreviewArtifactInterrupted), status: http.StatusConflict},
		{name: "stale", state: string(compatiblePreviewArtifactStale), status: http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := "resource-" + test.name
			entry := &compatiblePreviewRequestEntry{Request: descriptor.Request, Descriptor: descriptor, CreatedAt: time.Now(), AccessedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), recoveryState: test.state, recoveryReason: "controlled terminal state"}
			compatiblePreviewRequests.Lock()
			compatiblePreviewRequests.items[id] = entry
			compatiblePreviewRequests.Unlock()
			defer func() {
				compatiblePreviewRequests.Lock()
				delete(compatiblePreviewRequests.items, id)
				compatiblePreviewRequests.Unlock()
			}()
			request := httptest.NewRequest(http.MethodGet, "/requests/"+id+"/video", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}

	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{}
	compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{"resource-expired": {RequestID: "resource-expired", Reason: "request expired", TombstoneExpiresAt: time.Now().Add(time.Minute)}}
	compatiblePreviewRequests.Unlock()
	for _, test := range []struct {
		id     string
		status int
		code   string
	}{
		{id: "resource-expired", status: http.StatusGone, code: "expired"},
		{id: "resource-unknown", status: http.StatusNotFound, code: "unknown_request"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/requests/"+test.id+"/video", nil))
		if response.Code != test.status || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("%s status=%d body=%s", test.id, response.Code, response.Body.String())
		}
	}
}

func testGinContext(method, path string) *gin.Context {
	// Kept in a helper so lifecycle tests do not need to start the full router.
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(method, path, nil)
	context.Params = gin.Params{{Key: "requestId", Value: "expired-request"}}
	return context
}
