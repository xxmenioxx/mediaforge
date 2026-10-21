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
	"sync/atomic"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/airesource"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const lifecycleTestModel = "fixture-model:latest"

type lifecycleTestProvider struct {
	calls atomic.Int32
	err   error
}

func (provider *lifecycleTestProvider) Translate(context.Context, TranslationBatch) (TranslationBatchResult, error) {
	provider.calls.Add(1)
	return TranslationBatchResult{Cues: []TranslatedCue{}}, provider.err
}

type checkpointRecordingTranslationProvider struct {
	db          *gorm.DB
	operationID string
	failAt      int
	failure     error
	calls       int
	phases      []string
	rowVersions []uint64
}

func (provider *checkpointRecordingTranslationProvider) Translate(_ context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
	provider.calls++
	var checkpoint models.TranslationOperationCheckpoint
	if err := provider.db.First(&checkpoint, "operation_id = ?", provider.operationID).Error; err != nil {
		return TranslationBatchResult{}, err
	}
	provider.phases = append(provider.phases, checkpoint.ExecutionPhase)
	provider.rowVersions = append(provider.rowVersions, checkpoint.RowVersion)
	if checkpoint.ExecutionPhase != "INFERENCE_STARTED" || !checkpoint.InferenceStarted {
		return TranslationBatchResult{}, fmt.Errorf("provider request %d observed non-monotonic checkpoint phase %q", provider.calls, checkpoint.ExecutionPhase)
	}
	if provider.calls == provider.failAt {
		if provider.failure != nil {
			return TranslationBatchResult{}, provider.failure
		}
		return TranslationBatchResult{}, errors.New("simulated uncertain provider outcome")
	}
	result := TranslationBatchResult{ContractVersion: batch.ContractVersion}
	for _, cue := range batch.Cues {
		result.Cues = append(result.Cues, TranslatedCue{ID: cue.ID, Text: cue.Text + " translated"})
	}
	return result, nil
}

type admissionRaceStub struct {
	admitFn   func(context.Context, airesource.Request) airesource.Decision
	awaitFn   func(context.Context, airesource.WaitHandle) airesource.Decision
	cancelFn  func(context.Context, airesource.WaitHandle) airesource.Decision
	releaseFn func(context.Context, airesource.Lease) error
}

func (stub *admissionRaceStub) Admit(ctx context.Context, request airesource.Request) airesource.Decision {
	return stub.admitFn(ctx, request)
}

func (stub *admissionRaceStub) Await(ctx context.Context, handle airesource.WaitHandle) airesource.Decision {
	return stub.awaitFn(ctx, handle)
}

func (stub *admissionRaceStub) CancelWait(ctx context.Context, handle airesource.WaitHandle) airesource.Decision {
	return stub.cancelFn(ctx, handle)
}

func (stub *admissionRaceStub) Release(ctx context.Context, lease airesource.Lease) error {
	return stub.releaseFn(ctx, lease)
}

func TestAIModelLifecycleAdmitReleasesGrantedLeaseAfterCancellationRace(t *testing.T) {
	t.Run("cancel wait returns grant", func(t *testing.T) {
		awaitEntered := make(chan struct{})
		resumeAwait := make(chan struct{})
		cancelWaitEntered := make(chan struct{}, 1)
		released := make(chan airesource.Lease, 2)
		grantedLease := new(airesource.Lease)
		admission := &admissionRaceStub{
			admitFn: func(context.Context, airesource.Request) airesource.Decision {
				return airesource.Decision{Status: airesource.Waiting, Wait: new(airesource.WaitHandle)}
			},
			awaitFn: func(context.Context, airesource.WaitHandle) airesource.Decision {
				close(awaitEntered)
				<-resumeAwait
				return airesource.Decision{Status: airesource.Waiting}
			},
			cancelFn: func(context.Context, airesource.WaitHandle) airesource.Decision {
				cancelWaitEntered <- struct{}{}
				return airesource.Decision{Status: airesource.Admitted, Lease: grantedLease}
			},
			releaseFn: func(_ context.Context, lease airesource.Lease) error {
				released <- lease
				return nil
			},
		}
		manager := &aiModelLifecycle{admission: admission, capacityDomain: "nas-test"}
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := manager.admit(ctx, airesource.Request{}, nil)
			result <- err
		}()
		select {
		case <-awaitEntered:
		case <-time.After(2 * time.Second):
			t.Fatal("admission did not reach the Await barrier")
		}
		cancel()
		close(resumeAwait)
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled admission error = %v", err)
		}
		select {
		case <-cancelWaitEntered:
		default:
			t.Fatal("cancellation did not reconcile the wait ticket")
		}
		select {
		case <-released:
		default:
			t.Fatal("ADMITTED lease returned by CancelWait was not released")
		}
		select {
		case <-released:
			t.Fatal("grant lease was released more than once")
		default:
		}
	})

	t.Run("direct admission returns grant after cancellation", func(t *testing.T) {
		admitEntered := make(chan struct{})
		returnGrant := make(chan struct{})
		released := make(chan airesource.Lease, 1)
		admission := &admissionRaceStub{
			admitFn: func(context.Context, airesource.Request) airesource.Decision {
				close(admitEntered)
				<-returnGrant
				return airesource.Decision{Status: airesource.Admitted, Lease: new(airesource.Lease)}
			},
			awaitFn: func(context.Context, airesource.WaitHandle) airesource.Decision {
				t.Fatal("direct ADMITTED result unexpectedly awaited")
				return airesource.Decision{}
			},
			cancelFn: func(context.Context, airesource.WaitHandle) airesource.Decision {
				t.Fatal("direct ADMITTED result unexpectedly cancelled a wait")
				return airesource.Decision{}
			},
			releaseFn: func(_ context.Context, lease airesource.Lease) error {
				released <- lease
				return nil
			},
		}
		manager := &aiModelLifecycle{admission: admission, capacityDomain: "nas-test"}
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := manager.admit(ctx, airesource.Request{}, nil)
			result <- err
		}()
		select {
		case <-admitEntered:
		case <-time.After(2 * time.Second):
			t.Fatal("admission did not reach the grant barrier")
		}
		cancel()
		close(returnGrant)
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled direct admission error = %v", err)
		}
		select {
		case <-released:
		default:
			t.Fatal("direct ADMITTED lease was not released after cancellation")
		}
	})
}

