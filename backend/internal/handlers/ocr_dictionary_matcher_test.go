package handlers

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func testOCRDictionaryEntry(id, pattern, replacement string, priority int) models.OCRDictionaryEntry {
	return models.OCRDictionaryEntry{
		ID: id, Language: "eng", Pattern: pattern, Replacement: replacement,
		MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact",
		Priority: priority, Enabled: true, Source: "user", RuleVersion: 1,
	}
}

func TestOCRDictionaryMatcherUsesLiteralWordBoundariesAndLanguage(t *testing.T) {
	rules := []models.OCRDictionaryEntry{
		testOCRDictionaryEntry("im", "|'m", "I'm", 100),
		testOCRDictionaryEntry("ive", "|'ve", "I've", 100),
		testOCRDictionaryEntry("ill", "|'ll", "I'll", 100),
		testOCRDictionaryEntry("id", "|'d", "I'd", 100),
		testOCRDictionaryEntry("cant", "| can't", "I can't", 100),
		testOCRDictionaryEntry("dont", "| don't", "I don't", 100),
		testOCRDictionaryEntry("am_i", "am |", "am I", 100),
	}
	cases := []struct {
		name     string
		language string
		input    string
		want     string
		matches  int
	}{
		{name: "contraction im", language: "eng", input: "Well, |'m here!", want: "Well, I'm here!", matches: 1},
		{name: "contraction ive", language: "en", input: "|'ve seen it", want: "I've seen it", matches: 1},
		{name: "contraction ill", language: "english", input: "|'ll go", want: "I'll go", matches: 1},
		{name: "contraction id", language: "eng", input: "|'d wait", want: "I'd wait", matches: 1},
		{name: "phrase and punctuation", language: "eng", input: "No, | can't!", want: "No, I can't!", matches: 1},
		{name: "dont", language: "eng", input: "They | don't.", want: "They I don't.", matches: 1},
		{name: "guarded am i", language: "eng", input: "am |?", want: "am I?", matches: 1},
		{name: "proper name token", language: "eng", input: "Spam|'m ready", want: "Spam|'m ready", matches: 0},
		{name: "embedded contraction", language: "eng", input: "x|'m ready", want: "x|'m ready", matches: 0},
		{name: "contraction suffix", language: "eng", input: "|'milk", want: "|'milk", matches: 0},
		{name: "guarded am suffix", language: "eng", input: "am |answer", want: "am |answer", matches: 0},
		{name: "unknown language", language: "", input: "|'m", want: "|'m", matches: 0},
		{name: "spanish language", language: "spa", input: "|'m", want: "|'m", matches: 0},
		{name: "markup remains intact", language: "eng", input: "{\\i1}|'m{\\i0}", want: "{\\i1}I'm{\\i0}", matches: 1},
		{name: "bar in override block", language: "eng", input: "{\\custom |'m}text", want: "{\\custom |'m}text", matches: 0},
		{name: "html tags remain intact", language: "eng", input: "<i>|'m</i>", want: "<i>I'm</i>", matches: 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := matchOCRDictionaryText(test.input, test.language, rules)
			if result.Text != test.want {
				t.Fatalf("got %q, want %q", result.Text, test.want)
			}
			if len(result.Matches) != test.matches {
				t.Fatalf("got %d matches, want %d", len(result.Matches), test.matches)
			}
			if len(result.Ambiguous) != 0 {
				t.Fatalf("unexpected ambiguity: %#v", result.Ambiguous)
			}
		})
	}
}

func TestOCRDictionaryMatcherResolvesPriorityAndFailsClosedOnEqualPriority(t *testing.T) {
	phrase := testOCRDictionaryEntry("phrase", "| can't", "I can't", 10)
	word := testOCRDictionaryEntry("word", "can't", "cannot", 20)
	result := matchOCRDictionaryText("| can't!", "eng", []models.OCRDictionaryEntry{phrase, word})
	if result.Text != "| cannot!" || len(result.Matches) != 1 {
		t.Fatalf("higher priority overlap was not selected: %#v", result)
	}

	conflicting := testOCRDictionaryEntry("conflicting", "| can't", "we can't", 10)
	result = matchOCRDictionaryText("| can't!", "eng", []models.OCRDictionaryEntry{phrase, conflicting})
	if result.Text != "| can't!" || len(result.Matches) != 0 || len(result.Ambiguous) != 1 {
		t.Fatalf("equal-priority conflict did not fail closed: %#v", result)
	}
}

func TestOCRDictionaryMatcherHonorsContextAndCasePolicy(t *testing.T) {
	rule := testOCRDictionaryEntry("context", "|cant", "I cant", 1)
	rule.LeftContext = "say "
	rule.RightContext = "!"
	if got, count := applyOCRDictionaryRules("say |cant!", "eng", []models.OCRDictionaryEntry{rule}); got != "say I cant!" || count != 1 {
		t.Fatalf("literal context was not applied: got %q with %d matches", got, count)
	}
	if got, count := applyOCRDictionaryRules("she |cant?", "eng", []models.OCRDictionaryEntry{rule}); got != "she |cant?" || count != 0 {
		t.Fatalf("non-matching context changed text: got %q with %d matches", got, count)
	}

	rule = testOCRDictionaryEntry("case", "|'m", "I'm", 1)
	if got, count := applyOCRDictionaryRules("|'M", "eng", []models.OCRDictionaryEntry{rule}); got != "|'M" || count != 0 {
		t.Fatalf("exact case policy unexpectedly matched: got %q with %d matches", got, count)
	}
	rule.CasePolicy = "insensitive"
	if got, count := applyOCRDictionaryRules("|'M", "eng", []models.OCRDictionaryEntry{rule}); got != "I'm" || count != 1 {
		t.Fatalf("insensitive case policy did not emit the literal replacement: got %q with %d matches", got, count)
	}
}

