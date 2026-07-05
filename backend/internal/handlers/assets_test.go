package handlers

import (
	"testing"
	"time"
)

func TestLogicalAssetGroupPathUsesTopLevelFolder(t *testing.T) {
	tests := map[string]string{
		"1.mkv":                             "",
		"movies/1.mkv":                      "movies",
		"movies/movie1/movie.mkv":           "movies/movie1",
		"movies/movie1/extras/extra1.mkv":   "movies/movie1",
		"series/show/season1/episode01.mkv": "series/show",
		"series/show/season2/episode01.mkv": "series/show",
	}

	for input, expected := range tests {
		if actual := logicalAssetGroupPath(input); actual != expected {
			t.Fatalf("logicalAssetGroupPath(%q) = %q, expected %q", input, actual, expected)
		}
	}
}

func TestMergeAssetMetadataStateDeduplicatesCategoriesAndTags(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	merged := mergeAssetMetadataState(
		AssetMetadataState{
			Categories: []string{"anime", "movie"},
			Tags:       []string{"dvd-source"},
			UpdatedAt:  older,
		},
		AssetMetadataState{
			Categories: []string{"Anime", "extras"},
			Tags:       []string{"dvd-source", "mono"},
			UpdatedAt:  newer,
		},
	)

	if !merged.UpdatedAt.Equal(newer) {
		t.Fatalf("expected newest update timestamp")
	}
	assertStringList(t, merged.Categories, []string{"anime", "movie", "extras"})
	assertStringList(t, merged.Tags, []string{"dvd-source", "mono"})
}

func TestAssetKeySetUsesRelativePathToAvoidBasenameCollisions(t *testing.T) {
	keys := assetKeySet([]Asset{
		{RelativePath: "movies/Pearl Jam MSG Disc Two (2003)_t08.mkv", FileName: "Pearl Jam MSG Disc Two (2003)_t08.mkv"},
	})

	if _, ok := keys[assetKey("movies/Pearl Jam MSG Disc Two (2003)_t08.mkv")]; !ok {
		t.Fatalf("expected converted key to include relative path")
	}
	if _, ok := keys[assetKey("series/SIMPSONS_SEASON5_D1/extras/Pearl Jam MSG Disc Two (2003)_t08.mkv")]; ok {
		t.Fatalf("did not expect basename collision from a different source path")
	}
}

func assertStringList(t *testing.T, actual []string, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("actual=%v expected=%v", actual, expected)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("actual=%v expected=%v", actual, expected)
		}
	}
}
