package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubtitleProofreadingSRTReplacementsPreserveCueIdentityTimingAndLineEndings(t *testing.T) {
	source := "17\r\n00:00:01,200 --> 00:00:02,300\r\nThe | line\r\nSecond line\r\n\r\n9\r\n00:00:03,000 --> 00:00:04,000\r\nA clean cue\r\n"
	cues, err := parseSubtitleProofreadingCues("srt", []byte(source))
	if err != nil || len(cues) != 2 {
		t.Fatalf("parse SRT cues: count=%d err=%v", len(cues), err)
	}
	if cues[0].Ordinal != 1 || cues[0].Start != 1.2 || cues[0].End != 2.3 || cues[0].SourceText != "The | line\nSecond line" {
		t.Fatalf("first cue identity/timing/text changed: %+v", cues[0])
	}
	updated, err := applySubtitleProofreadingReplacements("srt", []byte(source), cues, map[string]string{cues[0].ID: "The | line!\nSecond line"})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(source, "The | line\r\nSecond line", "The | line!\r\nSecond line", 1)
	if string(updated) != want {
		t.Fatalf("SRT replacement changed structure or line endings\nwant: %q\n got: %q", want, updated)
	}
}

func TestSubtitleProofreadingASSAndSSAPreserveProtectedDialogueStructure(t *testing.T) {
	for _, format := range []string{"ass", "ssa"} {
		t.Run(format, func(t *testing.T) {
			source := "[Script Info]\r\nTitle: Test\r\n[Events]\r\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\r\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\i1\\pos(20,30)}♪ Hello ♪{\\i0}\\Nthere\r\n"
			cues, err := parseSubtitleProofreadingCues(format, []byte(source))
			if err != nil || len(cues) != 1 {
				t.Fatalf("parse protected subtitle: cues=%d err=%v", len(cues), err)
			}
			cue := cues[0]
			if cue.ASSDialogue == nil || !strings.Contains(cue.DisplayText, "⟦formatting⟧") {
				t.Fatalf("ASS/SSA formatting was not represented as protected text: %+v", cue)
			}
			providerText, err := subtitleProofreadingProviderText(cue)
			if err != nil || strings.Contains(providerText, translationProtectedAnchorPrefix) {
				t.Fatalf("provider received private ASS anchors instead of readable placeholders: %q err=%v", providerText, err)
			}
			providerCandidate := strings.Replace(providerText, "Hello", "Hi", 1)
			canonicalCandidate, err := normalizeSubtitleProofreadingProtectedText(cue, providerCandidate)
			if err != nil || validateSubtitleProofreadingCandidate(cue, canonicalCandidate, format, "en") != nil {
				t.Fatalf("valid placeholder-preserving provider text was rejected: %q err=%v", providerCandidate, err)
			}
			if _, err := normalizeSubtitleProofreadingProtectedText(cue, "Hi"); err == nil {
				t.Fatal("ASS/SSA edit without all protected formatting placeholders was accepted")
			}
			candidate := strings.Replace(cue.SourceText, "Hello", "Hi", 1)
			if err := validateSubtitleProofreadingCandidate(cue, candidate, format, "en"); err != nil {
				t.Fatalf("valid same-language text candidate rejected: %v", err)
			}
			updated, err := applySubtitleProofreadingReplacements(format, []byte(source), cues, map[string]string{cue.ID: candidate})
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(source, "Hello", "Hi", 1)
			if string(updated) != want {
				t.Fatalf("ASS/SSA structure changed\nwant: %q\n got: %q", want, updated)
			}
			if !validTranslatedSubtitleSidecar(format, updated) {
				t.Fatal("reconstructed ASS/SSA output failed format validation")
			}
		})
	}
}

