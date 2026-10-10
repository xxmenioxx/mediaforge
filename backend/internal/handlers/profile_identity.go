package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

const (
	profileIdentitySnapshotKey                = "profileIdentity"
	profileIdentityContractVersion            = "profile-identity-v1"
	profileQualificationFingerprintVersion    = "profile-definition-v1"
	effectiveProfileFingerprintVersion        = "effective-profile-v1"
	testEncodeConfigurationFingerprintVersion = "test-encode-configuration-v2"
)

type profileIdentityMetadata struct {
	ContractVersion             string `json:"contractVersion"`
	DefinitionFingerprint       string `json:"definitionFingerprint"`
	EffectiveProfileFingerprint string `json:"effectiveProfileFingerprint,omitempty"`
}

// resolvedProfileFingerprint identifies the reusable profile definition. Callers
// that need an asset-specific execution identity must use
// effectiveProfileFingerprint on the final resolved profile instead.
func resolvedProfileFingerprint(profile models.Profile) (string, error) {
	identity := struct {
		FingerprintVersion string                   `json:"fingerprintVersion"`
		ProfileVersion     int                      `json:"profileVersion"`
		TemplateKey        string                   `json:"templateKey"`
		TemplateVersion    int                      `json:"templateVersion"`
		Definition         models.ProfileDefinition `json:"definition"`
	}{
		FingerprintVersion: profileQualificationFingerprintVersion,
		ProfileVersion:     profile.ProfileVersion,
		TemplateKey:        profile.TemplateKey,
		TemplateVersion:    profile.TemplateVersion,
		Definition:         profile.Definition(),
	}
	identity.Definition.WorkerConfig = profileIdentityWorkerConfig(profile, false)
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode profile definition identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return profileQualificationFingerprintVersion + ":" + hex.EncodeToString(digest[:]), nil
}

// effectiveProfileFingerprint hashes the resolved profile settings used by
// FFmpeg while excluding source-path and diagnostic provenance fields. The
// selected encoder is supplied separately because Queue Preflight records it
// outside ProfileSnapshot.
func effectiveProfileFingerprint(profile models.Profile, selectedEncoder string) (string, error) {
	if stringsTrimmed := strings.TrimSpace(selectedEncoder); stringsTrimmed != "" {
		profile = applySelectedEncoder(profile, stringsTrimmed)
	}
	encodedProfile, err := json.Marshal(profile)
	if err != nil {
		return "", fmt.Errorf("encode effective profile before canonicalization: %w", err)
	}
	var canonicalProfile models.Profile
	if err := json.Unmarshal(encodedProfile, &canonicalProfile); err != nil {
		return "", fmt.Errorf("canonicalize effective profile: %w", err)
	}
	profile = canonicalProfile
	identity := struct {
		FingerprintVersion string                   `json:"fingerprintVersion"`
		Definition         models.ProfileDefinition `json:"definition"`
		EffectiveEncoder   string                   `json:"effectiveEncoder"`
	}{
		FingerprintVersion: effectiveProfileFingerprintVersion,
		Definition:         profile.Definition(),
		EffectiveEncoder:   resolvedVideoEncoder(profile),
	}
	identity.Definition.WorkerConfig = profileIdentityWorkerConfig(profile, true)
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode effective profile identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return effectiveProfileFingerprintVersion + ":" + hex.EncodeToString(digest[:]), nil
}

func profileIdentityWorkerConfig(profile models.Profile, includeResolvedRestoration bool) models.JSONMap {
	workerConfig := cloneWorkerConfig(profile.WorkerConfig)
	for _, key := range []string{
		"qsvAssetAnalysisPath", "derivedFromAsset", "sourceAssetPath", "sourcePath",
		interlaceAnalysisSnapshotKey, cadenceAnalysisSnapshotKey, cadenceRecommendationSnapshotKey,
		restorationAnalysisSnapshotKey, sourceAnalysisEvidenceSnapshotKey,
		"samplingPlan", "samplingPlanIdentity", "hevcLevelInputFingerprint",
		restorationProvenanceSnapshotKey,
	} {
		delete(workerConfig, key)
	}
	if !includeResolvedRestoration {
		delete(workerConfig, resolvedRestorationPlanKey)
	}
	if includeResolvedRestoration {
		if plan, ok := resolvedRestorationPlanFromProfile(models.Profile{WorkerConfig: workerConfig}); ok {
			plan.RecommendationProvenance = nil
			plan.Evidence = nil
			workerConfig[resolvedRestorationPlanKey] = *plan
		}
	}
	return workerConfig
}

func setProfileDefinitionFingerprint(snapshot models.JSONMap, fingerprint string) {
	if snapshot == nil || fingerprint == "" {
		return
	}
	identity := profileIdentityMetadata{ContractVersion: profileIdentityContractVersion}
	if existing, ok := profileIdentityFromSnapshot(snapshot); ok {
		identity = existing
	}
	identity.ContractVersion = profileIdentityContractVersion
	identity.DefinitionFingerprint = fingerprint
	snapshot[profileIdentitySnapshotKey] = identity
}

