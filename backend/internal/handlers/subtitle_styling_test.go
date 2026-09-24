package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func testEnabledSubtitleStylePolicy() SubtitleStylePolicy {
	policy := defaultSubtitleStylePolicy()
	policy.Settings.Enabled = true
	policy.Settings.ApplyToGeneratedASS = true
	policy.Settings.PreserveSourceStyles = true
	policy.Settings.OverrideExistingASSStyles = false
	policy.Version = subtitleStylePolicyHash(policy.Settings)
	return policy
}

func TestSubtitleStylingMissingAndGateDefaults(t *testing.T) {
	settings, err := normalizeSubtitleStylingSettings(nil)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled || !settings.ApplyToGeneratedASS || !settings.PreserveSourceStyles || settings.OverrideExistingASSStyles {
		t.Fatalf("unexpected safe defaults: %#v", settings)
	}
	for _, id := range []string{SubtitleStyleDefault, SubtitleStyleTop, SubtitleStyleLyrics, SubtitleStyleSign} {
		if _, ok := defaultSubtitleStylePresets()[id]; !ok {
			t.Fatalf("missing built-in %s", id)
		}
	}
	if _, err := normalizeSubtitleStylingSettings(map[string]any{"enabled": true, "preserveSourceStyles": false, "overrideExistingASSStyles": false}); err == nil {
		t.Fatal("ambiguous native policy was accepted")
	}
	if _, err := normalizeSubtitleStylingSettings(map[string]any{"version": 99}); err == nil {
		t.Fatal("unknown future version was accepted")
	}
}

func TestSubtitleColorContractAndRoundTrip(t *testing.T) {
	for input, want := range map[string]string{"#abc": "#AABBCC", "#abC123": "#ABC123"} {
		got, err := normalizeSubtitleHexColor(input)
		if err != nil || got != want {
			t.Fatalf("normalize %q = %q, %v", input, got, err)
		}
	}
	for _, invalid := range []string{"123456", "#12FG00", "#12345", "#"} {
		if _, err := normalizeSubtitleHexColor(invalid); err == nil {
			t.Fatalf("invalid color %q was accepted", invalid)
		}
	}
	for _, test := range []struct {
		color   string
		opacity int
		ass     string
	}{
		{color: "#123456", opacity: 100, ass: "&H00563412&"},
		{color: "#123456", opacity: 0, ass: "&HFF563412&"},
	} {
		ass, err := subtitleASSColor(test.color, test.opacity)
		if err != nil || ass != test.ass {
			t.Fatalf("ASS color=%q err=%v want %q", ass, err, test.ass)
		}
		color, opacity, err := subtitleASSColorRoundTrip(ass)
		if err != nil || color != "#123456" || opacity != test.opacity {
			t.Fatalf("round trip=%q,%d,%v", color, opacity, err)
		}
	}
	if _, err := subtitleASSColor("#FFFFFF", 101); err == nil {
		t.Fatal("out-of-range opacity was accepted")
	}
}

func TestSubtitlePaletteNormalizationIsBoundedAndDeterministic(t *testing.T) {
	raw := map[string]any{"savedColors": []any{"#abc", "#AABBCC", "#123456"}, "recentColors": []any{"#ffffff", "#FFFFFF", "#123456"}}
	settings, err := normalizeSubtitleStylingSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(settings.SavedColors, ",") != "#AABBCC,#123456" || strings.Join(settings.RecentColors, ",") != "#FFFFFF,#123456" {
		t.Fatalf("palette normalization=%#v", settings)
	}
	if _, err := normalizeSubtitleStylingSettings(map[string]any{"savedColors": []any{"#12FG00"}}); err == nil {
		t.Fatal("malformed saved color was silently accepted")
	}
	overflow := make([]any, maxSubtitleRecentColors+4)
	for index := range overflow {
		overflow[index] = "#" + strings.Repeat("0", 5) + "1"
	}
	settings, err = normalizeSubtitleStylingSettings(map[string]any{"recentColors": overflow})
	if err != nil || len(settings.RecentColors) != 1 || len(subtitlePaletteColors()) != 10 {
		t.Fatalf("bounded recent colors=%#v err=%v", settings.RecentColors, err)
	}
}

