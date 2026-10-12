package handlers

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveTrackPlanSubtitleActionsAndLanguageRules(t *testing.T) {
	scan := trackResolverScan()
	profile := map[string]any{
		"keepVideoStreams": []int{0}, "keepAudioStreams": []int{1, 2},
		"keepSubtitleStreams": []int{3, 4, 5, 6},
		"subtitleDisposition": "keep",
		"subtitleRules": []any{
			map[string]any{"language": "eng", "action": "remove"},
			map[string]any{"language": "jpn", "action": "keep_and_extract"},
			map[string]any{"streamIndex": 4, "action": "extract"},
		},
		"attachmentPolicy": "auto", "chapterPolicy": "remove",
	}
	plan, err := resolveTrackPlan(scan, profile)
	if err != nil {
		t.Fatal(err)
	}
	want := []SubtitleDisposition{SubtitleDispositionKeep, SubtitleDispositionExtract, SubtitleDispositionKeepAndExtract, SubtitleDispositionRemove}
	for index, action := range want {
		if plan.SubtitleStreams[index].Action != action {
			t.Fatalf("subtitle %d action=%q want=%q plan=%#v", index, plan.SubtitleStreams[index].Action, action, plan)
		}
	}
	if !plan.AttachmentsKept || len(plan.AttachmentStreams) != 1 {
		t.Fatalf("auto did not keep fonts for retained ASS: %#v", plan)
	}
	if plan.ChaptersKept || plan.ChapterPolicy != ChapterPolicyRemove {
		t.Fatalf("chapter removal was not resolved: %#v", plan)
	}
	if len(plan.SidecarOutputs) != 2 || plan.SidecarOutputs[0].StreamIndex != 4 || plan.SidecarOutputs[1].StreamIndex != 5 {
		t.Fatalf("sidecar decisions=%#v", plan.SidecarOutputs)
	}
}

func TestResolveTrackPlanPreferredSubtitleRulesRespectProfilePrecedenceAndUnknownLanguages(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{
		map[string]any{"index": 3, "codec": "subrip", "language": "es"},
		map[string]any{"index": 4, "codec": "subrip", "language": "eng"},
		map[string]any{"index": 5, "codec": "subrip", "language": "jpn"},
		map[string]any{"index": 6, "codec": "subrip", "language": "und"},
	}}
	profile := map[string]any{
		"subtitleDisposition": "keep", "attachmentPolicy": "auto", "chapterPolicy": "keep",
		"subtitleRules": []any{map[string]any{"language": "spa", "action": "extract"}},
		"preferredSubtitleRules": []any{
			map[string]any{"language": "spa", "action": "keep_and_extract"},
			map[string]any{"language": "eng", "action": "extract"},
			map[string]any{"language": "jpn", "action": "keep_and_extract"},
		},
	}
	plan, err := resolveTrackPlan(scan, profile)
	if err != nil {
		t.Fatal(err)
	}
	want := []SubtitleDisposition{SubtitleDispositionExtract, SubtitleDispositionExtract, SubtitleDispositionKeepAndExtract, SubtitleDispositionKeep}
	for index, action := range want {
		if plan.SubtitleStreams[index].Action != action {
			t.Fatalf("subtitle[%d] action=%q want=%q", index, plan.SubtitleStreams[index].Action, action)
		}
	}
	if plan.SubtitleStreams[0].ActionSource != "" {
		t.Fatalf("explicit Track Profile rule was incorrectly attributed to global preference: %#v", plan.SubtitleStreams[0])
	}
	if plan.SubtitleStreams[1].ActionSource != "preferred_subtitle_language" || plan.SubtitleStreams[2].ActionSource != "preferred_subtitle_language" {
		t.Fatalf("global preferred-rule provenance missing: %#v", plan.SubtitleStreams)
	}
	if plan.SubtitleStreams[3].ActionSource != "" {
		t.Fatalf("und matched a preferred language: %#v", plan.SubtitleStreams[3])
	}
}

func TestResolveTrackPlanPreferredBitmapLanguageKeepsCanonicalOCRPath(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{
		map[string]any{"index": 7, "codec": "hdmv_pgs_subtitle", "language": "jpn"},
	}}
	preferredRules := []any{map[string]any{"language": "jpn", "action": "extract"}}

	t.Run("original sidecar preserves bitmap extraction", func(t *testing.T) {
		plan, err := resolveTrackPlan(scan, map[string]any{
			"subtitleDisposition":    "keep",
			"attachmentPolicy":       "auto",
			"chapterPolicy":          "keep",
			"preferredSubtitleRules": preferredRules,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.SubtitleStreams) != 1 || plan.SubtitleStreams[0].Action != SubtitleDispositionExtract {
			t.Fatalf("preferred bitmap action=%#v", plan.SubtitleStreams)
		}
		if len(plan.SidecarOutputs) != 1 {
			t.Fatalf("original bitmap sidecar outputs=%#v", plan.SidecarOutputs)
		}
		sidecar := plan.SidecarOutputs[0]
		if sidecar.OCRRequired || sidecar.SourceType != "bitmap" || sidecar.Mode != "original" || !reflect.DeepEqual(sidecar.Stages, []string{"extract"}) {
			t.Fatalf("original bitmap sidecar unexpectedly used OCR: %#v", sidecar)
		}
	})

	t.Run("canonical SRT sidecar uses OCR pipeline", func(t *testing.T) {
		plan, err := resolveTrackPlan(scan, map[string]any{
			"trackDispositionVersion": canonicalTrackDispositionVersion,
			"subtitleDisposition":     "keep",
			"subtitleSidecarFormats":  []string{"srt"},
			"attachmentPolicy":        "auto",
			"chapterPolicy":           "keep",
			"preferredSubtitleRules":  preferredRules,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.SubtitleStreams) != 1 || plan.SubtitleStreams[0].Action != SubtitleDispositionExtract {
			t.Fatalf("preferred bitmap action=%#v", plan.SubtitleStreams)
		}
		if len(plan.SidecarOutputs) != 1 {
			t.Fatalf("canonical SRT bitmap sidecar outputs=%#v", plan.SidecarOutputs)
		}
		sidecar := plan.SidecarOutputs[0]
		if !sidecar.OCRRequired || sidecar.SourceType != "bitmap" || sidecar.Format != "srt" || !reflect.DeepEqual(sidecar.Stages, []string{"ocr", "normalize", "render"}) {
			t.Fatalf("canonical SRT bitmap sidecar did not use OCR pipeline: %#v", sidecar)
		}
	})
}

func TestResolveTrackPlanSameLanguageOrderIsDeterministic(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{
		map[string]any{"index": 8, "codec": "subrip", "language": "spa"},
		map[string]any{"index": 3, "codec": "ass", "language": "spa"},
	}}
	profile := map[string]any{"subtitleRules": []any{map[string]any{"language": "spa", "action": "keep_and_extract"}}, "attachmentPolicy": "auto"}
	first, err := resolveTrackPlan(scan, profile)
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolveTrackPlan(scan, profile)
	if err != nil {
		t.Fatal(err)
	}
	encodedFirst, _ := json.Marshal(first)
	encodedSecond, _ := json.Marshal(second)
	if string(encodedFirst) != string(encodedSecond) || first.SubtitleStreams[0].StreamIndex != 8 || first.SubtitleStreams[1].StreamIndex != 3 {
		t.Fatalf("resolution order changed: %s / %s", encodedFirst, encodedSecond)
	}
	if !first.AttachmentsKept {
		t.Fatal("retained embedded ASS did not preserve font attachments")
	}
}

func TestResolveTrackPlanASSCompatibilitySidecars(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{
		map[string]any{"index": 4, "codec": "ass", "language": "spa"},
		map[string]any{"index": 7, "codec": "ass", "language": "spa"},
	}}
	plan, err := resolveTrackPlan(scan, map[string]any{
		"trackDispositionVersion": 1,
		"subtitleDisposition":     "keep_and_extract",
		"subtitleSidecarFormats":  []string{"original", "srt", "ass"},
		"attachmentPolicy":        "auto",
		"chapterPolicy":           "keep",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SidecarOutputs) != 6 {
		t.Fatalf("sidecars=%#v", plan.SidecarOutputs)
	}
	wantFormats := []string{"ass", "srt", "ass", "ass", "srt", "ass"}
	wantModes := []string{"original", "converted", "converted", "original", "converted", "converted"}
	for index := range wantFormats {
		if plan.SidecarOutputs[index].Format != wantFormats[index] || plan.SidecarOutputs[index].Mode != wantModes[index] {
			t.Fatalf("sidecars=%#v", plan.SidecarOutputs)
		}
	}
	if !plan.AttachmentsKept || len(plan.Warnings) != 2 {
		t.Fatalf("attachments/warnings=%t %#v", plan.AttachmentsKept, plan.Warnings)
	}
}

