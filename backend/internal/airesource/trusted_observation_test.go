package airesource

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestSignedObservationFileSourceValidatesIdentityAndProjectsFreshness(t *testing.T) {
	config, key := newSignedObservationFixture(t)
	payload := signedObservationFixture(time.Now().UTC())
	writeSignedObservation(t, config.SnapshotPath, payload, key, false)

	observation, err := NewSignedObservationFileSource(config).CurrentObservation(context.Background(), config.CapacityDomain)
	if err != nil {
		t.Fatalf("CurrentObservation() error = %v", err)
	}
	if !observation.authenticatedSource || observation.CapacityDomain != config.CapacityDomain {
		t.Fatalf("trusted identity was not retained: %#v", observation)
	}
	if observation.Host.SourceIdentity != config.HostSourceIdentity || observation.Provider.SourceIdentity != config.ProviderIdentity || observation.Critical.Source != config.CriticalIdentity {
		t.Fatalf("component source identities were not retained: %#v", observation)
	}
	if observation.Host.Metrics["cpuUsagePercent"].Value != 22.5 || observation.Host.Metrics["swapPressurePercent"].Value != 25 {
		t.Fatalf("unexpected CPU/swap metrics: %#v", observation.Host.Metrics)
	}
	if _, err := NewSignedObservationFileSource(config).CurrentObservation(context.Background(), "other-domain"); err != ErrTrustedObservationIdentity {
		t.Fatalf("wrong capacity domain error = %v", err)
	}
}

func TestSignedObservationFileSourceRejectsForgedUnavailableAndMismatchedEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*signedObservationPayload)
		forged bool
		want   error
	}{
		{name: "forged signature", forged: true, want: ErrTrustedObservationUntrusted},
		{name: "collector identity", mutate: func(payload *signedObservationPayload) { payload.CollectorIdentity = "forged-collector" }, want: ErrTrustedObservationIdentity},
		{name: "host source identity", mutate: func(payload *signedObservationPayload) { payload.Host.SourceIdentity = "forged-host" }, want: ErrTrustedObservationIdentity},
		{name: "provider source identity", mutate: func(payload *signedObservationPayload) { payload.Provider.SourceIdentity = "forged-provider" }, want: ErrTrustedObservationIdentity},
		{name: "critical source identity", mutate: func(payload *signedObservationPayload) { payload.Critical.SourceIdentity = "forged-guard" }, want: ErrTrustedObservationIdentity},
		{name: "capacity domain", mutate: func(payload *signedObservationPayload) { payload.CapacityDomain = "other-domain" }, want: ErrTrustedObservationIdentity},
		{name: "invalid RAM", mutate: func(payload *signedObservationPayload) {
			payload.Host.AvailableMemoryBytes = payload.Host.TotalMemoryBytes + 1
		}, want: ErrTrustedObservationUntrusted},
		{name: "unknown swap", mutate: func(payload *signedObservationPayload) { payload.Host.SwapKnown = false }, want: ErrTrustedObservationUntrusted},
		{name: "invalid CPU", mutate: func(payload *signedObservationPayload) { payload.Host.CPUUsagePercent = 101 }, want: ErrTrustedObservationUntrusted},
		{name: "invalid resident identity", mutate: func(payload *signedObservationPayload) { payload.Provider.ResidentModels[0].ModelID = " " }, want: ErrTrustedObservationUntrusted},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, key := newSignedObservationFixture(t)
			payload := signedObservationFixture(time.Now().UTC())
			if test.mutate != nil {
				test.mutate(&payload)
			}
			writeSignedObservation(t, config.SnapshotPath, payload, key, test.forged)
			if _, err := NewSignedObservationFileSource(config).CurrentObservation(context.Background(), config.CapacityDomain); err != test.want {
				t.Fatalf("CurrentObservation() error = %v, want %v", err, test.want)
			}
		})
	}

	config, _ := newSignedObservationFixture(t)
	if _, err := NewSignedObservationFileSource(config).CurrentObservation(context.Background(), config.CapacityDomain); err != ErrTrustedObservationUnavailable {
		t.Fatalf("missing snapshot error = %v", err)
	}

	config, key := newSignedObservationFixture(t)
	target := filepath.Join(filepath.Dir(config.SnapshotPath), "snapshot-target.json")
	writeSignedObservation(t, target, signedObservationFixture(time.Now().UTC()), key, false)
	if err := os.Symlink(target, config.SnapshotPath); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSignedObservationFileSource(config).CurrentObservation(context.Background(), config.CapacityDomain); err != ErrTrustedObservationUnavailable {
		t.Fatalf("symlink snapshot error = %v", err)
	}
}