func TestSubtitleResolverKeepsRoleGeometryAndExplicitPositionSeparate(t *testing.T) {
	policy := testEnabledSubtitleStylePolicy()
	resolution := resolveSubtitleStyle(policy, subtitleStyleResolutionInput{Role: SubtitleRoleSigns, Geometry: &bitmapCueGeometry{CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 850, BitmapWidth: 720, BitmapHeight: 130}})
	if resolution.StyleID != SubtitleStyleSign || resolution.Alignment != 2 || resolution.Role != SubtitleRoleSigns {
		t.Fatalf("sign resolution=%#v", resolution)
	}
	unknown := resolveSubtitleStyle(policy, subtitleStyleResolutionInput{Role: SubtitleRoleUnknown, Geometry: &bitmapCueGeometry{CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 80, BitmapWidth: 720, BitmapHeight: 120}})
	if unknown.StyleID != SubtitleStyleDefault || unknown.Alignment != 8 {
		t.Fatalf("unknown geometry resolution=%#v", unknown)
	}
	for _, styleID := range []string{SubtitleStyleTop, SubtitleStyleLyrics} {
		resolved := resolveSubtitleStyle(policy, subtitleStyleResolutionInput{ExplicitStyleID: styleID})
		if resolved.StyleID != styleID {
			t.Fatalf("explicit %s resolution=%#v", styleID, resolved)
		}
	}
	explicit := resolveSubtitleStyle(policy, subtitleStyleResolutionInput{Role: SubtitleRoleSigns, ExplicitStyleID: SubtitleStyleLyrics, ExplicitAlignment: 9, HasExplicitPosition: true})
	if explicit.StyleID != SubtitleStyleLyrics || explicit.Alignment != 9 || !explicit.HasExactPosition {
		t.Fatalf("explicit resolution=%#v", explicit)
	}
}

func TestGeneratedASSUsesOneStyleAuthorityAndLeavesSRTNeutral(t *testing.T) {
	cues := []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "Top", Geometry: &bitmapCueGeometry{CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 500, BitmapY: 80, BitmapWidth: 720, BitmapHeight: 120}}}
	legacy := string(renderBitmapASSCues(cues))
	if !strings.Contains(legacy, "Style: Default") || strings.Contains(legacy, "MVF_MVF_Default_Generated") {
		t.Fatalf("legacy generated ASS changed: %q", legacy)
	}
	styled := string(renderBitmapASSCuesWithPolicy(cues, testEnabledSubtitleStylePolicy()))
	if !strings.Contains(styled, "MVF_MVF_Default_Generated") || !strings.Contains(styled, "Dialogue: 0,0:00:01.00,0:00:02.00,MVF_MVF_Default_Generated") || !strings.Contains(styled, "{\\an8}Top") {
		t.Fatalf("styled generated ASS=%q", styled)
	}
	srt := string(renderBitmapSRTCues(cues))
	if strings.Contains(srt, "\\an") || strings.Contains(srt, "MVF_") {
		t.Fatalf("SRT was styled: %q", srt)
	}
	styledSRT, err := styledASSFromCanonicalSRT([]byte("1\n00:00:01,000 --> 00:00:02,000\nDialogue\n"), testEnabledSubtitleStylePolicy())
	if err != nil || !strings.Contains(string(styledSRT), "Dialogue: 0,0:00:01.00,0:00:02.00,MVF_MVF_Default_Generated") {
		t.Fatalf("canonical SRT did not use generated ASS style authority: err=%v output=%q", err, styledSRT)
	}
}