func TestSubtitleProofreadingDeterministicFindingsRespectLanguageAndLegitimateSymbols(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		language string
		category string
	}{
		{name: "replacement glyph", text: "The \ufffdouse", language: "en", category: "replacement glyphs"},
		{name: "mojibake", text: "EstÃ¡ bien", language: "es", category: "encoding/mojibake"},
		{name: "contextual zero confusion", text: "I kn0w this", language: "en", category: "OCR-like confusion"},
		{name: "punctuation spacing", text: "Hello , world", language: "en", category: "punctuation/spacing"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cue := makeSubtitleProofreadingCue(0, test.text, test.text, 0, 1, nil, nil)
			finding, ok := deterministicSubtitleProofreadingFinding(cue, test.language)
			if !ok || finding.Category != test.category {
				t.Fatalf("expected %q finding, got %+v (present=%t)", test.category, finding, ok)
			}
		})
	}
	for _, text := range []string{"left | right", "こんにちは", "¿Qué tal?", "Hello, ¿qué tal?"} {
		cue := makeSubtitleProofreadingCue(0, text, text, 0, 1, nil, nil)
		if finding, ok := deterministicSubtitleProofreadingFinding(cue, subtitleProofreadingLanguage("", []subtitleProofreadingCue{cue})); ok {
			t.Errorf("legitimate or non-English text was flagged by English-oriented rules: %q (%s)", text, finding.Category)
		}
	}
	if got := subtitleProofreadingLanguage("spa", nil); got != "es" {
		t.Fatalf("Spanish sidecar language metadata resolved to %q", got)
	}
	if got := subtitleProofreadingLanguage("jpn", nil); got != "ja" {
		t.Fatalf("Japanese sidecar language metadata resolved to %q", got)
	}
}

func TestSubtitleProofreadingCandidateRejectsLanguageAndProtectedControlChanges(t *testing.T) {
	english := makeSubtitleProofreadingCue(0, "I know this story", "I know this story", 0, 1, nil, nil)
	if subtitleProofreadingCandidateLanguageMatches(english.SourceText, "Hola, esta es una historia", "en") {
		t.Fatal("wrong-language Spanish rewrite was accepted for English source")
	}
	spanish := makeSubtitleProofreadingCue(0, "¿Qué pasó?", "¿Qué pasó?", 0, 1, nil, nil)
	if subtitleProofreadingCandidateLanguageMatches(spanish.SourceText, "What happened?", "es") {
		t.Fatal("wrong-language English rewrite was accepted for Spanish source")
	}
	ja := makeSubtitleProofreadingCue(0, "こんにちは", "こんにちは", 0, 1, nil, nil)
	if subtitleProofreadingCandidateLanguageMatches(ja.SourceText, "Hello there", "ja") {
		t.Fatal("wrong-script rewrite was accepted for Japanese source")
	}
	assCues, err := parseSubtitleProofreadingCues("ass", []byte("[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\i1}hello{\\i0}\n"))
	if err != nil || len(assCues) != 1 {
		t.Fatalf("parse ASS cue: %v", err)
	}
	if err := validateSubtitleProofreadingCandidate(assCues[0], "injected{\\i0}text", "ass", "en"); err == nil {
		t.Fatal("ASS control injection was accepted")
	}
}

