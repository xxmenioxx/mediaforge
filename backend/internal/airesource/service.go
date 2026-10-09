package airesource

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	stateAdmitted            = "ADMITTED"
	stateWaiting             = "WAITING"
	stateReleased            = "RELEASED"
	stateCancelled           = "CANCELLED"
	stateExpired             = "EXPIRED"
	stateDenied              = "DENIED"
	stateUnavailable         = "UNAVAILABLE"
	maximumOperationLifetime = 2 * time.Hour
)

var ErrInvalidLease = errors.New("invalid or unknown AI resource lease")

// Service is a provider-neutral, durable admission port. Observations are
// pushed by owning adapters; this service does not poll providers or hosts.
type Service struct {
	db        *gorm.DB
	mu        sync.Mutex
	latest    map[string]Observation
	watchers  map[string]chan struct{}
	lastGrant map[string]Purpose
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, latest: map[string]Observation{}, watchers: map[string]chan struct{}{}, lastGrant: map[string]Purpose{}}
}

func (s *Service) Admit(ctx context.Context, request Request) Decision {
	if s == nil || s.db == nil {
		return Decision{Status: Unavailable, Reason: "admission_store_unavailable"}
	}
	now := time.Now().UTC()
	if reason := validateRequest(request); reason != "" {
		return Decision{Status: Denied, Reason: reason}
	}
	request = normalizeRequest(request)
	key := requestKey(request)
	s.mu.Lock()
	defer s.mu.Unlock()
	decision := Decision{Status: Unavailable, Reason: "admission_store_unavailable"}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		settings, err := loadSettings(tx)
		if err != nil {
			decision = Decision{Status: Unavailable, Reason: "resource_policy_unavailable"}
			return nil
		}
		var prior models.AIResourceAdmissionRecord
		if err := tx.Where("request_key = ?", key).First(&prior).Error; err == nil {
			if prior.State == stateWaiting && !settings.Enabled {
				prior.State, prior.ReasonCode = stateDenied, "protection_disabled"
				if err := tx.Save(&prior).Error; err != nil {
					return err
				}
				decision = Decision{Status: Denied, Reason: prior.ReasonCode}
				if err := recordOutcome(tx, decision, now); err != nil {
					return err
				}
				s.signal(prior.WaitID)
				return nil
			}
			if prior.State == stateWaiting && !prior.WaitDeadline.After(now) {
				prior.State, prior.ReasonCode = stateDenied, "admission_wait_expired"
				if err := tx.Save(&prior).Error; err != nil {
					return err
				}
				decision = Decision{Status: Denied, Reason: prior.ReasonCode}
				if err := recordOutcome(tx, decision, now); err != nil {
					return err
				}
				s.signal(prior.WaitID)
				return nil
			}
			if prior.State == stateAdmitted {
				grace := time.Duration(settings.LeaseCleanupGraceSeconds) * time.Second
				if !prior.OperationDeadline.Add(grace).After(now) {
					prior.State, prior.ReasonCode = stateExpired, "lease_expired"
					if err := tx.Save(&prior).Error; err != nil {
						return err
					}
					decision = Decision{Status: Denied, Reason: prior.ReasonCode}
					if err := recordOutcome(tx, decision, now); err != nil {
						return err
					}
					return nil
				}
			}
			decision = priorDecision(prior, request, key)
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if request.OperationDeadline.IsZero() {
			request.OperationDeadline = now.Add(15 * time.Minute)
		}
		if !request.OperationDeadline.After(now) || request.OperationDeadline.After(now.Add(maximumOperationLifetime)) {
			decision = Decision{Status: Denied, Reason: "operation_deadline_invalid"}
			return recordOutcome(tx, decision, now)
		}
		if !settings.Enabled {
			decision = Decision{Status: Denied, Reason: "protection_disabled"}
			return persistTerminalRequest(tx, request, key, decision, now)
		}
		observation, ok := s.latest[request.CapacityDomain]
		if !ok {
			decision = Decision{Status: Unavailable, Reason: "resource_observation_unavailable"}
			return persistTerminalRequest(tx, request, key, decision, now)
		}
		if err := expireLeases(tx, request.CapacityDomain, settings, now); err != nil {
			return err
		}
		reservations, err := loadReservations(tx, request.CapacityDomain, settings, now)
		if err != nil {
			return err
		}
		decision = evaluate(settings, request, observation, reservations, now)
		switch decision.Status {
		case Admitted:
			leaseID, err := opaqueID()
			if err != nil {
				return err
			}
			record := admissionRecord(request, key, leaseID, "", stateAdmitted, decision.Reason, now)
			record.EvaluatedHostObservedAt = observation.Host.ObservedAt
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
			decision.Lease = &Lease{id: leaseID, requestKey: key}
		case Waiting:
			waitID, err := opaqueID()
			if err != nil {
				return err
			}
			waitDeadline := now.Add(time.Duration(settings.MaximumAdmissionWaitSeconds) * time.Second)
			if request.OperationDeadline.Before(waitDeadline) {
				waitDeadline = request.OperationDeadline
			}
			record := admissionRecord(request, key, "", waitID, stateWaiting, decision.Reason, now)
			record.WaitDeadline = waitDeadline
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
			decision.Wait = &WaitHandle{id: waitID, requestKey: key}
		default:
			return persistTerminalRequest(tx, request, key, decision, now)
		}
		return recordOutcome(tx, decision, now)
	})
	if err != nil {
		return Decision{Status: Unavailable, Reason: "admission_store_unavailable"}
	}
	return decision
}

