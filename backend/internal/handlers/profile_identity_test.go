package handlers

import (
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
)

func TestT105EffectiveAssetProfileIdentityRoundTripsFrozenQSVExecution(t *testing.T) {
	profile := models.Profile{
		ID: 21, Name: "CONCIERTO", ProfileVersion: 3,
		VideoCodec: "hevc", CodecFamily: "hevc", EncoderPolicy: "locked",
		PreferredEncoder: "hevc_qsv", AllowedEncoders: models.StringList{"hevc_qsv"},
		QualityMode: "icq", QualityValue: 25,
		WorkerConfig: models.JSONMap{
			"videoEncoder": "hevc_qsv", "hardwareQualityPreset": "recommended", "hardwareQualityPresetScale": 2,
			"qsvRateControl": "icq", "hevcLevelMode": "auto", "cadenceMode": "auto",
			"frameStructureMode": "auto", "frameStructureGopFrames": 48,
			"frameStructureBFrameMode": "recommended", "frameStructureMaxBFrames": 3,
			"finalColorPolicy": "preserve", "qsvAssetAnalysisPath": "/raw/CONCIERTO.mkv",
		},
	}
	definitionFingerprint, err := resolvedProfileFingerprint(profile)
	if err != nil {
		t.Fatal(err)
	}
	streams := MediaStreamInventory{Duration: 3600, Video: []MediaStream{{
		Codec: "hevc", Width: 1920, Height: 1080, Bitrate: 8_000_000, FrameRate: "24000/1001",
		ColorRange: "tv", ColorSpace: "bt709", ColorTransfer: "bt709", ColorPrimaries: "bt709",
	}}}
	resolved := normalizeHardwareQualityPreset(profile)
	resolved = resolveEffectiveVideoMotionProfile(resolved,
		InterlaceAnalysis{Status: "progressive", FieldOrder: "progressive", Confidence: .99},
		CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "progressive", Confidence: .99},
		CadenceRecommendation{Version: 1, Operation: "preserve", OutputFrameRate: "24000/1001", Confidence: .99},
	)
	resolved.WorkerConfig["effectiveOutputFrameRate"] = "24000/1001"
	delete(resolved.WorkerConfig, "effectiveOutputFrameRateUnknown")
	resolved = resolveEffectiveVideoEncodingProfile(resolved, streams, "/raw/CONCIERTO.mkv")
	if workerStringValue(resolved.WorkerConfig["qsvRequestedRateControl"]) != "icq" ||
		workerIntValue(resolved.WorkerConfig["qsvEffectiveGlobalQuality"], 0) == 0 ||
		workerStringValue(resolved.WorkerConfig["hevcLevelEffective"]) == "" {
		t.Fatalf("T105 resolver did not freeze QSV/HEVC automatic decisions: %#v", resolved.WorkerConfig)
	}
	snapshot, err := scheduler.CaptureProfileSnapshot(resolved, time.Now().UTC(), "test_encode_effective_asset")
	if err != nil {
		t.Fatal(err)
	}
	setProfileDefinitionFingerprint(snapshot, definitionFingerprint)
	restored, err := scheduler.RestoreProfileSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restoredFingerprint, err := effectiveProfileFingerprint(restored, "hevc_qsv")
	if err != nil {
		t.Fatal(err)
	}
	setEffectiveProfileFingerprint(snapshot, restoredFingerprint)
	restored, err = scheduler.RestoreProfileSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	recomputedFingerprint, err := effectiveProfileFingerprint(restored, "hevc_qsv")
	if err != nil {
		t.Fatal(err)
	}
	identity, ok := profileIdentityFromSnapshot(snapshot)
	if !ok || identity.DefinitionFingerprint != definitionFingerprint || identity.EffectiveProfileFingerprint != recomputedFingerprint {
		t.Fatalf("T105 snapshot identities did not survive freeze/restore: %#v recomputed=%s", identity, recomputedFingerprint)
	}
	effective := models.JSONMap{
		"profileIdentityContractVersion": profileIdentityContractVersion,
		"profileDefinitionFingerprint":   definitionFingerprint,
		"effectiveProfileFingerprint":    restoredFingerprint,
		"effectiveEncoder":               "hevc_qsv",
		"override":                       models.JSONMap{}, "streamPlan": models.JSONMap{},
		"subtitleStylePolicyVersion": "subtitle-style-v1", "processingMode": ProcessingModeFullEncode,
		"profile": restored,
	}
	configurationHash, err := testEncodeConfigurationFingerprint(effective)
	if err != nil {
		t.Fatal(err)
	}
	effective["configurationHash"] = configurationHash
	test := models.TestEncode{
		ID: 105, SourceAssetID: 2105, SourcePath: "/raw/CONCIERTO.mkv", ConfigurationSource: "effective_asset",
		ProfileID: profile.ID, ProfileVersion: profile.ProfileVersion,
		ResolvedProfileFingerprint: definitionFingerprint, EffectiveProfileFingerprint: restoredFingerprint,
		EffectiveEncoder: "hevc_qsv", ConfigurationHash: configurationHash,
		RequestedConfiguration: models.JSONMap{
			"profileIdentityContractVersion": profileIdentityContractVersion,
			"profileDefinitionFingerprint":   definitionFingerprint,
		},
		EffectiveConfiguration: effective,
	}
	if reason := testEncodeProfileIdentityError(test); reason != "" {
		t.Fatalf("T105 frozen profile identities were rejected: %s", reason)
	}
	changed := restored
	changed.WorkerConfig = cloneWorkerConfig(restored.WorkerConfig)
	changed.WorkerConfig["qsvEffectiveGlobalQuality"] = workerIntValue(changed.WorkerConfig["qsvEffectiveGlobalQuality"], 25) + 1
	changedEffective := cloneTestEncodeJSONMap(effective)
	changedEffective["profile"] = changed
	if reason := testEncodeProfileIdentityError(models.TestEncode{
		ResolvedProfileFingerprint: definitionFingerprint, EffectiveProfileFingerprint: restoredFingerprint,
		EffectiveEncoder: "hevc_qsv", ConfigurationHash: configurationHash,
		RequestedConfiguration: test.RequestedConfiguration, EffectiveConfiguration: changedEffective,
	}); reason == "" {
		t.Fatal("changed T105 QSV decision retained a valid frozen identity")
	}
}