func TestSubtitleProofreadingResponseRequiresExactCueAndHashAndRejectsWrongLanguage(t *testing.T) {
	cue := makeSubtitleProofreadingCue(0, "I know the answer", "I know the answer", 0, 1, nil, nil)
	request := subtitleProofreadingProviderRequest{ContractVersion: subtitleProofreadingContractVersion, Language: "en", Cues: []subtitleProofreadingProviderCue{{CueID: cue.ID, SourceHash: cue.Hash, Text: cue.SourceText}}}
	if _, err := decodeSubtitleProofreadingResponse([]byte("{"), request, []subtitleProofreadingCue{cue}, "en", "proofreader"); err == nil {
		t.Fatal("malformed provider JSON was accepted")
	}
	valid := func(id, hash, text string) []byte {
		payload, _ := json.Marshal(subtitleProofreadingProviderResponse{
			ContractVersion: subtitleProofreadingContractVersion,
			Findings: []subtitleProofreadingProviderFinding{{
				CueID: id, SourceHash: hash, Status: "suggested", SuggestedText: text,
				Category: "spelling", ReasonCode: "spelling", Reason: "Check the word spelling.", Confidence: 0.4,
			}},
		})
		return payload
	}
	findings, err := decodeSubtitleProofreadingResponse(valid(cue.ID, cue.Hash, "I know the answer."), request, []subtitleProofreadingCue{cue}, "en", "proofreader")
	if err != nil || len(findings) != 1 || findings[0].Status != "suggested" {
		t.Fatalf("valid strict response rejected: findings=%+v err=%v", findings, err)
	}
	if _, err := decodeSubtitleProofreadingResponse(valid(cue.ID, "wrong-hash", "I know the answer."), request, []subtitleProofreadingCue{cue}, "en", "proofreader"); err == nil {
		t.Fatal("wrong source hash was accepted")
	}
	wrongLanguage, err := decodeSubtitleProofreadingResponse(valid(cue.ID, cue.Hash, "Hola, esta es la respuesta."), request, []subtitleProofreadingCue{cue}, "en", "proofreader")
	if err != nil || wrongLanguage[0].SuggestedText != "" || wrongLanguage[0].Status != "needs_review" {
		t.Fatalf("wrong-language suggestion was not held for review: %+v err=%v", wrongLanguage, err)
	}
	secondCue := makeSubtitleProofreadingCue(1, "I know the story", "I know the story", 2, 3, nil, nil)
	twoCueRequest := subtitleProofreadingProviderRequest{
		ContractVersion: subtitleProofreadingContractVersion, Language: "en",
		Cues: []subtitleProofreadingProviderCue{
			{CueID: cue.ID, SourceHash: cue.Hash, Text: cue.SourceText},
			{CueID: secondCue.ID, SourceHash: secondCue.Hash, Text: secondCue.SourceText},
		},
	}
	missing, _ := json.Marshal(subtitleProofreadingProviderResponse{
		ContractVersion: subtitleProofreadingContractVersion,
		Findings:        []subtitleProofreadingProviderFinding{{CueID: cue.ID, SourceHash: cue.Hash, Status: "clean"}},
	})
	if _, err := decodeSubtitleProofreadingResponse(missing, twoCueRequest, []subtitleProofreadingCue{cue, secondCue}, "en", "proofreader"); err == nil {
		t.Fatal("response missing a requested cue was accepted")
	}
	duplicated, _ := json.Marshal(subtitleProofreadingProviderResponse{
		ContractVersion: subtitleProofreadingContractVersion,
		Findings: []subtitleProofreadingProviderFinding{
			{CueID: cue.ID, SourceHash: cue.Hash, Status: "clean"},
			{CueID: cue.ID, SourceHash: cue.Hash, Status: "clean"},
		},
	})
	if _, err := decodeSubtitleProofreadingResponse(duplicated, twoCueRequest, []subtitleProofreadingCue{cue, secondCue}, "en", "proofreader"); err == nil {
		t.Fatal("response duplicating a cue was accepted")
	}
}

