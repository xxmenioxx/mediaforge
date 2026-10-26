package nasobserver

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"math"
	"testing"
	"time"
)

const syntheticMeminfo = "MemTotal: 24576000 kB\nMemAvailable: 14336000 kB\nSwapTotal: 8388608 kB\nSwapFree: 6291456 kB\n"

func TestParseLinuxHostSyntheticCounters(t *testing.T) {
	now := time.Now().UTC()
	host, err := ParseLinuxHost([]byte(syntheticMeminfo),
		[]byte("cpu 100 0 100 700 100 0 0 0 0 0\n"),
		[]byte("cpu 150 0 150 760 120 0 0 0 0 0\n"), "proc-linux-v1", now)
	if err != nil {
		t.Fatal(err)
	}
	if host.CPUUsagePercent != (100*float64(100)/float64(180)) ||
		host.AvailableMemoryBytes != 14336000*1024 ||
		host.SwapAvailableBytes != 6291456*1024 ||
		!host.CPUKnown || !host.SwapKnown || !host.CPUObservedAt.Equal(now) {
		t.Fatalf("unexpected host observation: %#v", host)
	}
}

func TestParseLinuxHostRejectsInvalidEvidence(t *testing.T) {
	for _, test := range []struct {
		name, mem, before, after string
	}{
		{"missing MemAvailable", "MemTotal: 100 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB", "cpu 1 1 1 1 1 1 1 1", "cpu 2 2 2 2 2 2 2 2"},
		{"swap contradictory", "MemTotal: 100 kB\nMemAvailable: 50 kB\nSwapTotal: 1 kB\nSwapFree: 2 kB", "cpu 1 1 1 1 1 1 1 1", "cpu 2 2 2 2 2 2 2 2"},
		{"missing delta", syntheticMeminfo, "cpu 1 1 1 1 1 1 1 1", "cpu 1 1 1 1 1 1 1 1"},
		{"decreased counter", syntheticMeminfo, "cpu 2 2 2 2 2 2 2 2", "cpu 1 1 1 1 1 1 1 1"},
		{"missing aggregate", syntheticMeminfo, "cpu0 1 1 1 1 1 1 1 1", "cpu0 2 2 2 2 2 2 2 2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseLinuxHost([]byte(test.mem), []byte(test.before), []byte(test.after), "proc", time.Now().UTC())
			if err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}

func TestParseOllamaPSParsesOnlySuppliedFixture(t *testing.T) {
	now := time.Now().UTC()
	provider, err := ParseOllamaPS([]byte(`{"models":[{"name":"translategemma:12b"},{"name":"qwen3:8b"}]}`), "provider-a", "ollama-ps-v1", now)
	if err != nil || !provider.Available || len(provider.ResidentModels) != 2 ||
		provider.ResidentModels[0].ModelID != "translategemma:12b" {
		t.Fatalf("unexpected provider: %#v, %v", provider, err)
	}
	for _, input := range []string{
		`{}`, `{"models":null}`, `{"models":[{"name":""}]}`,
		`{"models":[{"name":"x"},{"name":"x"}]}`, `not json`,
		`{"Models":[{"name":"x"}]}`, `{"models":[{"Name":"x"}]}`,
		`{"models":[],"models":[{"name":"x"}]}`,
		`{"models":[{"name":"x","name":"y"}]}`,
	} {
		if _, err := ParseOllamaPS([]byte(input), "provider-a", "ollama-ps-v1", now); err == nil {
			t.Fatalf("accepted invalid residency %s", input)
		}
	}
}

func ptr(value bool) *bool { return &value }

func TestGuardFromSignalsFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		name    string
		signals ServiceSignals
		want    string
	}{
		{"missing signal", ServiceSignals{StorageHealthy: ptr(true)}, "unavailable"},
		{"active playback", ServiceSignals{JellyfinPlaybackActive: ptr(true)}, "pressure"},
		{"active ffmpeg", ServiceSignals{FFmpegActive: ptr(true)}, "pressure"},
		{"active worker", ServiceSignals{MVForgeWorkerActive: ptr(true)}, "pressure"},
		{"unhealthy storage", ServiceSignals{StorageHealthy: ptr(false)}, "pressure"},
		{"all explicit idle and healthy", ServiceSignals{
			JellyfinPlaybackActive: ptr(false), FFmpegActive: ptr(false),
			MVForgeWorkerActive: ptr(false), StorageHealthy: ptr(true)}, "allow"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := GuardFromSignals(test.signals, "guard-v1", now)
			if got.Status != test.want {
				t.Fatalf("got %s, want %s", got.Status, test.want)
			}
		})
	}
}

func TestSignSnapshotMatchesEd25519Contract(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload := Payload{
		SchemaVersion: 1, CollectorIdentity: "collector-v1", CapacityDomain: "test-nas",
		Host: Host{SourceIdentity: "proc-v1", ObservedAt: now, TotalMemoryBytes: 24 << 30,
			AvailableMemoryBytes: 12 << 30, SwapKnown: true, SwapObservedAt: now,
			SwapTotalBytes: 8 << 30, SwapAvailableBytes: 6 << 30, CPUKnown: true,
			CPUObservedAt: now, CPUUsagePercent: 20},
		Provider: Provider{SourceIdentity: "ollama-ps-v1", Available: true, ObservedAt: now,
			ResidentModels: []ResidentModel{{ProviderConfigurationID: "provider-a", ModelID: "translategemma:12b"}}},
		Critical: Critical{SourceIdentity: "guard-v1", Status: "unavailable", ObservedAt: now},
	}
	data, err := SignSnapshot(private, payload)
	if err != nil {
		t.Fatal(err)
	}
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Signature == "" || len(envelope.Payload) == 0 {
		t.Fatal("empty signed envelope")
	}
	if string(envelope.Payload) == "" {
		t.Fatal("missing exact payload")
	}
	if _, err := SignSnapshot(private, Payload{}); err == nil {
		t.Fatal("incomplete evidence signed")
	}
	payload.Host.CPUUsagePercent = math.NaN()
	if _, err := SignSnapshot(private, payload); err == nil {
		t.Fatal("NaN signed")
	}
	_ = public
}