func TestTrustedAdmissionRejectsStaleOrForgedEvidenceAndUnsignedPublish(t *testing.T) {
	staleCases := []struct {
		name   string
		mutate func(*signedObservationPayload)
		reason string
	}{
		{name: "memory", mutate: func(payload *signedObservationPayload) {
			payload.Host.ObservedAt = time.Now().Add(-2 * time.Minute).UTC()
		}, reason: "host_memory_unavailable"},
		{name: "CPU", mutate: func(payload *signedObservationPayload) {
			payload.Host.CPUObservedAt = time.Now().Add(-2 * time.Minute).UTC()
		}, reason: "required_pressure_metric_unavailable"},
		{name: "swap", mutate: func(payload *signedObservationPayload) {
			payload.Host.SwapObservedAt = time.Now().Add(-2 * time.Minute).UTC()
		}, reason: "required_pressure_metric_unavailable"},
		{name: "provider", mutate: func(payload *signedObservationPayload) {
			payload.Provider.ObservedAt = time.Now().Add(-2 * time.Minute).UTC()
		}, reason: "provider_residency_unavailable"},
		{name: "critical guard", mutate: func(payload *signedObservationPayload) {
			payload.Critical.ObservedAt = time.Now().Add(-2 * time.Minute).UTC()
		}, reason: "critical_service_guard_unavailable"},
	}
	for _, stale := range staleCases {
		t.Run("stale "+stale.name, func(t *testing.T) {
			db, _ := newServiceFixture(t, serviceSettings())
			config, key := newSignedObservationFixture(t)
			payload := signedObservationFixture(time.Now().UTC())
			stale.mutate(&payload)
			writeSignedObservation(t, config.SnapshotPath, payload, key, false)
			service := NewServiceWithTrustedObservationSource(db, NewSignedObservationFileSource(config))
			request := serviceRequest("stale-"+stale.name, PurposeInference, "model-a")
			request.CapacityDomain = config.CapacityDomain
			decision := service.Admit(context.Background(), request)
			if decision.Status != Unavailable || decision.Lease != nil || decision.Reason != stale.reason {
				t.Fatalf("stale evidence decision = %#v, want reason %q", decision, stale.reason)
			}
		})
	}

	t.Run("forged update clears previously trusted cache", func(t *testing.T) {
		db, _ := newServiceFixture(t, serviceSettings())
		config, key := newSignedObservationFixture(t)
		payload := signedObservationFixture(time.Now().UTC())
		writeSignedObservation(t, config.SnapshotPath, payload, key, false)
		service := NewServiceWithTrustedObservationSource(db, NewSignedObservationFileSource(config))
		if err := service.PublishObservation(context.Background(), serviceObservation(time.Now().UTC(), 20*testGiB, GuardAllow)); err == nil {
			t.Fatal("trusted service accepted an unsigned in-memory observation")
		}
		firstRequest := serviceRequest("trusted-first", PurposeInference, "model-a")
		firstRequest.CapacityDomain = config.CapacityDomain
		first := service.Admit(context.Background(), firstRequest)
		if first.Status != Admitted || first.Lease == nil {
			t.Fatalf("valid signed sample admission = %#v", first)
		}
		if err := service.Release(context.Background(), *first.Lease); err != nil {
			t.Fatal(err)
		}
		payload.Host.AvailableMemoryBytes = 1
		writeSignedObservation(t, config.SnapshotPath, payload, key, true)
		secondRequest := serviceRequest("forged-second", PurposeInference, "model-a")
		secondRequest.CapacityDomain = config.CapacityDomain
		second := service.Admit(context.Background(), secondRequest)
		if second.Status != Unavailable || second.Lease != nil {
			t.Fatalf("forged sample admission = %#v", second)
		}
	})
}

