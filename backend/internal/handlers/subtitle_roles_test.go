package handlers

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestClassifySubtitleRoleUsesExplicitEvidenceAndKeepsForcedIndependent(t *testing.T) {
	tests := []struct {
		name       string
		stream     map[string]any
		role       SubtitleSemanticRole
		provenance string
	}{
		{"explicit full", map[string]any{"index": 0, "role": "full"}, SubtitleRoleFull, "metadata"},
		{"explicit forced", map[string]any{"index": 1, "semanticRole": "forced"}, SubtitleRoleForced, "metadata"},
		{"explicit signs", map[string]any{"index": 2, "subtitleRole": "signs"}, SubtitleRoleSigns, "metadata"},
		{"explicit commentary", map[string]any{"index": 3, "role": "commentary"}, SubtitleRoleCommentary, "metadata"},
		{"explicit sdh", map[string]any{"index": 4, "role": "sdh"}, SubtitleRoleSDH, "metadata"},
		{"source forced signs uses disposition precedence", map[string]any{"index": 5, "title": "Signs", "forced": true}, SubtitleRoleForced, "disposition"},
		{"explicit signs coexists with source forced", map[string]any{"index": 6, "role": "signs", "forced": true}, SubtitleRoleSigns, "metadata"},
		{"source unforced signs", map[string]any{"index": 7, "title": "Signs", "forced": false}, SubtitleRoleSigns, "heuristic"},
		{"source forced without stronger role", map[string]any{"index": 8, "forced": true}, SubtitleRoleForced, "disposition"},
		{"disposition commentary", map[string]any{"index": 9, "commentary": true}, SubtitleRoleCommentary, "disposition"},
		{"disposition sdh", map[string]any{"index": 10, "hearingImpaired": true}, SubtitleRoleSDH, "disposition"},
		{"ambiguous", map[string]any{"index": 11, "title": "Signs commentary"}, SubtitleRoleUnknown, "ambiguous/unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := classifySubtitleRole(models.ScanResult{Path: "/media/title.mkv", SizeBytes: 10}, test.stream, nil)
			if decision.Role != test.role || decision.Provenance != test.provenance || decision.Confidence == "" {
				t.Fatalf("decision=%#v want role=%q provenance=%q", decision, test.role, test.provenance)
			}
		})
	}
}