func TestTranslationMultiBatchCheckpointStaysMonotonicAndRecoversFailure(t *testing.T) {
	var apiCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ps" {
			http.NotFound(w, r)
			return
		}
		apiCalls.Add(1)
		_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest"}]}`))
	}))
	defer server.Close()
	db, admission, endpoint, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, lifecycleTestModel, 1, true, true)
	const operationID = "translation-multi-batch-checkpoint"
	defer deleteSubtitleTranslationOperation(operationID)
	configurationRevision := providerConfigurationRevision(endpoint, lifecycleTestModel)
	source := subtitleTranslationSourceResponse{
		Type: "external", SourceIdentity: "external:/fixture/source.en.srt:revision-a",
		SourceRevision: "revision-a", Representation: translationRepresentationSRT,
		ExternalPath: "/fixture/source.en.srt", Codec: "srt", Language: "eng",
	}
	request := subtitleTranslationOperationRequest{
		AssetPath: "/fixture/source.mkv", SourceType: "external", SourceReference: source.ExternalPath,
		Format: translationRepresentationSRT, SourceIdentity: source.SourceIdentity, SourceRevision: source.SourceRevision,
		RequestedSourceLanguage: "en", TargetLocale: "es-419", RequestedRepresentation: translationRepresentationSRT,
		Provider: translationProviderOllama, ProviderModel: lifecycleTestModel,
		ContextMode: "none", ContextSource: "none",
	}
	if err := createTranslationCheckpoint(db, operationID, request, configurationRevision, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := updateTranslationCheckpointPhase(db, operationID, "READY", ""); err != nil {
		t.Fatal(err)
	}
	manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
	providerFailure := errors.New("simulated uncertain provider outcome")
	provider := &checkpointRecordingTranslationProvider{
		db: db, operationID: operationID, failAt: 2, failure: providerFailure,
	}
	var validationCalls atomic.Int32
	validate := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var checkpoint models.TranslationOperationCheckpoint
		if err := db.First(&checkpoint, "operation_id = ?", operationID).Error; err != nil {
			return err
		}
		if checkpoint.SourceIdentity != request.SourceIdentity || checkpoint.SourceRevision != request.SourceRevision ||
			checkpoint.SelectedModel != request.ProviderModel || checkpoint.ProviderConfigurationRevision != configurationRevision {
			return errors.New("source or selected model changed before provider request")
		}
		validationCalls.Add(1)
		return nil
	}
	phaseUpdates := make(chan struct {
		requested string
		durable   string
		err       error
	}, 8)
	onPhase := func(phase, reason string) {
		if phase != "WAITING_FOR_RESOURCES" && phase != "WAITING_FOR_MODEL" && phase != "READY" && phase != "FAILED" {
			return
		}
		err := updateTranslationCheckpointPhase(db, operationID, phase, reason)
		var checkpoint models.TranslationOperationCheckpoint
		if err == nil {
			err = db.First(&checkpoint, "operation_id = ?", operationID).Error
		}
		phaseUpdates <- struct {
			requested string
			durable   string
			err       error
		}{requested: phase, durable: checkpoint.ExecutionPhase, err: err}
	}
	protectedProvider := lifecycleTranslationProvider{
		manager: manager, provider: provider, endpoint: endpoint,
		configurationRevision: configurationRevision, model: lifecycleTestModel,
		operationID: operationID, workflowID: translationAdmissionWorkflowID(request),
		validate: validate,
		markInferenceStarted: func() error {
			return markTranslationInferenceStarted(db, operationID, configurationRevision, lifecycleTestModel)
		},
		onInferenceError: func() {
			markTranslationRecoveryRequired(db, operationID, "provider_inference_outcome_uncertain")
		},
		onPhase: onPhase,
	}
	settings := defaultTranslationSettings()
	settings.MaxCuesPerRequest = 1
	content := []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n\n2\n00:00:02,000 --> 00:00:03,000\nWorld\n")
	var batchCheckpoints []models.TranslationOperationCheckpoint
	_, err := executeTextTranslation(context.Background(), source, content, request, protectedProvider, settings, nil, func(_, _ int) {
		var checkpoint models.TranslationOperationCheckpoint
		if queryErr := db.First(&checkpoint, "operation_id = ?", operationID).Error; queryErr == nil {
			batchCheckpoints = append(batchCheckpoints, checkpoint)
		}
	})
	if !errors.Is(err, providerFailure) {
		t.Fatalf("multi-batch Translation error = %v, want simulated provider failure", err)
	}
	if provider.calls != 2 || len(provider.phases) != 2 || provider.phases[0] != "INFERENCE_STARTED" || provider.phases[1] != "INFERENCE_STARTED" {
		t.Fatalf("provider request checkpoints = calls:%d phases:%v", provider.calls, provider.phases)
	}
	if len(provider.rowVersions) != 2 || provider.rowVersions[1] != provider.rowVersions[0] {
		t.Fatalf("second batch changed the durable inference checkpoint row version: %v", provider.rowVersions)
	}
	if len(batchCheckpoints) != 1 || batchCheckpoints[0].ExecutionPhase != "INFERENCE_STARTED" || !batchCheckpoints[0].InferenceStarted {
		t.Fatalf("checkpoint after first successful batch = %+v", batchCheckpoints)
	}
	if validationCalls.Load() != 2 {
		t.Fatalf("source/model revalidation calls = %d, want one before each request", validationCalls.Load())
	}
	readySnapshots := make([]string, 0, 2)
	for len(phaseUpdates) > 0 {
		update := <-phaseUpdates
		if update.err != nil {
			t.Fatalf("persist lifecycle phase %s: %v", update.requested, update.err)
		}
		if update.requested == "READY" {
			readySnapshots = append(readySnapshots, update.durable)
		}
	}
	if len(readySnapshots) != 2 || readySnapshots[0] != "READY" || readySnapshots[1] != "INFERENCE_STARTED" {
		t.Fatalf("readiness callbacks regressed durable phase across batches: %v", readySnapshots)
	}
	var inferenceAdmissions []models.AIResourceAdmissionRecord
	if err := db.Where("purpose = ?", string(airesource.PurposeInference)).Order("id asc").Find(&inferenceAdmissions).Error; err != nil {
		t.Fatal(err)
	}
	if len(inferenceAdmissions) != 2 || inferenceAdmissions[0].RequestID == inferenceAdmissions[1].RequestID {
		t.Fatalf("per-batch inference admissions = %+v", inferenceAdmissions)
	}
	for _, admissionRecord := range inferenceAdmissions {
		if admissionRecord.State != "RELEASED" {
			t.Fatalf("inference lease was not released after its provider request: %+v", admissionRecord)
		}
	}
	var checkpoint models.TranslationOperationCheckpoint
	if err := db.First(&checkpoint, "operation_id = ?", operationID).Error; err != nil {
		t.Fatal(err)
	}
	if checkpoint.ExecutionPhase != "RECOVERY_REQUIRED" || !checkpoint.InferenceStarted {
		t.Fatalf("uncertain second request checkpoint = %+v", checkpoint)
	}
	handler := AssetHandler{db: db, aiModelLifecycle: &AIModelLifecycle{manager: manager}}
	handler.recoverProtectedTranslationOperations()
	if provider.calls != 2 {
		t.Fatalf("restart recovery replayed an uncertain provider request: calls=%d", provider.calls)
	}
	subtitleExtractionOperations.RLock()
	recovered := subtitleExtractionOperations.items[operationID]
	subtitleExtractionOperations.RUnlock()
	if recovered == nil || recovered.Status != "error" || recovered.Phase != "recovery_required" {
		t.Fatalf("uncertain multi-batch operation recovery = %+v", recovered)
	}
	if apiCalls.Load() < 2 {
		t.Fatalf("residency was not independently observed for each batch: calls=%d", apiCalls.Load())
	}
}

