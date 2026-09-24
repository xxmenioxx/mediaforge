package handlers

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func compositionTestPolicy() SubtitleCompositionPolicy {
	style := defaultSubtitleStylePolicy()
	policy := defaultSubtitleCompositionPolicy(style)
	policy.StylePolicyVersion = style.Version
	return policy
}

func compositionTestSource(identity string, primary bool, role SubtitleSemanticRole, class CompositionCueClass) SubtitleCompositionSource {
	return SubtitleCompositionSource{Identity: identity, Revision: identity + "-r1", Representation: "ass", Language: "en", Role: role, CueClass: class, Primary: primary}
}

func compositionTestCue(source SubtitleCompositionSource, ordinal int, start, end float64, text string, style string) SubtitleCompositionCue {
	dialogue, err := parseTranslationProtectedDialogue(text)
	if err != nil {
		panic(err)
	}
	event := assEvent{SourceOrdinal: ordinal, Start: start, End: end, Style: style, Format: []string{"layer", "start", "end", "style", "text"}, Fields: []string{"0", "", "", style, text}, Dialogue: dialogue}
	return newSubtitleCompositionCue(event, source, source.CueClass)
}

func TestSubtitleCompositionReconcilesEvolvingCanonicalSetAcrossSupplementals(t *testing.T) {
	policy := compositionTestPolicy()
	primary := compositionTestSource("primary", true, SubtitleRoleFull, CompositionCueDialogue)
	supplementalA := compositionTestSource("supplemental-a", false, SubtitleRoleSigns, CompositionCueSign)
	supplementalB := compositionTestSource("supplemental-b", false, SubtitleRoleSigns, CompositionCueSign)
	primaryCue := compositionTestCue(primary, 0, 1, 2, "Dialogue", "Default")
	xA := compositionTestCue(supplementalA, 0, 3, 4, "Sign X", "Sign")
	xB := compositionTestCue(supplementalB, 0, 3, 4, "Sign X", "Sign")
	result, err := ReconcileSubtitleComposition(policy, []SubtitleCompositionCue{primaryCue}, []SubtitleCompositionCue{xA}, []SubtitleCompositionCue{xB})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 2 || result.Provenance.Counts[CompositionDecisionAdded] != 1 || result.Provenance.Counts[CompositionDecisionDeduplicated] != 1 {
		t.Fatalf("expected A to add and B to dedupe against evolving set: events=%d counts=%v decisions=%#v", len(result.Events), result.Provenance.Counts, result.Decisions)
	}
}

func TestSubtitleCompositionPrimaryPrecedenceAndEnrichment(t *testing.T) {
	policy := compositionTestPolicy()
	primary := compositionTestSource("primary", true, SubtitleRoleFull, CompositionCueDialogue)
	supplemental := compositionTestSource("supplemental", false, SubtitleRoleFull, CompositionCueDialogue)
	missingLayout := compositionTestCue(primary, 0, 1, 2, "Hello", "Default")
	meaningful := compositionTestCue(supplemental, 0, 1, 2, `{\pos(10,20)}Hello`, "Top")
	result, err := ReconcileSubtitleComposition(policy, []SubtitleCompositionCue{missingLayout}, []SubtitleCompositionCue{meaningful})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provenance.Counts[CompositionDecisionEnriched] != 1 || len(result.Events) != 1 || !result.Events[0].Layout.HasExplicitPosition {
		t.Fatalf("expected one enriched primary event: %#v", result)
	}

	meaningfulPrimary := compositionTestCue(primary, 0, 1, 2, `{\pos(1,2)}Hello`, "Top")
	duplicate, err := ReconcileSubtitleComposition(policy, []SubtitleCompositionCue{meaningfulPrimary}, []SubtitleCompositionCue{meaningful})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Provenance.Counts[CompositionDecisionKeptPrimary] != 1 || len(duplicate.Events) != 1 {
		t.Fatalf("primary meaningful layout must win: %#v", duplicate)
	}
}

func TestSubtitleCompositionPreservesSimultaneousDistinctCueEvidence(t *testing.T) {
	policy := compositionTestPolicy()
	primary := compositionTestSource("primary", true, SubtitleRoleFull, CompositionCueDialogue)
	sign := compositionTestSource("sign", false, SubtitleRoleSigns, CompositionCueSign)
	first := compositionTestCue(primary, 0, 1, 2, "Same", "Default")
	second := compositionTestCue(sign, 0, 1, 2, `{\an8}Same`, "Sign")
	result, err := ReconcileSubtitleComposition(policy, []SubtitleCompositionCue{first}, []SubtitleCompositionCue{second})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 2 || result.Provenance.Counts[CompositionDecisionAdded] != 1 {
		t.Fatalf("simultaneous role/layout-distinct events were merged: %#v", result)
	}
}

