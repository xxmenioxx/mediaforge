package handlers

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

// samplingEvidenceFromScan adapts persisted scan components into the canonical
// plan manifest. It deliberately does not run ffprobe, decode frames, or infer
// interval roles from global metadata.
func samplingEvidenceFromScan(scan models.ScanResult) []SamplingEvidence {
	evidence := []SamplingEvidence{
		{
			Identity:     "source:" + samplingScanFingerprint(scan),
			Source:       "source-fingerprint",
			Version:      "snapshot-cache",
			Scope:        "global",
			Availability: "available",
			Confidence:   1,
		},
		{
			Identity:     "stream-metadata:" + filepath.Clean(scan.Path),
			Source:       "stream-metadata",
			Version:      "scan-result",
			Scope:        "global",
			Availability: samplingAvailability(scan.VideoStreams, scan.VideoCodec),
			Confidence:   0.5,
			Weak:         true,
		},
		{
			Identity:     "bitrate:" + filepath.Clean(scan.Path),
			Source:       "global-bitrate",
			Version:      "scan-result",
			Scope:        "global",
			Availability: samplingAvailability(scan.Bitrate, scan.Duration),
			Confidence:   0.25,
			Signals:      map[string]float64{"bitrate": normalizedBitrateContext(scan.Bitrate)},
			Weak:         true,
		},
	}
	evidence = append(evidence, samplingFrameStructureEvidence(scan.FrameStructureAnalysis)...)
	evidence = append(evidence, samplingGlobalEvidence("cadence", "cadence", scan.CadenceAnalysis)...)
	evidence = append(evidence, samplingGlobalEvidence("interlace", "interlace", scan.InterlaceAnalysis)...)
	evidence = append(evidence, samplingGlobalEvidence("restoration", "restoration", scan.RestorationAnalysis)...)
	if timing, ok := samplingMap(scan.RawProbe["avTiming"]); ok {
		evidence = append(evidence, SamplingEvidence{
			Identity:     "packet-timing:" + filepath.Clean(scan.Path),
			Source:       "packet-timing",
			Version:      stringFromSamplingValue(timing["version"]),
			Scope:        "global",
			Availability: samplingAvailability(timing["status"], timing["packets"]),
			Confidence:   0.4,
			Weak:         true,
		})
	}
	return normalizeSamplingEvidence(evidence, defaultSamplingLimits().MaximumEvidence)
}

func samplingCandidatesFromScan(scan models.ScanResult) []SamplingCandidate {
	return samplingFrameStructureCandidates(scan.FrameStructureAnalysis)
}

type samplingPersistedPhysicalFingerprint struct {
	Path         string
	SizeBytes    int64
	ModifiedAtNS int64
}

// samplingSourceIdentity is the physical source identity bound into plan and
// window identities. Scan snapshots predate Mode, so compatibility checks use
// the persisted path/size/mtime tuple and the plan identity additionally binds
// the current mode when it is available.
func samplingSourceIdentity(path string, revision compatiblePreviewSourceRevision) string {
	return fmt.Sprintf("%s|%d|%d|%d", filepath.Clean(path), revision.Size, revision.ModifiedAtNS, revision.Mode)
}

func samplingScanPhysicalFingerprint(scan models.ScanResult) (samplingPersistedPhysicalFingerprint, bool) {
	cache, ok := samplingMap(scan.RawProbe["snapshotCache"])
	if !ok {
		return samplingPersistedPhysicalFingerprint{}, false
	}
	fingerprint, ok := samplingMap(cache["fingerprint"])
	if !ok {
		return samplingPersistedPhysicalFingerprint{}, false
	}
	path := filepath.Clean(stringFromSamplingValue(fingerprint["path"]))
	size, sizeErr := strconv.ParseInt(strings.TrimSpace(stringFromSamplingValue(fingerprint["sizeBytes"])), 10, 64)
	mtime, mtimeErr := strconv.ParseInt(strings.TrimSpace(stringFromSamplingValue(fingerprint["mtimeNs"])), 10, 64)
	if path == "." || sizeErr != nil || mtimeErr != nil {
		return samplingPersistedPhysicalFingerprint{}, false
	}
	return samplingPersistedPhysicalFingerprint{Path: path, SizeBytes: size, ModifiedAtNS: mtime}, true
}

