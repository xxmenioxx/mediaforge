package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

func newProfileSampleEstimateTestStore() *profileSampleEstimateOperationStore {
	return &profileSampleEstimateOperationStore{
		items:   map[string]*ProfileSampleEstimateOperation{},
		cancels: map[string]context.CancelFunc{},
	}
}

func waitForProfileSampleEstimateOperation(
	t *testing.T,
	store *profileSampleEstimateOperationStore,
	id string,
	match func(ProfileSampleEstimateOperation) bool,
) ProfileSampleEstimateOperation {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if operation, ok := store.copy(id); ok && match(operation) {
			return operation
		}
		time.Sleep(time.Millisecond)
	}
	operation, _ := store.copy(id)
	t.Fatalf("operation did not reach expected state: %#v", operation)
	return ProfileSampleEstimateOperation{}
}

func TestProfileSampleEstimateProgressUsesEncodedMediaTime(t *testing.T) {
	progress := profileSampleEstimateProgressValues(2, 3, 5, 20, 8, 0.31, time.Minute)
	if progress.Progress != 48 || progress.CurrentSampleProgress != 40 {
		t.Fatalf("unexpected progress: %#v", progress)
	}
	if progress.EncodedSeconds != 48 || progress.TotalSampleSeconds != 100 {
		t.Fatalf("unexpected encoded media time: %#v", progress)
	}
	if progress.Speed != 0.31 || progress.ETASeconds != 168 {
		t.Fatalf("unexpected throughput/ETA: %#v", progress)
	}
}

func TestProfileSampleEstimateProgressClampsValues(t *testing.T) {
	progress := profileSampleEstimateProgressValues(4, 5, 5, 20, 40, 0, 100*time.Second)
	if progress.Progress != 100 || progress.CurrentSampleProgress != 100 || progress.EncodedSeconds != 100 {
		t.Fatalf("progress was not clamped: %#v", progress)
	}
}

func TestProfileSampleEstimateProgressLineParsesTimeAndSpeed(t *testing.T) {
	seconds, speed, ok := profileSampleEstimateProgressLine("out_time_us=8000000", 0, 0)
	if !ok || seconds != 8 || speed != 0 {
		t.Fatalf("unexpected time parse: seconds=%v speed=%v ok=%v", seconds, speed, ok)
	}
	seconds, speed, ok = profileSampleEstimateProgressLine("speed=0.31x", seconds, speed)
	if !ok || seconds != 8 || speed != 0.31 {
		t.Fatalf("unexpected speed parse: seconds=%v speed=%v ok=%v", seconds, speed, ok)
	}
}

