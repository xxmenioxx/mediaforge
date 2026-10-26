package airesource

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"syscall"
	"time"
)

const (
	maxObservationSnapshotBytes  = 1 << 20
	maxObservationPublicKeyBytes = 1024
)

var (
	ErrTrustedObservationUnavailable = errors.New("trusted observation unavailable")
	ErrTrustedObservationUntrusted   = errors.New("trusted observation failed authentication")
	ErrTrustedObservationIdentity    = errors.New("trusted observation identity mismatch")
)

// SignedObservationFileConfig binds a local, read-only snapshot to the exact
// capacity domain and observation authorities expected by the application.
// The Ed25519 verification key is provisioned out of band and must be mounted
// read-only. The signing key never enters the application container.
type SignedObservationFileConfig struct {
	SnapshotPath       string
	PublicKeyPath      string
	CapacityDomain     string
	CollectorIdentity  string
	HostSourceIdentity string
	ProviderIdentity   string
	CriticalIdentity   string
}

// SignedObservationFileSource authenticates a bounded JSON snapshot with
// Ed25519 before decoding any capacity facts. It performs filesystem reads
// only; it never contacts Ollama or any service endpoint.
type SignedObservationFileSource struct {
	config SignedObservationFileConfig
}

var _ TrustedObservationSource = (*SignedObservationFileSource)(nil)

func NewSignedObservationFileSource(config SignedObservationFileConfig) *SignedObservationFileSource {
	return &SignedObservationFileSource{config: config}
}

// NewSignedObservationFileSourceFromEnvironment configures the production
// admission reader. Missing settings produce an unavailable source, never a
// permissive fallback to local RuntimeState or unsigned observations.
func NewSignedObservationFileSourceFromEnvironment() *SignedObservationFileSource {
	return NewSignedObservationFileSource(SignedObservationFileConfig{
		SnapshotPath:       os.Getenv("MVFORGE_TRUSTED_OBSERVATION_PATH"),
		PublicKeyPath:      os.Getenv("MVFORGE_TRUSTED_OBSERVATION_PUBLIC_KEY_PATH"),
		CapacityDomain:     os.Getenv("MVFORGE_TRUSTED_CAPACITY_DOMAIN"),
		CollectorIdentity:  os.Getenv("MVFORGE_TRUSTED_COLLECTOR_ID"),
		HostSourceIdentity: os.Getenv("MVFORGE_TRUSTED_HOST_SOURCE_ID"),
		ProviderIdentity:   os.Getenv("MVFORGE_TRUSTED_PROVIDER_SOURCE_ID"),
		CriticalIdentity:   os.Getenv("MVFORGE_TRUSTED_CRITICAL_SOURCE_ID"),
	})
}

type signedObservationEnvelope struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

type signedObservationPayload struct {
	SchemaVersion     int                        `json:"schemaVersion"`
	CollectorIdentity string                     `json:"collectorIdentity"`
	CapacityDomain    string                     `json:"capacityDomain"`
	Host              signedHostObservation      `json:"host"`
	Provider          signedProviderObservation  `json:"provider"`
	Critical          signedCriticalServiceGuard `json:"critical"`
}

type signedHostObservation struct {
	SourceIdentity       string    `json:"sourceIdentity"`
	ObservedAt           time.Time `json:"observedAt"`
	TotalMemoryBytes     int64     `json:"totalMemoryBytes"`
	AvailableMemoryBytes int64     `json:"availableMemoryBytes"`
	SwapKnown            bool      `json:"swapKnown"`
	SwapObservedAt       time.Time `json:"swapObservedAt"`
	SwapTotalBytes       int64     `json:"swapTotalBytes"`
	SwapAvailableBytes   int64     `json:"swapAvailableBytes"`
	CPUKnown             bool      `json:"cpuKnown"`
	CPUObservedAt        time.Time `json:"cpuObservedAt"`
	CPUUsagePercent      float64   `json:"cpuUsagePercent"`
}

type signedProviderObservation struct {
	SourceIdentity string                `json:"sourceIdentity"`
	Available      bool                  `json:"available"`
	ObservedAt     time.Time             `json:"observedAt"`
	ResidentModels []signedResidentModel `json:"residentModels"`
}

type signedResidentModel struct {
	ProviderConfigurationID string `json:"providerConfigurationId"`
	ModelID                 string `json:"modelId"`
}

type signedCriticalServiceGuard struct {
	SourceIdentity string      `json:"sourceIdentity"`
	Status         GuardStatus `json:"status"`
	ObservedAt     time.Time   `json:"observedAt"`
}

func (source *SignedObservationFileSource) CurrentObservation(ctx context.Context, capacityDomain string) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	if source == nil || !source.config.valid() {
		return Observation{}, ErrTrustedObservationUnavailable
	}
	if capacityDomain != source.config.CapacityDomain {
		return Observation{}, ErrTrustedObservationIdentity
	}

	publicKey, err := readVerificationKey(source.config.PublicKeyPath)
	if err != nil {
		return Observation{}, ErrTrustedObservationUnavailable
	}
	snapshot, err := readBoundedRegularFile(source.config.SnapshotPath, maxObservationSnapshotBytes, true)
	if err != nil {
		return Observation{}, ErrTrustedObservationUnavailable
	}
	var envelope signedObservationEnvelope
	if err := decodeStrictJSON(snapshot, &envelope); err != nil || len(envelope.Payload) == 0 {
		return Observation{}, ErrTrustedObservationUntrusted
	}
	signature, err := hex.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Observation{}, ErrTrustedObservationUntrusted
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), envelope.Payload, signature) {
		return Observation{}, ErrTrustedObservationUntrusted
	}

	var payload signedObservationPayload
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil {
		return Observation{}, ErrTrustedObservationUntrusted
	}
	if !source.config.matches(payload) {
		return Observation{}, ErrTrustedObservationIdentity
	}
	observation, ok := normalizeSignedObservation(payload)
	if !ok {
		return Observation{}, ErrTrustedObservationUntrusted
	}
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	return observation, nil
}