func samplingScanMatchesSource(scan models.ScanResult, path string, revision compatiblePreviewSourceRevision) bool {
	persisted, ok := samplingScanPhysicalFingerprint(scan)
	return ok && persisted.Path == filepath.Clean(path) && persisted.SizeBytes == revision.Size && persisted.ModifiedAtNS == revision.ModifiedAtNS
}

func samplingStaleScanEvidence(scan models.ScanResult) []SamplingEvidence {
	oldIdentity := "unavailable"
	if persisted, ok := samplingScanPhysicalFingerprint(scan); ok {
		oldIdentity = fmt.Sprintf("%s|%d|%d", persisted.Path, persisted.SizeBytes, persisted.ModifiedAtNS)
	}
	return []SamplingEvidence{{
		Identity:     "stale-scan:" + oldIdentity,
		Source:       "persisted-scan",
		Version:      "snapshot-cache",
		Scope:        "global",
		Availability: "stale",
		Weak:         true,
	}}
}

func canonicalSamplingPlanFromScanForSource(scan models.ScanResult, policy FrameStructureSamplingPolicy, primary *SamplingWindow, policyVersion, sourceFingerprint, sourceRevision string) SamplingPlan {
	evidence := samplingEvidenceFromScan(scan)
	if policyVersion == "" {
		policyVersion = samplingPlanPolicyVersion
	}
	base := canonicalSamplingPlan(scan.Duration, policy)
	return resolveCanonicalSamplingPlan(SamplingPlanInput{
		BasePlan:             base,
		SourceFingerprint:    sourceFingerprint,
		SourceRevision:       sourceRevision,
		SourceStreamIdentity: "0:v:0",
		PolicyVersion:        policyVersion,
		Primary:              primary,
		Evidence:             evidence,
		Candidates:           samplingCandidatesFromScan(scan),
	})
}

func canonicalSamplingPlanFromScan(scan models.ScanResult, policy FrameStructureSamplingPolicy, primary *SamplingWindow, policyVersion string) SamplingPlan {
	return canonicalSamplingPlanFromScanForSource(scan, policy, primary, policyVersion, samplingScanFingerprint(scan), samplingScanRevision(scan))
}

func samplingPlanFromPersistedScan(scan models.ScanResult, path string, revision compatiblePreviewSourceRevision, duration float64, policy FrameStructureSamplingPolicy, primary *SamplingWindow, policyVersion string) (SamplingPlan, bool) {
	currentIdentity := samplingSourceIdentity(path, revision)
	if samplingScanMatchesSource(scan, path, revision) {
		if scan.Duration <= 0 {
			scan.Duration = duration
		}
		return canonicalSamplingPlanFromScanForSource(scan, policy, primary, policyVersion, currentIdentity, currentIdentity), true
	}
	return resolveCanonicalSamplingPlan(SamplingPlanInput{
		BasePlan:             canonicalSamplingPlan(duration, policy),
		SourceFingerprint:    currentIdentity,
		SourceRevision:       currentIdentity,
		SourceStreamIdentity: "0:v:0",
		PolicyVersion:        policyVersion,
		Primary:              primary,
		Evidence:             samplingStaleScanEvidence(scan),
		FallbackKind:         samplingFallbackDistributed,
		FallbackReason:       "stale_persisted_scan",
	}), false
}