func TestTrustedPreviewDoesNotProcessExistingWaiters(t *testing.T) {
	db, _ := newServiceFixture(t, serviceSettings())
	config, key := newSignedObservationFixture(t)
	payload := signedObservationFixture(time.Now().UTC())
	payload.Critical.Status = GuardPressure
	writeSignedObservation(t, config.SnapshotPath, payload, key, false)
	service := NewServiceWithTrustedObservationSource(db, NewSignedObservationFileSource(config))
	waitingRequest := serviceRequest("preview-waiter", PurposeInference, "model-a")
	waitingRequest.CapacityDomain = config.CapacityDomain
	waiting := service.Admit(context.Background(), waitingRequest)
	if waiting.Status != Waiting || waiting.Wait == nil {
		t.Fatalf("initial admission = %#v, want WAITING", waiting)
	}

	payload.Critical.Status = GuardAllow
	writeSignedObservation(t, config.SnapshotPath, payload, key, false)
	previewRequest := serviceRequest("preview-only", PurposeInference, "model-a")
	previewRequest.CapacityDomain = config.CapacityDomain
	preview := service.Preview(context.Background(), previewRequest)
	if preview.Status != Admitted || preview.Lease != nil {
		t.Fatalf("preview = %#v, want lease-free ADMITTED decision", preview)
	}
	var record models.AIResourceAdmissionRecord
	if err := db.Where("request_id = ?", waitingRequest.RequestID).First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.State != stateWaiting || record.LeaseID != "" {
		t.Fatalf("Preview changed waiting request: state=%q lease=%q", record.State, record.LeaseID)
	}
}

func newSignedObservationFixture(t *testing.T) (SignedObservationFileConfig, ed25519.PrivateKey) {
	t.Helper()
	directory := t.TempDir()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(directory, "observation.pub")
	if err := os.WriteFile(keyPath, publicKey, 0644); err != nil {
		t.Fatal(err)
	}
	return SignedObservationFileConfig{
		SnapshotPath: filepath.Join(directory, "observation.json"), PublicKeyPath: keyPath,
		CapacityDomain: "test-nas", CollectorIdentity: "host-observer-v1",
		HostSourceIdentity: "linux-proc-v1", ProviderIdentity: "ollama-readonly-v1",
		CriticalIdentity: "service-guard-v1",
	}, privateKey
}

func signedObservationFixture(now time.Time) signedObservationPayload {
	return signedObservationPayload{
		SchemaVersion: 1, CollectorIdentity: "host-observer-v1", CapacityDomain: "test-nas",
		Host: signedHostObservation{
			SourceIdentity: "linux-proc-v1", ObservedAt: now, TotalMemoryBytes: 32 * testGiB,
			AvailableMemoryBytes: 20 * testGiB, SwapKnown: true, SwapObservedAt: now,
			SwapTotalBytes: 8 * testGiB, SwapAvailableBytes: 6 * testGiB,
			CPUKnown: true, CPUObservedAt: now, CPUUsagePercent: 22.5,
		},
		Provider: signedProviderObservation{
			SourceIdentity: "ollama-readonly-v1", Available: true, ObservedAt: now,
			ResidentModels: []signedResidentModel{{ProviderConfigurationID: "provider-a", ModelID: "model-a"}},
		},
		Critical: signedCriticalServiceGuard{SourceIdentity: "service-guard-v1", Status: GuardAllow, ObservedAt: now},
	}
}

func writeSignedObservation(t *testing.T, path string, payload signedObservationPayload, key ed25519.PrivateKey, forged bool) {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	signatureBytes := ed25519.Sign(key, payloadBytes)
	if forged {
		signatureBytes[len(signatureBytes)-1] ^= 1
	}
	envelope, err := json.Marshal(signedObservationEnvelope{Payload: payloadBytes, Signature: hex.EncodeToString(signatureBytes)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, envelope, 0600); err != nil {
		t.Fatal(err)
	}
}
