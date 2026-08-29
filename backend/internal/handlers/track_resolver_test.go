package handlers

import (
	"encoding/json"
	"path/filepath"
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
		"subtitleDisposition": "keep",
		"subtitleRules":       []any{map[string]any{"language": "", "action": "remove"}},
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

func TestQueueTrackPlanSnapshotIsImmutableAfterProfileEdit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:resolved-track-plan-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Clean("/media/raw/show/episode.mkv")
	if err := db.Create(&models.ScanResult{Path: path, AudioStreams: models.JSONList{map[string]any{"index": 1, "codec": "aac", "language": "jpn"}}, SubtitleStreams: models.JSONList{map[string]any{"index": 2, "codec": "ass", "language": "jpn"}}}).Error; err != nil {
		t.Fatal(err)
	}
	setting := models.AppSetting{
		Key: "trackProfiles",
		Value: models.JSONMap{"profiles": models.JSONList{
			models.JSONMap{"key": "tracks", "scope": "path", "trackDispositionVersion": 1, "audioMode": "all", "subtitleMode": "all", "subtitleDisposition": "keep", "attachmentPolicy": "auto", "chapterPolicy": "keep"},
		}},
	}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: path, TrackProfileKey: "tracks"}
	if err := NewQueueHandler(db).captureSupplementalProfiles(&job); err != nil {
		t.Fatal(err)
	}
	frozen, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || frozen.SubtitleStreams[0].Action != SubtitleDispositionKeep || !frozen.AttachmentsKept {
		t.Fatalf("canonical plan was not frozen: %#v", job.TrackProfileSnapshot)
	}
	setting.Value["profiles"] = models.JSONList{models.JSONMap{"key": "tracks", "scope": "path", "subtitleDisposition": "remove", "attachmentPolicy": "remove", "chapterPolicy": "remove"}}
	if err := db.Save(&setting).Error; err != nil {
		t.Fatal(err)
	}
	stillFrozen, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || stillFrozen.SubtitleStreams[0].Action != SubtitleDispositionKeep || !stillFrozen.ChaptersKept {
		t.Fatalf("queued decision changed after profile edit: %#v", stillFrozen)
	}
	override := currentConversionOverrideForJob(job, nil)
	if override.ResolvedTrackPlan == nil || override.ResolvedTrackPlan.SubtitleStreams[0].Action != SubtitleDispositionKeep {
		t.Fatalf("worker did not restore frozen track plan: %#v", override)
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
	if err := db.Create(&models.ScanResult{Path: path, AudioStreams: models.JSONList{map[string]any{"index": 1}, map[string]any{"index": 2}}}).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: path, TrackProfileSnapshot: models.JSONMap{
		"keepAudioStreams": []any{float64(1)}, resolvedTrackPlanSnapshotKey: models.JSONMap{
			"videoStreams": []any{}, "audioStreams": []any{map[string]any{"streamIndex": float64(1)}}, "audioSelectionExplicit": true,
			"subtitleStreams": []any{}, "attachmentPolicy": "keep", "attachmentsKept": true, "attachmentStreams": []any{},
			"chapterPolicy": "keep", "chaptersKept": true, "sidecarOutputs": []any{},
		},
	}}
	override := AssetConversionOverrideState{KeepAudioStreams: []int{2}}
	if err := NewQueueHandler(db).freezeEffectiveTrackPlan(&job, &override); err != nil {
		t.Fatal(err)
	}
	if override.ResolvedTrackPlan == nil || len(override.ResolvedTrackPlan.AudioStreams) != 1 || override.ResolvedTrackPlan.AudioStreams[0].StreamIndex != 2 {
		t.Fatalf("asset override did not win in frozen canonical plan: %#v", override.ResolvedTrackPlan)
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