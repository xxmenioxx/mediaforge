package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/airesource"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const (
	aiModelReadinessDeadline = 5 * time.Minute
	aiModelStatusCacheTTL    = 5 * time.Second
	aiModelReadinessPoll     = time.Second
	aiModelMaxActiveOps      = 250
)

var errResourceAdmissionUnavailable = errors.New("resource admission unavailable")

type aiModelAdmission interface {
	Admit(context.Context, airesource.Request) airesource.Decision
	Await(context.Context, airesource.WaitHandle) airesource.Decision
	CancelWait(context.Context, airesource.WaitHandle) airesource.Decision
	Release(context.Context, airesource.Lease) error
}

type lifecycleWaiter struct {
	id       string
	callback func(string, string)
}

type lifecycleFlight struct {
	done      chan struct{}
	cancel    context.CancelFunc
	cancelled bool
	waiters   map[string]lifecycleWaiter
	err       error
}

type aiModelState struct {
	state     string
	reason    string
	updatedAt time.Time
	expiresAt time.Time
	inUse     int
	activeOps map[string]struct{}
}

type aiModelLifecycle struct {
	db                *gorm.DB
	admission         aiModelAdmission
	providerAPI       ollamaProviderAPIClient
	enabled           bool
	capacityDomain    string
	readinessDeadline time.Duration
	readinessPoll     time.Duration

	mu      sync.Mutex
	flights map[string]*lifecycleFlight
	states  map[string]aiModelState

	statusMu    sync.Mutex
	statusKey   string
	statusAt    time.Time
	statusValue aiModelRuntimeStatus
}

// AIModelLifecycle is the shared lifecycle service injected into Translation
// and Settings handlers. Its resource admission service must be the same
// instance used by the #265 settings and observation integration.
type AIModelLifecycle struct {
	manager *aiModelLifecycle
}

func NewAIModelLifecycle(db *gorm.DB, admission *airesource.Service, client *http.Client) *AIModelLifecycle {
	return &AIModelLifecycle{manager: newAIModelLifecycle(
		db, admission, client, modelLifecycleEnabled(),
		os.Getenv("MVFORGE_AI_RESOURCE_CAPACITY_DOMAIN"),
	)}
}

func NewOllamaProviderHTTPClient() *http.Client {
	return &http.Client{Timeout: ollamaTranslationHTTPTimeout}
}

func (l *AIModelLifecycle) runtimeStatus(ctx context.Context, endpoint string) aiModelRuntimeStatus {
	if l == nil || l.manager == nil {
		return aiModelRuntimeStatus{State: "unavailable", Diagnostic: "AI model lifecycle is unavailable", Models: []aiModelRuntimeModel{}, Operations: []aiModelRuntimeOperation{}, Failures: []aiModelRuntimeOperation{}}
	}
	return l.manager.runtimeStatus(ctx, endpoint)
}

func (l *AIModelLifecycle) protectedEnabled() bool {
	return l != nil && l.manager != nil && l.manager.enabled
}

type aiModelRuntimeStatus struct {
	State      string                    `json:"state"`
	ObservedAt string                    `json:"observedAt,omitempty"`
	Diagnostic string                    `json:"diagnostic,omitempty"`
	Models     []aiModelRuntimeModel     `json:"models"`
	Operations []aiModelRuntimeOperation `json:"activeOperations"`
	Failures   []aiModelRuntimeOperation `json:"failures"`
}