func TestResolveTrackPlanSRTFallbackEligibilityAndNormalization(t *testing.T) {
	tests := []struct {
		name             string
		codec            string
		formats          []string
		fallback         bool
		wantFormats      []string
		wantModes        []string
		wantRequested    []string
		wantSRTFallback  bool
		logicalBoth      bool
		logicalDifferent bool
	}{
		{name: "converted ASS enabled", codec: "ass", formats: []string{"ass"}, fallback: true, wantFormats: []string{"srt", "ass"}, wantModes: []string{"converted", "converted"}, wantRequested: []string{"both", "both"}, wantSRTFallback: true, logicalBoth: true},
		{name: "converted PGS ASS enabled", codec: "hdmv_pgs_subtitle", formats: []string{"ass"}, fallback: true, wantFormats: []string{"srt", "ass"}, wantModes: []string{"converted", "converted"}, wantRequested: []string{"both", "both"}, wantSRTFallback: true, logicalBoth: true},
		{name: "converted VobSub ASS enabled", codec: "dvd_subtitle", formats: []string{"ass"}, fallback: true, wantFormats: []string{"srt", "ass"}, wantModes: []string{"converted", "converted"}, wantRequested: []string{"both", "both"}, wantSRTFallback: true, logicalBoth: true},
		{name: "converted SRT enabled", codec: "subrip", formats: []string{"srt"}, fallback: true, wantFormats: []string{"srt"}, wantModes: []string{"converted"}, wantRequested: []string{"srt"}},
		{name: "explicit converted ASS and SRT enabled", codec: "ass", formats: []string{"ass", "srt"}, fallback: true, wantFormats: []string{"srt", "ass"}, wantModes: []string{"converted", "converted"}, wantRequested: []string{"both", "both"}, logicalBoth: true},
		{name: "converted ASS disabled", codec: "ass", formats: []string{"ass"}, fallback: false, wantFormats: []string{"ass"}, wantModes: []string{"converted"}, wantRequested: []string{"ass"}},
		{name: "original native ASS enabled", codec: "ass", formats: []string{"original"}, fallback: true, wantFormats: []string{"ass", "srt"}, wantModes: []string{"original", "converted"}, wantRequested: []string{"original", "srt"}, wantSRTFallback: true, logicalDifferent: true},
		{name: "original native SSA enabled", codec: "ssa", formats: []string{"original"}, fallback: true, wantFormats: []string{"ssa", "srt"}, wantModes: []string{"original", "converted"}, wantRequested: []string{"original", "srt"}, wantSRTFallback: true, logicalDifferent: true},
		{name: "original SRT enabled", codec: "subrip", formats: []string{"original"}, fallback: true, wantFormats: []string{"srt"}, wantModes: []string{"original"}, wantRequested: []string{"original"}},
		{name: "original PGS enabled", codec: "hdmv_pgs_subtitle", formats: []string{"original"}, fallback: true, wantFormats: []string{"sup"}, wantModes: []string{"original"}, wantRequested: []string{"original"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scan := models.ScanResult{SubtitleStreams: models.JSONList{models.JSONMap{
				"index": 4, "codec": test.codec, "language": "eng", "forced": true,
			}}}
			plan, err := resolveTrackPlan(scan, map[string]any{
				"trackDispositionVersion": 1,
				"subtitleDisposition":     "extract",
				"subtitleSidecarFormats":  test.formats,
				"subtitleSettings":        map[string]any{generateSRTFallbackForASSKey: test.fallback},
				"attachmentPolicy":        "auto",
				"chapterPolicy":           "keep",
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.SidecarOutputs) != len(test.wantFormats) {
				t.Fatalf("sidecars=%#v, want %v", plan.SidecarOutputs, test.wantFormats)
			}
			for index, sidecar := range plan.SidecarOutputs {
				if sidecar.Format != test.wantFormats[index] || sidecar.Mode != test.wantModes[index] || sidecar.RequestedFormat != test.wantRequested[index] {
					t.Fatalf("sidecar[%d]=%#v", index, sidecar)
				}
				if sidecar.SRTFallbackApplied != (test.wantSRTFallback && sidecar.Format == "srt") {
					t.Fatalf("sidecar[%d] SRT fallback state=%t want=%t: %#v", index, sidecar.SRTFallbackApplied, test.wantSRTFallback && sidecar.Format == "srt", sidecar)
				}
				if (sidecar.Mode == "converted" && sidecar.Format == "ass") != (sidecar.StylingPolicy != nil) {
					t.Fatalf("sidecar[%d] styling policy applicability was not frozen: %#v", index, sidecar)
				}
				if !sidecar.Forced || !sidecar.SourceForced {
					t.Fatalf("forced metadata was not preserved: %#v", sidecar)
				}
			}
			if test.logicalBoth && plan.SidecarOutputs[0].LogicalTransformKey != plan.SidecarOutputs[1].LogicalTransformKey {
				t.Fatalf("converted fallback did not share logical both identity: %#v", plan.SidecarOutputs)
			}
			if test.logicalDifferent && plan.SidecarOutputs[0].LogicalTransformKey == plan.SidecarOutputs[1].LogicalTransformKey {
				t.Fatalf("original native output collapsed into logical both: %#v", plan.SidecarOutputs)
			}
		})
	}

	for _, codec := range []string{"hdmv_pgs_subtitle", "dvd_subtitle"} {
		if got := effectiveSubtitleSidecarFormats(codec, []string{"original"}, true); !reflect.DeepEqual(got, []string{"original"}) {
			t.Fatalf("original %s fallback formats=%#v, want [original]", codec, got)
		}
	}
}

func TestResolveTrackPlanAssetSidecarFormatOverride(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{map[string]any{"index": 4, "codec": "ass", "language": "spa"}}}
	plan, err := resolveTrackPlan(scan, map[string]any{
		"trackDispositionVersion":        1,
		"subtitleDisposition":            "extract",
		"subtitleSidecarFormats":         []string{"original", "srt"},
		"subtitleSidecarFormatsByStream": map[string]any{"4": []string{"original"}},
		"attachmentPolicy":               "auto",
		"chapterPolicy":                  "keep",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SidecarOutputs) != 1 || plan.SidecarOutputs[0].Format != "ass" || plan.SidecarOutputs[0].Mode != "original" || plan.AttachmentsKept {
		t.Fatalf("plan=%#v", plan)
	}
}

func TestResolveTrackPlanAcceptsBitmapCompatibilitySidecarsThroughOCR(t *testing.T) {
	for _, codec := range []string{"hdmv_pgs_subtitle", "dvd_subtitle"} {
		for _, format := range []string{"srt", "ass"} {
			t.Run(codec+"_"+format, func(t *testing.T) {
				scan := models.ScanResult{SubtitleStreams: models.JSONList{map[string]any{"index": 2, "codec": codec, "language": "spa"}}}
				plan, err := resolveTrackPlan(scan, map[string]any{
					"trackDispositionVersion": 1, "subtitleDisposition": "extract", "subtitleSidecarFormats": []string{format}, "attachmentPolicy": "auto", "chapterPolicy": "keep",
				})
				if err != nil {
					t.Fatal(err)
				}
				if len(plan.SidecarOutputs) != 1 || plan.SidecarOutputs[0].Format != format || plan.SidecarOutputs[0].Mode != "converted" || plan.SidecarOutputs[0].OCRLanguage != "auto" || plan.SidecarOutputs[0].OCRMode != "accurate" {
					t.Fatalf("OCR sidecar=%#v", plan.SidecarOutputs)
				}
			})
		}
	}
	for _, format := range []string{"srt", "ass"} {
		if _, _, err := resolveSubtitleSidecarFormat("unknown_bitmap", format); err == nil {
			t.Fatalf("unsupported subtitle codec was accepted for %s", strings.ToUpper(format))
		}
	}
	if format, mode, err := resolveSubtitleSidecarFormat("ass", ""); err != nil || format != "srt" || mode != "converted" {
		t.Fatalf("default converted format=%q mode=%q err=%v", format, mode, err)
	}
	if _, _, err := resolveSubtitleSidecarFormat("ass", "vtt"); err == nil {
		t.Fatal("unsupported converted format was accepted")
	}
}

func TestSubtitleCanonicalPlanParityAcrossAssetProfileAndLab(t *testing.T) {
	path := filepath.Clean("/media/raw/show/episode.mkv")
	scan := models.ScanResult{Path: path, SubtitleStreams: models.JSONList{map[string]any{
		"index": 3, "codec": "subrip", "language": "eng",
	}}}
	profile := map[string]any{
		"scope": "asset", "trackDispositionVersion": 1, "subtitleDisposition": "extract",
		"subtitleSidecarFormats": []string{"srt"}, "attachmentPolicy": "auto", "chapterPolicy": "keep",
	}
	pipeline, err := resolveTrackPlan(scan, profile)
	if err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 3, CodecType: "subtitle", CodecName: "subrip", Tags: map[string]string{"language": "eng"}}
	assetOperationPlan, err := resolveTrackPlanWithSubtitleOverrides(scan, profile, SubtitleOperationOverrides{
		Extract: true, StreamIndex: intPointer(3), Format: stringPointer("srt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assetPlans, unsupported := subtitleExtractionPlansFromResolvedTrackPlan(path, []FFProbeStream{stream}, assetOperationPlan)
	if len(unsupported) != 0 || len(assetPlans) != 1 || len(pipeline.SidecarOutputs) != 1 {
		t.Fatalf("asset=%#v unsupported=%#v pipeline=%#v", assetPlans, unsupported, pipeline.SidecarOutputs)
	}
	assetDecision := assetPlans[0].Decision
	profileDecision := pipeline.SidecarOutputs[0]
	compare := func(name string, got, want ResolvedTrackSidecar) {
		t.Helper()
		got.Default, got.Forced, got.Title = want.Default, want.Forced, want.Title
		got.SettingsSources = nil
		want.SettingsSources = nil
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s subtitle decision diverged: got=%#v want=%#v", name, got, want)
		}
	}
	compare("Asset Info", assetDecision, profileDecision)

	db, err := gorm.Open(sqlite.Open("file:subtitle-entrypoint-parity?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	translationSettings := defaultTranslationSettings()
	translationSettings.Provider = translationProviderOllama
	translationSettings.OllamaEndpoint = "http://ollama.example"
	translationSettings.OllamaModel = "translation-model"
	translationSettings.Enabled = true
	translationSettings.Readiness = translationReadinessReady
	translationSettings.ReadinessIdentity = translationProviderConfigurationIdentity(translationSettings)
	persistTranslationSettingsFixture(t, db, translationSettings)
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	labOverride, err := resolveLabTrackOverride(db, path, models.JSONMap(profile), AssetConversionOverrideState{})
	if err != nil || labOverride.ResolvedTrackPlan == nil || len(labOverride.ResolvedTrackPlan.SidecarOutputs) != 1 {
		t.Fatalf("lab plan=%#v err=%v", labOverride.ResolvedTrackPlan, err)
	}
	compare("Profile Lab", labOverride.ResolvedTrackPlan.SidecarOutputs[0], profileDecision)
	for name, decision := range map[string]ResolvedTrackSidecar{
		"Asset Info":  assetDecision,
		"Profile":     profileDecision,
		"Profile Lab": labOverride.ResolvedTrackPlan.SidecarOutputs[0],
	} {
		if decision.Mode == "translation" {
			t.Fatalf("%s unexpectedly created a translation sidecar: %#v", name, decision)
		}
	}
}

func TestInteractiveSubtitleEntryPointsUsePersistedASSFallback(t *testing.T) {
	path := filepath.Clean("/media/raw/show/episode.mkv")
	scan := models.ScanResult{Path: path, SubtitleStreams: models.JSONList{map[string]any{
		"index": 3, "codec": "ass", "language": "eng",
	}}}
	profile := map[string]any{
		"scope": "asset", "trackDispositionVersion": 1, "subtitleDisposition": "extract",
		"subtitleSidecarFormats": []string{"ass"}, "attachmentPolicy": "auto", "chapterPolicy": "keep",
	}
	db, err := gorm.Open(sqlite.Open("file:subtitle-entrypoint-fallback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: subtitleSettingsKey, Value: models.JSONMap{generateSRTFallbackForASSKey: true}}).Error; err != nil {
		t.Fatal(err)
	}

	plan, err := resolveTrackPlan(scan, applyPersistedSubtitleSettings(profile, generateSRTFallbackForASSSetting(db)))
	if err != nil {
		t.Fatal(err)
	}
	assetPlans, unsupported := subtitleExtractionPlansForRequest(path, []FFProbeStream{{
		Index: 3, CodecType: "subtitle", CodecName: "ass", Tags: map[string]string{"language": "eng"},
	}}, SubtitleExtractionInput{StreamIndex: intPointer(3), Format: "ass", GenerateSRTFallbackForASS: generateSRTFallbackForASSSetting(db)})
	if len(unsupported) != 0 || len(assetPlans) != 2 || len(plan.SidecarOutputs) != 2 {
		t.Fatalf("asset=%#v unsupported=%#v planner=%#v", assetPlans, unsupported, plan.SidecarOutputs)
	}
	if assetPlans[0].Decision.LogicalTransformKey != plan.SidecarOutputs[0].LogicalTransformKey || assetPlans[1].Decision.LogicalTransformKey != plan.SidecarOutputs[1].LogicalTransformKey {
		t.Fatalf("asset/planner logical transforms diverged: asset=%#v planner=%#v", assetPlans, plan.SidecarOutputs)
	}

	lab, err := resolveLabTrackOverride(db, path, models.JSONMap(profile), AssetConversionOverrideState{})
	if err != nil || lab.ResolvedTrackPlan == nil || len(lab.ResolvedTrackPlan.SidecarOutputs) != 2 {
		t.Fatalf("lab=%#v err=%v", lab.ResolvedTrackPlan, err)
	}
	for index := range plan.SidecarOutputs {
		if lab.ResolvedTrackPlan.SidecarOutputs[index].Format != plan.SidecarOutputs[index].Format || lab.ResolvedTrackPlan.SidecarOutputs[index].LogicalTransformKey != plan.SidecarOutputs[index].LogicalTransformKey {
			t.Fatalf("lab/planner decision %d diverged: lab=%#v planner=%#v", index, lab.ResolvedTrackPlan.SidecarOutputs, plan.SidecarOutputs)
		}
	}
}

func TestSRTFallbackPreservesRoleForcedAndSDHMetadata(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{
		map[string]any{"index": 3, "codec": "ass", "language": "spa", "role": "signs", "forced": true},
		map[string]any{"index": 4, "codec": "ssa", "language": "eng", "role": "sdh", "forced": true},
	}}
	plan, err := resolveTrackPlan(scan, map[string]any{
		"trackDispositionVersion": 1,
		"subtitleDisposition":     "extract",
		"subtitleSidecarFormats":  []string{"original"},
		"subtitleSettings":        map[string]any{generateSRTFallbackForASSKey: true},
		"attachmentPolicy":        "auto",
		"chapterPolicy":           "keep",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SidecarOutputs) != 4 {
		t.Fatalf("fallback metadata sidecars=%#v", plan.SidecarOutputs)
	}
	for _, sidecar := range plan.SidecarOutputs {
		if !sidecar.Forced || !sidecar.SourceForced || sidecar.Language == "" {
			t.Fatalf("forced/language metadata was lost: %#v", sidecar)
		}
		if sidecar.StreamIndex == 3 && sidecar.Role != SubtitleRoleSigns {
			t.Fatalf("signs role was lost: %#v", sidecar)
		}
		if sidecar.StreamIndex == 4 && sidecar.Role != SubtitleRoleSDH {
			t.Fatalf("SDH role was lost: %#v", sidecar)
		}
		wantMode := "original"
		wantFormat := "ass"
		if sidecar.StreamIndex == 4 {
			wantFormat = "ssa"
		}
		if sidecar.RequestedFormat == "srt" {
			wantMode, wantFormat = "converted", "srt"
		}
		if sidecar.Mode != wantMode || sidecar.Format != wantFormat {
			t.Fatalf("unexpected fallback representation: %#v", sidecar)
		}
		if sidecar.NamingKey != "subtitle:"+strconv.Itoa(sidecar.StreamIndex)+":"+sidecar.Mode+":"+sidecar.Format {
			t.Fatalf("unexpected naming key: %#v", sidecar)
		}
	}
	byStream := map[int][]ResolvedTrackSidecar{}
	for _, sidecar := range plan.SidecarOutputs {
		byStream[sidecar.StreamIndex] = append(byStream[sidecar.StreamIndex], sidecar)
	}
	for streamIndex, sidecars := range byStream {
		if len(sidecars) != 2 || sidecars[0].LogicalTransformKey == sidecars[1].LogicalTransformKey {
			t.Fatalf("stream %d original/fallback logical identity=%#v", streamIndex, sidecars)
		}
	}
}

func intPointer(value int) *int { return &value }

func TestResolveTrackPlanFreezesCanonicalBitmapOCRRule(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{map[string]any{"index": 4, "codec": "hdmv_pgs_subtitle", "language": "spa"}}}
	plan, err := resolveTrackPlan(scan, map[string]any{
		"trackDispositionVersion": 1, "subtitleDisposition": "extract", "subtitleSidecarFormats": []string{"srt"},
		"subtitleRules":    []any{map[string]any{"streamIndex": 4, "action": "extract", "sidecarFormats": []string{"srt"}, "ocrLanguage": "spa", "ocrMode": "accurate"}},
		"attachmentPolicy": "auto", "chapterPolicy": "keep",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SidecarOutputs) != 1 || plan.SidecarOutputs[0].OCRLanguage != "spa" || plan.SidecarOutputs[0].OCRMode != "accurate" {
		t.Fatalf("frozen OCR config=%#v", plan.SidecarOutputs)
	}
}

func TestSubtitleTransformsByIndexPreservesLegacyOCRMetadata(t *testing.T) {
	transforms := subtitleTransformsByIndex([]any{map[string]any{
		"streamIndex": 4, "format": "srt", "removeEmbedded": false, "makeDefault": true,
		"language": "spa", "ocrLanguage": "jpn", "ocrMode": "clean", "title": "Signs",
	}})
	transform := transforms[4]
	if transform.RemoveEmbedded || !transform.MakeDefault || transform.Language != "spa" || transform.OCRLanguage != "jpn" || transform.OCRMode != "clean" || transform.Title != "Signs" {
		t.Fatalf("legacy transform=%#v", transform)
	}
}

func TestResolveTrackPlanUnknownLanguageIsNotWildcard(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{
		map[string]any{"index": 2, "codec": "subrip", "language": "spa"},
		map[string]any{"index": 3, "codec": "subrip", "language": "eng"},
		map[string]any{"index": 4, "codec": "subrip", "language": ""},
	}}
	plan, err := resolveTrackPlan(scan, map[string]any{
		"subtitleDisposition": "keep",
		"subtitleRules":       []any{map[string]any{"language": "und", "action": "remove"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []SubtitleDisposition{SubtitleDispositionKeep, SubtitleDispositionKeep, SubtitleDispositionRemove}
	for index, action := range want {
		if plan.SubtitleStreams[index].Action != action {
			t.Fatalf("actions=%#v", plan.SubtitleStreams)
		}
	}
}

func TestResolveTrackPlanSpecificRulesDefaultAndStreamPrecedence(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{
		map[string]any{"index": 2, "codec": "subrip", "language": "spa"},
		map[string]any{"index": 4, "codec": "ass", "language": "spa"},
		map[string]any{"index": 5, "codec": "subrip", "language": "eng"},
	}}
	plan, err := resolveTrackPlan(scan, map[string]any{
		"subtitleDisposition": "remove",
		"subtitleRules": []any{
			map[string]any{"language": "spa", "action": "keep"},
			map[string]any{"streamIndex": 4, "action": "keep_and_extract"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []SubtitleDisposition{SubtitleDispositionKeep, SubtitleDispositionKeepAndExtract, SubtitleDispositionRemove}
	for index, action := range want {
		if plan.SubtitleStreams[index].Action != action {
			t.Fatalf("actions=%#v", plan.SubtitleStreams)
		}
	}
}

func TestResolveTrackPlanRejectsSelectorlessSubtitleRule(t *testing.T) {
	_, err := resolveTrackPlan(trackResolverScan(), map[string]any{
		"trackDispositionVersion": 1,
		"attachmentPolicy":        "auto",
		"chapterPolicy":           "keep",
		"subtitleDisposition":     "keep",
		"subtitleRules":           []any{map[string]any{"language": "", "action": "remove"}},
	})
	if err == nil || !strings.Contains(err.Error(), "requires a non-empty language or streamIndex") {
		t.Fatalf("selectorless rule error=%v", err)
	}
}

func TestResolveTrackPlanLegacySelectorlessSubtitleRuleIsIgnored(t *testing.T) {
	plan, err := resolveTrackPlan(trackResolverScan(), map[string]any{
		"subtitleDisposition": "keep",
		"subtitleRules": []any{
			map[string]any{
				"language": "",
				"action":   "remove",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, subtitle := range plan.SubtitleStreams {
		if subtitle.Action != SubtitleDispositionKeep {
			t.Fatalf("legacy selectorless rule changed subtitle action: %#v", plan.SubtitleStreams)
		}
	}
}

func TestResolveTrackPlanExplicitEmptyAndLegacyDefault(t *testing.T) {
	scan := trackResolverScan()
	empty, err := resolveTrackPlan(scan, map[string]any{"keepAudioStreams": []int{}, "keepSubtitleStreams": []int{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.AudioStreams) != 0 {
		t.Fatalf("explicit empty audio selection was lost: %#v", empty.AudioStreams)
	}
	for _, subtitle := range empty.SubtitleStreams {
		if subtitle.Action != SubtitleDispositionRemove {
			t.Fatalf("explicit empty subtitle selection was lost: %#v", empty.SubtitleStreams)
		}
	}

	fallback, err := resolveTrackPlan(scan, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fallback.AudioStreams) != len(scan.AudioStreams) || fallback.SubtitleStreams[0].Action != SubtitleDispositionKeep || !fallback.AttachmentsKept || !fallback.ChaptersKept {
		t.Fatalf("legacy preserve defaults changed: %#v", fallback)
	}
}

func TestResolveTrackPlanAttachmentAutoMatrix(t *testing.T) {
	tests := []struct {
		name   string
		codec  string
		action string
		keep   bool
	}{
		{"ASS retained", "ass", "keep", true},
		{"SSA retained", "ssa", "keep", true},
		{"ASS removed", "ass", "remove", false},
		{"ASS extract only", "ass", "extract", false},
		{"ASS keep and extract", "ass", "keep_and_extract", true},
		{"SRT retained", "subrip", "keep", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scan := models.ScanResult{SubtitleStreams: models.JSONList{map[string]any{"index": 2, "codec": test.codec}}, RawProbe: models.JSONMap{"streams": []any{map[string]any{"index": 3, "codec_type": "attachment", "codec_name": "ttf"}}}}
			plan, err := resolveTrackPlan(scan, map[string]any{"subtitleDisposition": test.action, "attachmentPolicy": "auto"})
			if err != nil {
				t.Fatal(err)
			}
			if plan.AttachmentsKept != test.keep {
				t.Fatalf("attachmentsKept=%t want=%t plan=%#v", plan.AttachmentsKept, test.keep, plan)
			}
		})
	}
}

func TestResolveTrackPlanUsesCanonicalAttachmentInventory(t *testing.T) {
	scan := models.ScanResult{
		SubtitleStreams: models.JSONList{map[string]any{"index": 2, "codec": "ass"}},
		AttachmentStreams: models.JSONList{map[string]any{
			"index": 7, "codec": "ttf", "filename": "Canonical.ttf", "mimeType": "application/x-truetype-font", "title": "Canonical font", "attachmentKind": "FONT", "fontFormat": "TTF",
		}},
		RawProbe: models.JSONMap{"streams": []any{map[string]any{"index": 99, "codec_type": "attachment", "codec_name": "bin_data", "tags": map[string]any{"filename": "wrong.bin"}}}},
	}
	plan, err := resolveTrackPlan(scan, map[string]any{"subtitleDisposition": "keep", "attachmentPolicy": "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.AttachmentsKept || len(plan.AttachmentStreams) != 1 {
		t.Fatalf("canonical attachment plan=%#v", plan)
	}
	attachment := plan.AttachmentStreams[0]
	if attachment.StreamIndex != 7 || attachment.Filename != "Canonical.ttf" || attachment.MIMEType != "application/x-truetype-font" || attachment.AttachmentKind != "FONT" || attachment.FontFormat != "TTF" {
		t.Fatalf("resolved attachment metadata=%#v", attachment)
	}
}

func TestResolveTrackPlanAttachmentPoliciesKeepInventory(t *testing.T) {
	scan := models.ScanResult{
		SubtitleStreams:   models.JSONList{map[string]any{"index": 2, "codec": "ass"}},
		AttachmentStreams: models.JSONList{map[string]any{"index": 7, "codec": "ttf", "filename": "Font.ttf", "attachmentKind": "FONT", "fontFormat": "TTF"}},
	}
	tests := []struct {
		policy string
		keep   bool
	}{
		{policy: "keep", keep: true},
		{policy: "remove", keep: false},
		{policy: "auto", keep: true},
	}
	for _, test := range tests {
		t.Run(test.policy, func(t *testing.T) {
			plan, err := resolveTrackPlan(scan, map[string]any{"subtitleDisposition": "keep", "attachmentPolicy": test.policy})
			if err != nil {
				t.Fatal(err)
			}
			if plan.AttachmentsKept != test.keep || len(plan.AttachmentStreams) != 1 {
				t.Fatalf("policy=%s plan=%#v", test.policy, plan)
			}
		})
	}
}

func TestResolveTrackPlanLegacyAttachmentInventoryAndKnownEmptyJSON(t *testing.T) {
	legacy := models.ScanResult{RawProbe: models.JSONMap{"streams": models.JSONList{
		models.JSONMap{"index": 7, "codec_type": "attachment", "codec_name": "otf", "tags": models.JSONMap{"filename": "Legacy.otf", "mimetype": "font/otf"}},
	}}}
	plan, err := resolveTrackPlan(legacy, map[string]any{"attachmentPolicy": "keep"})
	if err != nil || len(plan.AttachmentStreams) != 1 || plan.AttachmentStreams[0].StreamIndex != 7 || plan.AttachmentStreams[0].FontFormat != "OTF" {
		t.Fatalf("legacy canonical plan=%#v err=%v", plan, err)
	}

	empty, err := resolveTrackPlan(models.ScanResult{AttachmentStreams: models.JSONList{}}, map[string]any{"attachmentPolicy": "keep"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"attachmentStreams":[]`) {
		t.Fatalf("resolved known-empty attachments serialized as null: %s", encoded)
	}
}

func TestResolveTrackPlanMixedASSAndExplicitAttachmentPolicies(t *testing.T) {
	scan := models.ScanResult{
		SubtitleStreams: models.JSONList{
			map[string]any{"index": 2, "codec": "ass", "language": "spa"},
			map[string]any{"index": 3, "codec": "ass", "language": "eng"},
		},
		RawProbe: models.JSONMap{"streams": []any{map[string]any{"index": 4, "codec_type": "attachment", "codec_name": "ttf"}}},
	}
	mixed, err := resolveTrackPlan(scan, map[string]any{
		"subtitleDisposition": "keep", "attachmentPolicy": "auto",
		"subtitleRules": []any{map[string]any{"streamIndex": 2, "action": "extract"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !mixed.AttachmentsKept || mixed.AttachmentReason != "embedded ASS/SSA subtitle may require font attachments" || len(mixed.SidecarOutputs) != 1 || mixed.SidecarOutputs[0].Format != "ass" {
		t.Fatalf("mixed ASS resolution=%#v", mixed)
	}
	explicitKeep, err := resolveTrackPlan(scan, map[string]any{"subtitleDisposition": "extract", "attachmentPolicy": "keep"})
	if err != nil || !explicitKeep.AttachmentsKept || explicitKeep.FontAttachmentsExported {
		t.Fatalf("explicit keep resolution=%#v err=%v", explicitKeep, err)
	}
	explicitRemove, err := resolveTrackPlan(scan, map[string]any{"subtitleDisposition": "keep", "attachmentPolicy": "remove"})
	if err != nil || explicitRemove.AttachmentsKept || len(explicitRemove.Warnings) != 1 {
		t.Fatalf("explicit remove resolution=%#v err=%v", explicitRemove, err)
	}
}

func TestResolveFontAttachmentExportActivationMatrix(t *testing.T) {
	baseScan := func(codec string) models.ScanResult {
		return models.ScanResult{
			SubtitleStreams: models.JSONList{models.JSONMap{"index": 2, "codec": codec, "language": "spa"}},
			AttachmentStreams: models.JSONList{
				models.JSONMap{"index": 6, "type": "attachment", "codec": "mjpeg", "filename": "cover.jpg", "mimeType": "image/jpeg", "attachmentKind": "IMAGE"},
				models.JSONMap{"index": 7, "type": "attachment", "codec": "ttf", "filename": "Font.ttf", "mimeType": "font/ttf", "attachmentKind": "FONT", "fontFormat": "TTF"},
			},
			AttachmentInventoryAvailable: true,
		}
	}
	tests := []struct {
		name, codec, disposition string
		formats                  []string
		policy                   string
		wantFonts                int
	}{
		{name: "ASS original none", codec: "ass", disposition: "extract", formats: []string{"original"}, policy: "none"},
		{name: "ASS original all", codec: "ass", disposition: "extract", formats: []string{"original"}, policy: "all", wantFonts: 1},
		{name: "SSA original all", codec: "ssa", disposition: "extract", formats: []string{"original"}, policy: "all", wantFonts: 1},
		{name: "SRT original all", codec: "subrip", disposition: "extract", formats: []string{"original"}, policy: "all"},
		{name: "ASS compatibility only", codec: "ass", disposition: "extract", formats: []string{"srt"}, policy: "all"},
		{name: "ASS converted ASS", codec: "ass", disposition: "extract", formats: []string{"ass"}, policy: "all"},
		{name: "PGS OCR ASS", codec: "hdmv_pgs_subtitle", disposition: "extract", formats: []string{"ass"}, policy: "all"},
		{name: "ASS original and compatibility", codec: "ass", disposition: "extract", formats: []string{"original", "srt"}, policy: "all", wantFonts: 1},
		{name: "ASS embedded only", codec: "ass", disposition: "keep", formats: []string{"original"}, policy: "all"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := resolveTrackPlan(baseScan(test.codec), map[string]any{
				"trackDispositionVersion": 1, "subtitleDisposition": test.disposition, "subtitleSidecarFormats": test.formats,
				"attachmentPolicy": "remove", "fontAttachmentExportPolicy": test.policy, "chapterPolicy": "keep",
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.FontAttachments) != test.wantFonts || plan.FontAttachmentsExported != (test.wantFonts > 0) {
				t.Fatalf("font plan=%#v", plan)
			}
			if plan.AttachmentsKept {
				t.Fatal("font sidecar export changed final-MKV attachment removal")
			}
		})
	}
}

func TestResolveFontAttachmentPlanFreezesFormatsOrdinalsAndSafeNames(t *testing.T) {
	scan := models.ScanResult{
		SubtitleStreams: models.JSONList{models.JSONMap{"index": 2, "codec": "ass"}, models.JSONMap{"index": 3, "codec": "ass"}},
		AttachmentStreams: models.JSONList{
			models.JSONMap{"index": 6, "type": "attachment", "codec": "png", "filename": "cover.png", "mimeType": "image/png", "attachmentKind": "IMAGE"},
			models.JSONMap{"index": 7, "type": "attachment", "codec": "ttf", "filename": "../../Arial.ttf", "attachmentKind": "FONT", "fontFormat": "TTF"},
			models.JSONMap{"index": 8, "type": "attachment", "codec": "otf", "filename": `..\..\Arial.otf`, "attachmentKind": "FONT", "fontFormat": "OTF"},
			models.JSONMap{"index": 9, "type": "attachment", "codec": "ttc", "filename": "", "attachmentKind": "FONT", "fontFormat": "TTC"},
			models.JSONMap{"index": 10, "type": "attachment", "codec": "otc", "filename": "Collection.otc", "attachmentKind": "FONT", "fontFormat": "OTC"},
		},
		AttachmentInventoryAvailable: true,
	}
	plan, err := resolveTrackPlan(scan, map[string]any{
		"trackDispositionVersion": 1, "subtitleDisposition": "extract", "subtitleSidecarFormats": []string{"original"},
		"attachmentPolicy": "keep", "fontAttachmentExportPolicy": "all", "chapterPolicy": "keep",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.FontAttachments) != 4 {
		t.Fatalf("fonts=%#v", plan.FontAttachments)
	}
	wantNames := []string{"Arial.ttf", "Arial.otf", "attachment-9.ttc", "Collection.otc"}
	wantOrdinals := []int{1, 2, 3, 4}
	for index := range wantNames {
		if plan.FontAttachments[index].SafeFilename != wantNames[index] || plan.FontAttachments[index].AttachmentOrdinal != wantOrdinals[index] {
			t.Fatalf("font[%d]=%#v", index, plan.FontAttachments[index])
		}
	}
	if plan.FontAttachments[0].ArtifactID != "font-attachment:7" {
		t.Fatalf("artifact ID=%q", plan.FontAttachments[0].ArtifactID)
	}
}

func TestResolveFontAttachmentPlanDisambiguatesDuplicateNames(t *testing.T) {
	attachments := []ResolvedAttachmentStream{
		{StreamIndex: 7, Filename: "Arial.ttf", AttachmentKind: "FONT", FontFormat: "TTF"},
		{StreamIndex: 9, Filename: "Arial.ttf", AttachmentKind: "FONT", FontFormat: "TTF"},
	}
	fonts, err := resolveFontAttachmentPlan(FontAttachmentExportAll, true, attachments, []ResolvedTrackSidecar{{Format: "ass", Mode: "original"}})
	if err != nil || len(fonts) != 2 || fonts[0].SafeFilename != "Arial.ttf" || fonts[1].SafeFilename != "Arial.stream-9.ttf" {
		t.Fatalf("fonts=%#v err=%v", fonts, err)
	}
}

func TestResolveFontAttachmentPlanRequiresAvailableInventory(t *testing.T) {
	_, err := resolveFontAttachmentPlan(FontAttachmentExportAll, false, nil, []ResolvedTrackSidecar{{Format: "ass", Mode: "original"}})
	if err == nil || !strings.Contains(err.Error(), "refresh") {
		t.Fatalf("err=%v", err)
	}
	fonts, err := resolveFontAttachmentPlan(FontAttachmentExportAll, true, nil, []ResolvedTrackSidecar{{Format: "ass", Mode: "original"}})
	if err != nil || len(fonts) != 0 {
		t.Fatalf("known-empty fonts=%#v err=%v", fonts, err)
	}
}

func TestQueueTrackPlanSnapshotIsImmutableAfterProfileEdit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:resolved-track-plan-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Clean("/media/raw/show/episode.mkv")
	if err := db.Create(&models.ScanResult{Path: path, AudioStreams: models.JSONList{map[string]any{"index": 1, "codec": "aac", "language": "jpn"}}, SubtitleStreams: models.JSONList{map[string]any{"index": 2, "codec": "ass", "language": "jpn", "title": "Full"}}, AttachmentStreams: models.JSONList{map[string]any{"index": 7, "type": "attachment", "codec": "ttf", "filename": "Font.ttf", "mimeType": "font/ttf", "attachmentKind": "FONT", "fontFormat": "TTF"}}, AttachmentInventoryAvailable: true}).Error; err != nil {
		t.Fatal(err)
	}
	setting := models.AppSetting{
		Key: "trackProfiles",
		Value: models.JSONMap{"profiles": models.JSONList{
			models.JSONMap{"key": "tracks", "scope": "path", "trackDispositionVersion": 1, "audioMode": "all", "subtitleMode": "all", "subtitleDisposition": "keep_and_extract", "subtitleRules": models.JSONList{models.JSONMap{"streamIndex": 2, "roles": models.JSONList{"full"}, "action": "keep_and_extract"}}, "subtitleSidecarFormats": models.JSONList{"original", "srt"}, "attachmentPolicy": "auto", "fontAttachmentExportPolicy": "all", "chapterPolicy": "keep"},
		}},
	}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	translationSettings := defaultTranslationSettings()
	translationSettings.Provider = translationProviderOllama
	translationSettings.OllamaEndpoint = "http://ollama.example"
	translationSettings.OllamaModel = "translation-model"
	translationSettings.Enabled = true
	translationSettings.Readiness = translationReadinessReady
	translationSettings.ReadinessIdentity = translationProviderConfigurationIdentity(translationSettings)
	persistTranslationSettingsFixture(t, db, translationSettings)
	job := models.QueueJob{MediaPath: path, TrackProfileKey: "tracks"}
	if err := NewQueueHandler(db).captureSupplementalProfiles(&job); err != nil {
		t.Fatal(err)
	}
	frozen, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || frozen.SubtitleStreams[0].Action != SubtitleDispositionKeepAndExtract || frozen.SubtitleStreams[0].Role != SubtitleRoleFull || !frozen.AttachmentsKept || len(frozen.SidecarOutputs) != 2 || len(frozen.FontAttachments) != 1 {
		t.Fatalf("canonical plan was not frozen: %#v", job.TrackProfileSnapshot)
	}
	for _, sidecar := range frozen.SidecarOutputs {
		if sidecar.Mode == "translation" {
			t.Fatalf("Queue unexpectedly created a translation sidecar: %#v", sidecar)
		}
	}
	setting.Value["profiles"] = models.JSONList{models.JSONMap{"key": "tracks", "scope": "path", "subtitleDisposition": "remove", "subtitleSidecarFormats": models.JSONList{"original"}, "attachmentPolicy": "remove", "fontAttachmentExportPolicy": "none", "chapterPolicy": "remove"}}
	if err := db.Save(&setting).Error; err != nil {
		t.Fatal(err)
	}
	stillFrozen, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || stillFrozen.SubtitleStreams[0].Action != SubtitleDispositionKeepAndExtract || stillFrozen.SubtitleStreams[0].Role != SubtitleRoleFull || !stillFrozen.ChaptersKept || len(stillFrozen.SidecarOutputs) != 2 || len(stillFrozen.FontAttachments) != 1 {
		t.Fatalf("queued decision changed after profile edit: %#v", stillFrozen)
	}
	override := currentConversionOverrideForJob(job, nil)
	if override.ResolvedTrackPlan == nil || override.ResolvedTrackPlan.SubtitleStreams[0].Action != SubtitleDispositionKeepAndExtract || len(override.ResolvedTrackPlan.SidecarOutputs) != 2 {
		t.Fatalf("worker did not restore frozen track plan: %#v", override)
	}
	for _, sidecar := range override.ResolvedTrackPlan.SidecarOutputs {
		if sidecar.Mode == "translation" {
			t.Fatalf("worker unexpectedly restored a translation sidecar: %#v", sidecar)
		}
	}
}

func TestQueueTrackPlanFreezesBitmapOCRConfiguration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:resolved-track-plan-ocr-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Clean("/media/raw/show/bitmap.mkv")
	if err := db.Create(&models.ScanResult{Path: path, SubtitleStreams: models.JSONList{map[string]any{"index": 4, "codec": "hdmv_pgs_subtitle", "language": "spa"}}}).Error; err != nil {
		t.Fatal(err)
	}
	setting := models.AppSetting{Key: "trackProfiles", Value: models.JSONMap{"profiles": models.JSONList{models.JSONMap{
		"key": "ocr", "scope": "path", "trackDispositionVersion": 1, "subtitleDisposition": "extract", "subtitleSidecarFormats": models.JSONList{"srt"},
		"subtitleRules":    models.JSONList{models.JSONMap{"streamIndex": 4, "action": "extract", "sidecarFormats": models.JSONList{"srt"}, "ocrLanguage": "spa", "ocrMode": "accurate"}},
		"attachmentPolicy": "auto", "chapterPolicy": "keep",
	}}}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: path, TrackProfileKey: "ocr"}
	if err := NewQueueHandler(db).captureSupplementalProfiles(&job); err != nil {
		t.Fatal(err)
	}
	setting.Value["profiles"] = models.JSONList{models.JSONMap{"key": "ocr", "scope": "path", "trackDispositionVersion": 1, "subtitleDisposition": "extract", "subtitleSidecarFormats": models.JSONList{"srt"}, "subtitleRules": models.JSONList{models.JSONMap{"streamIndex": 4, "action": "extract", "ocrLanguage": "eng", "ocrMode": "raw"}}}}
	if err := db.Save(&setting).Error; err != nil {
		t.Fatal(err)
	}
	frozen, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || len(frozen.SidecarOutputs) != 1 || frozen.SidecarOutputs[0].OCRLanguage != "spa" || frozen.SidecarOutputs[0].OCRMode != "accurate" {
		t.Fatalf("frozen OCR plan=%#v", frozen)
	}
	planned := plannedSubtitleArtifacts(job)
	if len(planned) != 1 || planned[0].OCRLanguage != "spa" || planned[0].OCRMode != "accurate" {
		t.Fatalf("frozen OCR artifact=%#v", planned)
	}
}

func TestExplicitTrackDispositionVersionControlsCanonicalSemantics(t *testing.T) {
	canonical, err := canonicalTrackDispositionProfile(map[string]any{"subtitleMode": "all"})
	if err != nil || canonical {
		t.Fatal("legacy profile was incorrectly treated as canonical")
	}
	canonical, err = canonicalTrackDispositionProfile(map[string]any{
		"subtitleDisposition": "keep", "attachmentPolicy": "auto", "chapterPolicy": "keep",
	})
	if err != nil || canonical {
		t.Fatal("frontend fallback fields incorrectly migrated a legacy profile")
	}
	canonical, err = canonicalTrackDispositionProfile(map[string]any{
		"trackDispositionVersion": 1, "subtitleDisposition": "keep", "attachmentPolicy": "auto", "chapterPolicy": "keep",
	})
	if err != nil || !canonical {
		t.Fatal("explicit canonical profile was not recognized")
	}
	if _, err = canonicalTrackDispositionProfile(map[string]any{"trackDispositionVersion": 2}); err == nil {
		t.Fatal("unknown future disposition version was accepted")
	}
}

func TestQueueFreezesLegacyAndCanonicalTrackProfilesByExplicitVersion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:track-version-queue?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Clean("/media/raw/show/versioned.mkv")
	if err := db.Create(&models.ScanResult{Path: path, SubtitleStreams: models.JSONList{
		map[string]any{"index": 2, "codec": "ass"}, map[string]any{"index": 3, "codec": "subrip", "language": "spa"},
	}}).Error; err != nil {
		t.Fatal(err)
	}
	setting := models.AppSetting{Key: "trackProfiles", Value: models.JSONMap{"profiles": models.JSONList{
		models.JSONMap{"key": "legacy", "subtitleDisposition": "keep", "attachmentPolicy": "auto", "chapterPolicy": "keep"},
		models.JSONMap{"key": "canonical", "trackDispositionVersion": 1, "subtitleDisposition": "keep", "subtitleRules": models.JSONList{models.JSONMap{"language": "und", "action": "remove"}}, "attachmentPolicy": "auto", "chapterPolicy": "keep"},
	}}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	handler := NewQueueHandler(db)
	legacy := models.QueueJob{MediaPath: path, TrackProfileKey: "legacy"}
	if err := handler.captureSupplementalProfiles(&legacy); err != nil {
		t.Fatal(err)
	}
	if _, ok := ResolvedTrackPlanFromSnapshot(legacy.TrackProfileSnapshot); ok {
		t.Fatal("legacy queue snapshot unexpectedly contains a canonical plan")
	}
	canonical := models.QueueJob{MediaPath: path, TrackProfileKey: "canonical"}
	if err := handler.captureSupplementalProfiles(&canonical); err != nil {
		t.Fatal(err)
	}
	plan, ok := ResolvedTrackPlanFromSnapshot(canonical.TrackProfileSnapshot)
	if !ok || plan.SubtitleStreams[0].Action != SubtitleDispositionRemove || plan.SubtitleStreams[1].Action != SubtitleDispositionKeep || intValueSetting(canonical.TrackProfileSnapshot["trackDispositionVersion"], 0) != 1 {
		t.Fatalf("canonical queue snapshot=%#v", canonical.TrackProfileSnapshot)
	}
}

func TestFreezeEffectiveTrackPlanPreservesAssetOverridePrecedence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:effective-track-plan-asset-override?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Clean("/media/raw/show/episode-override.mkv")
	if err := db.Create(&models.ScanResult{Path: path, AudioStreams: models.JSONList{map[string]any{"index": 1}, map[string]any{"index": 2}}, SubtitleStreams: models.JSONList{map[string]any{"index": 4, "codec": "ass", "language": "spa"}, map[string]any{"index": 5, "codec": "ass", "language": "spa"}}}).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: path, TrackProfileSnapshot: models.JSONMap{
		"keepAudioStreams": []any{float64(1)}, "trackDispositionVersion": 1, "subtitleDisposition": "keep_and_extract", "subtitleSidecarFormats": models.JSONList{"original", "srt"}, "attachmentPolicy": "auto", "chapterPolicy": "keep", resolvedTrackPlanSnapshotKey: models.JSONMap{
			"videoStreams": []any{}, "audioStreams": []any{map[string]any{"streamIndex": float64(1)}}, "audioSelectionExplicit": true,
			"subtitleStreams": []any{}, "attachmentPolicy": "keep", "attachmentsKept": true, "attachmentStreams": []any{},
			"chapterPolicy": "keep", "chaptersKept": true, "sidecarOutputs": []any{},
		},
	}}
	override := AssetConversionOverrideState{KeepAudioStreams: []int{2}, KeepSubtitleStreams: []int{4}, SubtitleSidecarFormatsByStream: map[int][]string{4: {"original"}}, assetKeepSubtitleStreamsExplicit: true}
	if err := NewQueueHandler(db).freezeEffectiveTrackPlan(&job, &override); err != nil {
		t.Fatal(err)
	}
	if override.ResolvedTrackPlan == nil || len(override.ResolvedTrackPlan.AudioStreams) != 1 || override.ResolvedTrackPlan.AudioStreams[0].StreamIndex != 2 {
		t.Fatalf("asset override did not win in frozen canonical plan: %#v", override.ResolvedTrackPlan)
	}
	if len(override.ResolvedTrackPlan.SidecarOutputs) != 1 || override.ResolvedTrackPlan.SidecarOutputs[0].Format != "ass" {
		t.Fatalf("asset sidecar override was not frozen: %#v", override.ResolvedTrackPlan.SidecarOutputs)
	}
	for _, subtitle := range override.ResolvedTrackPlan.SubtitleStreams {
		if subtitle.StreamIndex == 5 && subtitle.Action != SubtitleDispositionRemove {
			t.Fatalf("asset exclusion was not frozen ahead of canonical policy: %#v", override.ResolvedTrackPlan.SubtitleStreams)
		}
	}
}

func TestFreezeEffectiveTrackPlanTreatsOmittedAssetSubtitleSelectionAsInheritance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:effective-track-plan-subtitle-inherit?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Clean("/media/raw/show/episode-inherit.mkv")
	if err := db.Create(&models.ScanResult{Path: path, SubtitleStreams: models.JSONList{
		map[string]any{"index": 2, "codec": "subrip", "language": "eng"},
		map[string]any{"index": 3, "codec": "ass", "language": "spa"},
	}}).Error; err != nil {
		t.Fatal(err)
	}
	initialPlan, err := resolvedTrackPlanMap(ResolvedTrackPlan{
		SubtitleStreams: []ResolvedSubtitleTrack{
			{StreamIndex: 2, Action: SubtitleDispositionKeep},
			{StreamIndex: 3, Action: SubtitleDispositionKeep},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: path, TrackProfileSnapshot: models.JSONMap{
		"scope": "path", "resolvedForAsset": path, "trackDispositionVersion": 1,
		"subtitleDisposition": "keep", "subtitleRules": []any{map[string]any{"streamIndex": 3, "action": "extract"}},
		"attachmentPolicy": "auto", "chapterPolicy": "keep", resolvedTrackPlanSnapshotKey: initialPlan,
	}}
	var override AssetConversionOverrideState
	if err := NewQueueHandler(db).freezeEffectiveTrackPlan(&job, &override); err != nil {
		t.Fatal(err)
	}
	if override.assetKeepSubtitleStreamsExplicit || override.KeepSubtitleStreams != nil {
		t.Fatalf("omitted asset selection became explicit during freeze: %#v", override)
	}
	if override.ResolvedTrackPlan == nil || len(override.ResolvedTrackPlan.SubtitleStreams) != 2 || override.ResolvedTrackPlan.SubtitleStreams[1].Action != SubtitleDispositionExtract {
		t.Fatalf("omitted asset selection stopped inheriting the Track Profile rule: %#v", override.ResolvedTrackPlan)
	}
}

func trackResolverScan() models.ScanResult {
	return models.ScanResult{
		VideoStreams: models.JSONList{map[string]any{"index": 0, "codec": "h264"}},
		AudioStreams: models.JSONList{map[string]any{"index": 1, "codec": "aac", "language": "jpn"}, map[string]any{"index": 2, "codec": "ac3", "language": "eng"}},
		SubtitleStreams: models.JSONList{
			map[string]any{"index": 3, "codec": "subrip", "language": "spa"},
			map[string]any{"index": 4, "codec": "ssa", "language": "spa"},
			map[string]any{"index": 5, "codec": "ass", "language": "jpn"},
			map[string]any{"index": 6, "codec": "subrip", "language": "eng"},
		},
		RawProbe: models.JSONMap{"streams": []any{map[string]any{"index": 7, "codec_type": "attachment", "codec_name": "ttf"}}}, Chapters: 4,
	}
}

func TestResolveTrackPlanV1IgnoresLegacySubtitleSelectionAndTransforms(t *testing.T) {
	scan := trackResolverScan()

	plan, err := resolveTrackPlan(scan, map[string]any{
		"trackDispositionVersion": 1,
		"subtitleDisposition":     "keep",
		"subtitleRules":           []any{},
		"attachmentPolicy":        "auto",
		"chapterPolicy":           "keep",

		// Legacy fields must not influence a canonical V1 profile.
		"keepSubtitleStreams": []int{},
		"subtitleTransforms": []any{
			map[string]any{
				"streamIndex":    3,
				"format":         "ass",
				"removeEmbedded": true,
			},
		},
	})

	if err != nil {
		t.Fatal(err)
	}

	for _, subtitle := range plan.SubtitleStreams {
		if subtitle.Action != SubtitleDispositionKeep {
			t.Fatalf(
				"V1 profile was changed by legacy subtitle fields: %#v",
				plan.SubtitleStreams,
			)
		}
	}

	if len(plan.SidecarOutputs) != 0 {
		t.Fatalf(
			"V1 profile generated sidecars from legacy subtitleTransforms: %#v",
			plan.SidecarOutputs,
		)
	}
}

func TestResolveTrackPlanAssetSubtitleSelectionOutranksCanonicalRulesAndOperations(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{
		map[string]any{"index": 2, "codec": "subrip", "language": "spa"},
		map[string]any{"index": 3, "codec": "ass", "language": "spa"},
		map[string]any{"index": 4, "codec": "hdmv_pgs_subtitle", "language": "spa"},
	}}
	profile := map[string]any{
		"trackDispositionVersion": 1,
		"subtitleDisposition":     "keep",
		"attachmentPolicy":        "auto",
		"chapterPolicy":           "keep",
		"subtitleRules": []any{
			map[string]any{"streamIndex": 2, "action": "keep_and_extract"},
			map[string]any{"streamIndex": 3, "action": "extract"},
			map[string]any{"streamIndex": 4, "action": "keep"},
		},
		assetKeepSubtitleStreamsConstraintKey: []int{2, 4},
	}
	streamIndex := 3
	plan, err := resolveTrackPlanWithSubtitleOverrides(scan, profile, SubtitleOperationOverrides{Extract: true, StreamIndex: &streamIndex})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SubtitleStreams) != 1 || plan.SubtitleStreams[0].StreamIndex != 3 || plan.SubtitleStreams[0].Action != SubtitleDispositionRemove {
		t.Fatalf("excluded asset stream was restored by a rule or operation override: %#v", plan.SubtitleStreams)
	}
	if len(plan.SidecarOutputs) != 0 {
		t.Fatalf("excluded asset stream produced sidecar outputs: %#v", plan.SidecarOutputs)
	}

	// Without the asset constraint, the canonical rule still behaves as before.
	delete(profile, assetKeepSubtitleStreamsConstraintKey)
	inherited, err := resolveTrackPlan(scan, profile)
	if err != nil {
		t.Fatal(err)
	}
	if inherited.SubtitleStreams[1].Action != SubtitleDispositionExtract || len(inherited.SidecarOutputs) == 0 {
		t.Fatalf("omitted asset selection did not inherit existing profile rules: %#v", inherited)
	}
	emptySelection := map[string]any{}
	for key, value := range profile {
		emptySelection[key] = value
	}
	emptySelection[assetKeepSubtitleStreamsConstraintKey] = []int{}
	empty, err := resolveTrackPlan(scan, emptySelection)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.SidecarOutputs) != 0 {
		t.Fatalf("explicit empty asset selection scheduled sidecars: %#v", empty.SidecarOutputs)
	}
	for _, subtitle := range empty.SubtitleStreams {
		if subtitle.Action != SubtitleDispositionRemove {
			t.Fatalf("explicit empty asset selection did not remove stream %d: %#v", subtitle.StreamIndex, empty.SubtitleStreams)
		}
	}
}

func TestFrozenAssetSubtitleSelectionSurvivesSelectionChangesOnRetryAndRestart(t *testing.T) {
	mediaPath := filepath.Clean("/raw/Episode.mkv")
	encoded, err := json.Marshal(AssetConversionOverrideState{KeepSubtitleStreams: []int{2, 4}})
	if err != nil {
		t.Fatal(err)
	}
	var frozenOverride map[string]any
	if err := json.Unmarshal(encoded, &frozenOverride); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{
		MediaPath: mediaPath,
		ProfileSnapshot: models.JSONMap{
			assetConversionOverrideSnapshotKey:        frozenOverride,
			assetSubtitleSelectionExplicitSnapshotKey: true,
		},
	}
	currentAssetState := map[string]AssetConversionOverrideState{
		mediaPath: {KeepSubtitleStreams: []int{3}},
	}
	for attempt := 1; attempt <= 2; attempt++ {
		override := conversionOverrideForJob(job, currentAssetState)
		if !reflect.DeepEqual(override.KeepSubtitleStreams, []int{2, 4}) || !override.assetKeepSubtitleStreamsExplicit {
			t.Fatalf("retry/restart %d read the changed asset selection instead of the frozen job: %#v", attempt, override)
		}
	}
	newJobOverride := currentConversionOverrideForJob(models.QueueJob{MediaPath: mediaPath}, currentAssetState)
	if !reflect.DeepEqual(newJobOverride.KeepSubtitleStreams, []int{3}) || !newJobOverride.assetKeepSubtitleStreamsExplicit {
		t.Fatalf("newly planned job did not read the updated selection: %#v", newJobOverride)
	}
}

func TestResolveTrackPlanV1HonorsResolvedPathSubtitleSelection(t *testing.T) {
	scan := trackResolverScan()

	plan, err := resolveTrackPlan(scan, map[string]any{
		"trackDispositionVersion": 1,
		"scope":                   "path",
		"resolvedForAsset":        "/media/raw/show/episode.mkv",
		"subtitleDisposition":     "keep",
		"subtitleRules":           []any{},
		"attachmentPolicy":        "auto",
		"chapterPolicy":           "keep",
		"keepSubtitleStreams":     []int{3, 4},
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := map[int]SubtitleDisposition{
		3: SubtitleDispositionKeep,
		4: SubtitleDispositionKeep,
		5: SubtitleDispositionRemove,
		6: SubtitleDispositionRemove,
	}

	for _, subtitle := range plan.SubtitleStreams {
		want := expected[subtitle.StreamIndex]

		if subtitle.Action != want {
			t.Fatalf(
				"subtitle stream %d action=%q want=%q; plan=%#v",
				subtitle.StreamIndex,
				subtitle.Action,
				want,
				plan.SubtitleStreams,
			)
		}
	}
}

func TestResolveTrackPlanDefaultTrackResolution(t *testing.T) {
	t.Run("resolves kept default audio", func(t *testing.T) {
		scan := models.ScanResult{
			AudioStreams: models.JSONList{
				map[string]any{
					"index":    1,
					"codec":    "aac",
					"language": "jpn",
				},
				map[string]any{
					"index":    2,
					"codec":    "ac3",
					"language": "eng",
				},
			},
		}

		plan, err := resolveTrackPlan(scan, map[string]any{
			"keepAudioStreams":     []int{1},
			"defaultAudioLanguage": "jpn",
		})
		if err != nil {
			t.Fatal(err)
		}

		if plan.DefaultAudio.Status != "resolved" {
			t.Fatalf("default audio status=%q plan=%#v", plan.DefaultAudio.Status, plan.DefaultAudio)
		}
		if plan.DefaultAudio.RequestedLanguage != "jpn" {
			t.Fatalf("default audio requested language=%q", plan.DefaultAudio.RequestedLanguage)
		}
		if plan.DefaultAudio.StreamIndex == nil || *plan.DefaultAudio.StreamIndex != 1 {
			t.Fatalf("default audio stream=%v", plan.DefaultAudio.StreamIndex)
		}
	})

	t.Run("does not resolve extracted subtitle as embedded default", func(t *testing.T) {
		scan := models.ScanResult{
			SubtitleStreams: models.JSONList{
				map[string]any{
					"index":    2,
					"codec":    "ass",
					"language": "eng",
				},
				map[string]any{
					"index":    4,
					"codec":    "ass",
					"language": "spa",
				},
			},
		}

		plan, err := resolveTrackPlan(scan, map[string]any{
			"trackDispositionVersion": 1,
			"subtitleDisposition":     "keep",
			"subtitleRules": []any{
				map[string]any{
					"streamIndex": 4,
					"action":      "extract",
				},
			},
			"defaultSubtitleLanguage": "spa",
			"attachmentPolicy":        "auto",
			"chapterPolicy":           "keep",
		})
		if err != nil {
			t.Fatal(err)
		}

		if plan.DefaultSubtitle.Status != "missing" {
			t.Fatalf(
				"default subtitle status=%q want=missing plan=%#v",
				plan.DefaultSubtitle.Status,
				plan.DefaultSubtitle,
			)
		}
		if plan.DefaultSubtitle.RequestedLanguage != "spa" {
			t.Fatalf(
				"default subtitle requested language=%q",
				plan.DefaultSubtitle.RequestedLanguage,
			)
		}
		if plan.DefaultSubtitle.StreamIndex != nil {
			t.Fatalf(
				"extracted subtitle was resolved as embedded default: %v",
				*plan.DefaultSubtitle.StreamIndex,
			)
		}
	})

	t.Run("resolves kept subtitle as default", func(t *testing.T) {
		scan := models.ScanResult{
			SubtitleStreams: models.JSONList{
				map[string]any{
					"index":    2,
					"codec":    "ass",
					"language": "eng",
				},
				map[string]any{
					"index":    4,
					"codec":    "ass",
					"language": "spa",
				},
			},
		}

		plan, err := resolveTrackPlan(scan, map[string]any{
			"trackDispositionVersion": 1,
			"subtitleDisposition":     "keep",
			"defaultSubtitleLanguage": "eng",
			"attachmentPolicy":        "auto",
			"chapterPolicy":           "keep",
		})
		if err != nil {
			t.Fatal(err)
		}

		if plan.DefaultSubtitle.Status != "resolved" {
			t.Fatalf(
				"default subtitle status=%q plan=%#v",
				plan.DefaultSubtitle.Status,
				plan.DefaultSubtitle,
			)
		}
		if plan.DefaultSubtitle.RequestedLanguage != "eng" {
			t.Fatalf(
				"default subtitle requested language=%q",
				plan.DefaultSubtitle.RequestedLanguage,
			)
		}
		if plan.DefaultSubtitle.StreamIndex == nil || *plan.DefaultSubtitle.StreamIndex != 2 {
			t.Fatalf(
				"default subtitle stream=%v",
				plan.DefaultSubtitle.StreamIndex,
			)
		}
	})

	t.Run("leaves empty defaults unchanged", func(t *testing.T) {
		plan, err := resolveTrackPlan(trackResolverScan(), map[string]any{
			"trackDispositionVersion": 1,
			"subtitleDisposition":     "keep",
			"attachmentPolicy":        "auto",
			"chapterPolicy":           "keep",
		})
		if err != nil {
			t.Fatal(err)
		}

		if plan.DefaultAudio.Status != "unchanged" {
			t.Fatalf("default audio=%#v", plan.DefaultAudio)
		}
		if plan.DefaultSubtitle.Status != "unchanged" {
			t.Fatalf("default subtitle=%#v", plan.DefaultSubtitle)
		}
	})
}