func TestEffectiveProfileFingerprintTracksFrozenEncodingDecisions(t *testing.T) {
	base := models.Profile{
		ID: 21, Name: "CONCIERTO", ProfileVersion: 3,
		VideoCodec: "hevc", QualityMode: "crf", QualityValue: 25,
		WorkerConfig: models.JSONMap{
			"videoEncoder":              "hevc_qsv",
			"qsvRateControl":            "icq",
			"globalQuality":             25,
			"qsvRequestedGlobalQuality": 25,
			"qsvEffectiveGlobalQuality": 25,
			"hevcLevelEffective":        "5.1",
			"hevcLevelTier":             "main",
			"frameStructureGopFrames":   48,
			"frameStructureMaxBFrames":  3,
			"effectiveCadenceOperation": "preserve",
			"effectiveFinalColorPolicy": "preserve",
			resolvedRestorationPlanKey: ResolvedRestorationPlan{
				ResolvedFilterChain:      "hqdn3d=4:3:6:4.5",
				Stages:                   []ResolvedRestorationStage{{Stage: "denoise", Filter: "hqdn3d=4:3:6:4.5"}},
				RecommendationProvenance: models.JSONMap{"sourceAssetPath": "/raw/concierto.mkv"},
			},
		},
	}
	baseFingerprint, err := effectiveProfileFingerprint(base, "hevc_qsv")
	if err != nil {
		t.Fatal(err)
	}

	pathOnly := base
	pathOnly.WorkerConfig = cloneWorkerConfig(base.WorkerConfig)
	pathOnly.WorkerConfig["qsvAssetAnalysisPath"] = "/raw/another-asset.mkv"
	pathOnly.WorkerConfig["hevcLevelInputFingerprint"] = "source-specific-analysis"
	pathFingerprint, err := effectiveProfileFingerprint(pathOnly, "hevc_qsv")
	if err != nil {
		t.Fatal(err)
	}
	if baseFingerprint != pathFingerprint {
		t.Fatal("asset analysis path or source-analysis fingerprint changed effective profile identity")
	}

	mutations := []struct {
		name   string
		change func(models.Profile) models.Profile
	}{
		{name: "QSV ICQ/GQ", change: func(p models.Profile) models.Profile { p.WorkerConfig["globalQuality"] = 27; return p }},
		{name: "HEVC level", change: func(p models.Profile) models.Profile { p.WorkerConfig["hevcLevelEffective"] = "5.2"; return p }},
		{name: "GOP frames", change: func(p models.Profile) models.Profile { p.WorkerConfig["frameStructureGopFrames"] = 60; return p }},
		{name: "B frames", change: func(p models.Profile) models.Profile { p.WorkerConfig["frameStructureMaxBFrames"] = 4; return p }},
		{name: "cadence", change: func(p models.Profile) models.Profile {
			p.WorkerConfig["effectiveCadenceOperation"] = "inverse_telecine"
			return p
		}},
		{name: "color", change: func(p models.Profile) models.Profile {
			p.WorkerConfig["effectiveFinalColorPolicy"] = "normalize_bt709"
			return p
		}},
		{name: "restoration", change: func(p models.Profile) models.Profile {
			plan := p.WorkerConfig[resolvedRestorationPlanKey].(ResolvedRestorationPlan)
			plan.ResolvedFilterChain = "hqdn3d=5:4:7:5"
			p.WorkerConfig[resolvedRestorationPlanKey] = plan
			return p
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := base
			changed.WorkerConfig = cloneWorkerConfig(base.WorkerConfig)
			changed = mutation.change(changed)
			fingerprint, err := effectiveProfileFingerprint(changed, "hevc_qsv")
			if err != nil {
				t.Fatal(err)
			}
			if fingerprint == baseFingerprint {
				t.Fatalf("%s change did not alter the frozen execution identity", mutation.name)
			}
		})
	}
}