type aiModelRuntimeModel struct {
	ModelID   string `json:"modelId"`
	Installed bool   `json:"installed"`
	Resident  bool   `json:"resident"`
	State     string `json:"state"`
	InUse     int    `json:"activeInferenceLeases"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Failure   string `json:"failure,omitempty"`
}

type aiModelRuntimeOperation struct {
	OperationID string `json:"operationId"`
	ModelID     string `json:"modelId"`
	Phase       string `json:"phase"`
	Reason      string `json:"reason,omitempty"`
}

func newAIModelLifecycle(db *gorm.DB, admission aiModelAdmission, client *http.Client, enabled bool, capacityDomain string) *aiModelLifecycle {
	return &aiModelLifecycle{
		db: db, admission: admission, providerAPI: ollamaProviderAPIClient{client: client}, enabled: enabled,
		capacityDomain:    strings.TrimSpace(capacityDomain),
		readinessDeadline: aiModelReadinessDeadline, readinessPoll: aiModelReadinessPoll,
		flights: map[string]*lifecycleFlight{}, states: map[string]aiModelState{},
	}
}

func modelLifecycleEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MVFORGE_AI_MODEL_LIFECYCLE_ENABLED")), "true")
}

func providerConfigurationRevision(endpoint, model string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(endpoint) + "\x00" + strings.TrimSpace(model)))
	return hex.EncodeToString(digest[:])
}

func newAdmissionRequestID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", errors.New("could not create a unique resource admission request")
	}
	return hex.EncodeToString(bytes[:]), nil
}

func lifecycleKey(configurationRevision, model string) string {
	return configurationRevision + "\x00" + model
}

func modelIsResident(models []ollamaResidentModel, exactModel string) (bool, time.Time) {
	for _, model := range models {
		if model.Name == exactModel || model.Model == exactModel {
			return true, model.ExpiresAt
		}
	}
	return false, time.Time{}
}

func (l *aiModelLifecycle) EnsureReady(ctx context.Context, operationID, endpoint, configurationRevision, model string, onPhase func(string, string)) error {
	if l == nil || l.admission == nil || l.db == nil {
		return errors.New("AI model lifecycle is unavailable")
	}
	if !l.enabled {
		return errors.New("protected AI model lifecycle is disabled")
	}
	if strings.TrimSpace(l.capacityDomain) == "" {
		reason := "resource admission capacity domain must be configured"
		if onPhase != nil {
			onPhase("WAITING_FOR_RESOURCES", reason)
		}
		return fmt.Errorf("%w: %s", errResourceAdmissionUnavailable, reason)
	}
	model = strings.TrimSpace(model)
	if model == "" || strings.TrimSpace(endpoint) == "" || strings.TrimSpace(configurationRevision) == "" {
		return errors.New("selected Ollama model configuration is incomplete")
	}
	key := lifecycleKey(configurationRevision, model)
	providerConfigurationID := ollamaProviderConfigurationID(endpoint)
	waiterID, err := newAdmissionRequestID()
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var flight *lifecycleFlight
	for {
		l.mu.Lock()
		flight = l.flights[key]
		if flight != nil && flight.cancelled {
			done := flight.done
			l.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				continue
			}
		}
		if flight == nil {
			sharedCtx, cancel := context.WithCancel(context.Background())
			flight = &lifecycleFlight{done: make(chan struct{}), cancel: cancel, waiters: map[string]lifecycleWaiter{}}
			l.flights[key] = flight
			go l.warmSingleflight(sharedCtx, key, endpoint, providerConfigurationID, configurationRevision, model, flight)
		}
		flight.waiters[waiterID] = lifecycleWaiter{id: waiterID, callback: onPhase}
		l.mu.Unlock()
		break
	}

	select {
	case <-ctx.Done():
		l.removeFlightWaiter(key, flight, waiterID)
		return ctx.Err()
	case <-flight.done:
		l.removeFlightWaiter(key, flight, waiterID)
		return flight.err
	}
}

func (l *aiModelLifecycle) removeFlightWaiter(key string, flight *lifecycleFlight, waiterID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(flight.waiters, waiterID)
	if len(flight.waiters) == 0 && !flight.cancelled {
		select {
		case <-flight.done:
		default:
			flight.cancelled = true
			flight.cancel()
		}
	}
}

func (l *aiModelLifecycle) emitFlightPhase(flight *lifecycleFlight, phase, reason string) {
	l.mu.Lock()
	waiters := make([]lifecycleWaiter, 0, len(flight.waiters))
	for _, waiter := range flight.waiters {
		waiters = append(waiters, waiter)
	}
	l.mu.Unlock()
	for _, waiter := range waiters {
		if waiter.callback != nil {
			waiter.callback(phase, reason)
		}
	}
}

func (l *aiModelLifecycle) warmSingleflight(ctx context.Context, key, endpoint, providerConfigurationID, configurationRevision, model string, flight *lifecycleFlight) {
	err := l.warm(ctx, key, endpoint, providerConfigurationID, configurationRevision, model, flight)
	l.mu.Lock()
	flight.err = err
	if l.flights[key] == flight {
		delete(l.flights, key)
	}
	close(flight.done)
	l.mu.Unlock()
}

func (l *aiModelLifecycle) warm(ctx context.Context, key, endpoint, providerConfigurationID, configurationRevision, model string, flight *lifecycleFlight) (resultErr error) {
	setState := func(state, reason string, expiry time.Time) {
		l.mu.Lock()
		current := l.states[key]
		current.state, current.reason, current.updatedAt, current.expiresAt = state, reason, time.Now().UTC(), expiry
		if current.activeOps == nil {
			current.activeOps = map[string]struct{}{}
		}
		l.states[key] = current
		l.mu.Unlock()
	}

	residents, err := l.providerAPI.loadedModels(ctx, endpoint)
	if err != nil {
		setState("FAILED", "provider_residency_unavailable", time.Time{})
		l.emitFlightPhase(flight, "FAILED", "Ollama residency could not be confirmed")
		return errors.New("Ollama residency could not be confirmed")
	}
	if resident, expiresAt := modelIsResident(residents, model); resident {
		setState("READY", "", expiresAt)
		l.emitFlightPhase(flight, "READY", "")
		return nil
	}

	installed := listOllamaInstalledModels(ctx, endpoint, l.providerAPI)
	if installed.State != "success" {
		setState("FAILED", "installed_model_catalog_unavailable", time.Time{})
		l.emitFlightPhase(flight, "FAILED", "Installed Ollama models could not be confirmed")
		return errors.New("installed Ollama models could not be confirmed; no model request was sent")
	}
	found := false
	for _, installedModel := range installed.InstalledModels {
		if installedModel.Name == model || installedModel.ID == model {
			found = true
			break
		}
	}
	if !found {
		setState("FAILED", "selected_model_not_installed", time.Time{})
		l.emitFlightPhase(flight, "FAILED", "Selected model is not installed")
		return errors.New("selected Ollama model is not installed; install it manually before retrying")
	}

	setState("WAITING_FOR_RESOURCES", "", time.Time{})
	l.emitFlightPhase(flight, "WAITING_FOR_RESOURCES", "Waiting for resource admission before loading the model")
	requestID, err := newAdmissionRequestID()
	if err != nil {
		setState("FAILED", "admission_request_identity_unavailable", time.Time{})
		return err
	}
	workflowID := "model-load:" + configurationRevision + ":" + model
	lease, err := l.admit(ctx, airesource.Request{
		CapacityDomain: l.capacityDomain, ProviderConfigurationID: providerConfigurationID, ModelID: model,
		ConsumerID: "asset-info-translation", WorkflowID: workflowID, RequestID: requestID,
		Purpose: airesource.PurposeModelLoad, OperationDeadline: time.Now().Add(subtitleOperationTimeout),
	}, func(reason string) {
		l.emitFlightPhase(flight, "WAITING_FOR_RESOURCES", reason)
	})
	if err != nil {
		if errors.Is(err, errResourceAdmissionUnavailable) {
			setState("WAITING_FOR_RESOURCES", boundedLifecycleReason(err.Error()), time.Time{})
			l.emitFlightPhase(flight, "WAITING_FOR_RESOURCES", boundedLifecycleReason(err.Error()))
			return err
		}
		setState("FAILED", "model_load_admission_failed", time.Time{})
		l.emitFlightPhase(flight, "FAILED", boundedLifecycleReason(err.Error()))
		return err
	}
	release := func() error {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return l.admission.Release(releaseCtx, *lease)
	}
	defer func() {
		if err := release(); err != nil && resultErr == nil {
			resultErr = errors.New("model load admission lease release failed")
		}
	}()

	readinessDeadline := l.readinessDeadline
	if readinessDeadline <= 0 {
		readinessDeadline = aiModelReadinessDeadline
	}
	readinessPoll := l.readinessPoll
	if readinessPoll <= 0 {
		readinessPoll = aiModelReadinessPoll
	}
	readyDeadline := time.Now().Add(readinessDeadline)
	setState("WARMING_UP", "", time.Time{})
	l.emitFlightPhase(flight, "WAITING_FOR_MODEL", "Waiting for the selected model to become resident")
	if err := l.providerAPI.warmModel(ctx, endpoint, model); err != nil {
		if resident, expiresAt := l.reconcileAmbiguousWarmup(endpoint, key, model); resident && ctx.Err() == nil {
			setState("READY", "", expiresAt)
			l.emitFlightPhase(flight, "READY", "")
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		setState("FAILED", "warmup_failed", time.Time{})
		l.emitFlightPhase(flight, "FAILED", "Ollama model warmup did not complete")
		return errors.New("Ollama model warmup did not complete")
	}
	for {
		if err := ctx.Err(); err != nil {
			l.reconcileAmbiguousWarmup(endpoint, key, model)
			return err
		}
		residents, err := l.providerAPI.loadedModels(ctx, endpoint)
		if err == nil {
			if resident, expiresAt := modelIsResident(residents, model); resident {
				setState("READY", "", expiresAt)
				l.emitFlightPhase(flight, "READY", "")
				return nil
			}
		}
		if !time.Now().Before(readyDeadline) {
			setState("FAILED", "model_readiness_timeout", time.Time{})
			l.emitFlightPhase(flight, "FAILED", "Ollama did not report the selected model as resident before the readiness deadline")
			return errors.New("Ollama did not report the selected model as resident before the readiness deadline")
		}
		timer := time.NewTimer(readinessPoll)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}
	}
}

func (l *aiModelLifecycle) admit(ctx context.Context, request airesource.Request, onWaiting func(string)) (*airesource.Lease, error) {
	if l.admission == nil || l.capacityDomain == "" {
		reason := "resource admission is unavailable; configure the capacity domain"
		if onWaiting != nil {
			onWaiting(reason)
		}
		return nil, fmt.Errorf("%w: %s", errResourceAdmissionUnavailable, reason)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	decision := l.admission.Admit(ctx, request)
	waitHandle := decision.Wait
	if err := ctx.Err(); err != nil {
		return nil, l.cancelledAdmissionError(err, waitHandle, decision)
	}
	if decision.Status == airesource.Waiting {
		if decision.Wait == nil {
			return nil, errors.New("resource admission returned WAITING without a wait ticket")
		}
		if onWaiting != nil {
			onWaiting(boundedAdmissionReason(decision.Reason))
		}
		decision = l.admission.Await(ctx, *decision.Wait)
		if err := ctx.Err(); err != nil {
			return nil, l.cancelledAdmissionError(err, waitHandle, decision)
		}
	}
	switch decision.Status {
	case airesource.Admitted:
		if decision.Lease == nil {
			return nil, errors.New("resource admission returned ADMITTED without a lease")
		}
		if err := ctx.Err(); err != nil {
			return nil, l.cancelledAdmissionError(err, waitHandle, decision)
		}
		return decision.Lease, nil
	case airesource.Waiting:
		waitErr := errors.New("resource admission remained WAITING after its bounded wait")
		if cleanupErr := l.cancelAdmissionAndRelease(waitHandle, decision); cleanupErr != nil {
			return nil, errors.Join(waitErr, fmt.Errorf("resource admission wait cleanup failed: %w", cleanupErr))
		}
		return nil, waitErr
	case airesource.Denied:
		return nil, fmt.Errorf("resource admission denied the request: %s", boundedAdmissionReason(decision.Reason))
	case airesource.Unavailable:
		reason := boundedAdmissionReason(decision.Reason)
		if onWaiting != nil {
			onWaiting(reason)
		}
		return nil, fmt.Errorf("%w: %s", errResourceAdmissionUnavailable, reason)
	default:
		return nil, errors.New("resource admission returned an unknown decision")
	}
}

func (l *aiModelLifecycle) cancelledAdmissionError(ctxErr error, waitHandle *airesource.WaitHandle, decisions ...airesource.Decision) error {
	if cleanupErr := l.cancelAdmissionAndRelease(waitHandle, decisions...); cleanupErr != nil {
		return errors.Join(ctxErr, fmt.Errorf("resource admission cleanup after cancellation failed: %w", cleanupErr))
	}
	return ctxErr
}

func (l *aiModelLifecycle) cancelAdmissionAndRelease(waitHandle *airesource.WaitHandle, decisions ...airesource.Decision) error {
	var cleanupErrors []error
	if waitHandle != nil {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cancelDecision := l.admission.CancelWait(cancelCtx, *waitHandle)
		cancel()
		switch cancelDecision.Status {
		case airesource.Admitted:
			if cancelDecision.Lease == nil {
				cleanupErrors = append(cleanupErrors, errors.New("admission cancellation returned ADMITTED without a lease"))
			}
		case airesource.Denied:
		case airesource.Waiting, airesource.Unavailable:
			cleanupErrors = append(cleanupErrors, errors.New("admission wait cancellation could not be confirmed"))
		default:
			cleanupErrors = append(cleanupErrors, errors.New("admission wait cancellation returned an unknown outcome"))
		}
		// CancelWait can win a grant race and return the granted lease. Keep it
		// alongside any lease returned by Admit/Await so every grant is released.
		decisions = append(decisions, cancelDecision)
	}
	for _, decision := range decisions {
		if decision.Lease == nil {
			continue
		}
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := l.admission.Release(releaseCtx, *decision.Lease)
		cancel()
		if err != nil {
			cleanupErrors = append(cleanupErrors, errors.New("resource admission lease release failed after cancellation"))
		}
	}
	return errors.Join(cleanupErrors...)
}

func boundedAdmissionReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "no actionable reason was supplied"
	}
	return boundedLifecycleReason(reason)
}

func boundedLifecycleReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 160 {
		return reason[:160]
	}
	return reason
}

func (l *aiModelLifecycle) reconcileAmbiguousWarmup(endpoint, key, expectedModel string) (bool, time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	residents, err := l.providerAPI.loadedModels(ctx, endpoint)
	if err != nil {
		l.setModelState(key, "FAILED", "residency_unknown_after_cancel", time.Time{})
		return false, time.Time{}
	}
	if resident, expiresAt := modelIsResident(residents, expectedModel); resident {
		l.setModelState(key, "IDLE", "", expiresAt)
		return true, expiresAt
	}
	l.setModelState(key, "UNLOADED", "", time.Time{})
	return false, time.Time{}
}

func (l *aiModelLifecycle) setModelState(key, state, reason string, expiresAt time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	current := l.states[key]
	current.state, current.reason, current.updatedAt, current.expiresAt = state, reason, time.Now().UTC(), expiresAt
	if current.activeOps == nil {
		current.activeOps = map[string]struct{}{}
	}
	l.states[key] = current
}

func (l *aiModelLifecycle) acquireInference(ctx context.Context, operationID, workflowID, endpoint, configurationRevision, model string, onPhase func(string, string)) (*airesource.Lease, error) {
	requestID, err := newAdmissionRequestID()
	if err != nil {
		return nil, err
	}
	lease, err := l.admit(ctx, airesource.Request{
		CapacityDomain: l.capacityDomain, ProviderConfigurationID: ollamaProviderConfigurationID(endpoint), ModelID: model,
		ConsumerID: "asset-info-translation", WorkflowID: workflowID, RequestID: requestID,
		Purpose: airesource.PurposeInference, OperationDeadline: time.Now().Add(subtitleOperationTimeout),
	}, func(reason string) {
		if onPhase != nil {
			onPhase("WAITING_FOR_RESOURCES", reason)
		}
	})
	if err != nil {
		return nil, err
	}
	key := lifecycleKey(configurationRevision, model)
	l.mu.Lock()
	state := l.states[key]
	state.inUse++
	state.state = "IN_USE"
	state.updatedAt = time.Now().UTC()
	if state.activeOps == nil {
		state.activeOps = map[string]struct{}{}
	}
	state.activeOps[operationID] = struct{}{}
	l.states[key] = state
	l.mu.Unlock()
	return lease, nil
}

func (l *aiModelLifecycle) releaseInference(ctx context.Context, operationID, configurationRevision, model string, lease *airesource.Lease) error {
	if lease == nil || l.admission == nil {
		return errors.New("inference admission lease is unavailable")
	}
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := l.admission.Release(releaseCtx, *lease)
	key := lifecycleKey(configurationRevision, model)
	l.mu.Lock()
	state := l.states[key]
	if state.inUse > 0 {
		state.inUse--
	}
	delete(state.activeOps, operationID)
	if state.inUse == 0 && state.state == "IN_USE" {
		state.state = "IDLE"
	}
	state.updatedAt = time.Now().UTC()
	l.states[key] = state
	l.mu.Unlock()
	if err != nil {
		return errors.New("resource admission lease release failed")
	}
	return nil
}

func (l *aiModelLifecycle) runtimeStatus(ctx context.Context, endpoint string) aiModelRuntimeStatus {
	if l == nil {
		return aiModelRuntimeStatus{State: "unavailable", Diagnostic: "AI model lifecycle is unavailable", Models: []aiModelRuntimeModel{}, Operations: []aiModelRuntimeOperation{}, Failures: []aiModelRuntimeOperation{}}
	}
	cacheKey := providerConfigurationRevision(endpoint, "runtime")
	l.statusMu.Lock()
	if l.statusKey == cacheKey && time.Since(l.statusAt) < aiModelStatusCacheTTL {
		cached := cloneAIModelRuntimeStatus(l.statusValue)
		l.statusMu.Unlock()
		return cached
	}
	l.statusMu.Unlock()

	statusCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result := aiModelRuntimeStatus{State: "unavailable", Models: []aiModelRuntimeModel{}, Operations: []aiModelRuntimeOperation{}, Failures: []aiModelRuntimeOperation{}}
	installed := listOllamaInstalledModels(statusCtx, endpoint, l.providerAPI)
	residents, residentErr := l.providerAPI.loadedModels(statusCtx, endpoint)
	if installed.State != "success" || residentErr != nil {
		result.Diagnostic = "Ollama installed or resident model status is unavailable"
	} else {
		result.State = "available"
		result.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
		byName := map[string]aiModelRuntimeModel{}
		for _, model := range installed.InstalledModels {
			byName[model.Name] = aiModelRuntimeModel{ModelID: model.Name, Installed: true, State: "UNLOADED"}
		}
		for _, model := range residents {
			name := model.Name
			if name == "" {
				name = model.Model
			}
			entry := byName[name]
			entry.ModelID, entry.Resident = name, true
			if !model.ExpiresAt.IsZero() {
				entry.ExpiresAt = model.ExpiresAt.UTC().Format(time.RFC3339Nano)
			}
			if entry.State == "" || entry.State == "UNLOADED" {
				entry.State = "IDLE"
			}
			byName[name] = entry
		}
		l.mu.Lock()
		for name, entry := range byName {
			key := lifecycleKey(providerConfigurationRevision(endpoint, name), name)
			state := l.states[key]
			if entry.Resident && state.state != "" {
				entry.State = state.state
				entry.Failure = state.reason
			} else if !entry.Resident {
				if state.state == "WAITING_FOR_RESOURCES" || state.state == "WARMING_UP" || state.state == "FAILED" {
					entry.State = state.state
					entry.Failure = state.reason
				} else {
					entry.State = "UNLOADED"
				}
			}
			entry.InUse = state.inUse
			byName[name] = entry
		}
		l.mu.Unlock()
		for _, model := range byName {
			result.Models = append(result.Models, model)
		}
		sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].ModelID < result.Models[j].ModelID })
	}
	result.Operations, result.Failures = l.runtimeOperations(statusCtx)
	l.statusMu.Lock()
	l.statusKey, l.statusAt, l.statusValue = cacheKey, time.Now(), cloneAIModelRuntimeStatus(result)
	l.statusMu.Unlock()
	return result
}

func (l *aiModelLifecycle) runtimeOperations(ctx context.Context) ([]aiModelRuntimeOperation, []aiModelRuntimeOperation) {
	operations := []aiModelRuntimeOperation{}
	failures := []aiModelRuntimeOperation{}
	if l.db == nil {
		return operations, failures
	}
	var rows []models.TranslationOperationCheckpoint
	if err := l.db.WithContext(ctx).Where("execution_phase IN ? AND cancelled = ?", []string{"WAITING_FOR_RESOURCES", "WAITING_FOR_MODEL", "READY", "INFERENCE_STARTED", "PUBLISHING"}, false).Order("updated_at desc").Limit(aiModelMaxActiveOps).Find(&rows).Error; err != nil {
		return operations, failures
	}
	for _, row := range rows {
		operations = append(operations, aiModelRuntimeOperation{OperationID: row.OperationID, ModelID: row.SelectedModel, Phase: row.ExecutionPhase, Reason: row.RecoveryReason})
	}
	var failedRows []models.TranslationOperationCheckpoint
	if err := l.db.WithContext(ctx).Where("execution_phase IN ?", []string{"FAILED", "RECOVERY_REQUIRED"}).Order("updated_at desc").Limit(aiModelMaxActiveOps).Find(&failedRows).Error; err == nil {
		for _, row := range failedRows {
			failures = append(failures, aiModelRuntimeOperation{OperationID: row.OperationID, ModelID: row.SelectedModel, Phase: row.ExecutionPhase, Reason: row.RecoveryReason})
		}
	}
	return operations, failures
}

func cloneAIModelRuntimeStatus(input aiModelRuntimeStatus) aiModelRuntimeStatus {
	copy := input
	copy.Models = append([]aiModelRuntimeModel(nil), input.Models...)
	copy.Operations = append([]aiModelRuntimeOperation(nil), input.Operations...)
	copy.Failures = append([]aiModelRuntimeOperation(nil), input.Failures...)
	return copy
}

type lifecycleTranslationProvider struct {
	manager               *aiModelLifecycle
	provider              TranslationProvider
	endpoint              string
	configurationRevision string
	model                 string
	operationID           string
	workflowID            string
	validate              func(context.Context) error
	markInferenceStarted  func() error
	onInferenceError      func()
	onPhase               func(string, string)
}

func (p lifecycleTranslationProvider) Translate(ctx context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
	if p.manager == nil || p.provider == nil {
		return TranslationBatchResult{}, errors.New("protected Translation provider is unavailable")
	}
	if err := p.manager.EnsureReady(ctx, p.operationID, p.endpoint, p.configurationRevision, p.model, p.onPhase); err != nil {
		return TranslationBatchResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return TranslationBatchResult{}, err
	}
	lease, err := p.manager.acquireInference(ctx, p.operationID, p.workflowID, p.endpoint, p.configurationRevision, p.model, p.onPhase)
	if err != nil {
		return TranslationBatchResult{}, err
	}
	defer func() {
		if lease != nil {
			_ = p.manager.releaseInference(ctx, p.operationID, p.configurationRevision, p.model, lease)
		}
	}()
	residents, err := p.manager.providerAPI.loadedModels(ctx, p.endpoint)
	if err != nil {
		return TranslationBatchResult{}, errors.New("Ollama residency could not be confirmed before inference")
	}
	if resident, _ := modelIsResident(residents, p.model); !resident {
		return TranslationBatchResult{}, errors.New("selected Ollama model is no longer resident; retry the Translation operation")
	}
	if err := ctx.Err(); err != nil {
		return TranslationBatchResult{}, err
	}
	if p.validate != nil {
		if err := p.validate(ctx); err != nil {
			return TranslationBatchResult{}, err
		}
	}
	if p.markInferenceStarted != nil {
		if err := p.markInferenceStarted(); err != nil {
			return TranslationBatchResult{}, errors.New("could not persist the Translation inference checkpoint")
		}
	}
	result, err := p.provider.Translate(ctx, batch)
	releaseErr := p.manager.releaseInference(ctx, p.operationID, p.configurationRevision, p.model, lease)
	lease = nil
	if err != nil {
		if p.onInferenceError != nil {
			p.onInferenceError()
		}
		return TranslationBatchResult{}, err
	}
	if releaseErr != nil {
		return TranslationBatchResult{}, releaseErr
	}
	return result, nil
}

func checkpointMetadata(request subtitleTranslationOperationRequest) (string, error) {
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > 16*1024 {
		return "", errors.New("Translation recovery metadata exceeded its bounded limit")
	}
	return string(encoded), nil
}

func translationAdmissionWorkflowID(request subtitleTranslationOperationRequest) string {
	key := strings.Join([]string{request.SourceIdentity, request.SourceRevision, request.Provider, request.ProviderModel, request.TargetLocale, request.RequestedRepresentation}, "\x00")
	digest := sha256.Sum256([]byte(key))
	return "translation-" + hex.EncodeToString(digest[:16])
}

func createTranslationCheckpoint(db *gorm.DB, operationID string, request subtitleTranslationOperationRequest, configurationRevision string, deadline time.Time) error {
	metadata, err := checkpointMetadata(request)
	if err != nil {
		return err
	}
	if db == nil || strings.TrimSpace(operationID) == "" || strings.TrimSpace(configurationRevision) == "" || deadline.IsZero() {
		return errors.New("Translation lifecycle checkpoint is incomplete")
	}
	return db.Create(&models.TranslationOperationCheckpoint{
		OperationID: operationID, SourceIdentity: request.SourceIdentity, SourceRevision: request.SourceRevision,
		ProviderConfigurationRevision: configurationRevision, SelectedModel: request.ProviderModel,
		AdmissionWorkflowID: translationAdmissionWorkflowID(request), ExecutionPhase: "PREPARING",
		Deadline: deadline.UTC(), CheckpointVersion: 1, RowVersion: 1, RequestMetadata: metadata,
	}).Error
}

func updateTranslationCheckpointPhase(db *gorm.DB, operationID, phase, reason string) error {
	if db == nil {
		return errors.New("Translation checkpoint store is unavailable")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var checkpoint models.TranslationOperationCheckpoint
		if err := tx.First(&checkpoint, "operation_id = ?", operationID).Error; err != nil {
			return err
		}
		if checkpoint.Cancelled || checkpoint.ExecutionPhase == "PUBLICATION_COMMITTED" {
			return errors.New("Translation checkpoint no longer accepts phase updates")
		}
		if checkpoint.ExecutionPhase == "RECOVERY_REQUIRED" {
			return nil
		}
		if checkpoint.InferenceStarted || checkpoint.ExecutionPhase == "INFERENCE_STARTED" || checkpoint.ExecutionPhase == "PUBLISHING" {
			if phase == "FAILED" || phase == "RECOVERY_REQUIRED" {
				phase = "RECOVERY_REQUIRED"
			} else {
				// Per-batch readiness and admission callbacks must not move a
				// checkpoint back before the first inference request.
				return nil
			}
		}
		result := tx.Model(&models.TranslationOperationCheckpoint{}).Where(
			"operation_id = ? AND row_version = ? AND cancelled = ? AND execution_phase = ?",
			operationID, checkpoint.RowVersion, false, checkpoint.ExecutionPhase,
		).Updates(map[string]any{
			"execution_phase": phase, "recovery_reason": boundedLifecycleReason(reason),
			"updated_at": time.Now().UTC(), "row_version": checkpoint.RowVersion + 1,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("Translation checkpoint changed during phase update")
		}
		return nil
	})
}

func markTranslationInferenceStarted(db *gorm.DB, operationID, configurationRevision, model string) error {
	if db == nil {
		return errors.New("Translation checkpoint store is unavailable")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var checkpoint models.TranslationOperationCheckpoint
		if err := tx.First(&checkpoint, "operation_id = ?", operationID).Error; err != nil {
			return err
		}
		if checkpoint.Cancelled || checkpoint.SelectedModel != model || checkpoint.ProviderConfigurationRevision != configurationRevision || !checkpoint.Deadline.After(time.Now()) {
			return errors.New("Translation checkpoint no longer authorizes inference")
		}
		if checkpoint.InferenceStarted {
			if checkpoint.ExecutionPhase == "INFERENCE_STARTED" {
				return nil
			}
			return errors.New("Translation inference checkpoint phase is inconsistent")
		}
		if checkpoint.ExecutionPhase != "READY" {
			return errors.New("Translation checkpoint no longer authorizes inference")
		}
		result := tx.Model(&models.TranslationOperationCheckpoint{}).Where(
			"operation_id = ? AND row_version = ? AND execution_phase = ? AND cancelled = ?",
			operationID, checkpoint.RowVersion, "READY", false,
		).Updates(map[string]any{
			"execution_phase": "INFERENCE_STARTED", "inference_started": true,
			"recovery_reason": "", "updated_at": time.Now().UTC(), "row_version": checkpoint.RowVersion + 1,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("Translation checkpoint changed before inference")
		}
		return nil
	})
}

func markTranslationRecoveryRequired(db *gorm.DB, operationID, reason string) {
	if db == nil || strings.TrimSpace(operationID) == "" {
		return
	}
	_ = db.Model(&models.TranslationOperationCheckpoint{}).Where("operation_id = ? AND execution_phase <> ?", operationID, "PUBLICATION_COMMITTED").Updates(map[string]any{
		"execution_phase": "RECOVERY_REQUIRED", "recovery_reason": boundedLifecycleReason(reason), "updated_at": time.Now().UTC(), "row_version": gorm.Expr("row_version + 1"),
	}).Error
}

func forceTranslationRecoveryRequired(db *gorm.DB, operationID, reason string) {
	if db == nil || strings.TrimSpace(operationID) == "" {
		return
	}
	_ = db.Model(&models.TranslationOperationCheckpoint{}).Where("operation_id = ?", operationID).Updates(map[string]any{
		"execution_phase": "RECOVERY_REQUIRED", "recovery_reason": boundedLifecycleReason(reason), "updated_at": time.Now().UTC(), "row_version": gorm.Expr("row_version + 1"),
	}).Error
}

func markTranslationCancelled(db *gorm.DB, operationID string) error {
	if db == nil {
		return errors.New("Translation checkpoint store is unavailable")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var checkpoint models.TranslationOperationCheckpoint
		if err := tx.First(&checkpoint, "operation_id = ?", operationID).Error; err != nil {
			return err
		}
		if checkpoint.ExecutionPhase == "PUBLICATION_COMMITTED" {
			return nil
		}
		checkpoint.Cancelled = true
		checkpoint.ExecutionPhase = "CANCELLED"
		if checkpoint.InferenceStarted {
			checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
			checkpoint.RecoveryReason = "cancelled_after_inference_started"
		}
		checkpoint.UpdatedAt = time.Now().UTC()
		result := tx.Model(&models.TranslationOperationCheckpoint{}).Where("operation_id = ? AND row_version = ?", operationID, checkpoint.RowVersion).Updates(map[string]any{
			"cancelled": true, "execution_phase": checkpoint.ExecutionPhase, "recovery_reason": checkpoint.RecoveryReason,
			"updated_at": checkpoint.UpdatedAt, "row_version": checkpoint.RowVersion + 1,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("Translation checkpoint changed during cancellation")
		}
		return nil
	})
}

func markTranslationCancelledIfPresent(db *gorm.DB, operationID string) (bool, error) {
	if db == nil {
		// Legacy in-memory subtitle operations have no durable checkpoint store.
		// Protected Translation operations always supply a database and checkpoint.
		return false, nil
	}
	if !db.Migrator().HasTable(&models.TranslationOperationCheckpoint{}) {
		return false, nil
	}
	var checkpoint models.TranslationOperationCheckpoint
	err := db.First(&checkpoint, "operation_id = ?", operationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, markTranslationCancelled(db, operationID)
}

type translationPublicationFileEvidence struct {
	Path           string `json:"path"`
	Format         string `json:"format"`
	ExpectedSHA256 string `json:"expectedSha256"`
	PreviousExists bool   `json:"previousExists"`
	PreviousSHA256 string `json:"previousSha256,omitempty"`
}

type translationPublicationManifest struct {
	Version int                                  `json:"version"`
	Files   []translationPublicationFileEvidence `json:"files"`
}

func translationFileSHA256(path string) (string, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !pathInfo.Mode().IsRegular() {
		return "", errors.New("publication fingerprint target is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(pathInfo, openedInfo) {
		return "", errors.New("publication fingerprint target changed during inspection")
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, 64*1024*1024+1))
	if err != nil {
		return "", err
	}
	if written > 64*1024*1024 {
		return "", errors.New("publication file exceeded its fingerprint limit")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func prepareTranslationPublicationIntent(db *gorm.DB, operationID string, targets []subtitleTranslationPublicationTarget) error {
	if len(targets) == 0 || len(targets) > 2 {
		return errors.New("Translation publication manifest has an invalid file count")
	}
	manifest := translationPublicationManifest{Version: 1, Files: make([]translationPublicationFileEvidence, 0, len(targets))}
	for _, target := range targets {
		expected, err := translationFileSHA256(target.StagePath)
		if err != nil {
			return errors.New("could not fingerprint staged Translation output")
		}
		entry := translationPublicationFileEvidence{Path: filepath.Clean(target.FinalPath), Format: target.Format, ExpectedSHA256: expected}
		previous, err := translationFileSHA256(target.FinalPath)
		if err == nil {
			entry.PreviousExists, entry.PreviousSHA256 = true, previous
		} else if !os.IsNotExist(err) {
			return errors.New("could not inspect existing Translation destination")
		}
		manifest.Files = append(manifest.Files, entry)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil || len(encoded) > 16*1024 {
		return errors.New("Translation publication manifest exceeded its bounded limit")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var checkpoint models.TranslationOperationCheckpoint
		if err := tx.First(&checkpoint, "operation_id = ?", operationID).Error; err != nil {
			return err
		}
		if checkpoint.Cancelled || !checkpoint.InferenceStarted || checkpoint.ExecutionPhase != "INFERENCE_STARTED" || checkpoint.PublicationIntent != "" {
			return errors.New("Translation checkpoint does not authorize publication")
		}
		result := tx.Model(&models.TranslationOperationCheckpoint{}).Where("operation_id = ? AND row_version = ? AND cancelled = ?", operationID, checkpoint.RowVersion, false).Updates(map[string]any{
			"execution_phase": "PUBLISHING", "publication_intent": string(encoded),
			"updated_at": time.Now().UTC(), "row_version": checkpoint.RowVersion + 1,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("Translation checkpoint changed before publication")
		}
		return nil
	})
}

func commitTranslationPublication(db *gorm.DB, operationID string) error {
	if db == nil {
		return errors.New("Translation checkpoint store is unavailable")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var checkpoint models.TranslationOperationCheckpoint
		if err := tx.First(&checkpoint, "operation_id = ?", operationID).Error; err != nil {
			return err
		}
		if checkpoint.Cancelled || checkpoint.ExecutionPhase != "PUBLISHING" || strings.TrimSpace(checkpoint.PublicationIntent) == "" {
			return errors.New("Translation publication checkpoint is incomplete or cancelled")
		}
		result := tx.Model(&models.TranslationOperationCheckpoint{}).Where("operation_id = ? AND row_version = ? AND execution_phase = ? AND cancelled = ?", operationID, checkpoint.RowVersion, "PUBLISHING", false).Updates(map[string]any{
			"execution_phase": "PUBLICATION_COMMITTED", "recovery_reason": "",
			"updated_at": time.Now().UTC(), "row_version": checkpoint.RowVersion + 1,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("Translation checkpoint changed before publication commit")
		}
		return nil
	})
}

func reconcileTranslationPublication(db *gorm.DB, checkpoint *models.TranslationOperationCheckpoint) bool {
	if checkpoint == nil || strings.TrimSpace(checkpoint.PublicationIntent) == "" {
		return false
	}
	var manifest translationPublicationManifest
	if len(checkpoint.PublicationIntent) > 16*1024 {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(checkpoint.PublicationIntent))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.Version != 1 || len(manifest.Files) == 0 || len(manifest.Files) > 2 {
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return false
	}
	seenPaths := map[string]struct{}{}
	for _, file := range manifest.Files {
		if !filepath.IsAbs(file.Path) || filepath.Clean(file.Path) != file.Path || strings.TrimSpace(file.Format) == "" || len(file.ExpectedSHA256) != sha256.Size*2 {
			return false
		}
		if _, exists := seenPaths[file.Path]; exists {
			return false
		}
		seenPaths[file.Path] = struct{}{}
		if _, err := hex.DecodeString(file.ExpectedSHA256); err != nil {
			return false
		}
		if file.PreviousExists && len(file.PreviousSHA256) != sha256.Size*2 || !file.PreviousExists && file.PreviousSHA256 != "" {
			return false
		}
		actual, err := translationFileSHA256(file.Path)
		if err != nil || actual != file.ExpectedSHA256 {
			return false
		}
	}
	if checkpoint.ExecutionPhase == "PUBLISHING" {
		result := db.Model(&models.TranslationOperationCheckpoint{}).Where("operation_id = ? AND row_version = ? AND execution_phase = ? AND cancelled = ?", checkpoint.OperationID, checkpoint.RowVersion, "PUBLISHING", false).Updates(map[string]any{
			"execution_phase": "PUBLICATION_COMMITTED", "recovery_reason": "", "updated_at": time.Now().UTC(), "row_version": checkpoint.RowVersion + 1,
		})
		if result.Error != nil || result.RowsAffected != 1 {
			return false
		}
		checkpoint.ExecutionPhase = "PUBLICATION_COMMITTED"
		checkpoint.RowVersion++
	}
	return checkpoint.ExecutionPhase == "PUBLICATION_COMMITTED"
}