func newLifecycleResourceFixture(t *testing.T, model string, maxInferences int, enabled, resident bool) (*gorm.DB, *airesource.Service, string, string) {
	t.Helper()
	endpoint := "http://ollama.lifecycle.test"
	providerID := ollamaProviderConfigurationID(endpoint)
	settings := airesource.DefaultSettings()
	settings.Enabled = enabled
	settings.MaxResidentModels = 1
	settings.MaxConcurrentInferences = maxInferences
	settings.MaxConcurrentModelLoads = 1
	settings.ModelEstimates = []airesource.ModelEstimate{{
		ProviderConfigurationID: providerID, ModelID: model,
		ResidentBytes: 1 << 30, LoadPeakAdditionalBytes: 1 << 30, InferenceBaseBytes: 512 << 20,
		KVCacheBytesPerToken: 1024, DefaultContextTokens: 1024, MaximumContextTokens: 8192,
		Source: "benchmark", Revision: "fixture-v1",
	}}
	encodedSettings, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	value := models.JSONMap{}
	if err := json.Unmarshal(encodedSettings, &value); err != nil {
		t.Fatal(err)
	}
	value, err = airesource.NormalizeSettingsValue(value)
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:ai-model-lifecycle-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.AppSetting{}, &models.AIResourceAdmissionRecord{}, &models.AIResourceAdmissionCounter{}, &models.TranslationOperationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: airesource.SettingsKey, Value: value}).Error; err != nil {
		t.Fatal(err)
	}
	service := airesource.NewService(db)
	if resident {
		publishLifecycleFixtureObservation(t, service, endpoint, model, true, airesource.GuardAllow)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, service, endpoint, providerID
}

func publishLifecycleFixtureObservation(t *testing.T, service *airesource.Service, endpoint, model string, resident bool, guard airesource.GuardStatus) {
	t.Helper()
	publishLifecycleFixtureObservationAt(t, service, endpoint, model, resident, guard, time.Now().UTC())
}

