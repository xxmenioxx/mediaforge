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

func TestResolveTrackPlanASSCompatibilitySidecars(t *testing.T) {
	scan := models.ScanResult{SubtitleStreams: models.JSONList{
		map[string]any{"index": 4, "codec": "ass", "language": "spa"},
		map[string]any{"index": 7, "codec": "ass", "language": "spa"},
	}}
	plan, err := resolveTrackPlan(scan, map[string]any{
		"trackDispositionVersion": 1,
		"subtitleDisposition":     "keep_and_extract",
		"subtitleSidecarFormats":  []string{"original", "srt"},
		"attachmentPolicy":        "auto",
		"chapterPolicy":           "keep",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SidecarOutputs) != 4 {
		t.Fatalf("sidecars=%#v", plan.SidecarOutputs)
	}
	wantFormats := []string{"ass", "srt", "ass", "srt"}
	wantModes := []string{"original", "converted", "original", "converted"}
	for index := range wantFormats {
		if plan.SidecarOutputs[index].Format != wantFormats[index] || plan.SidecarOutputs[index].Mode != wantModes[index] {
			t.Fatalf("sidecars=%#v", plan.SidecarOutputs)
		}
	}
	if !plan.AttachmentsKept || len(plan.Warnings) != 2 {
		t.Fatalf("attachments/warnings=%t %#v", plan.AttachmentsKept, plan.Warnings)
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
		t.Run(codec, func(t *testing.T) {
			scan := models.ScanResult{SubtitleStreams: models.JSONList{map[string]any{"index": 2, "codec": codec, "language": "spa"}}}
			plan, err := resolveTrackPlan(scan, map[string]any{
				"trackDispositionVersion": 1, "subtitleDisposition": "extract", "subtitleSidecarFormats": []string{"srt"}, "attachmentPolicy": "auto", "chapterPolicy": "keep",
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.SidecarOutputs) != 1 || plan.SidecarOutputs[0].Format != "srt" || plan.SidecarOutputs[0].Mode != "converted" || plan.SidecarOutputs[0].OCRLanguage != "auto" || plan.SidecarOutputs[0].OCRMode != "accurate" {
				t.Fatalf("OCR sidecar=%#v", plan.SidecarOutputs)
			}
		})
	}
	if _, _, err := resolveSubtitleSidecarFormat("unknown_bitmap", "srt"); err == nil {
		t.Fatal("unsupported subtitle codec was accepted for SRT")
	}
}

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
	if err := db.Create(&models.ScanResult{Path: path, AudioStreams: models.JSONList{map[string]any{"index": 1, "codec": "aac", "language": "jpn"}}, SubtitleStreams: models.JSONList{map[string]any{"index": 2, "codec": "ass", "language": "jpn"}}, AttachmentStreams: models.JSONList{map[string]any{"index": 7, "type": "attachment", "codec": "ttf", "filename": "Font.ttf", "mimeType": "font/ttf", "attachmentKind": "FONT", "fontFormat": "TTF"}}, AttachmentInventoryAvailable: true}).Error; err != nil {
		t.Fatal(err)
	}
	setting := models.AppSetting{
		Key: "trackProfiles",
		Value: models.JSONMap{"profiles": models.JSONList{
			models.JSONMap{"key": "tracks", "scope": "path", "trackDispositionVersion": 1, "audioMode": "all", "subtitleMode": "all", "subtitleDisposition": "keep_and_extract", "subtitleSidecarFormats": models.JSONList{"original", "srt"}, "attachmentPolicy": "auto", "fontAttachmentExportPolicy": "all", "chapterPolicy": "keep"},
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
	if !ok || frozen.SubtitleStreams[0].Action != SubtitleDispositionKeepAndExtract || !frozen.AttachmentsKept || len(frozen.SidecarOutputs) != 2 || len(frozen.FontAttachments) != 1 {
		t.Fatalf("canonical plan was not frozen: %#v", job.TrackProfileSnapshot)
	}
	setting.Value["profiles"] = models.JSONList{models.JSONMap{"key": "tracks", "scope": "path", "subtitleDisposition": "remove", "subtitleSidecarFormats": models.JSONList{"original"}, "attachmentPolicy": "remove", "fontAttachmentExportPolicy": "none", "chapterPolicy": "remove"}}
	if err := db.Save(&setting).Error; err != nil {
		t.Fatal(err)
	}
	stillFrozen, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || stillFrozen.SubtitleStreams[0].Action != SubtitleDispositionKeepAndExtract || !stillFrozen.ChaptersKept || len(stillFrozen.SidecarOutputs) != 2 || len(stillFrozen.FontAttachments) != 1 {
		t.Fatalf("queued decision changed after profile edit: %#v", stillFrozen)
	}
	override := currentConversionOverrideForJob(job, nil)
	if override.ResolvedTrackPlan == nil || override.ResolvedTrackPlan.SubtitleStreams[0].Action != SubtitleDispositionKeepAndExtract || len(override.ResolvedTrackPlan.SidecarOutputs) != 2 {
		t.Fatalf("worker did not restore frozen track plan: %#v", override)
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
	if err := db.Create(&models.ScanResult{Path: path, AudioStreams: models.JSONList{map[string]any{"index": 1}, map[string]any{"index": 2}}, SubtitleStreams: models.JSONList{map[string]any{"index": 4, "codec": "ass", "language": "spa"}}}).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: path, TrackProfileSnapshot: models.JSONMap{
		"keepAudioStreams": []any{float64(1)}, "trackDispositionVersion": 1, "subtitleDisposition": "keep_and_extract", "subtitleSidecarFormats": models.JSONList{"original", "srt"}, "attachmentPolicy": "auto", "chapterPolicy": "keep", resolvedTrackPlanSnapshotKey: models.JSONMap{
			"videoStreams": []any{}, "audioStreams": []any{map[string]any{"streamIndex": float64(1)}}, "audioSelectionExplicit": true,
			"subtitleStreams": []any{}, "attachmentPolicy": "keep", "attachmentsKept": true, "attachmentStreams": []any{},
			"chapterPolicy": "keep", "chaptersKept": true, "sidecarOutputs": []any{},
		},
	}}
	override := AssetConversionOverrideState{KeepAudioStreams: []int{2}, SubtitleSidecarFormatsByStream: map[int][]string{4: {"original"}}}
	if err := NewQueueHandler(db).freezeEffectiveTrackPlan(&job, &override); err != nil {
		t.Fatal(err)
	}
	if override.ResolvedTrackPlan == nil || len(override.ResolvedTrackPlan.AudioStreams) != 1 || override.ResolvedTrackPlan.AudioStreams[0].StreamIndex != 2 {
		t.Fatalf("asset override did not win in frozen canonical plan: %#v", override.ResolvedTrackPlan)
	}
	if len(override.ResolvedTrackPlan.SidecarOutputs) != 1 || override.ResolvedTrackPlan.SidecarOutputs[0].Format != "ass" {
		t.Fatalf("asset sidecar override was not frozen: %#v", override.ResolvedTrackPlan.SidecarOutputs)
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
