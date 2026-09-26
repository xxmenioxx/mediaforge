package handlers

import (
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestResolveTrackPlanSubtitleOperationOverridePrecedence(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{map[string]any{
		"index": 4, "codec": "hdmv_pgs_subtitle", "language": "jpn",
	}}}
	tests := []struct {
		name         string
		profile      map[string]any
		overrides    SubtitleOperationOverrides
		wantFormat   string
		wantLanguage string
		wantMode     string
		wantSource   string
	}{
		{
			name:    "explicit operation values override asset profile",
			profile: subtitleSettingsTestProfile("asset_profile", "ass", "spa", "clean"),
			overrides: SubtitleOperationOverrides{
				Extract: true, Format: stringPointer("srt"), OCRLanguage: stringPointer("eng"), OCRMode: stringPointer("raw"),
			},
			wantFormat: "srt", wantLanguage: "eng", wantMode: "raw", wantSource: "operation_override",
		},
		{
			name:       "asset profile values apply when operation fields are omitted",
			profile:    subtitleSettingsTestProfile("asset_profile", "ass", "spa", "clean"),
			overrides:  SubtitleOperationOverrides{Extract: true},
			wantFormat: "ass", wantLanguage: "spa", wantMode: "clean", wantSource: "asset_profile",
		},
		{
			name:       "global settings apply when no asset profile value exists",
			profile:    subtitleSettingsTestProfile("global_settings", "ass", "eng", "raw"),
			overrides:  SubtitleOperationOverrides{Extract: true},
			wantFormat: "ass", wantLanguage: "eng", wantMode: "raw", wantSource: "global_settings",
		},
		{
			name:       "built in values apply when settings are absent",
			profile:    map[string]any{},
			overrides:  SubtitleOperationOverrides{Extract: true},
			wantFormat: "srt", wantLanguage: "auto", wantMode: "accurate", wantSource: "built_in",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := resolveTrackPlanWithSubtitleOverrides(scan, test.profile, test.overrides)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.SidecarOutputs) != 1 {
				t.Fatalf("sidecar outputs=%#v", plan.SidecarOutputs)
			}
			decision := plan.SidecarOutputs[0]
			if decision.Format != test.wantFormat || decision.OCRLanguage != test.wantLanguage || decision.OCRMode != test.wantMode {
				t.Fatalf("effective decision=%#v want format=%q language=%q mode=%q", decision, test.wantFormat, test.wantLanguage, test.wantMode)
			}
			for _, field := range []string{"format", "ocrLanguage", "ocrMode"} {
				if source := decision.SettingsSources[field]; source != test.wantSource {
					t.Fatalf("%s source=%q want=%q: %#v", field, source, test.wantSource, decision.SettingsSources)
				}
			}
		})
	}

	original, err := resolveTrackPlanWithSubtitleOverrides(
		models.ScanResult{SubtitleStreams: models.JSONList{map[string]any{"index": 3, "codec": "ass", "language": "eng"}}},
		subtitleSettingsTestProfile("asset_profile", "srt", "", ""),
		SubtitleOperationOverrides{Extract: true, Format: stringPointer("original")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(original.SidecarOutputs) != 1 || original.SidecarOutputs[0].Format != "ass" || original.SidecarOutputs[0].Mode != "original" || original.SidecarOutputs[0].SettingsSources["format"] != "operation_override" {
		t.Fatalf("explicit original extraction did not win: %#v", original.SidecarOutputs)
	}
}

func TestResolvedSubtitleAssetProfileInheritsGlobalSettingsAndUsesCanonicalOCRPlan(t *testing.T) {
	db := assetConfigurationTestDB(t)
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Clean(filepath.Join(t.TempDir(), "episode.mkv"))
	scan := models.ScanResult{Path: mediaPath, SubtitleStreams: models.JSONList{map[string]any{
		"index": 4, "codec": "hdmv_pgs_subtitle", "language": "jpn",
	}}}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	defaults := defaultAssetDefaultsValue()
	tracks := settingProfileObject(defaults["tracks"])
	tracks["custom"] = models.JSONMap{
		"trackDispositionVersion": canonicalTrackDispositionVersion,
		"subtitleDisposition":     "keep",
		"subtitleSidecarFormats":  models.JSONList{"srt"},
		"ocrMode":                 "accurate",
		"attachmentPolicy":        "auto",
		"chapterPolicy":           "keep",
	}
	defaults["tracks"] = tracks
	if err := db.Create(&models.AppSetting{Key: assetDefaultsSettingKey, Value: defaults}).Error; err != nil {
		t.Fatal(err)
	}
	style := defaultSubtitleStylingSettings()
	style.Enabled = true
	style.ApplyToGeneratedASS = true
	subtitleSettings := models.JSONMap{
		generateSRTFallbackForASSKey: true,
		subtitleStylingPolicyKey:     models.JSONMap(subtitleStylingSettingsMap(style)),
	}
	if err := db.Create(&models.AppSetting{Key: subtitleSettingsKey, Value: subtitleSettings}).Error; err != nil {
		t.Fatal(err)
	}
	assetProfile := models.JSONMap{
		"key": "asset-tracks", "scope": "path",
		"trackDispositionVersion": canonicalTrackDispositionVersion,
		"subtitleDisposition":     "keep",
		"subtitleSidecarFormats":  models.JSONList{"ass"},
		"ocrLanguage":             "spa",
		"ocrMode":                 "clean",
		"attachmentPolicy":        "auto",
		"chapterPolicy":           "keep",
	}
	if err := db.Create(&models.AppSetting{Key: "trackProfiles", Value: models.JSONMap{"profiles": models.JSONList{assetProfile}}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProfileAssignment{
		TargetType: assetScopePath, TargetPath: filepath.Dir(mediaPath), MediaType: "tracks",
		Selection: "profile", ProfileKey: "asset-tracks",
	}).Error; err != nil {
		t.Fatal(err)
	}

	profile, err := resolvedSubtitleTrackProfileForAsset(db, mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := resolveTrackPlanWithSubtitleOverrides(scan, profile, SubtitleOperationOverrides{Extract: true, StreamIndex: intPointer(4)})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SidecarOutputs) != 2 {
		t.Fatalf("configured ASS plus SRT fallback sidecars=%#v", plan.SidecarOutputs)
	}
	var sawFallback, sawStyledASS bool
	for _, sidecar := range plan.SidecarOutputs {
		if sidecar.OCRLanguage != "spa" || sidecar.OCRMode != "clean" || sidecar.SettingsSources["ocrLanguage"] != "asset_profile" || sidecar.SettingsSources["ocrMode"] != "asset_profile" {
			t.Fatalf("asset OCR settings were not inherited by the canonical plan: %#v", sidecar)
		}
		switch sidecar.Format {
		case "srt":
			sawFallback = sidecar.SRTFallbackApplied && sidecar.SettingsSources["srtFallback"] == "global_settings"
		case "ass":
			sawStyledASS = sidecar.StylingPolicy != nil && sidecar.StylingPolicy.Enabled && sidecar.StylingPolicy.ApplyToGeneratedASS && sidecar.SettingsSources["styling"] == "global_settings"
		}
	}
	if !sawFallback || !sawStyledASS {
		t.Fatalf("global fallback/styling were not represented: %#v", plan.SidecarOutputs)
	}

	explicit, err := resolveTrackPlanWithSubtitleOverrides(scan, profile, SubtitleOperationOverrides{
		Extract: true, StreamIndex: intPointer(4), Format: stringPointer("srt"),
		OCRLanguage: stringPointer("eng"), OCRMode: stringPointer("raw"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(explicit.SidecarOutputs) != 1 || explicit.SidecarOutputs[0].Format != "srt" || explicit.SidecarOutputs[0].OCRLanguage != "eng" || explicit.SidecarOutputs[0].OCRMode != "raw" {
		t.Fatalf("explicit operation overrides did not win: %#v", explicit.SidecarOutputs)
	}
	for _, field := range []string{"format", "ocrLanguage", "ocrMode"} {
		if explicit.SidecarOutputs[0].SettingsSources[field] != "operation_override" {
			t.Fatalf("explicit %s provenance=%#v", field, explicit.SidecarOutputs[0].SettingsSources)
		}
	}
}

func TestResolvedBitmapBothPlansPreserveStreamAndOCRInputs(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	stream := FFProbeStream{
		Index: 4, ID: 12, CodecType: "subtitle", CodecName: "hdmv_pgs_subtitle",
		Tags:        map[string]string{"language": "jpn", "title": "Signs"},
		Disposition: map[string]int{"forced": 1},
	}
	profile := map[string]any{
		"trackDispositionVersion": canonicalTrackDispositionVersion,
		"subtitleDisposition":     "keep", "subtitleSidecarFormats": []string{"both"},
		"ocrLanguage": "spa", "ocrMode": "clean", "attachmentPolicy": "auto", "chapterPolicy": "keep",
	}
	plan, err := resolveTrackPlanWithSubtitleOverrides(
		models.ScanResult{Path: mediaPath, SubtitleStreams: streamSummaries([]FFProbeStream{stream}, "subtitle")},
		profile, SubtitleOperationOverrides{Extract: true, StreamIndex: intPointer(stream.Index)},
	)
	if err != nil {
		t.Fatal(err)
	}
	plans, unsupported := subtitleExtractionPlansFromResolvedTrackPlan(mediaPath, []FFProbeStream{stream}, plan)
	if len(unsupported) != 0 || len(plans) != 2 {
		t.Fatalf("plans=%#v unsupported=%#v", plans, unsupported)
	}
	_, targets, err := prepareSubtitleExtractionTargets(mediaPath, plans, nil, SubtitleExtractionInput{SubtitleStyling: defaultSubtitleStylePolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets=%#v", targets)
	}
	for _, target := range targets {
		if target.stream.ID != stream.ID || target.stream.Tags["title"] != "Signs" || target.stream.Disposition["forced"] != 1 {
			t.Fatalf("source stream metadata was not preserved into OCR target: %#v", target.stream)
		}
		if target.plan.Decision.OCRLanguage != "spa" || target.plan.Decision.OCRMode != "clean" || target.plan.Decision.LogicalTransformKey != targets[0].plan.Decision.LogicalTransformKey {
			t.Fatalf("both output targets did not share canonical OCR inputs/identity: %#v", targets)
		}
	}
}

func TestResolvedTextSubtitlePlansUseConfiguredDefaultsAndExplicitOriginal(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	stream := FFProbeStream{
		Index: 3, ID: 8, CodecType: "subtitle", CodecName: "ass",
		Tags: map[string]string{"language": "eng", "title": "Signs"},
	}
	scan := models.ScanResult{Path: mediaPath, SubtitleStreams: streamSummaries([]FFProbeStream{stream}, "subtitle")}
	profile := subtitleSettingsTestProfile("asset_profile", "ass", "", "")
	configured, err := resolveTrackPlanWithSubtitleOverrides(scan, profile, SubtitleOperationOverrides{Extract: true, StreamIndex: intPointer(3)})
	if err != nil {
		t.Fatal(err)
	}
	plans, unsupported := subtitleExtractionPlansFromResolvedTrackPlan(mediaPath, []FFProbeStream{stream}, configured)
	if len(unsupported) != 0 || len(plans) != 1 || plans[0].Format != "ass" || plans[0].Mode != "converted" || plans[0].SourceStream.ID != stream.ID {
		t.Fatalf("configured text subtitle plan=%#v unsupported=%#v", plans, unsupported)
	}

	explicitSRT, err := resolveTrackPlanWithSubtitleOverrides(scan, profile, SubtitleOperationOverrides{
		Extract: true, StreamIndex: intPointer(3), Format: stringPointer("srt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(explicitSRT.SidecarOutputs) != 1 || explicitSRT.SidecarOutputs[0].Format != "srt" || explicitSRT.SidecarOutputs[0].Mode != "converted" || explicitSRT.SidecarOutputs[0].SettingsSources["format"] != "operation_override" {
		t.Fatalf("explicit SRT conversion did not override configured ASS: %#v", explicitSRT.SidecarOutputs)
	}

	explicitOriginal, err := resolveTrackPlanWithSubtitleOverrides(scan, map[string]any{}, SubtitleOperationOverrides{
		Extract: true, StreamIndex: intPointer(3), Format: stringPointer("original"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(explicitOriginal.SidecarOutputs) != 1 || explicitOriginal.SidecarOutputs[0].Format != "ass" || explicitOriginal.SidecarOutputs[0].Mode != "original" {
		t.Fatalf("explicit original extraction was not retained: %#v", explicitOriginal.SidecarOutputs)
	}
}

func subtitleSettingsTestProfile(source, format, language, mode string) map[string]any {
	profile := map[string]any{
		"subtitleDisposition": "keep", "subtitleSidecarFormats": []string{format},
		"ocrLanguage": language, "ocrMode": mode,
	}
	for _, field := range []string{"subtitleSidecarFormats", "ocrLanguage", "ocrMode"} {
		if workerStringValue(profile[field]) != "" || field == "subtitleSidecarFormats" {
			setSubtitleSettingSource(profile, field, source)
		}
	}
	return profile
}

func stringPointer(value string) *string { return &value }