// Preview evaluates the exact live policy without journaling a request,
// acquiring a lease, incrementing outcomes, or creating a wait ticket.
func (s *Service) Preview(ctx context.Context, request Request) Decision {
	if s == nil || s.db == nil {
		return Decision{Status: Unavailable, Reason: "admission_store_unavailable"}
	}
	if reason := validateRequest(request); reason != "" {
		return Decision{Status: Denied, Reason: reason}
	}
	request = normalizeRequest(request)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	var decision Decision
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		settings, err := loadSettings(tx)
		if err != nil {
			decision = Decision{Status: Unavailable, Reason: "resource_policy_unavailable"}
			return nil
		}
		if !settings.Enabled {
			decision = Decision{Status: Denied, Reason: "protection_disabled"}
			return nil
		}
		observation, ok := s.latest[request.CapacityDomain]
		if !ok {
			decision = Decision{Status: Unavailable, Reason: "resource_observation_unavailable"}
			return nil
		}
		reservations, err := loadReservations(tx, request.CapacityDomain, settings, now)
		if err != nil {
			return err
		}
		decision = evaluate(settings, request, observation, reservations, now)
		return nil
	})
	if err != nil {
		return Decision{Status: Unavailable, Reason: "admission_store_unavailable"}
	}
	return decision
}