func setEffectiveProfileFingerprint(snapshot models.JSONMap, fingerprint string) {
	if snapshot == nil || fingerprint == "" {
		return
	}
	identity := profileIdentityMetadata{ContractVersion: profileIdentityContractVersion}
	if existing, ok := profileIdentityFromSnapshot(snapshot); ok {
		identity = existing
	}
	identity.ContractVersion = profileIdentityContractVersion
	identity.EffectiveProfileFingerprint = fingerprint
	snapshot[profileIdentitySnapshotKey] = identity
}

func profileIdentityFromSnapshot(snapshot models.JSONMap) (profileIdentityMetadata, bool) {
	if snapshot == nil || snapshot[profileIdentitySnapshotKey] == nil {
		return profileIdentityMetadata{}, false
	}
	encoded, err := json.Marshal(snapshot[profileIdentitySnapshotKey])
	if err != nil {
		return profileIdentityMetadata{}, false
	}
	var identity profileIdentityMetadata
	if err := json.Unmarshal(encoded, &identity); err != nil || identity.ContractVersion != profileIdentityContractVersion {
		return profileIdentityMetadata{}, false
	}
	return identity, true
}

func testEncodeConfigurationFingerprint(effective models.JSONMap) (string, error) {
	identity := models.JSONMap{
		"fingerprintVersion":           testEncodeConfigurationFingerprintVersion,
		"profileDefinitionFingerprint": effective["profileDefinitionFingerprint"],
		"effectiveProfileFingerprint":  effective["effectiveProfileFingerprint"],
		"effectiveEncoder":             effective["effectiveEncoder"],
		"override":                     effective["override"],
		"streamPlan":                   effective["streamPlan"],
		"subtitleStylePolicyVersion":   effective["subtitleStylePolicyVersion"],
		"processingMode":               effective["processingMode"],
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode Test Encode configuration identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return testEncodeConfigurationFingerprintVersion + ":" + hex.EncodeToString(digest[:]), nil
}

func testEncodeProfileIdentityError(test models.TestEncode) string {
	if test.ResolvedProfileFingerprint == "" {
		return "legacy Test Encode is missing the reusable profile-definition identity"
	}
	if !strings.HasPrefix(test.ResolvedProfileFingerprint, profileQualificationFingerprintVersion+":") {
		return "legacy Test Encode uses an unsupported profile-definition fingerprint version"
	}
	if test.EffectiveProfileFingerprint == "" {
		return "legacy Test Encode is missing the frozen effective-profile identity"
	}
	if !strings.HasPrefix(test.EffectiveProfileFingerprint, effectiveProfileFingerprintVersion+":") {
		return "legacy Test Encode uses an unsupported effective-profile fingerprint version"
	}
	if test.ConfigurationHash == "" || !strings.HasPrefix(test.ConfigurationHash, testEncodeConfigurationFingerprintVersion+":") {
		return "legacy Test Encode ConfigurationHash does not identify the versioned frozen execution"
	}
	requestedDefinition := workerStringValue(test.RequestedConfiguration["profileDefinitionFingerprint"])
	if workerStringValue(test.RequestedConfiguration["profileIdentityContractVersion"]) != profileIdentityContractVersion {
		return "requested Test Encode snapshot is missing the profile identity contract version"
	}
	if requestedDefinition == "" || requestedDefinition != test.ResolvedProfileFingerprint {
		return "requested Test Encode snapshot does not match the persisted profile-definition identity"
	}
	effectiveDefinition := workerStringValue(test.EffectiveConfiguration["profileDefinitionFingerprint"])
	if workerStringValue(test.EffectiveConfiguration["profileIdentityContractVersion"]) != profileIdentityContractVersion || effectiveDefinition != test.ResolvedProfileFingerprint {
		return "post-plan Test Encode snapshot does not preserve the requested profile-definition identity"
	}
	if workerStringValue(test.EffectiveConfiguration["effectiveProfileFingerprint"]) != test.EffectiveProfileFingerprint {
		return "post-plan Test Encode snapshot does not match the persisted effective-profile identity"
	}
	var frozenProfile models.Profile
	if err := decodeProfileQualificationValue(test.EffectiveConfiguration["profile"], &frozenProfile); err != nil {
		return "post-plan Test Encode snapshot is missing its frozen effective profile"
	}
	computedEffectiveFingerprint, err := effectiveProfileFingerprint(frozenProfile, test.EffectiveEncoder)
	if err != nil || computedEffectiveFingerprint != test.EffectiveProfileFingerprint {
		return "post-plan Test Encode profile does not match the persisted effective-profile identity"
	}
	if workerStringValue(test.EffectiveConfiguration["configurationHash"]) != test.ConfigurationHash {
		return "post-plan Test Encode snapshot does not match the persisted ConfigurationHash"
	}
	computedConfigurationHash, err := testEncodeConfigurationFingerprint(test.EffectiveConfiguration)
	if err != nil || computedConfigurationHash != test.ConfigurationHash {
		return "post-plan Test Encode configuration fields do not match the persisted ConfigurationHash"
	}
	return ""
}
