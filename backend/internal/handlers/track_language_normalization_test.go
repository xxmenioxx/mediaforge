package handlers

import (
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNormalizedTrackLanguageValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: " KO ", want: "kor"},
		{input: "ko-KR", want: "kor"},
		{input: "KO_kr", want: "kor"},
		{input: "KOR", want: "kor"},
		{input: "fr", want: "fra"},
		{input: "Fr-FR", want: "fra"},
		{input: "fr_FR", want: "fra"},
		{input: "FRE", want: "fra"},
		{input: "FRA", want: "fra"},
		{input: "es", want: "spa"},
		{input: "ESP", want: "spa"},
		{input: "EN", want: "eng"},
		{input: "JA", want: "jpn"},
		{input: "GER", want: "deu"},
		{input: "CHI", want: "zho"},
		{input: "UND", want: "und"},
		{input: "", want: "und"},
		{input: " FR-CA ", want: "fr-ca"},
		{input: " X-private ", want: "x-private"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			if got := normalizedTrackLanguageValue(test.input); got != test.want {
				t.Fatalf("normalizedTrackLanguageValue(%q)=%q, want %q", test.input, got, test.want)
			}
		})
	}

	for _, value := range []string{"ko-KR", "kor", "fr-FR", "fre", "fra", "ESP"} {
		if _, ok := canonicalSubtitleLanguage(value); !ok {
			t.Errorf("canonicalSubtitleLanguage(%q) rejected an existing catalog alias", value)
		}
	}
	for _, value := range []string{"und", "fr-CA", "x-private"} {
		if _, ok := canonicalSubtitleLanguage(value); ok {
			t.Errorf("canonicalSubtitleLanguage(%q) accepted a value outside the subtitle catalog", value)
		}
	}
}

func TestNormalizedTrackLanguageListPreservesOrderAndDeduplicatesAliases(t *testing.T) {
	got := normalizedTrackLanguageList([]string{"KO", "kor", " fr-FR ", "fre", "en", "FR-CA", ""})
	want := models.JSONList{"kor", "fra", "eng", "fr-ca"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized languages=%#v, want %#v", got, want)
	}
}

func TestResolveTrackProfileForAssetMatchesCanonicalLanguagesWithoutCollapsingIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:track-language-normalization?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Clean("/media/raw/show/episode.mkv")
	scan := models.ScanResult{
		Path: path,
		AudioStreams: models.JSONList{
			map[string]any{"index": 1, "language": "ko"},
			map[string]any{"index": 2, "language": "ko-KR"},
			map[string]any{"index": 3, "language": "kor"},
			map[string]any{"index": 4, "language": "fr-CA"},
		},
		SubtitleStreams: models.JSONList{
			map[string]any{"index": 5, "codec": "subrip", "language": "fr"},
			map[string]any{"index": 6, "codec": "ass", "language": "fr-FR"},
			map[string]any{"index": 7, "codec": "subrip", "language": "fre"},
			map[string]any{"index": 8, "codec": "subrip", "language": "fr-CA"},
			map[string]any{"index": 9, "codec": "subrip", "language": "und"},
		},
	}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	profile := map[string]any{
		"audioMode": "languages", "audioLanguages": models.JSONList{"ko-KR"}, "dropCommentary": false,
		"subtitleMode": "languages", "subtitleLanguages": models.JSONList{"fr-FR"},
		"subtitleDisposition": "keep", "attachmentPolicy": "auto", "chapterPolicy": "keep",
	}
	resolved := resolveTrackProfileForAsset(db, path, profile)
	if got, want := workerSliceValue(resolved["keepAudioStreams"]), []any{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("audio indexes=%#v, want %#v", got, want)
	}
	if got, want := workerSliceValue(resolved["keepSubtitleStreams"]), []any{5, 6, 7}; !reflect.DeepEqual(got, want) {
		t.Fatalf("subtitle indexes=%#v, want %#v", got, want)
	}
	plan, err := resolveTrackPlan(scan, resolved)
	if err != nil {
		t.Fatal(err)
	}
	command := shellJoin(FFmpegCommandBuilder{}.Build(MediaJobPlan{
		InputPath: "/media/raw/episode.mkv", OutputPath: "/media/staging/episode.mkv", Overwrite: true,
		Profile: models.Profile{VideoCodec: "copy", AudioCodec: "copy", PreserveSubtitles: true, PreserveChapters: true},
		Streams: MediaStreamInventory{
			Video:    []MediaStream{{Index: 0}},
			Audio:    []MediaAudioStream{{Index: 1}, {Index: 2}, {Index: 3}, {Index: 4}},
			Subtitle: []MediaStream{{Index: 5}, {Index: 6}, {Index: 7}, {Index: 8}, {Index: 9}},
		},
		ResolvedTracks: &plan,
	}))
	for _, index := range []int{1, 2, 3, 5, 6, 7} {
		assertContains(t, command, "-map 0:"+strconv.Itoa(index))
	}
	for _, index := range []int{4, 8, 9} {
		assertNotContains(t, command, "-map 0:"+strconv.Itoa(index))
	}

	var stored models.ScanResult
	if err := db.First(&stored, "path = ?", path).Error; err != nil {
		t.Fatal(err)
	}
	if workerStringValue(settingProfileObject(stored.AudioStreams[1])["language"]) != "ko-KR" || workerStringValue(settingProfileObject(stored.SubtitleStreams[1])["language"]) != "fr-FR" {
		t.Fatalf("source tags were rewritten: audio=%#v subtitles=%#v", stored.AudioStreams, stored.SubtitleStreams)
	}

	explicit := map[string]any{
		"audioMode": "languages", "audioLanguages": []string{"kor"}, "keepAudioStreams": []int{2},
		"subtitleMode": "languages", "subtitleLanguages": []string{"fra"}, "keepSubtitleStreams": []int{},
	}
	explicit = resolveTrackProfileForAsset(db, path, explicit)
	if got, want := workerSliceValue(explicit["keepAudioStreams"]), []any{2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit audio indexes=%#v, want %#v", got, want)
	}
	if got := workerSliceValue(explicit["keepSubtitleStreams"]); got == nil || len(got) != 0 {
		t.Fatalf("explicit empty subtitle selection was replaced: %#v", explicit["keepSubtitleStreams"])
	}
}