func (h AssetHandler) samplingPlanForFrozenWindow(path, sourceFingerprint, sourceRevision string, duration, start, windowDuration float64, policyVersion string) SamplingPlan {
	primary := SamplingWindow{
		StartSeconds:    start,
		DurationSeconds: windowDuration,
		Position:        0,
		Primary:         true,
	}
	policy := frameStructureSamplingPolicy(h.db)
	revision, revisionErr := sourceRevisionForPath(path)
	var scan models.ScanResult
	haveScan := h.db != nil && h.db.Migrator().HasTable(&models.ScanResult{}) &&
		h.db.Where("path = ?", filepath.Clean(path)).Order("updated_at desc, id desc").First(&scan).Error == nil
	if revisionErr == nil {
		currentIdentity := samplingSourceIdentity(path, revision)
		primary.Position = primaryPosition(start, windowDuration, duration)
		if haveScan {
			plan, _ := samplingPlanFromPersistedScan(scan, path, revision, duration, policy, &primary, policyVersion)
			return plan
		}
		return resolveCanonicalSamplingPlan(SamplingPlanInput{
			BasePlan:             canonicalSamplingPlan(duration, policy),
			SourceFingerprint:    currentIdentity,
			SourceRevision:       currentIdentity,
			SourceStreamIdentity: "0:v:0",
			PolicyVersion:        policyVersion,
			Primary:              &primary,
		})
	}
	if h.db != nil && h.db.Migrator().HasTable(&models.ScanResult{}) {
		if haveScan {
			primary.Position = primaryPosition(start, windowDuration, duration)
			return resolveCanonicalSamplingPlan(SamplingPlanInput{
				BasePlan:             canonicalSamplingPlan(duration, policy),
				SourceFingerprint:    sourceFingerprint,
				SourceRevision:       sourceRevision,
				SourceStreamIdentity: "0:v:0",
				PolicyVersion:        policyVersion,
				Primary:              &primary,
				Evidence:             samplingStaleScanEvidence(scan),
				FallbackKind:         samplingFallbackDistributed,
				FallbackReason:       "source_identity_unavailable",
			})
		}
	}
	primary.Position = primaryPosition(start, windowDuration, duration)
	return resolveCanonicalSamplingPlan(SamplingPlanInput{
		BasePlan:             canonicalSamplingPlan(duration, policy),
		SourceFingerprint:    sourceFingerprint,
		SourceRevision:       sourceRevision,
		SourceStreamIdentity: "0:v:0",
		PolicyVersion:        policyVersion,
		Primary:              &primary,
	})
}

func samplingScanFingerprint(scan models.ScanResult) string {
	if fingerprint, ok := samplingScanPhysicalFingerprint(scan); ok {
		return strings.Join([]string{fingerprint.Path, strconv.FormatInt(fingerprint.SizeBytes, 10), strconv.FormatInt(fingerprint.ModifiedAtNS, 10)}, "|")
	}
	return fmt.Sprintf("%s|%d|%d", filepath.Clean(scan.Path), scan.SizeBytes, scan.UpdatedAt.UnixNano())
}

func samplingScanRevision(scan models.ScanResult) string {
	return fmt.Sprintf("%s|%d|%d", samplingScanFingerprint(scan), scan.ID, scan.UpdatedAt.UnixNano())
}

func samplingPlanFromQueueJob(job models.QueueJob) (SamplingPlan, bool) {
	if job.ProfileSnapshot == nil {
		return SamplingPlan{}, false
	}
	value, ok := samplingMap(job.ProfileSnapshot["samplingPlan"])
	if !ok {
		return SamplingPlan{}, false
	}
	return samplingPlanFromMap(value)
}

