package nasobserver

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/airesource"
)

// TestSignedSnapshotReaderRoundTrip verifies the actual canonical #431 reader,
// not just the JSON envelope structure or the isolated Ed25519 signer.
func TestSignedSnapshotReaderRoundTrip(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload := Payload{
		SchemaVersion:     1,
		CollectorIdentity: "fixture-collector",
		CapacityDomain:    "fixture-nas",
		Host: Host{
			SourceIdentity:       "fixture-proc",
			ObservedAt:           now,
			TotalMemoryBytes:     24 << 30,
			AvailableMemoryBytes: 12 << 30,
			SwapKnown:            true,
			SwapObservedAt:       now,
			SwapTotalBytes:       8 << 30,
			SwapAvailableBytes:   6 << 30,
			CPUKnown:             true,
			CPUObservedAt:        now,
			CPUUsagePercent:      22.5,
		},
		Provider: Provider{
			SourceIdentity: "fixture-ollama",
			Available:      true,
			ObservedAt:     now,
			ResidentModels: []ResidentModel{
				{ProviderConfigurationID: "provider-fixture", ModelID: "translategemma:12b"},
			},
		},
		Critical: Critical{
			SourceIdentity: "fixture-guard",
			Status:         "unavailable",
			ObservedAt:     now,
		},
	}
	signed, err := SignSnapshot(privateKey, payload)
	if err != nil {
		t.Fatal(err)
	}
	var envelope Envelope
	if err := json.Unmarshal(signed, &envelope); err != nil {
		t.Fatal(err)
	}
	signature, err := hex.DecodeString(envelope.Signature)
	if err != nil || !ed25519.Verify(publicKey, envelope.Payload, signature) {
		t.Fatal("envelope signature does not authenticate exact payload")
	}
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "snapshot.json")
	keyPath := filepath.Join(dir, "observer.pub")
	if err := os.WriteFile(snapshotPath, signed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, publicKey, 0600); err != nil {
		t.Fatal(err)
	}
	source := airesource.NewSignedObservationFileSource(airesource.SignedObservationFileConfig{
		SnapshotPath:       snapshotPath,
		PublicKeyPath:      keyPath,
		CapacityDomain:     payload.CapacityDomain,
		CollectorIdentity:  payload.CollectorIdentity,
		HostSourceIdentity: payload.Host.SourceIdentity,
		ProviderIdentity:   payload.Provider.SourceIdentity,
		CriticalIdentity:   payload.Critical.SourceIdentity,
	})
	got, err := source.CurrentObservation(t.Context(), payload.CapacityDomain)
	if err != nil {
		t.Fatalf("canonical reader rejected valid signed fixture: %v", err)
	}
	if got.Host.Metrics["cpuUsagePercent"].Value != 22.5 ||
		got.Host.Metrics["swapPressurePercent"].Value != 25 ||
		got.Provider.ResidentModels[0].ModelID != "translategemma:12b" ||
		got.Critical.Status != airesource.GuardUnavailable {
		t.Fatalf("signed fixture not preserved through canonical reader: %#v", got)
	}

	// Re-serialize modified evidence while preserving the original signature.
	// This is always rejected, even when all new values look plausible.
	envelope.Payload = json.RawMessage(append([]byte(nil), envelope.Payload...))
	var modified Payload
	if err := json.Unmarshal(envelope.Payload, &modified); err != nil {
		t.Fatal(err)
	}
	modified.Host.CPUUsagePercent = 23.5
	envelope.Payload, err = json.Marshal(modified)
	if err != nil {
		t.Fatal(err)
	}
	tampered, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, tampered, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.CurrentObservation(t.Context(), payload.CapacityDomain); err != airesource.ErrTrustedObservationUntrusted {
		t.Fatalf("tampered signed evidence accepted or returned wrong error: %v", err)
	}
}
