package handlers

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
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

func TestSubtitleEligibilityIsBackendAuthoritativeAndBounded(t *testing.T) {
	policy := defaultSubtitleStylePolicy()
	ass := subtitleExternalEligibility(ExternalSubtitle{Path: "/library/episode.eng.ass", Format: "ass", SourceRevision: "ass-revision"}, policy)
	if !ass.Restyle.Eligible || !ass.ReplaceEligible || ass.NativeOverrideEligible || ass.SourceIdentity != "external:/library/episode.eng.ass" {
		t.Fatalf("native ASS eligibility=%#v", ass)
	}
	srt := subtitleExternalEligibility(ExternalSubtitle{Path: "/library/episode.eng.srt", Format: "srt", SourceRevision: "srt-revision"}, policy)
	if !srt.SRTToASS.Eligible || srt.ReplaceEligible || srt.Restyle.Eligible {
		t.Fatalf("SRT eligibility=%#v", srt)
	}
	stream := FFProbeStream{Index: 4, CodecType: "subtitle", CodecName: "hdmv_pgs_subtitle"}
	embedded := subtitleEmbeddedEligibility("/library/episode.mkv", stream, "media-revision", policy)
	if !embedded.Regenerate.Eligible || embedded.StreamIndex == nil || *embedded.StreamIndex != 4 || embedded.SourceIdentity != "embedded:/library/episode.mkv#stream=4" {
		t.Fatalf("bitmap eligibility=%#v", embedded)
	}
	unknown := subtitleEmbeddedEligibility("/library/episode.mkv", FFProbeStream{Index: 9, CodecType: "subtitle", CodecName: "dvb_subtitle"}, "media-revision", policy)
	if unknown.Regenerate.Eligible || unknown.Regenerate.Reason == "" {
		t.Fatalf("unsupported eligibility=%#v", unknown)
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

func TestNativeASSFullPreservationFixtureAndMVFCollision(t *testing.T) {
	source := `[Script Info]
Title: full preservation fixture
ScriptType: v4.00+
X-Unknown: retain-me

; source comment line
[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: MVF_Default,Arial,20,&H00112233,&H00445566,&H00778899,&H00AABBCC,0,0,1,2,1,2,11,12,13,1
Style: MVF_Top,Arial,24,&H00ABCDEF,&H00123456,&H00654321,&H00010203,-1,0,1,3,2,8,14,15,16,1
Style: Native,Arial,18,&H00FEDCBA,&H00000000,&H00101010,&H00202020,0,0,1,1,0,2,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Comment: 7,0:00:00.00,0:00:01.00,Native,,1,2,3,comment,{\an7}comment line
Dialogue: 3,0:00:01.00,0:00:03.00,MVF_Default,Actor,011,022,033,fx,{\an8\pos(100,200)\move(100,200,120,220,0,100)\k20}Hello\Nworld
Dialogue: 1,0:00:03.00,0:00:04.00,MVF_Top,,004,005,006,,{\p1}m 0 0 l 100 0 100 100 0 100
`
	policy := testEnabledSubtitleStylePolicy()
	defaultOutput, err := restyleSourcePreservingASS([]byte(source), "ass", policy, SubtitleRoleFull, "full-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if string(defaultOutput) != source {
		t.Fatalf("default native processing changed protected fixture:\n%s", defaultOutput)
	}

	policy.Settings.OverrideExistingASSStyles = true
	override, err := restyleSourcePreservingASS([]byte(source), "ass", policy, SubtitleRoleFull, "full-fixture")
	if err != nil {
		t.Fatal(err)
	}
	text := string(override)
	for _, protected := range []string{
		"X-Unknown: retain-me", "; source comment line", "Style: MVF_Default,Arial,20,&H00112233",
		"Style: MVF_Top,Arial,24,&H00ABCDEF", "Comment: 7,0:00:00.00,0:00:01.00,Native,,1,2,3,comment",
		`{\an8\pos(100,200)\move(100,200,120,220,0,100)\k20}Hello\Nworld`,
		`{\p1}m 0 0 l 100 0 100 100 0 100`,
	} {
		if !strings.Contains(text, protected) {
			t.Fatalf("override removed protected native structure %q from %q", protected, text)
		}
	}
	if strings.Contains(text, "Dialogue: 3,0:00:01.00,0:00:03.00,MVF_Default,") || strings.Contains(text, "Dialogue: 1,0:00:03.00,0:00:04.00,MVF_Top,") {
		t.Fatal("explicit override did not change eligible base event style assignments")
	}
	if !strings.Contains(text, "Style: MVF_MVF_Default_Generated") {
		t.Fatalf("override did not append collision-safe generated styles: %s", text)
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

func TestSubtitleSRTToASSUsesStyledSiblingWithoutChangingSRT(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.srt")
	targetPath := filepath.Join(root, "source.styled.ass")
	source := "1\n00:00:01,000 --> 00:00:02,000\nDialogue\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	revision, err := mediaFileFingerprint(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	policy := testEnabledSubtitleStylePolicy()
	workspace := filepath.Join(root, "operation")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	operationID := "subtitle-test-srt-to-ass"
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{ID: operationID, Kind: "restyle", Status: "running", publicationCommittedCh: make(chan struct{})}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()
	runSubtitleRestyleOperation(context.Background(), operationID, workspace, subtitleRestyleRequest{Mode: "srt_to_ass", SourcePath: sourcePath, SourceRevision: revision, TargetPath: targetPath, Representation: "srt", Policy: policy})
	gotSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	gotASS, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotSource) != source || !strings.Contains(string(gotASS), "MVF_") || strings.Contains(string(gotASS), "\\an") == false {
		t.Fatalf("unexpected SRT sibling output: source=%q ASS=%q", gotSource, gotASS)
	}
}

func TestSubtitleRestyleFailureCleansStagingAndReplaceRequiresExplicitOptIn(t *testing.T) {
	root := t.TempDir()
	missingSource := filepath.Join(root, "missing.ass")
	missingWorkspace := filepath.Join(root, "failed-workspace")
	missingID := "subtitle-test-restyle-failure-cleanup"
	missingRequest := subtitleRestyleRequest{Mode: "restyle", SourcePath: missingSource, SourceRevision: "missing-revision", TargetPath: filepath.Join(root, "missing.styled.ass"), Representation: "ass", Policy: defaultSubtitleStylePolicy()}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[missingID] = &subtitleExtractionOperation{ID: missingID, Kind: "restyle", Status: "running", publicationCommittedCh: make(chan struct{})}
	subtitleExtractionOperations.Unlock()
	if err := os.MkdirAll(missingWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	runSubtitleRestyleOperation(context.Background(), missingID, missingWorkspace, missingRequest)
	subtitleExtractionOperations.RLock()
	failed := cloneSubtitleExtractionOperationSnapshot(subtitleExtractionOperations.items[missingID])
	subtitleExtractionOperations.RUnlock()
	if failed.Status != "error" || !failed.Retryable {
		t.Fatalf("failed restyle was not retryable: %#v", failed)
	}
	if _, err := os.Stat(missingWorkspace); !os.IsNotExist(err) {
		t.Fatalf("failed restyle left staging workspace: %v", err)
	}
	if _, err := os.Stat(missingRequest.TargetPath); !os.IsNotExist(err) {
		t.Fatalf("failed restyle published output: %v", err)
	}

	sourcePath := filepath.Join(root, "replace.ass")
	source := "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Native,Arial,20,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,1,2,0,2,10,10,10,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Native,,0,0,0,,Replace me\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	revision, err := mediaFileFingerprint(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	policy := testEnabledSubtitleStylePolicy()
	policy.Settings.OverrideExistingASSStyles = true
	policy.Version = subtitleStylePolicyHash(policy.Settings)
	replaceID := "subtitle-test-restyle-replace-opt-in"
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[replaceID] = &subtitleExtractionOperation{ID: replaceID, Kind: "restyle", Status: "running", publicationCommittedCh: make(chan struct{})}
	subtitleExtractionOperations.Unlock()
	workspace := filepath.Join(root, "replace-workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	runSubtitleRestyleOperation(context.Background(), replaceID, workspace, subtitleRestyleRequest{Mode: "restyle", SourcePath: sourcePath, SourceRevision: revision, TargetPath: sourcePath, Representation: "ass", Replace: true, Policy: policy})
	got, err := os.ReadFile(sourcePath)
	if err != nil || !strings.Contains(string(got), "MVF_") || string(got) == source {
		t.Fatalf("explicit replacement did not publish the authorized restyle: err=%v content=%q", err, got)
	}
	subtitleExtractionOperations.Lock()
	delete(subtitleExtractionOperations.items, missingID)
	delete(subtitleExtractionOperations.items, replaceID)
	subtitleExtractionOperations.Unlock()
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

func TestSubtitleRestyleCancellationSignalsBeforePublicationMutex(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.ass")
	targetPath := filepath.Join(root, "source.styled.ass")
	source := "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Arial,20,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,1,2,0,2,10,10,10,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hello\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	revision, err := mediaFileFingerprint(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspace")
	operationID := "subtitle-test-restyle-cancel-barrier"
	workCtx, cancel := context.WithCancel(context.Background())
	operation := &subtitleExtractionOperation{ID: operationID, Kind: "restyle", Status: "running", publicationCommittedCh: make(chan struct{}), cancel: cancel}
	request := subtitleRestyleRequest{Mode: "restyle", SourcePath: sourcePath, SourceRevision: revision, TargetPath: targetPath, Representation: "ass", Policy: defaultSubtitleStylePolicy()}
	operation.restyleRequest = &request
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()

	publicationEntered := make(chan struct{})
	subtitlePublicationBeforeLockHook = func() { close(publicationEntered) }
	defer func() { subtitlePublicationBeforeLockHook = nil }()
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	stagePath := filepath.Join(workspace, "subtitle-output.ass")
	if err := os.WriteFile(stagePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	subtitlePublicationMu.Lock()
	publishDone := make(chan struct{})
	go func() {
		err := publishSubtitleRestyleStage(workCtx, operationID, workspace, request, stagePath)
		if err != nil {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "cancelled"
				operation.Phase = "cancelled"
				operation.Message = "Subtitle restyle cancelled"
			})
		}
		close(publishDone)
	}()
	<-publicationEntered

	cancelDone := make(chan struct{})
	go func() {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Params = gin.Params{{Key: "id", Value: operationID}}
		(&AssetHandler{}).CancelSubtitleExtractionOperation(context)
		close(cancelDone)
	}()
	<-workCtx.Done()
	subtitlePublicationMu.Unlock()
	<-publishDone
	<-cancelDone

	subtitleExtractionOperations.RLock()
	snapshot := cloneSubtitleExtractionOperationSnapshot(subtitleExtractionOperations.items[operationID])
	subtitleExtractionOperations.RUnlock()
	if snapshot.Status != "cancelled" {
		t.Fatalf("restyle cancellation did not win publication barrier: %#v", snapshot)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("canceled restyle published target: %v", err)
	}
	cleanupSubtitleOperationWorkspace(workspace)
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("canceled restyle left workspace: %v", err)
	}
}

func TestSubtitleRestyleCommittedPublicationIsNotCanceledOrRolledBack(t *testing.T) {
	operationID := "subtitle-test-restyle-committed"
	cancelCalled := false
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{
		ID: operationID, Kind: "restyle", Status: "running", publicationCommitted: true,
		cancel: func() { cancelCalled = true }, publicationCommittedCh: make(chan struct{}),
	}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: operationID}}
	(&AssetHandler{}).CancelSubtitleExtractionOperation(context)
	if cancelCalled {
		t.Fatal("cancel request rolled back or canceled a committed publication")
	}
	subtitleExtractionOperations.RLock()
	status := subtitleExtractionOperations.items[operationID].Status
	subtitleExtractionOperations.RUnlock()
	if status != "running" {
		t.Fatalf("committed publication changed terminal state: %q", status)
	}
}

func TestSubtitleRestyleFreezesPolicySnapshotForRunningWork(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.srt")
	if err := os.WriteFile(sourcePath, []byte("1\n00:00:01,000 --> 00:00:02,000\nFrozen\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	revision, err := mediaFileFingerprint(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	policyA := testEnabledSubtitleStylePolicy()
	defaultPreset := policyA.Settings.BuiltInOverrides[SubtitleStyleDefault]
	defaultPreset.PrimaryColor = "#FF0000"
	defaultPreset.Opacity = 100
	policyA.Settings.BuiltInOverrides[SubtitleStyleDefault] = defaultPreset
	policyA.Version = subtitleStylePolicyHash(policyA.Settings)
	policyB := cloneSubtitleStylePolicy(policyA)
	defaultPreset.PrimaryColor = "#0000FF"
	policyB.Settings.BuiltInOverrides[SubtitleStyleDefault] = defaultPreset
	policyB.Version = subtitleStylePolicyHash(policyB.Settings)
	request := subtitleRestyleRequest{Mode: "srt_to_ass", SourcePath: sourcePath, SourceRevision: revision, TargetPath: filepath.Join(root, "source.styled.ass"), Representation: "srt", Policy: policyA}
	operationID, err := startSubtitleRestyleOperation(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Policy = policyB
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()
	completed := waitForSubtitleOperationStatus(t, operationID, "completed")
	if completed.StylePolicyVersion != policyA.Version {
		t.Fatalf("operation version=%q want frozen policy A=%q", completed.StylePolicyVersion, policyA.Version)
	}
	output, err := os.ReadFile(request.TargetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "&H000000FF&") || strings.Contains(string(output), "&H00FF0000&") {
		t.Fatalf("frozen policy output did not use policy A red: %q", output)
	}
	if policyB.Version == policyA.Version {
		t.Fatal("policy mutation did not produce a new version")
	}
}

func TestSubtitleQueueUsesFrozenStylePolicySnapshotAndVersion(t *testing.T) {
	policyA := testEnabledSubtitleStylePolicy()
	defaultPreset := defaultSubtitleStylePresets()[SubtitleStyleDefault]
	defaultPreset.PrimaryColor = "#FF0000"
	policyA.Settings.BuiltInOverrides[SubtitleStyleDefault] = defaultPreset
	policyA.Version = subtitleStylePolicyHash(policyA.Settings)
	job := models.QueueJob{TrackProfileSnapshot: models.JSONMap{subtitleStylingPolicyKey: subtitleStylingSettingsMap(policyA.Settings)}}
	resolved := subtitleStylePolicyForQueueJob(job)
	if resolved.Version != policyA.Version || resolved.Settings.BuiltInOverrides[SubtitleStyleDefault].PrimaryColor != "#FF0000" {
		t.Fatalf("queue did not use frozen style policy: got=%#v want=%#v", resolved, policyA)
	}
	policyB := cloneSubtitleStylePolicy(policyA)
	defaultPreset = policyB.Settings.BuiltInOverrides[SubtitleStyleDefault]
	defaultPreset.PrimaryColor = "#0000FF"
	policyB.Settings.BuiltInOverrides[SubtitleStyleDefault] = defaultPreset
	policyB.Version = subtitleStylePolicyHash(policyB.Settings)
	if resolved.Version == policyB.Version || resolved.Settings.BuiltInOverrides[SubtitleStyleDefault].PrimaryColor == "#0000FF" {
		t.Fatal("mutating future Settings policy changed the Queue snapshot")
	}
}