// PublishObservation replaces the latest externally-owned sample and wakes
// waiters for that domain. It never performs a read against the provider.
func (s *Service) PublishObservation(ctx context.Context, observation Observation) error {
	if s == nil || s.db == nil || strings.TrimSpace(observation.CapacityDomain) == "" {
		return errors.New("capacity domain and admission store are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	observation.CapacityDomain = strings.TrimSpace(observation.CapacityDomain)
	if current, ok := s.latest[observation.CapacityDomain]; ok {
		observation = mergeObservation(current, observation)
	}
	s.latest[observation.CapacityDomain] = cloneObservation(observation)
	return s.processWaiters(ctx, observation.CapacityDomain, time.Now().UTC())
}

// Await blocks only on an event or the durable ticket's deadline. It does not
// keep a worker polling policy or provider endpoints.
func (s *Service) Await(ctx context.Context, handle WaitHandle) Decision {
	if s == nil || s.db == nil || handle.id == "" || handle.requestKey == "" {
		return Decision{Status: Unavailable, Reason: "wait_handle_invalid"}
	}
	if ctx.Err() != nil {
		return s.CancelWait(context.Background(), handle)
	}
	for {
		s.mu.Lock()
		var record models.AIResourceAdmissionRecord
		err := s.db.WithContext(ctx).Where("wait_id = ? AND request_key = ?", handle.id, handle.requestKey).First(&record).Error
		if err != nil {
			s.mu.Unlock()
			if ctx.Err() != nil {
				return s.CancelWait(context.Background(), handle)
			}
			return Decision{Status: Unavailable, Reason: "wait_ticket_unavailable"}
		}
		if record.State != stateWaiting {
			decision := decisionFromRecord(record, handle.requestKey)
			s.mu.Unlock()
			return decision
		}
		channel := s.watchers[handle.id]
		if channel == nil {
			channel = make(chan struct{}, 1)
			s.watchers[handle.id] = channel
		}
		deadline := record.WaitDeadline
		s.mu.Unlock()

		timer := time.NewTimer(time.Until(deadline))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return s.CancelWait(context.Background(), handle)
		case <-channel:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			s.expireWait(handle, time.Now().UTC())
		}
	}
}

// CancelWait resolves grant/cancel races explicitly: if a grant already won,
// the returned lease belongs to the caller and must be released if abandoned.
func (s *Service) CancelWait(ctx context.Context, handle WaitHandle) Decision {
	if s == nil || s.db == nil || handle.id == "" || handle.requestKey == "" {
		return Decision{Status: Unavailable, Reason: "wait_handle_invalid"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var decision Decision
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record models.AIResourceAdmissionRecord
		if err := tx.Where("wait_id = ? AND request_key = ?", handle.id, handle.requestKey).First(&record).Error; err != nil {
			return err
		}
		switch record.State {
		case stateWaiting:
			record.State, record.ReasonCode = stateCancelled, "wait_cancelled"
			if err := tx.Save(&record).Error; err != nil {
				return err
			}
			decision = Decision{Status: Denied, Reason: record.ReasonCode}
			if err := recordOutcome(tx, decision, time.Now().UTC()); err != nil {
				return err
			}
		case stateAdmitted:
			decision = decisionFromRecord(record, handle.requestKey)
		default:
			decision = decisionFromRecord(record, handle.requestKey)
		}
		return nil
	})
	if err != nil {
		return Decision{Status: Unavailable, Reason: "wait_ticket_unavailable"}
	}
	s.signal(handle.id)
	return decision
}

// Release is idempotent. It changes only the exact lease journaled for the
// opaque handle; it never unloads a resident model or changes provider state.
func (s *Service) Release(ctx context.Context, lease Lease) error {
	if s == nil || s.db == nil || lease.id == "" || lease.requestKey == "" {
		return ErrInvalidLease
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var capacityDomain string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record models.AIResourceAdmissionRecord
		if err := tx.Where("lease_id = ? AND request_key = ?", lease.id, lease.requestKey).First(&record).Error; err != nil {
			return ErrInvalidLease
		}
		capacityDomain = record.CapacityDomain
		if record.State == stateReleased || record.State == stateExpired {
			return nil
		}
		if record.State != stateAdmitted {
			return ErrInvalidLease
		}
		record.State, record.ReasonCode = stateReleased, "lease_released"
		return tx.Save(&record).Error
	})
	if err != nil {
		return err
	}
	if _, ok := s.latest[capacityDomain]; ok {
		return s.processWaiters(ctx, capacityDomain, time.Now().UTC())
	}
	return nil
}

func (s *Service) Outcomes(ctx context.Context) (Counters, error) {
	var counters Counters
	if s == nil || s.db == nil {
		return counters, errors.New("admission store unavailable")
	}
	var rows []models.AIResourceAdmissionCounter
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return counters, err
	}
	for _, row := range rows {
		switch DecisionStatus(row.Outcome) {
		case Admitted:
			counters.Admitted += row.Count
		case Waiting:
			counters.Waiting += row.Count
		case Denied:
			counters.Denied += row.Count
		case Unavailable:
			counters.Unavailable += row.Count
		}
	}
	return counters, nil
}