func TestSubtitleProofreadingASSProviderResponseKeepsVisibleAndCanonicalTextSeparate(t *testing.T) {
	source := "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\i1}Hello{\\i0}\\Nthere\n"
	cues, err := parseSubtitleProofreadingCues("ass", []byte(source))
	if err != nil || len(cues) != 1 {
		t.Fatalf("parse ASS cue: count=%d err=%v", len(cues), err)
	}
	cue := cues[0]
	providerText, err := subtitleProofreadingProviderText(cue)
	if err != nil {
		t.Fatal(err)
	}
	request := subtitleProofreadingProviderRequest{
		ContractVersion: subtitleProofreadingContractVersion, Language: "en",
		Cues: []subtitleProofreadingProviderCue{{CueID: cue.ID, SourceHash: cue.Hash, Text: providerText}},
	}
	response := subtitleProofreadingProviderResponse{
		ContractVersion: subtitleProofreadingContractVersion,
		Findings: []subtitleProofreadingProviderFinding{{
			CueID: cue.ID, SourceHash: cue.Hash, Status: "suggested",
			SuggestedText: strings.Replace(providerText, "Hello", "Hi", 1), Category: "spelling",
			ReasonCode: "spelling", Reason: "Review the spelling.",
		}},
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := decodeSubtitleProofreadingResponse(payload, request, cues, "en", "proofreader")
	if err != nil || len(findings) != 1 {
		t.Fatalf("decode protected ASS suggestion: findings=%+v err=%v", findings, err)
	}
	if findings[0].SuggestedText != response.Findings[0].SuggestedText || findings[0].validatedText == findings[0].SuggestedText || findings[0].validatedText == "" {
		t.Fatalf("ASS UI candidate and canonical publication text were not separated: %+v", findings[0])
	}
	updated, err := applySubtitleProofreadingReplacements("ass", []byte(source), cues, map[string]string{cue.ID: findings[0].validatedText})
	if err != nil || !strings.Contains(string(updated), "{\\i1}Hi{\\i0}\\Nthere") {
		t.Fatalf("publish validated ASS suggestion: %q err=%v", updated, err)
	}
}

func TestOllamaProofreadingUsesSeparateBoundedProofreadingContract(t *testing.T) {
	cue := makeSubtitleProofreadingCue(0, "I know this", "I know this", 0, 1, nil, nil)
	client := &http.Client{Transport: subtitleProofreadingRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request ollamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Ollama chat request: %v", err)
			return nil, err
		}
		if request.Model != "local-proofreader" || len(request.Messages) != 1 || !strings.Contains(request.Messages[0].Content, "subtitle proofreader") || strings.Contains(request.Messages[0].Content, "subtitle translator") {
			t.Errorf("request did not use the distinct proofreading contract: %+v", request)
		}
		if !strings.Contains(request.Messages[0].Content, cue.Hash) {
			t.Errorf("bounded request omitted the source cue hash")
		}
		inner := `{"contractVersion":"proofread-v1","findings":[{"cueId":"` + cue.ID + `","sourceTextHash":"` + cue.Hash + `","status":"suggested","suggestedText":"I know this.","category":"punctuation/spacing","reasonCode":"punctuation-spacing","reason":"A short sentence may end with punctuation.","confidence":0.5}]}`
		encoded, err := json.Marshal(map[string]any{"message": map[string]string{"content": inner}, "prompt_eval_count": 10, "eval_count": 5, "total_duration": int64(1_000_000)})
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(encoded))), Request: r,
		}, nil
	})}
	provider, err := newOllamaTranslationProviderWithClient(translationSettings{Provider: translationProviderOllama, OllamaEndpoint: "http://ollama.local", OllamaModel: "local-proofreader", MaxRequestBytes: 128 * 1024}, client)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := provider.Proofread(ctx, "en", []subtitleProofreadingCue{cue})
	if err != nil || len(result.Findings) != 1 || result.Metrics.Model != "local-proofreader" {
		t.Fatalf("proofreading provider failed: result=%+v err=%v", result, err)
	}
}

type subtitleProofreadingRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip subtitleProofreadingRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestSubtitleProofreadingRevisionPublishRestoreAndRejectStaleSource(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", stateRoot)
	sidecarPath := filepath.Join(t.TempDir(), "movie.en.srt")
	before := []byte("1\n00:00:01,000 --> 00:00:02,000\nI know this\n")
	after := []byte("1\n00:00:01,000 --> 00:00:02,000\nI know this.\n")
	if err := os.WriteFile(sidecarPath, before, 0o640); err != nil {
		t.Fatal(err)
	}
	priorRevision, _, err := publishedSubtitleRevisionFor(sidecarPath, "")
	if err != nil {
		t.Fatal(err)
	}
	manifest := subtitleProofreadingRevisionManifest{
		MediaPath: sidecarPath, SidecarPath: sidecarPath, Format: "srt", BeforeRevision: priorRevision.key(),
		AfterContentHash: subtitleProofreadingTextHash(string(after)), ContractVersion: subtitleProofreadingContractVersion,
		Kind: "apply", CreatedAt: time.Now().UTC(),
	}
	revisionID, err := persistSubtitleProofreadingPriorRevision(manifest, before)
	if err != nil {
		t.Fatal(err)
	}
	subtitlePublicationMu.Lock()
	err = publishSubtitleProofreadingContentLocked(sidecarPath, priorRevision, "", after)
	subtitlePublicationMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	manifest.AfterContentHash = subtitleProofreadingTextHash(string(after))
	if err := updateSubtitleProofreadingManifest(revisionID, manifest); err != nil {
		t.Fatal(err)
	}
	loaded, restored, err := loadSubtitleProofreadingRevision(revisionID)
	if err != nil || loaded.SidecarPath != sidecarPath || string(restored) != string(before) {
		t.Fatalf("stored prior revision was not restorable: manifest=%+v bytes=%q err=%v", loaded, restored, err)
	}
	currentRevision, _, err := publishedSubtitleRevisionFor(sidecarPath, "")
	if err != nil {
		t.Fatal(err)
	}
	subtitlePublicationMu.Lock()
	err = publishSubtitleProofreadingContentLocked(sidecarPath, currentRevision, "", restored)
	subtitlePublicationMu.Unlock()
	if err != nil {
		t.Fatalf("restore saved prior bytes: %v", err)
	}
	currentBytes, err := os.ReadFile(sidecarPath)
	if err != nil || string(currentBytes) != string(before) {
		t.Fatalf("restored bytes mismatch: %q err=%v", currentBytes, err)
	}
	if err := os.WriteFile(sidecarPath, []byte("external change"), 0o640); err != nil {
		t.Fatal(err)
	}
	subtitlePublicationMu.Lock()
	err = publishSubtitleProofreadingContentLocked(sidecarPath, priorRevision, "", after)
	subtitlePublicationMu.Unlock()
	if err == nil {
		t.Fatal("stale captured revision was published")
	}
	staleBytes, readErr := os.ReadFile(sidecarPath)
	if readErr != nil || string(staleBytes) != "external change" {
		t.Fatalf("stale publication changed current bytes: %q err=%v", staleBytes, readErr)
	}
}