func publishLifecycleFixtureObservationAt(t *testing.T, service *airesource.Service, endpoint, model string, resident bool, guard airesource.GuardStatus, observedAt time.Time) {
	t.Helper()
	providerModels := []airesource.ResidentModel{}
	if resident {
		providerModels = append(providerModels, airesource.ResidentModel{ProviderConfigurationID: ollamaProviderConfigurationID(endpoint), ModelID: model})
	}
	if err := service.PublishObservation(context.Background(), airesource.Observation{
		CapacityDomain: "nas-a",
		Host: airesource.HostObservation{
			AvailableMemoryKnown: true, TotalMemoryBytes: 64 << 30, AvailableMemoryBytes: 48 << 30,
			ObservedAt: observedAt, Metrics: map[string]airesource.MetricObservation{},
		},
		Provider: airesource.ProviderObservation{Available: true, ObservedAt: observedAt, ResidentModels: providerModels},
		Critical: airesource.CriticalServiceObservation{Source: "test_fixture", Status: guard, ObservedAt: observedAt},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAIModelLifecycleAdmissionOutcomesFailClosed(t *testing.T) {
	t.Run("denied model load", func(t *testing.T) {
		var warmups atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/ps":
				_, _ = w.Write([]byte(`{"models":[]}`))
			case "/api/tags":
				_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest"}]}`))
			case "/api/generate":
				warmups.Add(1)
				_, _ = w.Write([]byte(`{"done":true}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, lifecycleTestModel, 1, false, false)
		manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
		err := manager.EnsureReady(context.Background(), "translation-denied", server.URL, providerConfigurationRevision(server.URL, lifecycleTestModel), lifecycleTestModel, nil)
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("expected denied admission, got %v", err)
		}
		if warmups.Load() != 0 {
			t.Fatal("denied model-load admission issued /api/generate")
		}
	})

	t.Run("stale telemetry", func(t *testing.T) {
		var warmups atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/ps":
				_, _ = w.Write([]byte(`{"models":[]}`))
			case "/api/tags":
				_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest"}]}`))
			case "/api/generate":
				warmups.Add(1)
				_, _ = w.Write([]byte(`{"done":true}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, lifecycleTestModel, 1, true, false)
		publishLifecycleFixtureObservationAt(t, admission, server.URL, lifecycleTestModel, false, airesource.GuardAllow, time.Now().Add(-time.Hour))
		manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
		err := manager.EnsureReady(context.Background(), "translation-stale", server.URL, providerConfigurationRevision(server.URL, lifecycleTestModel), lifecycleTestModel, nil)
		if err == nil || !errors.Is(err, errResourceAdmissionUnavailable) {
			t.Fatalf("expected unavailable admission for stale telemetry, got %v", err)
		}
		if warmups.Load() != 0 {
			t.Fatal("stale telemetry issued /api/generate")
		}
	})

	t.Run("critical service pressure", func(t *testing.T) {
		var warmups atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/ps":
				_, _ = w.Write([]byte(`{"models":[]}`))
			case "/api/tags":
				_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest"}]}`))
			case "/api/generate":
				warmups.Add(1)
				_, _ = w.Write([]byte(`{"done":true}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, lifecycleTestModel, 1, true, false)
		publishLifecycleFixtureObservation(t, admission, server.URL, lifecycleTestModel, false, airesource.GuardPressure)
		manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
		ctx, cancel := context.WithCancel(context.Background())
		waiting := make(chan struct{}, 1)
		result := make(chan error, 1)
		go func() {
			result <- manager.EnsureReady(ctx, "translation-guard", server.URL, providerConfigurationRevision(server.URL, lifecycleTestModel), lifecycleTestModel, func(phase, message string) {
				if phase == "WAITING_FOR_RESOURCES" && strings.Contains(message, "critical_service_pressure") {
					select {
					case waiting <- struct{}{}:
					default:
					}
				}
			})
		}()
		select {
		case <-waiting:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("critical-service pressure did not produce a resource wait")
		}
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled critical-service wait error = %v", err)
		}
		if warmups.Load() != 0 {
			t.Fatal("critical-service pressure issued /api/generate")
		}
	})
}

func TestAIModelLifecycleDoesNotDownloadMissingModelsOrIgnoreProviderOffline(t *testing.T) {
	for _, test := range []struct {
		name      string
		psStatus  int
		tagModels string
		wantErr   string
	}{
		{name: "model not installed", tagModels: `{"models":[{"name":"different-model:latest"}]}`, wantErr: "not installed"},
		{name: "provider offline", psStatus: http.StatusServiceUnavailable, tagModels: `{"models":[{"name":"fixture-model:latest"}]}`, wantErr: "residency"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var warmups atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/ps":
					if test.psStatus != 0 {
						w.WriteHeader(test.psStatus)
						return
					}
					_, _ = w.Write([]byte(`{"models":[]}`))
				case "/api/tags":
					_, _ = w.Write([]byte(test.tagModels))
				case "/api/generate":
					warmups.Add(1)
					_, _ = w.Write([]byte(`{"done":true}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, lifecycleTestModel, 1, true, false)
			manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
			err := manager.EnsureReady(context.Background(), "translation-provider-state", server.URL, providerConfigurationRevision(server.URL, lifecycleTestModel), lifecycleTestModel, nil)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected %q error, got %v", test.wantErr, err)
			}
			if warmups.Load() != 0 {
				t.Fatal("provider state failure issued /api/generate")
			}
		})
	}
}

func TestAIModelRuntimeStatusReportsResidencyWaitsAndFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest"}]}`))
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest","model":"fixture-model:latest","expires_at":"2030-01-01T00:00:00Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, lifecycleTestModel, 1, true, false)
	for _, row := range []models.TranslationOperationCheckpoint{
		{OperationID: "waiting-runtime", SelectedModel: lifecycleTestModel, ExecutionPhase: "WAITING_FOR_RESOURCES"},
		{OperationID: "failed-runtime", SelectedModel: lifecycleTestModel, ExecutionPhase: "FAILED"},
		{OperationID: "recovery-runtime", SelectedModel: lifecycleTestModel, ExecutionPhase: "RECOVERY_REQUIRED"},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	manager := newAIModelLifecycle(db, admission, server.Client(), false, "nas-a")
	status := manager.runtimeStatus(context.Background(), server.URL)
	if status.State != "available" || status.ObservedAt == "" {
		t.Fatalf("runtime observation = %+v", status)
	}
	if len(status.Models) != 1 || !status.Models[0].Installed || !status.Models[0].Resident || status.Models[0].State != "IDLE" || status.Models[0].ExpiresAt == "" {
		t.Fatalf("installed/resident model projection = %+v", status.Models)
	}
	if len(status.Operations) != 1 || status.Operations[0].OperationID != "waiting-runtime" {
		t.Fatalf("active lifecycle operations = %+v", status.Operations)
	}
	if len(status.Failures) != 2 {
		t.Fatalf("bounded lifecycle failures = %+v", status.Failures)
	}
}

func TestAIModelLifecycleLastWaiterCancellationReconcilesBeforeRetry(t *testing.T) {
	var resident atomic.Bool
	var warmups atomic.Int32
	var psCalls atomic.Int32
	started := make(chan struct{}, 1)
	warmupRelease := make(chan struct{})
	warmupFinished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/ps":
			psCalls.Add(1)
			if resident.Load() {
				_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest","model":"fixture-model:latest"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"models":[]}`))
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/generate":
			warmups.Add(1)
			started <- struct{}{}
			<-warmupRelease
			_, _ = w.Write([]byte(`{"done":true}`))
			close(warmupFinished)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, lifecycleTestModel, 1, true, false)
	publishLifecycleFixtureObservation(t, admission, server.URL, lifecycleTestModel, false, airesource.GuardAllow)
	manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
	revision := providerConfigurationRevision(server.URL, lifecycleTestModel)
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		first <- manager.EnsureReady(ctx, "translation-cancel-last", server.URL, revision, lifecycleTestModel, nil)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		cancel()
		close(warmupRelease)
		t.Fatal("warmup did not start")
	}
	resident.Store(true)
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("last waiter cancellation = %v", err)
	}
	close(warmupRelease)
	<-warmupFinished
	if err := manager.EnsureReady(context.Background(), "translation-retry-new-operation", server.URL, revision, lifecycleTestModel, nil); err != nil {
		t.Fatalf("safe retry after residency reconciliation: %v", err)
	}
	if psCalls.Load() < 2 {
		t.Fatalf("/api/ps calls = %d, want cancellation reconciliation", psCalls.Load())
	}
	if warmups.Load() != 1 {
		t.Fatalf("warmup retries = %d, want no duplicate warmup for confirmed residency", warmups.Load())
	}
}

func TestOllamaLifecycleWarmupContractAndPSResidency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	generated := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/generate":
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode warmup body: %v", err)
			}
			if _, ok := body["prompt"]; ok {
				t.Error("warmup unexpectedly sent a prompt")
			}
			if len(body) != 3 {
				t.Errorf("warmup fields = %v, want exactly model, stream, keep_alive", body)
			}
			var model string
			var stream bool
			var keepAlive string
			_ = json.Unmarshal(body["model"], &model)
			_ = json.Unmarshal(body["stream"], &stream)
			_ = json.Unmarshal(body["keep_alive"], &keepAlive)
			if model != lifecycleTestModel || stream || keepAlive != "30m" {
				t.Errorf("warmup request model=%q stream=%v keep_alive=%q", model, stream, keepAlive)
			}
			generated = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"done":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/ps":
			if generated {
				_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest","model":"fixture-model:latest","expires_at":"2030-01-01T00:00:00Z"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"models":[]}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := ollamaProviderAPIClient{client: server.Client()}
	if err := client.warmModel(context.Background(), server.URL, lifecycleTestModel); err != nil {
		t.Fatal(err)
	}
	residents, err := client.loadedModels(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resident, _ := modelIsResident(residents, lifecycleTestModel); !resident {
		t.Fatalf("/api/ps did not report the exact selected model: %+v", residents)
	}
}

func TestOllamaPSDistinguishesValidEmptyFromMalformedResidency(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want int
	}{
		{name: "valid empty list", body: `{"models":[]}`, want: 0},
		{name: "missing list", body: `{}`, want: -1},
		{name: "null list", body: `{"models":null}`, want: -1},
		{name: "identity missing", body: `{"models":[{}]}`, want: -1},
		{name: "malformed JSON", body: `not-json`, want: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			residents, err := (ollamaProviderAPIClient{client: server.Client()}).loadedModels(context.Background(), server.URL)
			if test.want < 0 {
				if err == nil {
					t.Fatalf("malformed residency accepted: %+v", residents)
				}
				return
			}
			if err != nil || len(residents) != test.want {
				t.Fatalf("valid empty residency = %+v, err=%v", residents, err)
			}
		})
	}
}