func (s *Service) OutcomeReasons(ctx context.Context) ([]OutcomeReason, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("admission store unavailable")
	}
	return OrderedOutcomeReasons(ctx, s.db)
}

func (s *Service) Policy(ctx context.Context) (Settings, error) {
	if s == nil || s.db == nil {
		return Settings{}, errors.New("admission store unavailable")
	}
	return loadSettings(s.db.WithContext(ctx))
}

// ObservationFresh reports only whether all three authorities supplied a
// recent sample. It does not reveal or claim Ollama process configuration.
func (s *Service) ObservationFresh(domain string, now time.Time) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := loadSettings(s.db)
	if err != nil {
		return false
	}
	observation, ok := s.latest[domain]
	if !ok {
		return false
	}
	return observationIsFresh(observation, now, time.Duration(settings.TelemetryMaximumAgeSeconds)*time.Second)
}

func (s *Service) ObservationStatus(domain string, now time.Time) AuthorityStatus {
	status := AuthorityStatus{HostMemory: "unknown", ProviderResidency: "unknown", CriticalServiceGuard: "unknown"}
	if s == nil || s.db == nil || strings.TrimSpace(domain) == "" {
		return status
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := loadSettings(s.db)
	if err != nil {
		return status
	}
	maximumAge := time.Duration(settings.TelemetryMaximumAgeSeconds) * time.Second
	observation, ok := s.latest[domain]
	if !ok {
		return status
	}
	status.HostMemory = authorityFreshness(observation.Host.ObservedAt, now, maximumAge)
	status.ProviderResidency = authorityFreshness(observation.Provider.ObservedAt, now, maximumAge)
	status.CriticalServiceGuard = authorityFreshness(observation.Critical.ObservedAt, now, maximumAge)
	if status.HostMemory == "fresh" && !observation.Host.AvailableMemoryKnown {
		status.HostMemory = "unavailable"
	}
	if status.ProviderResidency == "fresh" && !observation.Provider.Available {
		status.ProviderResidency = "unavailable"
	}
	if status.CriticalServiceGuard == "fresh" {
		if strings.TrimSpace(observation.Critical.Source) == "" {
			status.CriticalServiceGuard = "unavailable"
		} else {
			switch observation.Critical.Status {
			case GuardAllow:
				status.CriticalServiceGuard = "allow"
			case GuardPressure:
				status.CriticalServiceGuard = "pressure"
			default:
				status.CriticalServiceGuard = "unavailable"
			}
		}
	}
	return status
}

func authorityFreshness(observedAt, now time.Time, maximumAge time.Duration) string {
	if observedAt.IsZero() {
		return "unknown"
	}
	if !fresh(observedAt, now, maximumAge) {
		return "stale"
	}
	return "fresh"
}

func (s *Service) expireWait(handle WaitHandle, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.db.Transaction(func(tx *gorm.DB) error {
		var record models.AIResourceAdmissionRecord
		if err := tx.Where("wait_id = ? AND request_key = ?", handle.id, handle.requestKey).First(&record).Error; err != nil {
			return err
		}
		if record.State != stateWaiting || record.WaitDeadline.After(now) {
			return nil
		}
		record.State, record.ReasonCode = stateDenied, "admission_wait_expired"
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		return recordOutcome(tx, Decision{Status: Denied, Reason: record.ReasonCode}, now)
	})
	s.signal(handle.id)
}

func (s *Service) processWaiters(ctx context.Context, domain string, now time.Time) error {
	observation := s.latest[domain]
	var settings Settings
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		settings, err = loadSettings(tx)
		if err != nil {
			return err
		}
		return expireLeases(tx, domain, settings, now)
	}); err != nil {
		return err
	}
	var rows []models.AIResourceAdmissionRecord
	if err := s.db.WithContext(ctx).Where("capacity_domain = ? AND state = ?", domain, stateWaiting).Order("created_at asc, id asc").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if !row.WaitDeadline.After(now) {
			if err := s.transitionWait(row, stateDenied, "admission_wait_expired", Denied, now); err != nil {
				return err
			}
		}
	}
	blocked := map[Purpose]bool{}
	for {
		var pending []models.AIResourceAdmissionRecord
		if err := s.db.WithContext(ctx).Where("capacity_domain = ? AND state = ?", domain, stateWaiting).Order("created_at asc, id asc").Find(&pending).Error; err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
		lanes := map[Purpose]models.AIResourceAdmissionRecord{}
		for _, row := range pending {
			purpose := Purpose(row.Purpose)
			if _, exists := lanes[purpose]; !exists {
				lanes[purpose] = row
			}
		}
		order := []Purpose{PurposeModelLoad, PurposeInference}
		if s.lastGrant[domain] == PurposeModelLoad {
			order = []Purpose{PurposeInference, PurposeModelLoad}
		}
		selected := Purpose("")
		for _, purpose := range order {
			if _, ok := lanes[purpose]; ok && !blocked[purpose] {
				selected = purpose
				break
			}
		}
		if selected == "" {
			return nil
		}
		row := lanes[selected]
		decision := Decision{Status: Unavailable, Reason: "resource_observation_unavailable"}
		if observation.CapacityDomain != "" {
			reservations, err := loadReservations(s.db.WithContext(ctx), domain, settings, now)
			if err != nil {
				return err
			}
			request := requestFromRecord(row)
			decision = evaluate(settings, request, observation, reservations, now)
		}
		switch decision.Status {
		case Admitted:
			leaseID, err := opaqueID()
			if err != nil {
				return err
			}
			if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var current models.AIResourceAdmissionRecord
				if err := tx.First(&current, row.ID).Error; err != nil {
					return err
				}
				if current.State != stateWaiting {
					return nil
				}
				current.State, current.LeaseID, current.ReasonCode = stateAdmitted, leaseID, decision.Reason
				current.EvaluatedHostObservedAt = observation.Host.ObservedAt
				if err := tx.Save(&current).Error; err != nil {
					return err
				}
				return recordOutcome(tx, decision, now)
			}); err != nil {
				return err
			}
			s.lastGrant[domain] = selected
			blocked[selected] = false
			s.signal(row.WaitID)
		case Waiting:
			if err := s.updateWaitingReason(row, decision.Reason); err != nil {
				return err
			}
			blocked[selected] = true
		case Denied:
			if err := s.transitionWait(row, stateDenied, decision.Reason, Denied, now); err != nil {
				return err
			}
		case Unavailable:
			if err := s.transitionWait(row, stateUnavailable, decision.Reason, Unavailable, now); err != nil {
				return err
			}
		}
	}
}