func TestProfileSampleEstimateOperationReturnsImmediatelyAndCompletes(t *testing.T) {
	store := newProfileSampleEstimateTestStore()
	slot := make(chan struct{}, 1)
	release := make(chan struct{})
	started := make(chan struct{})
	runner := func(ctx context.Context, _ profileSampleEstimateInput, report func(profileSampleEstimateProgress)) (models.JSONMap, error) {
		close(started)
		report(profileSampleEstimateProgressValues(2, 3, 5, 20, 8, 0.5, 2*time.Second))
		select {
		case <-release:
			return models.JSONMap{"estimatedVideoBytes": int64(1234)}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	begin := time.Now()
	operation := launchProfileSampleEstimateOperation(store, slot, profileSampleEstimateInput{}, time.Minute, runner)
	if time.Since(begin) > 100*time.Millisecond {
		t.Fatal("starting the operation waited for estimate execution")
	}
	<-started
	running := waitForProfileSampleEstimateOperation(t, store, operation.ID, func(item ProfileSampleEstimateOperation) bool {
		return item.Status == profileSampleEstimateRunning && item.Progress == 48
	})
	if running.Phase != "encoding" || running.CurrentSample != 3 {
		t.Fatalf("unexpected running operation: %#v", running)
	}
	close(release)
	completed := waitForProfileSampleEstimateOperation(t, store, operation.ID, func(item ProfileSampleEstimateOperation) bool {
		return item.Status == profileSampleEstimateCompleted
	})
	if completed.Progress != 100 || completed.Result["estimatedVideoBytes"] != int64(1234) {
		t.Fatalf("unexpected completed operation: %#v", completed)
	}
	if len(slot) != 0 {
		t.Fatal("capacity slot was not released after success")
	}
}

func TestProfileSampleEstimateOperationCancelWhileQueued(t *testing.T) {
	store := newProfileSampleEstimateTestStore()
	slot := make(chan struct{}, 1)
	slot <- struct{}{}
	var called atomic.Bool
	operation := launchProfileSampleEstimateOperation(store, slot, profileSampleEstimateInput{}, time.Minute, func(context.Context, profileSampleEstimateInput, func(profileSampleEstimateProgress)) (models.JSONMap, error) {
		called.Store(true)
		return nil, nil
	})
	if _, found, canceled := store.cancel(operation.ID); !found || !canceled {
		t.Fatal("queued operation was not canceled")
	}
	canceled := waitForProfileSampleEstimateOperation(t, store, operation.ID, func(item ProfileSampleEstimateOperation) bool {
		return item.Status == profileSampleEstimateCanceled
	})
	if canceled.Phase != profileSampleEstimateCanceled || called.Load() {
		t.Fatalf("unexpected queued cancellation: %#v called=%v", canceled, called.Load())
	}
	if len(slot) != 1 {
		t.Fatal("queued cancellation modified a slot it did not acquire")
	}
	<-slot
}

func TestProfileSampleEstimateOperationCancelWhileRunning(t *testing.T) {
	store := newProfileSampleEstimateTestStore()
	slot := make(chan struct{}, 1)
	started := make(chan struct{})
	operation := launchProfileSampleEstimateOperation(store, slot, profileSampleEstimateInput{}, time.Minute, func(ctx context.Context, _ profileSampleEstimateInput, _ func(profileSampleEstimateProgress)) (models.JSONMap, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	<-started
	if _, found, canceled := store.cancel(operation.ID); !found || !canceled {
		t.Fatal("running operation was not canceled")
	}
	waitForProfileSampleEstimateOperation(t, store, operation.ID, func(item ProfileSampleEstimateOperation) bool {
		return item.Status == profileSampleEstimateCanceled
	})
	deadline := time.Now().Add(time.Second)
	for len(slot) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(slot) != 0 {
		t.Fatal("capacity slot was not released after running cancellation")
	}
}

func TestProfileSampleEstimateOperationFailureSurfacesErrorAndReleasesSlot(t *testing.T) {
	store := newProfileSampleEstimateTestStore()
	slot := make(chan struct{}, 1)
	operation := launchProfileSampleEstimateOperation(store, slot, profileSampleEstimateInput{}, time.Minute, func(context.Context, profileSampleEstimateInput, func(profileSampleEstimateProgress)) (models.JSONMap, error) {
		return nil, errors.New("encode exploded")
	})
	failed := waitForProfileSampleEstimateOperation(t, store, operation.ID, func(item ProfileSampleEstimateOperation) bool {
		return item.Status == profileSampleEstimateFailed
	})
	if failed.Phase != profileSampleEstimateFailed || failed.Error != "encode exploded" {
		t.Fatalf("unexpected failed operation: %#v", failed)
	}
	if len(slot) != 0 {
		t.Fatal("capacity slot was not released after failure")
	}
}

func TestProfileSampleEstimateOperationDoesNotUseRequestContext(t *testing.T) {
	requestContext, cancelRequest := context.WithCancel(context.Background())
	store := newProfileSampleEstimateTestStore()
	slot := make(chan struct{}, 1)
	release := make(chan struct{})
	started := make(chan struct{})
	operation := launchProfileSampleEstimateOperation(store, slot, profileSampleEstimateInput{}, time.Minute, func(ctx context.Context, _ profileSampleEstimateInput, _ func(profileSampleEstimateProgress)) (models.JSONMap, error) {
		close(started)
		select {
		case <-release:
			return models.JSONMap{"ok": true}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	<-started
	cancelRequest()
	if requestContext.Err() != context.Canceled {
		t.Fatal("test request context was not canceled")
	}
	time.Sleep(10 * time.Millisecond)
	if current, _ := store.copy(operation.ID); current.Status != profileSampleEstimateRunning {
		t.Fatalf("operation inherited request cancellation: %#v", current)
	}
	close(release)
	waitForProfileSampleEstimateOperation(t, store, operation.ID, func(item ProfileSampleEstimateOperation) bool {
		return item.Status == profileSampleEstimateCompleted
	})
}

func TestProfileSampleEstimateOperationEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	profileSampleEstimateOperations.Lock()
	profileSampleEstimateOperations.items = map[string]*ProfileSampleEstimateOperation{}
	profileSampleEstimateOperations.cancels = map[string]context.CancelFunc{}
	profileSampleEstimateOperations.Unlock()

	request := httptest.NewRequest(http.MethodPost, "/api/assets/preview/estimate/operations", strings.NewReader(`{"path":""}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	(AssetHandler{}).StartProfileSampleEstimateOperation(ctx)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var started ProfileSampleEstimateOperation
	if err := json.Unmarshal(recorder.Body.Bytes(), &started); err != nil || started.ID == "" {
		t.Fatalf("invalid start response: %v %s", err, recorder.Body.String())
	}
	waitForProfileSampleEstimateOperation(t, &profileSampleEstimateOperations, started.ID, func(item ProfileSampleEstimateOperation) bool {
		return item.Status == profileSampleEstimateFailed
	})

	getRecorder := httptest.NewRecorder()
	getContext, _ := gin.CreateTestContext(getRecorder)
	getContext.Params = gin.Params{{Key: "id", Value: started.ID}}
	(AssetHandler{}).GetProfileSampleEstimateOperation(getContext)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", getRecorder.Code, getRecorder.Body.String())
	}

	unknownRecorder := httptest.NewRecorder()
	unknownContext, _ := gin.CreateTestContext(unknownRecorder)
	unknownContext.Params = gin.Params{{Key: "id", Value: "unknown"}}
	(AssetHandler{}).GetProfileSampleEstimateOperation(unknownContext)
	if unknownRecorder.Code != http.StatusNotFound {
		t.Fatalf("unknown get status = %d", unknownRecorder.Code)
	}

	cancelRecorder := httptest.NewRecorder()
	cancelContext, _ := gin.CreateTestContext(cancelRecorder)
	cancelContext.Params = gin.Params{{Key: "id", Value: started.ID}}
	(AssetHandler{}).CancelProfileSampleEstimateOperation(cancelContext)
	if cancelRecorder.Code != http.StatusConflict {
		t.Fatalf("terminal cancel status = %d", cancelRecorder.Code)
	}
}

func TestSampleEstimateSynchronousEndpointStillReturnsValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/api/assets/preview/estimate", strings.NewReader(`{"path":""}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	(AssetHandler{}).SampleEstimate(ctx)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "readable media path") {
		t.Fatalf("sync response = %d %s", recorder.Code, recorder.Body.String())
	}
}