func TestAIModelLifecycleSingleflightAndIndependentWaiterCancellation(t *testing.T) {
	model := lifecycleTestModel
	var endpoint string
	var resident atomic.Bool
	var warmups atomic.Int32
	warmupStarted := make(chan struct{}, 1)
	warmupRelease := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/ps":
			if resident.Load() {
				_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest","model":"fixture-model:latest","expires_at":"2030-01-01T00:00:00Z"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"models":[]}`))
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest","model":"fixture-model:latest"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/generate":
			warmups.Add(1)
			warmupStarted <- struct{}{}
			<-warmupRelease
			resident.Store(true)
			_, _ = w.Write([]byte(`{"done":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	endpoint = server.URL
	db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, endpoint, model, 2, true, false)
	publishLifecycleFixtureObservation(t, admission, endpoint, model, false, airesource.GuardAllow)
	manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
	revision := providerConfigurationRevision(endpoint, model)

	ctx1, cancel1 := context.WithCancel(context.Background())
	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() { first <- manager.EnsureReady(ctx1, "translation-1", endpoint, revision, model, nil) }()
	select {
	case <-warmupStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("warmup did not begin")
	}
	go func() {
		second <- manager.EnsureReady(context.Background(), "translation-2", endpoint, revision, model, nil)
	}()
	key := lifecycleKey(revision, model)
	deadline := time.Now().Add(2 * time.Second)
	for {
		manager.mu.Lock()
		flight := manager.flights[key]
		waiters := 0
		if flight != nil {
			waiters = len(flight.waiters)
		}
		manager.mu.Unlock()
		if waiters == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("singleflight did not attach both waiters; observed %d", waiters)
		}
		time.Sleep(time.Millisecond)
	}
	cancel1()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter error = %v", err)
	}
	close(warmupRelease)
	if err := <-second; err != nil {
		t.Fatalf("remaining waiter did not become ready: %v", err)
	}
	if got := warmups.Load(); got != 1 {
		t.Fatalf("warmup requests = %d, want 1", got)
	}
	var released int64
	if err := db.Model(&models.AIResourceAdmissionRecord{}).Where("purpose = ? AND state = ?", string(airesource.PurposeModelLoad), "RELEASED").Count(&released).Error; err != nil {
		t.Fatal(err)
	}
	if released != 1 {
		var records []models.AIResourceAdmissionRecord
		if err := db.Select("purpose", "state", "reason_code", "operation_deadline", "created_at", "updated_at").Find(&records).Error; err != nil {
			t.Fatal(err)
		}
		t.Fatalf("released model-load leases = %d, want 1; records=%+v", released, records)
	}
}

func TestAIModelLifecycleFailsClosedWithoutFreshAdmissionObservation(t *testing.T) {
	var warmups atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest"}]}`))
		case "/api/generate":
			warmups.Add(1)
			_, _ = w.Write([]byte(`{"done":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, lifecycleTestModel, 1, true, false)
	manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
	if err := manager.EnsureReady(context.Background(), "translation-unavailable", server.URL, providerConfigurationRevision(server.URL, lifecycleTestModel), lifecycleTestModel, nil); err == nil || !strings.Contains(err.Error(), "admission") {
		t.Fatalf("expected fail-closed admission error, got %v", err)
	}
	if got := warmups.Load(); got != 0 {
		t.Fatalf("warmup requests without an observation = %d, want 0", got)
	}
}

func TestAIModelLifecycleRequiresInferenceAdmissionForEveryCall(t *testing.T) {
	model := lifecycleTestModel
	var psCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/ps" {
			psCalls.Add(1)
			_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest","model":"fixture-model:latest"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, model, 2, true, true)
	publishLifecycleFixtureObservation(t, admission, server.URL, model, true, airesource.GuardAllow)
	manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
	provider := &lifecycleTestProvider{}
	revision := providerConfigurationRevision(server.URL, model)
	wrapper := lifecycleTranslationProvider{
		manager: manager, provider: provider, endpoint: server.URL, configurationRevision: revision,
		model: model, operationID: "translation-op", workflowID: "workflow-stable",
		markInferenceStarted: func() error { return nil },
	}
	for i := 0; i < 2; i++ {
		if _, err := wrapper.Translate(context.Background(), TranslationBatch{}); err != nil {
			t.Fatalf("inference %d: %v", i+1, err)
		}
	}
	if got := provider.calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want 2", got)
	}
	var inferenceLeases int64
	if err := db.Model(&models.AIResourceAdmissionRecord{}).Where("purpose = ? AND state = ?", string(airesource.PurposeInference), "RELEASED").Count(&inferenceLeases).Error; err != nil {
		t.Fatal(err)
	}
	if inferenceLeases != 2 {
		t.Fatalf("released inference admission leases = %d, want 2", inferenceLeases)
	}
	if psCalls.Load() < 2 {
		t.Fatalf("residency checks = %d, want a check before each inference", psCalls.Load())
	}
}

func TestAIModelLifecycleWaitCancellationAndAdmissionRelease(t *testing.T) {
	db, admission, _, providerID := newLifecycleResourceFixture(t, lifecycleTestModel, 1, true, true)
	now := time.Now().UTC()
	first := admission.Admit(context.Background(), airesource.Request{
		CapacityDomain: "nas-a", ProviderConfigurationID: providerID, ModelID: lifecycleTestModel,
		ConsumerID: "translation-test", WorkflowID: "stable-workflow", RequestID: "first-inference",
		Purpose: airesource.PurposeInference, OperationDeadline: now.Add(time.Minute),
	})
	if first.Status != airesource.Admitted || first.Lease == nil {
		t.Fatalf("first inference admission = %+v", first)
	}
	manager := newAIModelLifecycle(db, admission, nil, true, "nas-a")
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() {
		_, err := manager.admit(ctx, airesource.Request{
			CapacityDomain: "nas-a", ProviderConfigurationID: providerID, ModelID: lifecycleTestModel,
			ConsumerID: "translation-test", WorkflowID: "stable-workflow", RequestID: "second-inference",
			Purpose: airesource.PurposeInference, OperationDeadline: time.Now().Add(time.Minute),
		}, func(string) { waiting <- struct{}{} })
		result <- err
	}()
	select {
	case <-waiting:
	case <-time.After(2 * time.Second):
		t.Fatal("second inference did not wait for the concurrency lease")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled resource waiter error = %v", err)
	}
	var cancelledWaits int64
	if err := db.Model(&models.AIResourceAdmissionRecord{}).Where("request_id = ? AND state = ?", "second-inference", "CANCELLED").Count(&cancelledWaits).Error; err != nil {
		t.Fatal(err)
	}
	if cancelledWaits != 1 {
		t.Fatalf("cancelled wait records = %d, want 1", cancelledWaits)
	}
	if err := admission.Release(context.Background(), *first.Lease); err != nil {
		t.Fatalf("release inference lease: %v", err)
	}
	if err := admission.Release(context.Background(), *first.Lease); err != nil {
		t.Fatalf("idempotent release inference lease: %v", err)
	}
}

func TestAIModelLifecycleReadinessDeadlineIsBounded(t *testing.T) {
	model := lifecycleTestModel
	var warmups atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest"}]}`))
		case "/api/generate":
			warmups.Add(1)
			_, _ = w.Write([]byte(`{"done":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, model, 1, true, false)
	publishLifecycleFixtureObservation(t, admission, server.URL, model, false, airesource.GuardAllow)
	manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
	manager.readinessDeadline = 25 * time.Millisecond
	manager.readinessPoll = time.Millisecond
	started := time.Now()
	err := manager.EnsureReady(context.Background(), "translation-timeout", server.URL, providerConfigurationRevision(server.URL, model), model, nil)
	if err == nil || !strings.Contains(err.Error(), "readiness deadline") {
		t.Fatalf("readiness error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("readiness exceeded test deadline: %v", elapsed)
	}
	if warmups.Load() != 1 {
		t.Fatalf("warmup attempts = %d, want one bounded attempt", warmups.Load())
	}
}

func TestTranslationCheckpointRecoveryFailsClosedAfterUncertainInference(t *testing.T) {
	db, _, _, _ := newLifecycleResourceFixture(t, lifecycleTestModel, 1, true, false)
	checkpoint := models.TranslationOperationCheckpoint{
		OperationID: "original-translation-operation", SourceIdentity: "asset:identity", SourceRevision: "source-revision",
		ProviderConfigurationRevision: "provider-revision", SelectedModel: lifecycleTestModel,
		AdmissionWorkflowID: "stable-workflow", ExecutionPhase: "INFERENCE_STARTED", InferenceStarted: true,
		Deadline: time.Now().Add(time.Hour), CheckpointVersion: 1, RequestMetadata: "{}",
	}
	if err := db.Create(&checkpoint).Error; err != nil {
		t.Fatal(err)
	}
	handler := NewAssetHandler(db)
	handler.recoverProtectedTranslationOperations()
	var recovered models.TranslationOperationCheckpoint
	if err := db.First(&recovered, "operation_id = ?", checkpoint.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.ExecutionPhase != "RECOVERY_REQUIRED" {
		t.Fatalf("recovered phase = %q, want RECOVERY_REQUIRED", recovered.ExecutionPhase)
	}
	subtitleExtractionOperations.RLock()
	operation := subtitleExtractionOperations.items[checkpoint.OperationID]
	subtitleExtractionOperations.RUnlock()
	if operation == nil || operation.Status != "error" || operation.Phase != "recovery_required" {
		t.Fatalf("uncertain inference was not restored as non-resumable: %+v", operation)
	}
}

func TestTranslationWaitingRecoveryReconstructsSameOperationAndWaitsWithoutBypass(t *testing.T) {
	var generateCalls atomic.Int32
	var chatCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"fixture-model:latest"}]}`))
		case "/api/generate":
			generateCalls.Add(1)
			_, _ = w.Write([]byte(`{"done":true}`))
		case "/api/chat":
			chatCalls.Add(1)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	db, admission, _, _ := newLifecycleResourceFixtureWithEndpoint(t, server.URL, lifecycleTestModel, 1, true, false)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "source.mkv")
	subtitlePath := filepath.Join(root, "source.en.srt")
	if err := os.WriteFile(mediaPath, []byte("fixture media locator"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subtitlePath, []byte("1\n00:00:00,000 --> 00:00:01,000\nHello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := models.JSONMap{"rawRoot": root}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: paths}).Error; err != nil {
		t.Fatal(err)
	}
	translationConfig := defaultTranslationSettings()
	translationConfig.Provider = translationProviderOllama
	translationConfig.OllamaModel = lifecycleTestModel
	translationConfig.Enabled = true
	translationConfig.Readiness = translationReadinessUntested
	settings := translationSettingsValue(translationConfig)
	if err := db.Create(&models.AppSetting{Key: translationSettingsKey, Value: settings}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: ollamaProviderSettingsKey, Value: models.JSONMap{"endpoint": server.URL}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Library{}, &models.AssetRecord{}); err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Translation Fixture", SourcePath: root, DestinationPath: root, Type: "movies"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{
		Path: mediaPath, RootPath: root, RelativePath: filepath.Base(mediaPath),
		FileName: filepath.Base(mediaPath), LibraryID: library.ID, LibraryName: library.Name, Status: "accepted",
	}).Error; err != nil {
		t.Fatal(err)
	}
	baseHandler := AssetHandler{db: db}
	request := subtitleTranslationOperationRequest{
		AssetPath: mediaPath, SourceType: "external", SourceReference: subtitlePath,
		RequestedSourceLanguage: "en", TargetLocale: "es-419", RequestedRepresentation: "srt",
		Provider: translationProviderOllama, ProviderModel: lifecycleTestModel,
		ContextMode: "none", ContextSource: "none",
	}
	_, source, _, err := baseHandler.resolveTranslationExecutionSource(context.Background(), request)
	if err != nil {
		t.Fatalf("resolve fixture Translation source: %v", err)
	}
	request.Format = source.Representation
	request.SourceIdentity, request.SourceRevision = source.SourceIdentity, source.SourceRevision
	if err := validateSubtitleTranslationOperationRequest(request); err != nil {
		t.Fatal(err)
	}
	if err := MigrateOllamaProviderSettings(db); err != nil {
		t.Fatalf("migrate shared provider fixture: %v", err)
	}
	currentSettings := loadTranslationSettings(db)
	configurationRevision := providerConfigurationRevision(currentSettings.OllamaEndpoint, currentSettings.OllamaModel)
	operationID := "durable-wait-original-operation"
	checkpoint := models.TranslationOperationCheckpoint{
		OperationID: operationID, SourceIdentity: request.SourceIdentity, SourceRevision: request.SourceRevision,
		ProviderConfigurationRevision: configurationRevision, SelectedModel: lifecycleTestModel,
		AdmissionWorkflowID: translationAdmissionWorkflowID(request), ExecutionPhase: "WAITING_FOR_RESOURCES",
		Deadline: time.Now().Add(time.Hour), CheckpointVersion: 1, RowVersion: 1,
	}
	if err := createTranslationCheckpoint(db, operationID, request, configurationRevision, checkpoint.Deadline); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&checkpoint).Where("operation_id = ?", operationID).Updates(map[string]any{"execution_phase": "WAITING_FOR_RESOURCES", "admission_workflow_id": checkpoint.AdmissionWorkflowID}).Error; err != nil {
		t.Fatal(err)
	}
	manager := newAIModelLifecycle(db, admission, server.Client(), true, "nas-a")
	handler := AssetHandler{db: db, aiModelLifecycle: &AIModelLifecycle{manager: manager}, ollamaHTTPClient: server.Client()}
	handler.recoverProtectedTranslationOperations()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var recovered models.TranslationOperationCheckpoint
		if err := db.First(&recovered, "operation_id = ?", operationID).Error; err != nil {
			t.Fatal(err)
		}
		if recovered.ExecutionPhase == "WAITING_FOR_RESOURCES" && recovered.RowVersion > checkpoint.RowVersion {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}
	subtitleExtractionOperations.RLock()
	recoveredOperation := subtitleExtractionOperations.items[operationID]
	subtitleExtractionOperations.RUnlock()
	if recoveredOperation == nil || recoveredOperation.ID != operationID || recoveredOperation.Status != "running" || recoveredOperation.Phase != "waiting_for_resources" {
		currentSettings := loadTranslationSettings(db)
		var storedCheckpoint models.TranslationOperationCheckpoint
		_ = db.First(&storedCheckpoint, "operation_id = ?", operationID).Error
		t.Fatalf("original waiting operation was not restored: %+v; settings=%+v stored_checkpoint=%+v config_validation=%v", recoveredOperation, currentSettings, storedCheckpoint, validateTranslationProviderCheckpoint(currentSettings, storedCheckpoint))
	}
	if generateCalls.Load() != 0 || chatCalls.Load() != 0 {
		t.Fatalf("unavailable admission bypassed: generate=%d chat=%d", generateCalls.Load(), chatCalls.Load())
	}
	if err := markTranslationCancelled(db, operationID); err != nil {
		t.Fatal(err)
	}
	if recoveredOperation.cancel != nil {
		recoveredOperation.cancel()
	}
	for time.Now().Before(deadline) {
		subtitleExtractionOperations.RLock()
		status := subtitleExtractionOperations.items[operationID].Status
		subtitleExtractionOperations.RUnlock()
		if status == "cancelled" {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}
	subtitleExtractionOperations.RLock()
	status := subtitleExtractionOperations.items[operationID].Status
	subtitleExtractionOperations.RUnlock()
	if status != "cancelled" {
		t.Fatalf("cancelled recovered operation status = %q", status)
	}
}