func TestTestEncodeConfigurationFingerprintExcludesRunMetadataAndIncludesExecution(t *testing.T) {
	base := models.JSONMap{
		"profileDefinitionFingerprint": "profile-definition-v1:definition",
		"effectiveProfileFingerprint":  "effective-profile-v1:execution",
		"effectiveEncoder":             "hevc_qsv",
		"override":                     models.JSONMap{"videoCodec": "hevc"},
		"streamPlan":                   models.JSONMap{"audio": []int{0}, "subtitle": []int{1}},
		"subtitleStylePolicyVersion":   "subtitle-style-v2",
		"processingMode":               ProcessingModeFullEncode,
		"executor":                     "test-encode-local",
		"runtimeSnapshotId":            81,
		"outputPath":                   "/library/test.mkv",
	}
	baseFingerprint, err := testEncodeConfigurationFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	runMetadataChanged := cloneTestEncodeJSONMap(base)
	runMetadataChanged["executor"] = "another-executor"
	runMetadataChanged["runtimeSnapshotId"] = 99
	runMetadataChanged["outputPath"] = "/another/library/test.mkv"
	unchangedFingerprint, err := testEncodeConfigurationFingerprint(runMetadataChanged)
	if err != nil {
		t.Fatal(err)
	}
	if baseFingerprint != unchangedFingerprint {
		t.Fatal("executor, runtime snapshot, or output path contaminated candidate configuration identity")
	}

	executionChanged := cloneTestEncodeJSONMap(base)
	executionChanged["effectiveProfileFingerprint"] = "effective-profile-v1:changed"
	changedFingerprint, err := testEncodeConfigurationFingerprint(executionChanged)
	if err != nil {
		t.Fatal(err)
	}
	if baseFingerprint == changedFingerprint {
		t.Fatal("effective profile change did not alter candidate configuration identity")
	}
	assetOverrideChanged := cloneTestEncodeJSONMap(base)
	assetOverrideChanged["override"] = models.JSONMap{"videoCodec": "h265", "qualityValue": 27}
	overrideFingerprint, err := testEncodeConfigurationFingerprint(assetOverrideChanged)
	if err != nil {
		t.Fatal(err)
	}
	if baseFingerprint == overrideFingerprint {
		t.Fatal("Asset Conversion Override change did not alter candidate configuration identity")
	}
}