func samplingFrameStructureEvidence(value map[string]any) []SamplingEvidence {
	if value == nil {
		return nil
	}
	version := stringFromSamplingValue(value["version"])
	if version == "" {
		version = "unknown"
	}
	windows, ok := samplingSlice(value["windows"])
	if !ok || len(windows) == 0 {
		return []SamplingEvidence{{
			Identity:     "frame-structure:global:" + version,
			Source:       "frame-structure",
			Version:      version,
			Scope:        "global",
			Availability: samplingAvailability(value["status"], value["confidence"]),
			Confidence:   samplingNumber(value["confidenceScore"]),
		}}
	}
	evidence := make([]SamplingEvidence, 0, len(windows))
	for _, rawWindow := range windows {
		window, ok := samplingMap(rawWindow)
		if !ok {
			continue
		}
		start := samplingNumber(window["startSeconds"])
		duration := samplingNumber(window["durationSeconds"])
		if duration <= 0 {
			continue
		}
		analysis, _ := samplingMap(window["analysis"])
		signals := map[string]float64{}
		if complexity := frameStructureComplexitySignal(analysis); complexity > 0 {
			signals["complexity"] = complexity
		}
		confidence := samplingNumber(analysis["confidenceScore"])
		if confidence <= 0 {
			confidence = samplingNumber(value["confidenceScore"])
		}
		identity := fmt.Sprintf("frame-structure:%s:%.3f:%.3f", version, start, duration)
		evidence = append(evidence, SamplingEvidence{
			Identity:     identity,
			Source:       "frame-structure",
			Version:      version,
			Scope:        "interval",
			Availability: samplingAvailability(analysis["status"], analysis["confidence"], analysis["framesAnalyzed"]),
			Confidence:   confidence,
			Signals:      signals,
		})
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].Identity < evidence[j].Identity })
	return evidence
}

func samplingFrameStructureCandidates(value map[string]any) []SamplingCandidate {
	evidence := samplingFrameStructureEvidence(value)
	candidates := make([]SamplingCandidate, 0, len(evidence))
	for _, item := range evidence {
		if item.Scope != "interval" || item.Availability != "available" {
			continue
		}
		parts := strings.Split(item.Identity, ":")
		if len(parts) != 4 {
			continue
		}
		start := samplingNumberString(parts[2])
		duration := samplingNumberString(parts[3])
		candidates = append(candidates, SamplingCandidate{
			Window:       SamplingWindow{StartSeconds: start, DurationSeconds: duration},
			EvidenceRefs: []string{item.Identity},
		})
	}
	return candidates
}

func frameStructureComplexitySignal(analysis map[string]any) float64 {
	if analysis == nil {
		return 0
	}
	if value := samplingNumber(analysis["complexityScore"]); value > 0 {
		return quantizeSamplingScore(value)
	}
	bRatio := samplingNumber(analysis["bFrameRatio"])
	gop := samplingNumber(analysis["averageGopLength"])
	if bRatio <= 0 && gop <= 0 {
		return 0
	}
	// This is bounded selection evidence only; it is not a quality verdict.
	return quantizeSamplingScore(mathMinOne(0.5*bRatio + 0.5*mathMinOne(gop/30)))
}

func samplingGlobalEvidence(identity, source string, value map[string]any) []SamplingEvidence {
	if len(value) == 0 {
		return nil
	}
	version := stringFromSamplingValue(value["version"])
	if version == "" {
		version = "scan-result"
	}
	return []SamplingEvidence{{
		Identity:     identity + ":global:" + version,
		Source:       source,
		Version:      version,
		Scope:        "global",
		Availability: samplingAvailability(value["status"], value["confidence"]),
		Confidence:   samplingNumber(value["confidenceScore"]),
	}}
}

func samplingMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case models.JSONMap:
		return map[string]any(typed), true
	default:
		return nil, false
	}
}

func samplingSlice(value any) ([]any, bool) {
	if typed, ok := value.([]any); ok {
		return typed, true
	}
	return nil, false
}

func samplingNumber(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case jsonNumber:
		return typed.Float()
	default:
		return samplingNumberString(stringFromSamplingValue(value))
	}
}

type jsonNumber string

func (value jsonNumber) Float() float64 {
	var result float64
	_, _ = fmt.Sscanf(string(value), "%f", &result)
	return result
}

func samplingNumberString(value string) float64 {
	var result float64
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%f", &result)
	return result
}

func stringFromSamplingValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func samplingAvailability(values ...any) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(stringFromSamplingValue(value)))
		if status == "unverified" || status == "ambiguous" || status == "unavailable" || status == "error" {
			return status
		}
	}
	return "available"
}

func normalizedBitrateContext(bitrate int64) float64 {
	if bitrate <= 0 {
		return 0
	}
	return quantizeSamplingScore(float64(bitrate) / float64(bitrate+10000000))
}

func mathMinOne(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