func TestTranslationPublicationCrashReconciliationUsesFingerprints(t *testing.T) {
	db, _, _, _ := newLifecycleResourceFixture(t, lifecycleTestModel, 1, true, false)
	root := t.TempDir()
	finalPath := filepath.Join(root, "translated.srt")
	content := []byte("1\n00:00:00,000 --> 00:00:01,000\ntranslated\n")
	if err := os.WriteFile(finalPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	fingerprint := sha256Bytes(content)
	manifest, err := json.Marshal(translationPublicationManifest{Version: 1, Files: []translationPublicationFileEvidence{{
		Path: finalPath, Format: "srt", ExpectedSHA256: fingerprint,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := models.TranslationOperationCheckpoint{
		OperationID: "publication-crash", SourceIdentity: "asset:identity", SourceRevision: "source-revision",
		ProviderConfigurationRevision: "provider-revision", SelectedModel: lifecycleTestModel,
		AdmissionWorkflowID: "stable-workflow", ExecutionPhase: "PUBLISHING", InferenceStarted: true,
		Deadline: time.Now().Add(time.Hour), CheckpointVersion: 1, RequestMetadata: "{}", PublicationIntent: string(manifest),
	}
	if err := db.Create(&checkpoint).Error; err != nil {
		t.Fatal(err)
	}
	if !reconcileTranslationPublication(db, &checkpoint) {
		t.Fatal("matching final output was not reconciled to PUBLICATION_COMMITTED")
	}
	var committed models.TranslationOperationCheckpoint
	if err := db.First(&committed, "operation_id = ?", checkpoint.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	if committed.ExecutionPhase != "PUBLICATION_COMMITTED" {
		t.Fatalf("publication phase = %q", committed.ExecutionPhase)
	}
	if err := os.WriteFile(finalPath, []byte("unexpected edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if reconcileTranslationPublication(db, &committed) {
		t.Fatal("changed output was accepted as a committed publication")
	}
	forceTranslationRecoveryRequired(db, checkpoint.OperationID, "publication_mismatch")
	actual, err := os.ReadFile(finalPath)
	if err != nil || string(actual) != "unexpected edit" {
		t.Fatal("ambiguous publication recovery modified the destination")
	}
}

func TestTranslationCheckpointMetadataDoesNotStoreProviderPayloadOrEndpoint(t *testing.T) {
	metadata, err := checkpointMetadata(subtitleTranslationOperationRequest{
		AssetPath: "/media/source.mkv", SourceType: "external", Format: "srt", SourceIdentity: "asset:identity",
		SourceReference: "/media/source.srt", RequestedSourceLanguage: "en", TargetLocale: "es-419",
		RequestedRepresentation: "srt", Provider: translationProviderOllama, ProviderModel: lifecycleTestModel,
		ContextMode: "none", ContextSource: "none", SourceRevision: "revision-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"prompt", "cue", "translation text", "credential", "http://", "provider response"} {
		if strings.Contains(strings.ToLower(metadata), forbidden) {
			t.Fatalf("checkpoint metadata unexpectedly contains %q: %s", forbidden, metadata)
		}
	}
}

func TestTranslationWaitingCheckpointValidationFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	request := subtitleTranslationOperationRequest{
		AssetPath: "/media/source.mkv", SourceType: "external", Format: "srt", SourceIdentity: "asset:identity",
		SourceReference: "/media/source.srt", RequestedSourceLanguage: "en", TargetLocale: "es-419",
		RequestedRepresentation: "srt", Provider: translationProviderOllama, ProviderModel: lifecycleTestModel,
		ContextMode: "none", ContextSource: "none", SourceRevision: "source-revision",
	}
	checkpoint := models.TranslationOperationCheckpoint{
		OperationID: "original-operation", SourceIdentity: request.SourceIdentity, SourceRevision: request.SourceRevision,
		ProviderConfigurationRevision: "provider-revision", SelectedModel: lifecycleTestModel,
		AdmissionWorkflowID: "stable-workflow", ExecutionPhase: "WAITING_FOR_RESOURCES",
		Deadline: now.Add(time.Minute), CheckpointVersion: 1, RowVersion: 1,
	}
	if err := validateTranslationWaitingCheckpoint(checkpoint, request, now); err != nil {
		t.Fatalf("complete waiting checkpoint rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*models.TranslationOperationCheckpoint, *subtitleTranslationOperationRequest)
	}{
		{name: "unsupported version", mutate: func(row *models.TranslationOperationCheckpoint, _ *subtitleTranslationOperationRequest) {
			row.CheckpointVersion = 2
		}},
		{name: "cancelled", mutate: func(row *models.TranslationOperationCheckpoint, _ *subtitleTranslationOperationRequest) {
			row.Cancelled = true
		}},
		{name: "inference already started", mutate: func(row *models.TranslationOperationCheckpoint, _ *subtitleTranslationOperationRequest) {
			row.InferenceStarted = true
		}},
		{name: "expired deadline", mutate: func(row *models.TranslationOperationCheckpoint, _ *subtitleTranslationOperationRequest) {
			row.Deadline = now.Add(-time.Second)
		}},
		{name: "unknown phase", mutate: func(row *models.TranslationOperationCheckpoint, _ *subtitleTranslationOperationRequest) {
			row.ExecutionPhase = "PREPARING"
		}},
		{name: "changed model", mutate: func(_ *models.TranslationOperationCheckpoint, req *subtitleTranslationOperationRequest) {
			req.ProviderModel = "changed-model"
		}},
		{name: "changed source identity", mutate: func(_ *models.TranslationOperationCheckpoint, req *subtitleTranslationOperationRequest) {
			req.SourceIdentity = "different-asset"
		}},
		{name: "changed source revision", mutate: func(_ *models.TranslationOperationCheckpoint, req *subtitleTranslationOperationRequest) {
			req.SourceRevision = "different-revision"
		}},
		{name: "missing operation identity", mutate: func(row *models.TranslationOperationCheckpoint, _ *subtitleTranslationOperationRequest) {
			row.OperationID = ""
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			row, req := checkpoint, request
			test.mutate(&row, &req)
			if err := validateTranslationWaitingCheckpoint(row, req, now); err == nil {
				t.Fatal("invalid waiting checkpoint was accepted")
			}
		})
	}
	if err := validateTranslationProviderCheckpoint(translationSettings{Provider: translationProviderOllama, OllamaEndpoint: "http://ollama.current", OllamaModel: lifecycleTestModel}, checkpoint); err == nil {
		t.Fatal("changed provider endpoint/configuration revision was accepted")
	}
	if _, err := decodeTranslationCheckpointRequest(models.TranslationOperationCheckpoint{RequestMetadata: `{"unexpected":"field"}`}); err == nil {
		t.Fatal("unknown checkpoint metadata field was accepted")
	}
}

func TestTranslationInferenceRequiresDurableReadyPhaseAndCancellationWins(t *testing.T) {
	db, _, _, _ := newLifecycleResourceFixture(t, lifecycleTestModel, 1, true, false)
	checkpoint := models.TranslationOperationCheckpoint{
		OperationID: "ready-phase-cas", SourceIdentity: "asset:identity", SourceRevision: "source-revision",
		ProviderConfigurationRevision: "provider-revision", SelectedModel: lifecycleTestModel,
		AdmissionWorkflowID: "stable-workflow", ExecutionPhase: "PREPARING", Deadline: time.Now().Add(time.Hour),
		CheckpointVersion: 1, RowVersion: 1, RequestMetadata: "{}",
	}
	if err := db.Create(&checkpoint).Error; err != nil {
		t.Fatal(err)
	}
	if err := markTranslationInferenceStarted(db, checkpoint.OperationID, checkpoint.ProviderConfigurationRevision, checkpoint.SelectedModel); err == nil {
		t.Fatal("inference began without a durable readiness phase")
	}
	if err := updateTranslationCheckpointPhase(db, checkpoint.OperationID, "READY", ""); err != nil {
		t.Fatalf("persist ready phase: %v", err)
	}
	if err := markTranslationInferenceStarted(db, checkpoint.OperationID, checkpoint.ProviderConfigurationRevision, checkpoint.SelectedModel); err != nil {
		t.Fatalf("durable ready phase did not authorize inference: %v", err)
	}
	if err := markTranslationCancelled(db, checkpoint.OperationID); err != nil {
		t.Fatal(err)
	}
	if err := markTranslationInferenceStarted(db, checkpoint.OperationID, checkpoint.ProviderConfigurationRevision, checkpoint.SelectedModel); err == nil {
		t.Fatal("cancelled checkpoint authorized inference")
	}
	var stored models.TranslationOperationCheckpoint
	if err := db.First(&stored, "operation_id = ?", checkpoint.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.Cancelled || stored.ExecutionPhase != "RECOVERY_REQUIRED" || stored.RowVersion <= checkpoint.RowVersion {
		t.Fatalf("cancellation did not win durable checkpoint CAS: %+v", stored)
	}
}

func newLifecycleResourceFixtureWithEndpoint(t *testing.T, endpoint, model string, maxInferences int, enabled, resident bool) (*gorm.DB, *airesource.Service, string, string) {
	t.Helper()
	originalEndpoint := endpoint
	providerID := ollamaProviderConfigurationID(originalEndpoint)
	settings := airesource.DefaultSettings()
	settings.Enabled = enabled
	settings.MaxResidentModels = 1
	settings.MaxConcurrentInferences = maxInferences
	settings.MaxConcurrentModelLoads = 1
	settings.ModelEstimates = []airesource.ModelEstimate{{
		ProviderConfigurationID: providerID, ModelID: model,
		ResidentBytes: 1 << 30, LoadPeakAdditionalBytes: 1 << 30, InferenceBaseBytes: 512 << 20,
		KVCacheBytesPerToken: 1024, DefaultContextTokens: 1024, MaximumContextTokens: 8192,
		Source: "benchmark", Revision: "fixture-v1",
	}}
	encodedSettings, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	value := models.JSONMap{}
	if err := json.Unmarshal(encodedSettings, &value); err != nil {
		t.Fatal(err)
	}
	value, err = airesource.NormalizeSettingsValue(value)
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:ai-model-lifecycle-endpoint-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.AppSetting{}, &models.AIResourceAdmissionRecord{}, &models.AIResourceAdmissionCounter{}, &models.TranslationOperationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: airesource.SettingsKey, Value: value}).Error; err != nil {
		t.Fatal(err)
	}
	service := airesource.NewService(db)
	if resident {
		publishLifecycleFixtureObservation(t, service, originalEndpoint, model, true, airesource.GuardAllow)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, service, endpoint, providerID
}