func (s *Service) updateWaitingReason(row models.AIResourceAdmissionRecord, reason string) error {
	return s.db.Model(&models.AIResourceAdmissionRecord{}).Where("id = ? AND state = ?", row.ID, stateWaiting).Update("reason_code", reason).Error
}

func (s *Service) transitionWait(row models.AIResourceAdmissionRecord, state, reason string, status DecisionStatus, now time.Time) error {
	return s.db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		var current models.AIResourceAdmissionRecord
		if err := tx.First(&current, row.ID).Error; err != nil {
			return err
		}
		if current.State != stateWaiting {
			return nil
		}
		current.State, current.ReasonCode = state, reason
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		if err := recordOutcome(tx, Decision{Status: status, Reason: reason}, now); err != nil {
			return err
		}
		s.signal(current.WaitID)
		return nil
	})
}

func (s *Service) signal(waitID string) {
	if ch := s.watchers[waitID]; ch != nil {
		close(ch)
		delete(s.watchers, waitID)
	}
}

func validateRequest(request Request) string {
	fields := []string{request.CapacityDomain, request.ProviderConfigurationID, request.ModelID, request.ConsumerID, request.RequestID}
	for _, field := range fields {
		if strings.TrimSpace(field) == "" || len(field) > 256 {
			return "request_identity_invalid"
		}
	}
	if request.Purpose != PurposeModelLoad && request.Purpose != PurposeInference {
		return "invalid_purpose"
	}
	if request.ContextTokens < 0 || request.Sequences < 0 {
		return "inference_profile_mismatch"
	}
	return ""
}

