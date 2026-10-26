// Package nasobserver provides read-only, fixture-testable collection and
// signing primitives. It is NOT a deployed NAS observer or an authorization
// path for real Ollama inference.
package nasobserver

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
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxProcBytes     = 128 << 10
	maxResidencyBody = 1 << 20
	maxSnapshotBytes = 1 << 20
	maxModels        = 256
)

type Host struct {
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

type ResidentModel struct {
	ProviderConfigurationID string `json:"providerConfigurationId"`
	ModelID                 string `json:"modelId"`
}

type Provider struct {
	SourceIdentity string          `json:"sourceIdentity"`
	Available      bool            `json:"available"`
	ObservedAt     time.Time       `json:"observedAt"`
	ResidentModels []ResidentModel `json:"residentModels"`
}

type Critical struct {
	SourceIdentity string    `json:"sourceIdentity"`
	Status         string    `json:"status"`
	ObservedAt     time.Time `json:"observedAt"`
}

type Payload struct {
	SchemaVersion     int      `json:"schemaVersion"`
	CollectorIdentity string   `json:"collectorIdentity"`
	CapacityDomain    string   `json:"capacityDomain"`
	Host              Host     `json:"host"`
	Provider          Provider `json:"provider"`
	Critical          Critical `json:"critical"`
}

type Envelope struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

func validIdentity(s string) bool {
	return s != "" && len(s) <= 256 && strings.TrimSpace(s) == s && !strings.ContainsRune(s, 0)
}

// ParseLinuxHost computes memory, swap and CPU from synthetic or captured Linux
// procfs bytes. CPU requires two monotonically advancing aggregate counters;
// a single CPU load average or point-in-time reading is not accepted.
func ParseLinuxHost(meminfo, statBefore, statAfter []byte, source string, observedAt time.Time) (Host, error) {
	if !validIdentity(source) || observedAt.IsZero() {
		return Host{}, errors.New("host observation source and timestamp required")
	}
	memory, err := parseMeminfo(meminfo)
	if err != nil {
		return Host{}, err
	}
	before, err := parseCPUCounters(statBefore)
	if err != nil {
		return Host{}, err
	}
	after, err := parseCPUCounters(statAfter)
	if err != nil {
		return Host{}, err
	}
	var totalDelta, idleDelta uint64
	for i := range before {
		if after[i] < before[i] {
			return Host{}, errors.New("nonmonotonic aggregate CPU counters")
		}
		delta := after[i] - before[i]
		if ^uint64(0)-totalDelta < delta {
			return Host{}, errors.New("aggregate CPU counter overflow")
		}
		totalDelta += delta
		if i == 3 || i == 4 { // idle + iowait; guest fields are excluded
			idleDelta += delta
		}
	}
	if totalDelta == 0 || idleDelta > totalDelta {
		return Host{}, errors.New("aggregate CPU delta unavailable")
	}
	cpu := 100 * float64(totalDelta-idleDelta) / float64(totalDelta)
	return Host{
		SourceIdentity: source, ObservedAt: observedAt,
		TotalMemoryBytes: memory["MemTotal"], AvailableMemoryBytes: memory["MemAvailable"],
		SwapKnown: true, SwapObservedAt: observedAt,
		SwapTotalBytes: memory["SwapTotal"], SwapAvailableBytes: memory["SwapFree"],
		CPUKnown: true, CPUObservedAt: observedAt, CPUUsagePercent: cpu,
	}, nil
}

func parseMeminfo(content []byte) (map[string]int64, error) {
	if len(content) == 0 || len(content) > maxProcBytes {
		return nil, errors.New("invalid proc meminfo size")
	}
	values := make(map[string]int64, 4)
	for _, line := range strings.Split(string(content), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		switch key {
		case "MemTotal", "MemAvailable", "SwapTotal", "SwapFree":
			if _, present := values[key]; present || len(parts) != 3 || parts[2] != "kB" {
				return nil, fmt.Errorf("invalid or duplicate proc metric %s", key)
			}
			kb, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil || kb < 0 || kb > int64(^uint64(0)>>1)/1024 {
				return nil, fmt.Errorf("invalid proc metric %s", key)
			}
			values[key] = kb * 1024
		}
	}
	if len(values) != 4 || values["MemTotal"] <= 0 ||
		values["MemAvailable"] > values["MemTotal"] ||
		values["SwapFree"] > values["SwapTotal"] {
		return nil, errors.New("missing or contradictory memory/swap evidence")
	}
	return values, nil
}

func parseCPUCounters(content []byte) ([8]uint64, error) {
	var result [8]uint64
	if len(content) == 0 || len(content) > maxProcBytes {
		return result, errors.New("invalid proc stat size")
	}
	line := strings.SplitN(string(content), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 9 || fields[0] != "cpu" {
		return result, errors.New("aggregate cpu counters unavailable")
	}
	for index := range result {
		value, err := strconv.ParseUint(fields[index+1], 10, 64)
		if err != nil {
			return result, errors.New("invalid aggregate cpu counter")
		}
		result[index] = value
	}
	return result, nil
}

// CollectLinuxHost is a read-only Linux procfs adapter. It must be explicitly
// invoked by a separately reviewed host process, never by preflight or CI.
// No network access, Docker socket or production application state is used.
func CollectLinuxHost(ctx context.Context, procRoot, source string, interval time.Duration) (Host, error) {
	if !filepath.IsAbs(procRoot) || interval < 100*time.Millisecond || interval > 5*time.Second {
		return Host{}, errors.New("absolute proc root and bounded CPU interval required")
	}
	before, err := readBounded(ctx, filepath.Join(procRoot, "stat"), maxProcBytes)
	if err != nil {
		return Host{}, fmt.Errorf("read initial CPU counters: %w", err)
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Host{}, ctx.Err()
	case <-timer.C:
	}
	after, err := readBounded(ctx, filepath.Join(procRoot, "stat"), maxProcBytes)
	if err != nil {
		return Host{}, fmt.Errorf("read subsequent CPU counters: %w", err)
	}
	memory, err := readBounded(ctx, filepath.Join(procRoot, "meminfo"), maxProcBytes)
	if err != nil {
		return Host{}, fmt.Errorf("read memory/swap counters: %w", err)
	}
	return ParseLinuxHost(memory, before, after, source, time.Now().UTC())
}

func readBounded(ctx context.Context, path string, maximum int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, errors.New("proc input unreadable or oversized")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

// ParseOllamaPS consumes bounded bytes obtained separately from the read-only
// /api/ps endpoint. It never opens a connection or proves endpoint identity.
// A missing models field fails closed; an explicit empty list means none resident.
func ParseOllamaPS(content []byte, providerID, source string, observedAt time.Time) (Provider, error) {
	if !validIdentity(source) || !validIdentity(providerID) || observedAt.IsZero() ||
		len(content) == 0 || len(content) > maxResidencyBody {
		return Provider{}, errors.New("invalid Ollama residency evidence")
	}
	if err := validateUniqueJSON(content); err != nil {
		return Provider{}, errors.New("malformed Ollama residency JSON")
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(content, &response); err != nil || response == nil {
		return Provider{}, errors.New("missing or malformed Ollama residency")
	}
	modelsJSON, present := response["models"]
	if !present {
		return Provider{}, errors.New("missing or malformed Ollama residency")
	}
	var models []json.RawMessage
	if err := json.Unmarshal(modelsJSON, &models); err != nil || models == nil || len(models) > maxModels {
		return Provider{}, errors.New("missing or malformed Ollama residency")
	}
	residents := make([]ResidentModel, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, modelJSON := range models {
		var model map[string]json.RawMessage
		if err := json.Unmarshal(modelJSON, &model); err != nil || model == nil {
			return Provider{}, errors.New("unidentified resident model")
		}
		nameJSON, present := model["name"]
		var name string
		if !present || json.Unmarshal(nameJSON, &name) != nil || !validIdentity(name) {
			return Provider{}, errors.New("unidentified resident model")
		}
		if _, exists := seen[name]; exists {
			return Provider{}, errors.New("duplicate resident model identity")
		}
		seen[name] = struct{}{}
		residents = append(residents, ResidentModel{ProviderConfigurationID: providerID, ModelID: name})
	}
	return Provider{SourceIdentity: source, Available: true, ObservedAt: observedAt, ResidentModels: residents}, nil
}

func validateUniqueJSON(content []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid JSON object key")
			}
			if _, duplicate := keys[key]; duplicate {
				return errors.New("duplicate JSON object key")
			}
			keys[key] = struct{}{}
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("unterminated JSON array")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

// ServiceSignals describe independently observed critical-service activity.
// Nil means unknown, NOT idle. The logic never reads a process list and does
// not mistake a running Jellyfin service for an active or idle playback session.
type ServiceSignals struct {
	JellyfinPlaybackActive *bool
	FFmpegActive           *bool
	MVForgeWorkerActive    *bool
	StorageHealthy         *bool
}

func GuardFromSignals(signals ServiceSignals, source string, observedAt time.Time) Critical {
	status := "unavailable"
	if validIdentity(source) && !observedAt.IsZero() {
		switch {
		case isTrue(signals.JellyfinPlaybackActive), isTrue(signals.FFmpegActive), isTrue(signals.MVForgeWorkerActive):
			status = "pressure"
		case signals.StorageHealthy != nil && !*signals.StorageHealthy:
			status = "pressure"
		case signals.JellyfinPlaybackActive != nil && signals.FFmpegActive != nil &&
			signals.MVForgeWorkerActive != nil && signals.StorageHealthy != nil:
			status = "allow"
		}
	}
	return Critical{SourceIdentity: source, Status: status, ObservedAt: observedAt}
}

func isTrue(v *bool) bool { return v != nil && *v }

// SignSnapshot signs the exact embedded JSON bytes understood by the existing
// #431 reader. It deliberately does not load private keys or publish files:
// signer custody and atomic publication belong to a reviewed NAS-only service.
func SignSnapshot(privateKey ed25519.PrivateKey, payload Payload) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize || payload.SchemaVersion != 1 ||
		!validIdentity(payload.CollectorIdentity) || !validIdentity(payload.CapacityDomain) ||
		!validIdentity(payload.Host.SourceIdentity) || !validIdentity(payload.Provider.SourceIdentity) ||
		!validIdentity(payload.Critical.SourceIdentity) ||
		payload.Host.ObservedAt.IsZero() || payload.Host.CPUObservedAt.IsZero() ||
		payload.Host.SwapObservedAt.IsZero() || payload.Provider.ObservedAt.IsZero() ||
		payload.Critical.ObservedAt.IsZero() {
		return nil, errors.New("incomplete signing identity or evidence")
	}
	if payload.Host.TotalMemoryBytes <= 0 || payload.Host.AvailableMemoryBytes < 0 ||
		payload.Host.AvailableMemoryBytes > payload.Host.TotalMemoryBytes ||
		payload.Host.SwapTotalBytes < 0 || payload.Host.SwapAvailableBytes < 0 ||
		payload.Host.SwapAvailableBytes > payload.Host.SwapTotalBytes ||
		!payload.Host.CPUKnown || !payload.Host.SwapKnown ||
		math.IsNaN(payload.Host.CPUUsagePercent) || math.IsInf(payload.Host.CPUUsagePercent, 0) ||
		payload.Host.CPUUsagePercent < 0 || payload.Host.CPUUsagePercent > 100 ||
		payload.Critical.Status != "allow" && payload.Critical.Status != "pressure" && payload.Critical.Status != "unavailable" ||
		len(payload.Provider.ResidentModels) > maxModels {
		return nil, errors.New("invalid signed observation values")
	}
	if payload.Critical.Status == "allow" && !payload.Provider.Available {
		return nil, errors.New("provider unavailable while guard claims allow")
	}
	seen := map[string]struct{}{}
	for _, m := range payload.Provider.ResidentModels {
		if !validIdentity(m.ProviderConfigurationID) || !validIdentity(m.ModelID) {
			return nil, errors.New("invalid resident model")
		}
		k := m.ProviderConfigurationID + "\x00" + m.ModelID
		if _, ok := seen[k]; ok {
			return nil, errors.New("duplicate resident model")
		}
		seen[k] = struct{}{}
	}
	packed, err := json.Marshal(payload)
	if err != nil || len(packed) > maxSnapshotBytes/2 {
		return nil, errors.New("unable to encode bounded observation")
	}
	signed := Envelope{Payload: packed, Signature: hex.EncodeToString(ed25519.Sign(privateKey, packed))}
	output, err := json.Marshal(signed)
	if err != nil || len(output) > maxSnapshotBytes {
		return nil, errors.New("unable to encode bounded signed envelope")
	}
	return append(output, '\n'), nil
}

// VerifyEnvelopeForTest is intentionally omitted. The authoritative verifier
// remains airesource.SignedObservationFileSource.