func TestSubtitleRoleGeneratedSnapshotAnnotationRemainsAutomatic(t *testing.T) {
	scan := models.ScanResult{Path: "/media/title.mkv", SizeBytes: 10, SubtitleStreams: models.JSONList{
		map[string]any{"index": 4, "codec": "subrip", "language": "spa", "title": "Signs"},
	}}
	annotateSubtitleRoleSummaries(&scan)
	plan, err := resolveTrackPlan(scan, map[string]any{"subtitleDisposition": "keep", "attachmentPolicy": "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.SubtitleStreams[0]; got.Role != SubtitleRoleSigns || got.RoleProvenance != "heuristic" || got.RoleProvenance == "metadata" {
		t.Fatalf("generated role was promoted to source metadata: %#v", got)
	}
}

func TestSubtitleRoleForcedDispositionAndEffectiveForcedRemainIndependent(t *testing.T) {
	scan := models.ScanResult{Path: "/media/title.mkv", SizeBytes: 10, SubtitleStreams: models.JSONList{
		map[string]any{"index": 3, "codec": "subrip", "language": "spa", "title": "Signs", "forced": false},
		map[string]any{"index": 4, "codec": "subrip", "language": "spa", "role": "forced", "forced": false},
	}}
	plan, err := resolveTrackPlan(scan, map[string]any{"subtitleDisposition": "keep", "attachmentPolicy": "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.SubtitleStreams[0].Role != SubtitleRoleSigns || plan.SubtitleStreams[0].EffectiveForced {
		t.Fatalf("signs without source forced disposition became forced: %#v", plan.SubtitleStreams[0])
	}
	if plan.SubtitleStreams[1].Role != SubtitleRoleForced || plan.SubtitleStreams[1].Forced || !plan.SubtitleStreams[1].EffectiveForced {
		t.Fatalf("canonical forced role did not set effective forced independently: %#v", plan.SubtitleStreams[1])
	}
}

func TestSubtitleRoleOverrideBindsToSnapshotAndStreamSignature(t *testing.T) {
	scan := models.ScanResult{Path: "/media/title.mkv", SizeBytes: 10, SubtitleStreams: models.JSONList{
		map[string]any{"index": 4, "codec": "hdmv_pgs_subtitle", "language": "jpn"},
	}}
	overrides := bindSubtitleRoleOverrides(scan, map[int]SubtitleRoleOverride{4: {Role: SubtitleRoleSigns}})
	profile := map[string]any{"subtitleRoleOverrides": overrides}
	decision := classifySubtitleRole(scan, scan.SubtitleStreams[0].(map[string]any), profile)
	if decision.Role != SubtitleRoleSigns || decision.Provenance != "manual" || decision.OverrideState != "manual" {
		t.Fatalf("bound override was not applied: %#v", decision)
	}

	changedSource := scan
	changedSource.SizeBytes++
	stale := classifySubtitleRole(changedSource, changedSource.SubtitleStreams[0].(map[string]any), profile)
	if stale.Provenance == "manual" || stale.OverrideState != "stale/ignored" {
		t.Fatalf("stale source override was applied: %#v", stale)
	}

	changedStream := models.JSONMap{"index": 4, "codec": "subrip", "language": "jpn"}
	stale = classifySubtitleRole(scan, changedStream, profile)
	if stale.Provenance == "manual" || stale.OverrideState != "stale/ignored" {
		t.Fatalf("stale stream override was applied to reused index: %#v", stale)
	}
}

func TestSubtitleRoleOverrideUnrelatedSaveDoesNotRebindReusedStreamIndex(t *testing.T) {
	oldScan := models.ScanResult{Path: "/media/title.mkv", SizeBytes: 10, SubtitleStreams: models.JSONList{
		map[string]any{"index": 4, "codec": "hdmv_pgs_subtitle", "language": "jpn", "title": "Signs"},
	}}
	existing := bindSubtitleRoleOverrides(oldScan, map[int]SubtitleRoleOverride{4: {Role: SubtitleRoleSigns}})
	newScan := oldScan
	newScan.SizeBytes++
	newScan.SubtitleStreams = models.JSONList{map[string]any{"index": 4, "codec": "subrip", "language": "eng", "title": "Full"}}
	retained := subtitleRoleOverridesForConversionUpdate(newScan, existing, nil, false)
	if retained[4].SourceSnapshotFingerprint != existing[4].SourceSnapshotFingerprint || retained[4].StreamSignature != existing[4].StreamSignature {
		t.Fatalf("unrelated save rebound stale override: got=%#v existing=%#v", retained[4], existing[4])
	}
	decision := classifySubtitleRole(newScan, newScan.SubtitleStreams[0].(map[string]any), map[string]any{"subtitleRoleOverrides": retained})
	if decision.OverrideState != "stale/ignored" || decision.Provenance == "manual" {
		t.Fatalf("stale override became valid after unrelated save: %#v", decision)
	}
}

func TestResolveTrackPlanRoleRulesPreserveSameLanguageStreams(t *testing.T) {
	scan := models.ScanResult{Path: "/media/title.mkv", SizeBytes: 10, SubtitleStreams: models.JSONList{
		map[string]any{"index": 3, "codec": "subrip", "language": "spa", "title": "Full"},
		map[string]any{"index": 4, "codec": "subrip", "language": "spa", "title": "Signs", "role": "signs", "forced": true},
		map[string]any{"index": 5, "codec": "subrip", "language": "spa", "title": "Commentary"},
		map[string]any{"index": 6, "codec": "subrip", "language": "eng", "title": "Full"},
	}}
	plan, err := resolveTrackPlan(scan, map[string]any{
		"subtitleDisposition": "remove",
		"subtitleRules": []any{
			map[string]any{"language": "spa", "roles": []string{"full", "forced", "signs"}, "action": "keep"},
			map[string]any{"language": "spa", "roles": []string{"commentary"}, "action": "remove"},
			map[string]any{"language": "eng", "roles": []string{"full"}, "action": "keep"},
		},
		"attachmentPolicy": "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RoleClassifierVersion != subtitleSemanticRoleClassifierVersion || plan.SubtitleStreams[0].RoleClassifierVersion != subtitleSemanticRoleClassifierVersion {
		t.Fatalf("classifier version was not frozen through the resolved plan: %#v", plan)
	}
	wantActions := []SubtitleDisposition{SubtitleDispositionKeep, SubtitleDispositionKeep, SubtitleDispositionRemove, SubtitleDispositionKeep}
	wantRoles := []SubtitleSemanticRole{SubtitleRoleFull, SubtitleRoleSigns, SubtitleRoleCommentary, SubtitleRoleFull}
	for index, subtitle := range plan.SubtitleStreams {
		if subtitle.Action != wantActions[index] || subtitle.Role != wantRoles[index] || subtitle.StreamIndex != index+3 {
			t.Fatalf("subtitle[%d]=%#v want action=%q role=%q", index, subtitle, wantActions[index], wantRoles[index])
		}
	}
	if !plan.SubtitleStreams[1].EffectiveForced || !plan.SubtitleStreams[1].Forced {
		t.Fatalf("signs forced semantics were not kept independent: %#v", plan.SubtitleStreams[1])
	}
}

func TestSubtitleArtifactPreservesRoleClassifierVersion(t *testing.T) {
	values := subtitleArtifactsFromJSON(subtitleArtifactsJSON([]SubtitleArtifact{{
		StreamIndex: 4, Format: "srt", Role: SubtitleRoleFull, RoleClassifierVersion: subtitleSemanticRoleClassifierVersion, Status: "planned",
	}}))
	if len(values) != 1 || values[0].RoleClassifierVersion != subtitleSemanticRoleClassifierVersion {
		t.Fatalf("artifact classifier version was not persisted: %#v", values)
	}
}

func TestSubtitleRolePublicationPreservesForcedAndSDHSemantics(t *testing.T) {
	forced := true
	if got := subtitleStreamDisposition(StreamMetadataOverride{}, MediaStream{}, true, false, false); got != "forced" {
		t.Fatalf("role forced disposition=%q", got)
	}
	if got := subtitleStreamDisposition(StreamMetadataOverride{}, MediaStream{}, false, true, false); got != "comment" {
		t.Fatalf("commentary disposition=%q", got)
	}
	if got := subtitleStreamDisposition(StreamMetadataOverride{}, MediaStream{}, false, false, true); got != "hearing_impaired" {
		t.Fatalf("sdh disposition=%q", got)
	}
	if got := subtitleStreamDisposition(StreamMetadataOverride{}, MediaStream{Forced: forced}, true, false, true); got != "forced+hearing_impaired" {
		t.Fatalf("forced sdh disposition=%q", got)
	}

	used := map[string]struct{}{}
	if got := resolvedSubtitleStagedPath("/staging/Movie", SubtitleArtifact{Language: "eng", Format: "srt", Forced: true}, used); got != "/staging/Movie.eng.forced.srt" {
		t.Fatalf("forced sidecar path=%q", got)
	}
	if got := resolvedSubtitleStagedPath("/staging/Movie", SubtitleArtifact{Language: "eng", Format: "srt", Role: SubtitleRoleSDH}, used); got != "/staging/Movie.eng.sdh.srt" {
		t.Fatalf("sdh sidecar path=%q", got)
	}
	if got := resolvedSubtitleStagedPath("/staging/Movie", SubtitleArtifact{Language: "eng", Format: "srt", Role: SubtitleRoleSDH, Forced: true}, used); got != "/staging/Movie.eng.forced.sdh.srt" {
		t.Fatalf("forced sdh sidecar path=%q", got)
	}
}