func (config SignedObservationFileConfig) valid() bool {
	return filepathIsAbsolute(config.SnapshotPath) && filepathIsAbsolute(config.PublicKeyPath) &&
		exactIdentity(config.CapacityDomain) && exactIdentity(config.CollectorIdentity) &&
		exactIdentity(config.HostSourceIdentity) && exactIdentity(config.ProviderIdentity) &&
		exactIdentity(config.CriticalIdentity)
}

func (config SignedObservationFileConfig) matches(payload signedObservationPayload) bool {
	return payload.SchemaVersion == 1 && payload.CapacityDomain == config.CapacityDomain &&
		payload.CollectorIdentity == config.CollectorIdentity &&
		payload.Host.SourceIdentity == config.HostSourceIdentity &&
		payload.Provider.SourceIdentity == config.ProviderIdentity &&
		payload.Critical.SourceIdentity == config.CriticalIdentity
}

func normalizeSignedObservation(payload signedObservationPayload) (Observation, bool) {
	host := payload.Host
	if !host.SwapKnown || !host.CPUKnown || host.ObservedAt.IsZero() || host.SwapObservedAt.IsZero() || host.CPUObservedAt.IsZero() ||
		host.TotalMemoryBytes <= 0 || host.AvailableMemoryBytes < 0 || host.AvailableMemoryBytes > host.TotalMemoryBytes ||
		host.SwapTotalBytes < 0 || host.SwapAvailableBytes < 0 || host.SwapAvailableBytes > host.SwapTotalBytes ||
		math.IsNaN(host.CPUUsagePercent) || math.IsInf(host.CPUUsagePercent, 0) || host.CPUUsagePercent < 0 || host.CPUUsagePercent > 100 {
		return Observation{}, false
	}
	swapPressure := 0.0
	if host.SwapTotalBytes > 0 {
		swapPressure = float64(host.SwapTotalBytes-host.SwapAvailableBytes) / float64(host.SwapTotalBytes) * 100
	}
	hostObservation := HostObservation{
		SourceIdentity: host.SourceIdentity, AvailableMemoryKnown: true,
		TotalMemoryBytes: host.TotalMemoryBytes, AvailableMemoryBytes: host.AvailableMemoryBytes,
		ObservedAt: host.ObservedAt,
		Metrics: map[string]MetricObservation{
			"cpuUsagePercent":     {Available: true, Value: host.CPUUsagePercent, ObservedAt: host.CPUObservedAt},
			"swapPressurePercent": {Available: true, Value: swapPressure, ObservedAt: host.SwapObservedAt},
		},
	}
	provider := payload.Provider
	if provider.ObservedAt.IsZero() || len(provider.ResidentModels) > 256 {
		return Observation{}, false
	}
	seenModels := make(map[string]struct{}, len(provider.ResidentModels))
	residentModels := make([]ResidentModel, 0, len(provider.ResidentModels))
	for _, resident := range provider.ResidentModels {
		if !exactIdentity(resident.ProviderConfigurationID) || !exactIdentity(resident.ModelID) {
			return Observation{}, false
		}
		key := resident.ProviderConfigurationID + "\x00" + resident.ModelID
		if _, exists := seenModels[key]; exists {
			return Observation{}, false
		}
		seenModels[key] = struct{}{}
		residentModels = append(residentModels, ResidentModel{ProviderConfigurationID: resident.ProviderConfigurationID, ModelID: resident.ModelID})
	}
	if payload.Critical.ObservedAt.IsZero() || (payload.Critical.Status != GuardAllow && payload.Critical.Status != GuardPressure && payload.Critical.Status != GuardUnavailable) {
		return Observation{}, false
	}
	return Observation{
		CapacityDomain: payload.CapacityDomain,
		Host:           hostObservation,
		Provider: ProviderObservation{
			SourceIdentity: provider.SourceIdentity, Available: provider.Available,
			ObservedAt: provider.ObservedAt, ResidentModels: residentModels,
		},
		Critical:            CriticalServiceObservation{Source: payload.Critical.SourceIdentity, Status: payload.Critical.Status, ObservedAt: payload.Critical.ObservedAt},
		authenticatedSource: true,
	}, true
}

func readVerificationKey(path string) ([]byte, error) {
	contents, info, err := readRegularFile(path, maxObservationPublicKeyBytes)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0022 != 0 || len(contents) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("observation verification key permissions or size are invalid")
	}
	return contents, nil
}

func readBoundedRegularFile(path string, maximum int64, rejectWritable bool) ([]byte, error) {
	contents, info, err := readRegularFile(path, maximum)
	if err != nil {
		return nil, err
	}
	if rejectWritable && info.Mode().Perm()&0022 != 0 {
		return nil, fmt.Errorf("observation snapshot permissions are invalid")
	}
	return contents, nil
}

func readRegularFile(path string, maximum int64) ([]byte, os.FileInfo, error) {
	if !filepathIsAbsolute(path) || maximum < 1 {
		return nil, nil, fmt.Errorf("absolute observation path is required")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("observation path is not a regular file")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("observation path is not a regular file")
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(contents)) > maximum {
		return nil, nil, fmt.Errorf("observation file exceeds its size bound")
	}
	return contents, info, nil
}

func decodeStrictJSON(contents []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func exactIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= 256
}

func filepathIsAbsolute(path string) bool {
	return path != "" && strings.HasPrefix(path, string(os.PathSeparator))
}