func TestSubtitleCompositionRepeatedTextAtDifferentTimesAndAmbiguousMatches(t *testing.T) {
	policy := compositionTestPolicy()
	primary := compositionTestSource("primary", true, SubtitleRoleFull, CompositionCueDialogue)
	supplemental := compositionTestSource("supplemental", false, SubtitleRoleFull, CompositionCueDialogue)
	first := compositionTestCue(primary, 0, 1, 2, "Repeat", "Default")
	second := compositionTestCue(primary, 1, 5, 6, "Repeat", "Default")
	late := compositionTestCue(supplemental, 0, 5, 6, "Repeat", "Default")
	result, err := ReconcileSubtitleComposition(policy, []SubtitleCompositionCue{first, second}, []SubtitleCompositionCue{late})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 2 || result.Provenance.Counts[CompositionDecisionDeduplicated] != 1 {
		t.Fatalf("repeated text at different times was collapsed: %#v", result)
	}

	nearFirst := compositionTestCue(primary, 2, 1.1, 2.1, "Repeat", "Default")
	ambiguous, err := ReconcileSubtitleComposition(policy, []SubtitleCompositionCue{first, nearFirst}, []SubtitleCompositionCue{compositionTestCue(supplemental, 1, 1.05, 2.05, "Repeat", "Default")})
	if err != nil {
		t.Fatal(err)
	}
	if !ambiguous.Conflict || ambiguous.Provenance.Counts[CompositionDecisionAmbiguousConflict] != 1 {
		t.Fatalf("ambiguous timing/text match was not surfaced: %#v", ambiguous)
	}
}

func TestSubtitleCompositionLanguageAndPreferredGate(t *testing.T) {
	policy := compositionTestPolicy()
	primary := compositionTestSource("primary", true, SubtitleRoleFull, CompositionCueDialogue)
	supplemental := compositionTestSource("supplemental", false, SubtitleRoleSigns, CompositionCueSign)
	policy.PromotePreferred = true
	if preferred, _ := subtitleCompositionPromotion(policy, []SubtitleCompositionSource{supplemental}); preferred {
		t.Fatal("supplemental-only composition passed preferred gate")
	}
	if preferred, _ := subtitleCompositionPromotion(policy, []SubtitleCompositionSource{primary, supplemental}); !preferred {
		t.Fatal("configured full-dialogue primary did not pass explicit promotion gate")
	}
	unknown := supplemental
	unknown.Identity = "unknown"
	unknown.Language = "und"
	if _, err := ReconcileSubtitleComposition(policy, []SubtitleCompositionCue{compositionTestCue(primary, 0, 1, 2, "A", "Default")}, []SubtitleCompositionCue{compositionTestCue(unknown, 0, 1, 2, "B", "Sign")}); err == nil {
		t.Fatal("unknown language was auto-composed")
	}
	cross := supplemental
	cross.Identity = "cross"
	cross.Language = "es"
	if _, err := ReconcileSubtitleComposition(policy, []SubtitleCompositionCue{compositionTestCue(primary, 0, 1, 2, "A", "Default")}, []SubtitleCompositionCue{compositionTestCue(cross, 0, 1, 2, "B", "Sign")}); err == nil {
		t.Fatal("cross-language composition was auto-authorized")
	}
	policy.AllowCrossLanguage = true
	cross.ExplicitLanguage = true
	if _, err := ReconcileSubtitleComposition(policy, []SubtitleCompositionCue{compositionTestCue(primary, 0, 1, 2, "A", "Default")}, []SubtitleCompositionCue{compositionTestCue(cross, 0, 1, 2, "B", "Sign")}); err != nil {
		t.Fatalf("explicit cross-language policy rejected: %v", err)
	}
}

func TestSubtitleCompositionLyricsUsesExistingStyleResolverBoundary(t *testing.T) {
	policy := compositionTestPolicy()
	source := compositionTestSource("lyrics", true, SubtitleRoleFull, CompositionCueLyrics)
	cue := compositionTestCue(source, 0, 1, 2, "Song", "")
	resolution := resolveCompositionCueStyle(policy, cue)
	if resolution.StyleID != SubtitleStyleLyrics {
		t.Fatalf("lyrics did not pass ExplicitStyleID=MVF_Lyrics to existing resolver: %#v", resolution)
	}
	if normalizeSubtitleSemanticRole(string(resolution.Role)) == "lyrics" {
		t.Fatal("lyrics became a global subtitle semantic role")
	}
}