func TestResolveDefaultTrackLanguagesAndRulesUseCanonicalAliases(t *testing.T) {
	scan := models.ScanResult{
		AudioStreams: models.JSONList{map[string]any{"index": 1, "codec": "aac", "language": "ko-KR"}},
		SubtitleStreams: models.JSONList{
			map[string]any{"index": 2, "codec": "subrip", "language": "fr-FR"},
			map[string]any{"index": 3, "codec": "ass", "language": "fre"},
		},
	}
	profile := map[string]any{
		"keepAudioStreams": []int{1}, "keepSubtitleStreams": []int{2, 3},
		"defaultAudioLanguage": "KOR", "defaultSubtitleLanguage": "FR",
		"subtitleDisposition": "keep",
		"attachmentPolicy":    "auto", "chapterPolicy": "keep",
	}
	defaultPlan, err := resolveTrackPlan(scan, profile)
	if err != nil {
		t.Fatal(err)
	}
	if defaultPlan.DefaultAudio.Status != "resolved" || defaultPlan.DefaultAudio.StreamIndex == nil || *defaultPlan.DefaultAudio.StreamIndex != 1 {
		t.Fatalf("default audio alias did not resolve: %#v", defaultPlan.DefaultAudio)
	}
	if defaultPlan.DefaultSubtitle.Status != "resolved" || defaultPlan.DefaultSubtitle.StreamIndex == nil || *defaultPlan.DefaultSubtitle.StreamIndex != 2 {
		t.Fatalf("default subtitle alias did not resolve: %#v", defaultPlan.DefaultSubtitle)
	}
	ruleProfile := map[string]any{}
	for key, value := range profile {
		ruleProfile[key] = value
	}
	ruleProfile["subtitleRules"] = []any{map[string]any{"language": "FRE", "action": "extract"}}
	ruleProfile["subtitleSidecarFormats"] = []string{"srt"}
	plan, err := resolveTrackPlan(scan, ruleProfile)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SubtitleStreams) != 2 || plan.SubtitleStreams[0].Action != SubtitleDispositionExtract || plan.SubtitleStreams[1].Action != SubtitleDispositionExtract || plan.SubtitleStreams[0].StreamIndex == plan.SubtitleStreams[1].StreamIndex {
		t.Fatalf("same-language subtitle indexes were not kept distinct: %#v", plan.SubtitleStreams)
	}
	if len(plan.SidecarOutputs) != 2 || plan.SubtitleStreams[0].Language != "fra" || plan.SidecarOutputs[0].Language != "fr-fr" || plan.SidecarOutputs[1].Language != "fre" {
		t.Fatalf("canonical matching changed regional sidecar naming or aliases: streams=%#v sidecars=%#v", plan.SubtitleStreams, plan.SidecarOutputs)
	}
}