func requestKey(request Request) string {
	identity := strings.Join([]string{request.CapacityDomain, request.ConsumerID, request.RequestID, string(request.Purpose)}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func normalizeRequest(request Request) Request {
	request.CapacityDomain = strings.TrimSpace(request.CapacityDomain)
	request.ProviderConfigurationID = strings.TrimSpace(request.ProviderConfigurationID)
	request.ModelID = strings.TrimSpace(request.ModelID)
	request.ConsumerID = strings.TrimSpace(request.ConsumerID)
	request.RequestID = strings.TrimSpace(request.RequestID)
	return request
}

func mergeObservation(current, incoming Observation) Observation {
	if incoming.Host.ObservedAt.IsZero() || incoming.Host.ObservedAt.Before(current.Host.ObservedAt) {
		incoming.Host = current.Host
	}
	if incoming.Provider.ObservedAt.IsZero() || incoming.Provider.ObservedAt.Before(current.Provider.ObservedAt) {
		incoming.Provider = current.Provider
	}
	if incoming.Critical.ObservedAt.IsZero() || incoming.Critical.ObservedAt.Before(current.Critical.ObservedAt) {
		incoming.Critical = current.Critical
	}
	return incoming
}

func cloneObservation(observation Observation) Observation {
	if observation.Provider.ResidentModels != nil {
		observation.Provider.ResidentModels = append([]ResidentModel(nil), observation.Provider.ResidentModels...)
	}
	if observation.Host.Metrics != nil {
		metrics := make(map[string]MetricObservation, len(observation.Host.Metrics))
		for key, value := range observation.Host.Metrics {
			metrics[key] = value
		}
		observation.Host.Metrics = metrics
	}
	return observation
}

func opaqueID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("create opaque admission handle: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func loadSettings(db *gorm.DB) (Settings, error) {
	var setting models.AppSetting
	err := db.Where("key = ?", SettingsKey).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	return DecodeSettings(setting.Value)
}

func loadReservations(db *gorm.DB, domain string, settings Settings, now time.Time) ([]activeReservation, error) {
	var rows []models.AIResourceAdmissionRecord
	grace := time.Duration(settings.LeaseCleanupGraceSeconds) * time.Second
	if err := db.Where("capacity_domain = ? AND state = ? AND operation_deadline > ?", domain, stateAdmitted, now.Add(-grace)).Find(&rows).Error; err != nil {
		return nil, err
	}
	reservations := make([]activeReservation, 0, len(rows))
	for _, row := range rows {
		reservations = append(reservations, activeReservation{
			ProviderConfigurationID: row.ProviderConfigurationID, ModelID: row.ModelID,
			Purpose: Purpose(row.Purpose), ContextTokens: row.ContextTokens,
			Sequences: row.Sequences, EvaluatedHostObservedAt: row.EvaluatedHostObservedAt,
		})
	}
	return reservations, nil
}

func expireLeases(db *gorm.DB, domain string, settings Settings, now time.Time) error {
	cutoff := now.Add(-time.Duration(settings.LeaseCleanupGraceSeconds) * time.Second)
	var expired []models.AIResourceAdmissionRecord
	if err := db.Where("capacity_domain = ? AND state = ? AND operation_deadline <= ?", domain, stateAdmitted, cutoff).Find(&expired).Error; err != nil {
		return err
	}
	for _, record := range expired {
		record.State, record.ReasonCode = stateExpired, "lease_expired"
		if err := db.Save(&record).Error; err != nil {
			return err
		}
		if err := recordOutcome(db, Decision{Status: Denied, Reason: record.ReasonCode}, now); err != nil {
			return err
		}
	}
	return nil
}

func admissionRecord(request Request, key, leaseID, waitID, state, reason string, now time.Time) models.AIResourceAdmissionRecord {
	return models.AIResourceAdmissionRecord{
		RequestKey: key, LeaseID: leaseID, WaitID: waitID, CapacityDomain: request.CapacityDomain,
		ProviderConfigurationID: request.ProviderConfigurationID, ModelID: request.ModelID,
		ConsumerID: request.ConsumerID, RequestID: request.RequestID, Purpose: string(request.Purpose),
		State: state, ReasonCode: reason, ContextTokens: request.ContextTokens, Sequences: request.Sequences,
		OperationDeadline: request.OperationDeadline, CreatedAt: now, UpdatedAt: now,
	}
}

func priorDecision(record models.AIResourceAdmissionRecord, request Request, key string) Decision {
	if record.ProviderConfigurationID != request.ProviderConfigurationID || record.ModelID != request.ModelID || record.ContextTokens != request.ContextTokens || record.Sequences != request.Sequences {
		return Decision{Status: Denied, Reason: "request_identity_conflict"}
	}
	return decisionFromRecord(record, key)
}

func decisionFromRecord(record models.AIResourceAdmissionRecord, key string) Decision {
	switch record.State {
	case stateAdmitted:
		return Decision{Status: Admitted, Reason: record.ReasonCode, Lease: &Lease{id: record.LeaseID, requestKey: key}}
	case stateWaiting:
		return Decision{Status: Waiting, Reason: record.ReasonCode, Wait: &WaitHandle{id: record.WaitID, requestKey: key}, RetryAfter: time.Until(record.WaitDeadline)}
	case stateReleased:
		return Decision{Status: Denied, Reason: "lease_already_released"}
	case stateCancelled:
		return Decision{Status: Denied, Reason: "wait_cancelled"}
	case stateExpired:
		return Decision{Status: Denied, Reason: "lease_expired"}
	case stateUnavailable:
		return Decision{Status: Unavailable, Reason: record.ReasonCode}
	default:
		return Decision{Status: Denied, Reason: record.ReasonCode}
	}
}

func requestFromRecord(record models.AIResourceAdmissionRecord) Request {
	return Request{CapacityDomain: record.CapacityDomain, ProviderConfigurationID: record.ProviderConfigurationID, ModelID: record.ModelID,
		ConsumerID: record.ConsumerID, RequestID: record.RequestID, Purpose: Purpose(record.Purpose), ContextTokens: record.ContextTokens,
		Sequences: record.Sequences, OperationDeadline: record.OperationDeadline}
}

func recordOutcome(tx *gorm.DB, decision Decision, now time.Time) error {
	key := string(decision.Status) + ":" + decision.Reason
	row := models.AIResourceAdmissionCounter{Key: key, Outcome: string(decision.Status), Reason: decision.Reason, Count: 1, UpdatedAt: now}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.Assignments(map[string]any{
		"count": gorm.Expr("count + 1"), "updated_at": now,
	})}).Create(&row).Error
}

func persistTerminalRequest(tx *gorm.DB, request Request, key string, decision Decision, now time.Time) error {
	state := stateDenied
	if decision.Status == Unavailable {
		state = stateUnavailable
	}
	record := admissionRecord(request, key, "", "", state, decision.Reason, now)
	if err := tx.Create(&record).Error; err != nil {
		return err
	}
	return recordOutcome(tx, decision, now)
}

// OrderedOutcomeReasons is useful to deterministic status projections.
func OrderedOutcomeReasons(ctx context.Context, db *gorm.DB) ([]OutcomeReason, error) {
	var rows []models.AIResourceAdmissionCounter
	if err := db.WithContext(ctx).Order("outcome asc, reason asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	outcomes := make([]OutcomeReason, 0, len(rows))
	for _, row := range rows {
		outcomes = append(outcomes, OutcomeReason{Outcome: row.Outcome, Reason: row.Reason, Count: row.Count})
	}
	sort.Slice(outcomes, func(i, j int) bool {
		if outcomes[i].Outcome == outcomes[j].Outcome {
			return outcomes[i].Reason < outcomes[j].Reason
		}
		return outcomes[i].Outcome < outcomes[j].Outcome
	})
	return outcomes, nil
}