func TestSubtitleCompositionSRTNormalizationAndCanonicalSerialization(t *testing.T) {
	document, err := parseCompositionSRT("1\n00:00:01,000 --> 00:00:02,500\nHello\nworld\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Events) != 1 || document.Events[0].Start != 1 || document.Events[0].End != 2.5 || len(document.Events[0].Dialogue.Parts) != 3 || !strings.Contains(document.Events[0].Dialogue.Parts[1].Text, `\N`) {
		t.Fatalf("unexpected SRT normalization: %#v", document)
	}
	encoded, err := serializeASSDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseASSDocument("ass", string(encoded))
	if err != nil || len(parsed.Events) != 1 || parsed.Events[0].Start != 1 {
		t.Fatalf("canonical ASS did not reparse: err=%v output=%s", err, encoded)
	}
}

func TestSubtitleCompositionIdentityIgnoresOperationTimeAndAttachmentsAreDeterministic(t *testing.T) {
	policy := compositionTestPolicy()
	sources := []SubtitleCompositionSource{compositionTestSource("primary", true, SubtitleRoleFull, CompositionCueDialogue), compositionTestSource("supp", false, SubtitleRoleSigns, CompositionCueSign)}
	first, err := SubtitleCompositionOutputIdentity(sources, policy, policy.StylePolicyVersion, "ass", "preferred")
	if err != nil {
		t.Fatal(err)
	}
	second, err := SubtitleCompositionOutputIdentity(sources, policy, policy.StylePolicyVersion, "ass", "preferred")
	if err != nil || first != second {
		t.Fatalf("identity was not deterministic: %q %q %v", first, second, err)
	}
	attachments := []SubtitleCompositionAttachment{{SourceIdentity: "a", OriginalName: "font.ttf", Content: []byte("font")}, {SourceIdentity: "b", OriginalName: "font.ttf", Content: []byte("font")}, {SourceIdentity: "c", OriginalName: "font.ttf", Content: []byte("other")}}
	decisions, err := reconcileSubtitleCompositionAttachments(attachments)
	if err != nil || len(decisions) != 3 || decisions[1].Action != "deduplicated" || decisions[2].SafeName == decisions[0].SafeName {
		t.Fatalf("attachment reconciliation was not deterministic: %#v %v", decisions, err)
	}
	if !bytes.Equal(attachments[0].Content, []byte("font")) {
		t.Fatal("attachment reconciliation mutated source content")
	}
}

func TestComposeSubtitleDocumentsPreservesNativeStylesAndNamespacesCollisions(t *testing.T) {
	primarySource := compositionTestSource("primary", true, SubtitleRoleFull, CompositionCueDialogue)
	supplementalSource := compositionTestSource("supp", false, SubtitleRoleSigns, CompositionCueSign)
	primaryASS := `[Script Info]
[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: MVF_Default,Arial,20,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,1,2,1,2,10,10,20,1
[Events]
Format: Layer, Start, End, Style, Text
Dialogue: 0,0:00:01.00,0:00:02.00,MVF_Default,{\pos(10,20)}Hello
`
	supplementalASS := `[Script Info]
[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: MVF_Default,Arial,24,&H0000FFFF,&H0000FFFF,&H00000000,&H00000000,1,0,1,3,1,8,10,10,20,1
[Events]
Format: Layer, Start, End, Style, Text
Dialogue: 0,0:00:01.00,0:00:02.00,MVF_Default,{\an8}Sign
`
	primary, err := parseASSDocument("ass", primaryASS)
	if err != nil {
		t.Fatal(err)
	}
	supplemental, err := parseASSDocument("ass", supplementalASS)
	if err != nil {
		t.Fatal(err)
	}
	document, result, encoded, err := ComposeSubtitleDocuments(compositionTestPolicy(), SubtitleCompositionDocument{Source: primarySource, Document: primary}, SubtitleCompositionDocument{Source: supplementalSource, Document: supplemental})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 2 || len(document.Events) != 2 {
		t.Fatalf("simultaneous native events were not preserved: %#v", result)
	}
	if !strings.Contains(string(encoded), "MVF_Default__src_") || !strings.Contains(string(encoded), `\pos(10,20)`) || !strings.Contains(string(encoded), `\an8`) {
		t.Fatalf("native style/override evidence was lost or collided: %s", encoded)
	}
}

func TestResolveTrackPlanFreezesDeterministicCompositionPolicy(t *testing.T) {
	scan := models.ScanResult{Path: "/media/episode.mkv", SizeBytes: 42, SubtitleStreams: models.JSONList{
		map[string]any{"index": 4, "codec": "ass", "language": "eng", "role": "full", "title": "Full"},
		map[string]any{"index": 7, "codec": "ass", "language": "eng", "role": "signs", "title": "Signs"},
	}}
	profile := map[string]any{
		"subtitleComposition":                    map[string]any{"timingToleranceMillis": 500, "promotePreferred": true, "maxSupplementalSources": 1},
		"subtitleCompositionPrimaryStream":       4,
		"subtitleCompositionSupplementalStreams": []any{7},
	}
	first, err := resolveTrackPlan(scan, profile)
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolveTrackPlan(scan, profile)
	if err != nil {
		t.Fatal(err)
	}
	if first.Composition == nil || !first.Composition.Enabled || !first.Composition.PreferredEligible || len(first.Composition.Supplementals) != 1 {
		t.Fatalf("composition policy was not resolved through Track Profile: %#v", first.Composition)
	}
	firstJSON, _ := json.Marshal(first.Composition)
	secondJSON, _ := json.Marshal(second.Composition)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("equivalent Track Profile inputs were not deterministic: %s vs %s", firstJSON, secondJSON)
	}
	legacy, err := resolveTrackPlan(scan, map[string]any{})
	if err != nil || legacy.Composition != nil {
		t.Fatalf("legacy profile unexpectedly gained composition state: %#v %v", legacy.Composition, err)
	}
}

