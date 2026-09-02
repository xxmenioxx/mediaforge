package handlers

import (
	"encoding/json"
	"testing"
)

func TestTrackDomainEnumsRoundTripJSON(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "subtitle keep", value: SubtitleDispositionKeep, want: `"keep"`},
		{name: "subtitle remove", value: SubtitleDispositionRemove, want: `"remove"`},
		{name: "subtitle extract", value: SubtitleDispositionExtract, want: `"extract"`},
		{name: "subtitle keep and extract", value: SubtitleDispositionKeepAndExtract, want: `"keep_and_extract"`},
		{name: "attachment auto", value: AttachmentPolicyAuto, want: `"auto"`},
		{name: "attachment keep", value: AttachmentPolicyKeep, want: `"keep"`},
		{name: "attachment remove", value: AttachmentPolicyRemove, want: `"remove"`},
		{name: "chapter keep", value: ChapterPolicyKeep, want: `"keep"`},
		{name: "chapter remove", value: ChapterPolicyRemove, want: `"remove"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != test.want {
				t.Fatalf("encoded=%s want=%s", encoded, test.want)
			}
		})
	}

	var disposition SubtitleDisposition
	if err := json.Unmarshal([]byte(`"keep_and_extract"`), &disposition); err != nil || disposition != SubtitleDispositionKeepAndExtract {
		t.Fatalf("subtitle round trip=%q err=%v", disposition, err)
	}
	var attachments AttachmentPolicy
	if err := json.Unmarshal([]byte(`"auto"`), &attachments); err != nil || attachments != AttachmentPolicyAuto {
		t.Fatalf("attachment round trip=%q err=%v", attachments, err)
	}
	var chapters ChapterPolicy
	if err := json.Unmarshal([]byte(`"remove"`), &chapters); err != nil || chapters != ChapterPolicyRemove {
		t.Fatalf("chapter round trip=%q err=%v", chapters, err)
	}
}

func TestTrackDomainEnumsRejectInvalidValues(t *testing.T) {
	if _, err := ParseSubtitleDisposition("copy_and_remove"); err == nil {
		t.Fatal("invalid subtitle disposition was accepted")
	}
	if _, err := ParseAttachmentPolicy("fonts_only"); err == nil {
		t.Fatal("invalid attachment policy was accepted")
	}
	if _, err := ParseChapterPolicy("auto"); err == nil {
		t.Fatal("invalid chapter policy was accepted")
	}
	if _, err := json.Marshal(SubtitleDisposition("invalid")); err == nil {
		t.Fatal("invalid subtitle disposition serialized")
	}
	var disposition SubtitleDisposition
	if err := json.Unmarshal([]byte(`"invalid"`), &disposition); err == nil {
		t.Fatal("invalid subtitle disposition deserialized")
	}
}

func TestCanonicalSubtitleAndAttachmentSemantics(t *testing.T) {
	for _, test := range []struct {
		action   SubtitleDisposition
		embedded bool
		sidecar  bool
	}{
		{action: SubtitleDispositionKeep, embedded: true},
		{action: SubtitleDispositionRemove},
		{action: SubtitleDispositionExtract, sidecar: true},
		{action: SubtitleDispositionKeepAndExtract, embedded: true, sidecar: true},
	} {
		if test.action.KeepsEmbedded() != test.embedded || test.action.ExtractsSidecar() != test.sidecar {
			t.Fatalf("action=%q embedded=%t sidecar=%t", test.action, test.action.KeepsEmbedded(), test.action.ExtractsSidecar())
		}
	}

	keep, err := ResolveAttachmentRetention(AttachmentPolicyAuto, []ResolvedSubtitleTrack{{Codec: "ass", Action: SubtitleDispositionKeep}})
	if err != nil || !keep {
		t.Fatalf("auto did not preserve fonts for embedded ASS: keep=%t err=%v", keep, err)
	}
	keep, err = ResolveAttachmentRetention(AttachmentPolicyAuto, []ResolvedSubtitleTrack{{Codec: "ssa", Action: SubtitleDispositionExtract}})
	if err != nil || keep {
		t.Fatalf("auto preserved attachments for extract-only SSA: keep=%t err=%v", keep, err)
	}
	keep, err = ResolveAttachmentRetention(AttachmentPolicyKeep, nil)
	if err != nil || !keep {
		t.Fatalf("keep policy was not authoritative: keep=%t err=%v", keep, err)
	}
	keep, err = ResolveAttachmentRetention(AttachmentPolicyRemove, []ResolvedSubtitleTrack{{Codec: "ass", Action: SubtitleDispositionKeep}})
	if err != nil || keep {
		t.Fatalf("remove policy was not authoritative: keep=%t err=%v", keep, err)
	}
}

func TestResolvedTrackPlanSerializesCanonicalDecisions(t *testing.T) {
	plan := ResolvedTrackPlan{
		VideoStreams:      []ResolvedTrackStream{{StreamIndex: 0, Codec: "mpeg2video"}},
		AudioStreams:      []ResolvedTrackStream{{StreamIndex: 1, Codec: "ac3", Language: "eng"}},
		SubtitleStreams:   []ResolvedSubtitleTrack{{StreamIndex: 2, Codec: "ass", Language: "eng", Action: SubtitleDispositionKeepAndExtract}},
		AttachmentPolicy:  AttachmentPolicyAuto,
		AttachmentStreams: []ResolvedAttachmentStream{{StreamIndex: 3, Codec: "ttf", Filename: "Font.ttf", MIMEType: "font/ttf", AttachmentKind: "FONT", FontFormat: "TTF"}},
		ChapterPolicy:     ChapterPolicyKeep,
		SidecarOutputs:    []ResolvedTrackSidecar{{StreamIndex: 2, Codec: "ass", Language: "eng", Format: "ass"}},
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ResolvedTrackPlan
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.SubtitleStreams) != 1 || decoded.SubtitleStreams[0].Action != SubtitleDispositionKeepAndExtract || decoded.AttachmentPolicy != AttachmentPolicyAuto || decoded.ChapterPolicy != ChapterPolicyKeep {
		t.Fatalf("resolved plan lost canonical decisions: %#v", decoded)
	}
	if len(decoded.AttachmentStreams) != 1 || decoded.AttachmentStreams[0].Filename != "Font.ttf" || decoded.AttachmentStreams[0].MIMEType != "font/ttf" || decoded.AttachmentStreams[0].AttachmentKind != "FONT" || decoded.AttachmentStreams[0].FontFormat != "TTF" {
		t.Fatalf("resolved plan lost attachment metadata: %#v", decoded.AttachmentStreams)
	}
}