func TestDeterministicOCRRepairDoesNotReplaceAnUnmatchedBar(t *testing.T) {
	if _, ok := ocrDictionaryCandidate("|", "eng"); ok {
		t.Fatal("legacy token repair proposed a global bar replacement")
	}
	if _, ok := deterministicOCRSuggestion("|'m", "eng"); ok {
		t.Fatal("dictionary suggestion used an implicit English fallback without strict language evidence")
	}
}

func TestCanonicalOCRRequestKeepsStrictLanguageAuthoritySeparate(t *testing.T) {
	cases := []struct {
		name       string
		requested  string
		detected   string
		want       string
		wantSource string
	}{
		{name: "english short alias", requested: "en", want: "eng", wantSource: "request"},
		{name: "english canonical alias", requested: "eng", want: "eng", wantSource: "request"},
		{name: "english name alias", requested: "English", want: "eng", wantSource: "request"},
		{name: "stream english tag", detected: "en", want: "eng", wantSource: "stream-tag"},
		{name: "spanish", requested: "es", want: "spa", wantSource: "request"},
		{name: "japanese", requested: "ja", want: "jpn", wantSource: "request"},
		{name: "unknown request does not inherit stream tag", requested: "unknown", detected: "en", want: "", wantSource: ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			stream := FFProbeStream{Index: 1, ID: 2, CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": test.detected}}
			request := canonicalOCRRequestFor("/tmp/ocr-language-evidence.mkv", stream, test.requested, "clean", 0, 0)
			if request.DictionaryLanguage != test.want || request.DictionaryLanguageSource != test.wantSource {
				t.Fatalf("strict dictionary evidence = %q from %q, want %q from %q", request.DictionaryLanguage, request.DictionaryLanguageSource, test.want, test.wantSource)
			}
			if test.want == "" && request.Language != "eng" {
				t.Fatalf("legacy OCR fallback changed unexpectedly: got %q", request.Language)
			}
		})
	}
}

func TestLegacySnapshotDoesNotInheritEnglishDictionaryRules(t *testing.T) {
	snapshot := &publishedCanonicalOCRSnapshot{Evidence: canonicalOCREvidence{
		RequestedLanguage: "eng", Integrity: "legacy-text-cues-v1",
		Cues: []canonicalBitmapSubtitleCue{{SourceOrdinal: 1, Text: "|'m ready"}},
	}}
	if err := attachOCRDictionarySnapshot(snapshot, nil, ""); err != nil {
		t.Fatal(err)
	}
	report := analyzeCanonicalOCRSnapshot(snapshot)
	if report.DictionaryLanguage != "" || len(report.Findings) != 1 || report.Findings[0].Suggestion != "" {
		t.Fatalf("legacy normalized eng unexpectedly gained English rules: %#v", report)
	}
	if err := attachOCRDictionarySnapshot(snapshot, nil, "en"); err != nil {
		t.Fatal(err)
	}
	if snapshot.DictionaryLanguage != "eng" || snapshot.DictionaryLanguageSource != "reevaluation-selection" {
		t.Fatalf("explicit legacy language was not bound: %q from %q", snapshot.DictionaryLanguage, snapshot.DictionaryLanguageSource)
	}
}

func TestCanonicalAnalyzerOffersValidatedContextualDictionarySuggestion(t *testing.T) {
	snapshot := &publishedCanonicalOCRSnapshot{
		DictionaryRules:    []models.OCRDictionaryEntry{testOCRDictionaryEntry("builtin:eng:im", "|'m", "I'm", 100)},
		DictionaryRevision: "dictionary-revision",
		DictionaryLanguage: "eng",
		Evidence: canonicalOCREvidence{
			RequestedLanguage: "eng", DictionaryLanguage: "eng", DictionaryLanguageSource: "request",
			Integrity: "published-canonical-cues-v1",
			Cues:      []canonicalBitmapSubtitleCue{{SourceOrdinal: 7, Text: "Well, |'m here."}},
		},
	}
	report := analyzeCanonicalOCRSnapshot(snapshot)
	if len(report.Findings) != 1 {
		t.Fatalf("expected one OCR finding, got %#v", report.Findings)
	}
	finding := report.Findings[0]
	if finding.Suggestion != "Well, I'm here." || finding.CandidateSource != "contextual-ocr-dictionary" || len(finding.DictionaryRuleIDs) != 1 || finding.DictionaryRuleIDs[0] != "builtin:eng:im" {
		t.Fatalf("canonical analyzer did not offer the validated rule candidate: %#v", finding)
	}
	if report.DictionaryRevision != "dictionary-revision" || report.DictionaryLanguage != "eng" {
		t.Fatalf("dictionary binding missing from report: %#v", report)
	}
}