func TestSubtitleCompositionEligibilityIsBackendAuthoritativeAndBounded(t *testing.T) {
	external := []ExternalSubtitle{{Path: "/raw/Episode.en.ass", Language: "eng", Format: "ass", SourceRevision: "r1", Eligibility: &SubtitleSourceEligibility{SourceIdentity: "external:/raw/Episode.en.ass", SourceRevision: "r1", Representation: "ass", Language: "eng", Role: SubtitleRoleFull, CompositionClass: CompositionCueDialogue}}}
	validStream := 4
	embedded := []SubtitleSourceEligibility{{SourceIdentity: "embedded:/raw/Episode.mkv#stream=4", SourceRevision: "media-r1", Representation: "ass", Language: "eng", Role: SubtitleRoleSigns, CompositionClass: CompositionCueSign, StreamIndex: &validStream, Regenerate: SubtitleModeEligibility{Eligible: true}}}
	eligibility := subtitleCompositionEligibility(external, embedded, defaultSubtitleStylePolicy())
	if !eligibility.Eligible || len(eligibility.PrimaryCandidates) != 1 || len(eligibility.SupplementalCandidates) != 2 {
		t.Fatalf("backend did not expose eligible primary/supplemental sources: %#v", eligibility)
	}
	unknown := external[0]
	unknown.Language = ""
	unknown.Eligibility.Language = ""
	unknownEligibility := subtitleCompositionEligibility([]ExternalSubtitle{unknown}, nil, defaultSubtitleStylePolicy())
	if unknownEligibility.Eligible || len(unknownEligibility.PrimaryCandidates) != 0 {
		t.Fatalf("unknown language was exposed as an eligible primary: %#v", unknownEligibility)
	}
}

func TestSubtitleCompositionUsesCanonicalSRTAndBitmapOCRCueBoundaries(t *testing.T) {
	policy := compositionTestPolicy()
	primarySource := compositionTestSource("srt-primary", true, SubtitleRoleFull, CompositionCueDialogue)
	primarySource.Representation = "srt"
	supplementalSource := compositionTestSource("ass-supplemental", false, SubtitleRoleSigns, CompositionCueSign)
	srt, err := parseCompositionSRT("1\n00:00:01,000 --> 00:00:02,000\nDialogue\n")
	if err != nil {
		t.Fatal(err)
	}
	ass, err := parseASSDocument("ass", "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,,{\\an8}Sign\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ComposeSubtitleDocuments(policy, SubtitleCompositionDocument{Source: primarySource, Document: srt}, SubtitleCompositionDocument{Source: supplementalSource, Document: ass}); err != nil {
		t.Fatalf("SRT + ASS composition failed: %v", err)
	}
	bitmapSource := compositionTestSource("bitmap", false, SubtitleRoleSigns, CompositionCueSign)
	bitmapSource.Representation = "bitmap"
	bitmapCues := subtitleCompositionCuesFromBitmap([]canonicalBitmapSubtitleCue{{SourceOrdinal: 0, Start: 1, End: 2, Text: "OCR sign", Geometry: &bitmapCueGeometry{CanvasWidth: 720, CanvasHeight: 480, BitmapX: 40, BitmapY: 30, BitmapWidth: 640, BitmapHeight: 80}}}, bitmapSource)
	if len(bitmapCues) != 1 || !bitmapCues[0].Layout.HasBitmapGeometry {
		t.Fatalf("bitmap composition did not consume canonical OCR geometry: %#v", bitmapCues)
	}
}

func TestSubtitleCompositionPolicyBoundsFailClosed(t *testing.T) {
	policy := compositionTestPolicy()
	policy.MaxSupplementalSources = compositionMaxSupplementals + 1
	if err := validateSubtitleCompositionPolicy(policy); err == nil {
		t.Fatal("composition policy accepted an unbounded supplemental count")
	}
	policy = compositionTestPolicy()
	policy.TimingToleranceMillis = compositionMaxToleranceMS + 1
	if err := validateSubtitleCompositionPolicy(policy); err == nil {
		t.Fatal("composition policy accepted an unsafe timing tolerance")
	}
}