func TestSubtitleProofreadingApplyRequiresConfirmationAndSupportsRestoreAndStaleRejection(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", stateRoot)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	sidecarPath := filepath.Join(root, "movie.en.srt")
	before := []byte("1\n00:00:01,000 --> 00:00:02,000\nI know the answer\n")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecarPath, before, 0o640); err != nil {
		t.Fatal(err)
	}
	db := translationLibrarySourceTestDB(t, root, mediaPath)
	handler := AssetHandler{db: db}
	source, err := resolveExternalSubtitleTranslationSource(mediaPath, sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	revision, _, err := publishedSubtitleRevisionFor(source.ExternalPath, source.LogicalTransformKey)
	if err != nil {
		t.Fatal(err)
	}
	cues, err := parseSubtitleProofreadingCues("srt", before)
	if err != nil || len(cues) != 1 {
		t.Fatalf("parse source cues: count=%d err=%v", len(cues), err)
	}
	cue := cues[0]
	finding := subtitleProofreadingFinding{
		CueID: cue.ID, Ordinal: cue.Ordinal, SourceHash: cue.Hash,
		OriginalText: cue.DisplayText, Suggested: "I know the answer.", SuggestedProviderText: "I know the answer.",
		Status: "suggested", Category: "punctuation/spacing", ReasonCode: "punctuation-spacing", Reason: "Review punctuation.",
	}
	operationID := "proofreading-apply-" + strings.ReplaceAll(t.Name(), "/", "-")
	request := subtitleProofreadingOperationRequest{
		MediaPath: mediaPath, SidecarPath: source.ExternalPath, Format: "srt",
		Revision: revision, RevisionKey: source.SourceRevision, Language: "en", SourceType: "translated",
		Cues: cues, TotalCueCount: 1,
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{
		ID: operationID, Kind: "proofreading", Status: "completed", Proofreading: &subtitleProofreadingReport{
			SidecarPath: source.ExternalPath, Format: "srt", Mode: "quick", SourceType: "translated",
			Revision: source.SourceRevision, Provider: "ollama", Model: "translation", ProviderState: "ready",
			TotalCount: 1, SuggestedCount: 1, Findings: []subtitleProofreadingFinding{finding},
		}, proofreadingRequest: &request,
	}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()
	decision := subtitleProofreadingDecisionInput{CueID: cue.ID, SourceHash: cue.Hash, Action: "accept"}
	if _, err := handler.applySubtitleProofreading(subtitleProofreadingApplyInput{
		Path: mediaPath, SubtitlePath: sidecarPath, OperationID: operationID,
		Revision: source.SourceRevision, Decisions: []subtitleProofreadingDecisionInput{decision},
	}); err == nil {
		t.Fatal("unconfirmed AI edit was applied")
	}
	current, err := os.ReadFile(sidecarPath)
	if err != nil || string(current) != string(before) {
		t.Fatalf("unconfirmed decision changed the sidecar: %q err=%v", current, err)
	}
	applied, err := handler.applySubtitleProofreading(subtitleProofreadingApplyInput{
		Path: mediaPath, SubtitlePath: sidecarPath, OperationID: operationID,
		Revision: source.SourceRevision, Confirmed: true, Decisions: []subtitleProofreadingDecisionInput{decision},
	})
	if err != nil || applied.AffectedCueCount != 1 || applied.ChangedCueCount != 1 {
		t.Fatalf("confirmed batch was not published with exact counts: result=%+v err=%v", applied, err)
	}
	current, err = os.ReadFile(sidecarPath)
	if err != nil || !strings.Contains(string(current), "I know the answer.") {
		t.Fatalf("approved text was not atomically published: %q err=%v", current, err)
	}
	restored, err := handler.restoreSubtitleProofreadingRevision(subtitleProofreadingRestoreInput{
		Path: mediaPath, SubtitlePath: sidecarPath, RevisionID: applied.RevisionID, Expected: applied.SourceRevision,
	})
	if err != nil || !restored.Restored || restored.SourceRevision == applied.SourceRevision {
		t.Fatalf("explicit restore did not produce the new source revision: result=%+v err=%v", restored, err)
	}
	current, err = os.ReadFile(sidecarPath)
	if err != nil || string(current) != string(before) {
		t.Fatalf("restore did not recover the prior sidecar bytes: %q err=%v", current, err)
	}
	external := []byte("external concurrent update")
	if err := os.WriteFile(sidecarPath, external, 0o640); err != nil {
		t.Fatal(err)
	}
	staleExpected := sourceRevisionKeyForExternal(sidecarPath, source.LogicalTransformKey)
	if _, err := handler.restoreSubtitleProofreadingRevision(subtitleProofreadingRestoreInput{
		Path: mediaPath, SubtitlePath: sidecarPath, RevisionID: restored.RevisionID, Expected: staleExpected,
	}); err == nil {
		t.Fatal("restore overwrote an external concurrent source change")
	}
	current, err = os.ReadFile(sidecarPath)
	if err != nil || string(current) != string(external) {
		t.Fatalf("stale restore changed the external sidecar: %q err=%v", current, err)
	}
}

func TestCancelledSubtitleProofreadingKeepsDeterministicFindingsAndSourceBytes(t *testing.T) {
	sidecarPath := filepath.Join(t.TempDir(), "movie.en.srt")
	source := []byte("1\n00:00:01,000 --> 00:00:02,000\nI kn0w this\n")
	if err := os.WriteFile(sidecarPath, source, 0o640); err != nil {
		t.Fatal(err)
	}
	revision, _, err := publishedSubtitleRevisionFor(sidecarPath, "")
	if err != nil {
		t.Fatal(err)
	}
	cues, err := parseSubtitleProofreadingCues("srt", source)
	if err != nil || len(cues) != 1 {
		t.Fatalf("parse deterministic source: cues=%d err=%v", len(cues), err)
	}
	finding, ok := deterministicSubtitleProofreadingFinding(cues[0], "en")
	if !ok {
		t.Fatal("expected the deterministic OCR-like finding")
	}
	operationID := "proofreading-cancelled-" + strings.ReplaceAll(t.Name(), "/", "-")
	request := subtitleProofreadingOperationRequest{
		SidecarPath: sidecarPath, Format: "srt", Revision: revision, RevisionKey: sourceRevisionKeyForExternal(sidecarPath, ""),
		Mode: "quick", Language: "en", Cues: cues, TotalCueCount: 1,
		Deterministic: map[string]subtitleProofreadingFinding{cues[0].ID: finding},
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{
		ID: operationID, Kind: "proofreading", Status: "running", proofreadingRequest: &request,
	}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runSubtitleProofreadingOperation(ctx, operationID, request, nil)
	operation, ok := currentSubtitleProofreadingOperation(operationID)
	if !ok || operation.Status != "cancelled" || operation.Proofreading == nil || operation.Proofreading.ProviderState != "cancelled" || operation.Proofreading.ReviewCount != 1 || operation.Proofreading.UnassessedCount != 0 {
		t.Fatalf("cancelled review did not retain deterministic evidence: operation=%+v exists=%t", operation, ok)
	}
	current, err := os.ReadFile(sidecarPath)
	if err != nil || string(current) != string(source) {
		t.Fatalf("cancelled review changed source bytes: %q err=%v", current, err)
	}
}

func TestUnavailableSubtitleProofreadingProviderKeepsFindingsAndSourceBytes(t *testing.T) {
	sidecarPath := filepath.Join(t.TempDir(), "movie.en.srt")
	source := []byte("1\n00:00:01,000 --> 00:00:02,000\nI kn0w this\n")
	if err := os.WriteFile(sidecarPath, source, 0o640); err != nil {
		t.Fatal(err)
	}
	revision, _, err := publishedSubtitleRevisionFor(sidecarPath, "")
	if err != nil {
		t.Fatal(err)
	}
	cues, err := parseSubtitleProofreadingCues("srt", source)
	if err != nil || len(cues) != 1 {
		t.Fatalf("parse deterministic source: cues=%d err=%v", len(cues), err)
	}
	finding, ok := deterministicSubtitleProofreadingFinding(cues[0], "en")
	if !ok {
		t.Fatal("expected the deterministic OCR-like finding")
	}
	operationID := "proofreading-provider-disabled-" + strings.ReplaceAll(t.Name(), "/", "-")
	request := subtitleProofreadingOperationRequest{
		SidecarPath: sidecarPath, Format: "srt", Revision: revision, RevisionKey: sourceRevisionKeyForExternal(sidecarPath, ""),
		Mode: "quick", Language: "en", Cues: cues, TotalCueCount: 1,
		Deterministic: map[string]subtitleProofreadingFinding{cues[0].ID: finding},
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{
		ID: operationID, Kind: "proofreading", Status: "running", proofreadingRequest: &request,
	}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()
	runSubtitleProofreadingOperation(context.Background(), operationID, request, nil)
	operation, ok := currentSubtitleProofreadingOperation(operationID)
	if !ok || operation.Status != "completed" || operation.Proofreading == nil || operation.Proofreading.ProviderState != "disabled" || operation.Proofreading.ReviewCount != 1 {
		t.Fatalf("unavailable provider did not preserve deterministic results: operation=%+v exists=%t", operation, ok)
	}
	current, err := os.ReadFile(sidecarPath)
	if err != nil || string(current) != string(source) {
		t.Fatalf("provider-unavailable analysis changed source bytes: %q err=%v", current, err)
	}
}

func TestSubtitleProofreadingStageFailureLeavesOriginalUntouched(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permission failure is not enforceable when running as root")
	}
	directory := t.TempDir()
	sidecarPath := filepath.Join(directory, "movie.en.srt")
	source := []byte("1\n00:00:01,000 --> 00:00:02,000\nI know this\n")
	updated := []byte("1\n00:00:01,000 --> 00:00:02,000\nI know this.\n")
	if err := os.WriteFile(sidecarPath, source, 0o640); err != nil {
		t.Fatal(err)
	}
	revision, _, err := publishedSubtitleRevisionFor(sidecarPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(directory, 0o700) }()
	if err := publishSubtitleProofreadingContentLocked(sidecarPath, revision, "", updated); err == nil {
		t.Fatal("publication succeeded when atomic staging could not create its temporary file")
	}
	current, err := os.ReadFile(sidecarPath)
	if err != nil || string(current) != string(source) {
		t.Fatalf("failed staging changed the original sidecar: %q err=%v", current, err)
	}
}

func TestSubtitleProofreadingSourceTypeDoesNotLabelImportsAsOCR(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "movie.mkv")
	sidecarPath := filepath.Join(filepath.Dir(mediaPath), "movie.en.srt")
	if got := subtitleProofreadingSourceType(nil, mediaPath, sidecarPath, "srt", ""); got != "imported/external" {
		t.Fatalf("unmanaged external sidecar source type=%q, want imported/external", got)
	}
	identity, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	metadata := identity.metadata()
	operationID := "proofreading-source-type-" + strings.ReplaceAll(t.Name(), "/", "-")
	invalidMetadata := SubtitleTranslationMetadata{}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{
		ID: operationID, Kind: "translation", Status: "completed",
		Result: &SubtitleExtractionResult{Artifacts: []SubtitleArtifact{{
			Mode: "translation", PublishedPath: sidecarPath, Translation: &invalidMetadata,
		}}},
	}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()
	if got := subtitleProofreadingSourceType(nil, mediaPath, sidecarPath, "srt", ""); got != "unknown" {
		t.Fatalf("invalid translation lineage source type=%q, want unknown", got)
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID].Result.Artifacts[0].Translation = &metadata
	subtitleExtractionOperations.Unlock()
	if got := subtitleProofreadingSourceType(nil, mediaPath, sidecarPath, "srt", ""); got != "translated" {
		t.Fatalf("exact translated artifact source type=%q, want translated", got)
	}
}