func TestGenericASSBoundaryPreservesSourceAndRestylesOnlyBaseAssignment(t *testing.T) {
	source := `[Script Info]
Title: source

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: MVF_Default,Arial,20,&H00FF0000,&H00FF0000,&H00000000,&H00000000,0,0,1,2,0,2,10,10,10,1
Style: MVF_Top,Arial,20,&H00FFFF00,&H00FFFF00,&H00000000,&H00000000,0,0,1,2,0,8,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
	Dialogue: 2,0:00:01.00,0:00:03.00,MVF_Default,Actor,010,020,030,fx,{\\an8\\pos(100,200)\\move(100,200,120,220,0,100)\\k20}Hello\\Nworld
`
	document, err := parseASSDocument("ass", source)
	if err != nil || len(document.Events) != 1 {
		t.Fatalf("generic parse=%#v err=%v", document, err)
	}
	if got, err := restyleSourcePreservingASS([]byte(source), "ass", defaultSubtitleStylePolicy(), SubtitleRoleFull, "fixture"); err != nil || string(got) != source {
		t.Fatalf("default source preservation changed source: %v %q", err, got)
	}
	policy := testEnabledSubtitleStylePolicy()
	policy.Settings.OverrideExistingASSStyles = true
	restyled, err := restyleSourcePreservingASS([]byte(source), "ass", policy, SubtitleRoleFull, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	text := string(restyled)
	if !strings.Contains(text, "Style: MVF_Default,Arial,20,&H00FF0000") || !strings.Contains(text, "Dialogue: 2,0:00:01.00,0:00:03.00,MVF_MVF_Default_Generated") || !strings.Contains(text, `\\pos(100,200)\\move(100,200,120,220,0,100)\\k20}`) {
		t.Fatalf("native restyle corrupted protected structure: %q", text)
	}
	collisionSource := strings.Replace(source, "Style: MVF_Default,Arial", "Style: MVF_Default,Arial\nStyle: MVF_MVF_Default_Generated,Arial", 1)
	collision, err := restyleSourcePreservingASS([]byte(collisionSource), "ass", policy, SubtitleRoleFull, "fixture")
	if err != nil || strings.Contains(string(collision), "Dialogue: 2,0:00:01.00,0:00:03.00,MVF_MVF_Default_Generated,") {
		t.Fatalf("source generated-style collision was not namespaced deterministically: err=%v output=%q", err, collision)
	}
	if topName := generatedSubtitleStyleName(defaultSubtitleStylePresets()[SubtitleStyleTop], map[string]struct{}{"MVF_MVF_Top_Generated": {}}, "fixture"); topName == "MVF_MVF_Top_Generated" {
		t.Fatal("MVF_Top generated name did not avoid a source collision")
	}
}

func TestSubtitleSettingsUpdateIsAtomicAndCustomIdentityIsStable(t *testing.T) {
	value := models.JSONMap{"generateSRTFallbackForASS": true, "subtitleStyling": map[string]any{
		"enabled": true,
		"customPresets": []any{
			map[string]any{"id": "custom_one", "displayName": "Same", "primaryColor": "#abc"},
			map[string]any{"id": "custom_two", "displayName": "Same", "primaryColor": "#def"},
		},
	}}
	normalized, err := normalizeSubtitleSettingsForUpdate(value)
	if err != nil {
		t.Fatal(err)
	}
	custom, ok := normalized["subtitleStyling"].(map[string]any)["customPresets"].([]any)
	if !ok || len(custom) != 2 {
		t.Fatalf("custom presets=%#v", normalized)
	}
	if custom[0].(map[string]any)["id"] == custom[1].(map[string]any)["id"] {
		t.Fatal("custom display names replaced stable IDs")
	}
	if _, err := normalizeSubtitleSettingsForUpdate(models.JSONMap{"subtitleStyling": map[string]any{"enabled": true, "builtInOverrides": map[string]any{"MVF_Default": map[string]any{"primaryColor": "#12FG00"}}}}); err == nil {
		t.Fatal("invalid setting update was accepted")
	}
}

func TestSubtitleRestyleUsesExactSourceAndNonDestructiveSiblingPublication(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.ass")
	targetPath := filepath.Join(root, "source.styled.ass")
	workspace := filepath.Join(root, "operation")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	source := "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, TertiaryColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, AlphaLevel, Encoding\nStyle: Source,Arial,20,&H00FFFFFF,&H00FFFFFF,&H00000000,&H64000000,-1,0,1,2,0,2,10,10,10,0,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Source,,0,0,0,,Hello\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	revision, err := mediaFileFingerprint(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	operationID := "subtitle-test-restyle-sibling"
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{ID: operationID, Status: "running", publicationCommittedCh: make(chan struct{})}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()

	runSubtitleRestyleOperation(context.Background(), operationID, workspace, subtitleRestyleRequest{
		Mode: "restyle", SourcePath: sourcePath, SourceRevision: revision, TargetPath: targetPath, Representation: "ass", Policy: defaultSubtitleStylePolicy(),
	})
	gotSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	gotTarget, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotSource) != source || string(gotTarget) != source {
		t.Fatalf("default restyle changed source or sibling: source=%q target=%q", gotSource, gotTarget)
	}
	subtitleExtractionOperations.RLock()
	operation := subtitleExtractionOperations.items[operationID]
	subtitleExtractionOperations.RUnlock()
	if operation == nil || operation.Status != "completed" || !operation.publicationCommitted {
		t.Fatalf("unexpected restyle operation state: %#v", operation)
	}
}

func TestSubtitleRestyleRejectsStaleSourceAndCancellationBeforePublication(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.ssa")
	targetPath := filepath.Join(root, "source.styled.ssa")
	stagePath := filepath.Join(root, "stage.ssa")
	if err := os.WriteFile(sourcePath, []byte("[Script Info]\nScriptType: v4.00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	revision, err := mediaFileFingerprint(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("[Script Info]\nScriptType: v4.00\nTitle: changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagePath, []byte("[Script Info]\nScriptType: v4.00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "operation")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := publishSubtitleRestyleStage(context.Background(), "subtitle-test-stale", workspace, subtitleRestyleRequest{SourcePath: sourcePath, SourceRevision: revision, TargetPath: targetPath, Replace: false}, stagePath); err == nil {
		t.Fatal("stale source was published")
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("stale publication left target: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := publishSubtitleRestyleStage(canceled, "subtitle-test-canceled", workspace, subtitleRestyleRequest{SourcePath: sourcePath, SourceRevision: "", TargetPath: targetPath, Replace: false}, stagePath); err == nil {
		t.Fatal("canceled publication was accepted")
	}
}
