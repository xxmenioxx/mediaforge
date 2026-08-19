package handlers

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestAssetSequenceNaturalOrder(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name: "zero based numeric sequence",
			input: []string{
				"02.mkv",
				"00.mkv",
				"01.mkv",
			},
			expected: []string{
				"00.mkv",
				"01.mkv",
				"02.mkv",
			},
		},
		{
			name: "one based numeric sequence",
			input: []string{
				"10.mkv",
				"02.mkv",
				"01.mkv",
			},
			expected: []string{
				"01.mkv",
				"02.mkv",
				"10.mkv",
			},
		},
		{
			name: "base file followed by numbered files",
			input: []string{
				"title_02.mkv",
				"title.mkv",
				"title_01.mkv",
			},
			expected: []string{
				"title.mkv",
				"title_01.mkv",
				"title_02.mkv",
			},
		},
		{
			name: "number directly appended",
			input: []string{
				"asset10.mkv",
				"asset2.mkv",
				"asset.mkv",
				"asset1.mkv",
			},
			expected: []string{
				"asset.mkv",
				"asset1.mkv",
				"asset2.mkv",
				"asset10.mkv",
			},
		},
		{
			name: "episode style numbering",
			input: []string{
				"Show S01E10.mkv",
				"Show S01E2.mkv",
				"Show S01E1.mkv",
			},
			expected: []string{
				"Show S01E1.mkv",
				"Show S01E2.mkv",
				"Show S01E10.mkv",
			},
		},
		{
			name: "numeric prefix with episode title",
			input: []string{
				"10-El Final.mkv",
				"02-La Batalla.mkv",
				"01-El Momento De Decisión.mkv",
			},
			expected: []string{
				"01-El Momento De Decisión.mkv",
				"02-La Batalla.mkv",
				"10-El Final.mkv",
			},
		},
		{
			name: "season x episode notation",
			input: []string{
				"1X10 - El final.mp4",
				"1X02 - Segundo.mp4",
				"1X01 - El ciclope o la maldición de los dioses.mp4",
			},
			expected: []string{
				"1X01 - El ciclope o la maldición de los dioses.mp4",
				"1X02 - Segundo.mp4",
				"1X10 - El final.mp4",
			},
		},
		{
			name: "episode number before release suffix",
			input: []string{
				"Arbegas_El rayo custodio_10_[Ladyarmaroid.org].mkv",
				"Arbegas_El rayo custodio_02_[Ladyarmaroid.org].mkv",
				"Arbegas_El rayo custodio_01_[Ladyarmaroid.org].mkv",
			},
			expected: []string{
				"Arbegas_El rayo custodio_01_[Ladyarmaroid.org].mkv",
				"Arbegas_El rayo custodio_02_[Ladyarmaroid.org].mkv",
				"Arbegas_El rayo custodio_10_[Ladyarmaroid.org].mkv",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := append([]string(nil), tt.input...)

			sort.SliceStable(got, func(i, j int) bool {
				return assetSequenceLess(got[i], got[j])
			})

			if len(got) != len(tt.expected) {
				t.Fatalf("got %d items, want %d", len(got), len(tt.expected))
			}

			for index := range got {
				if got[index] != tt.expected[index] {
					t.Fatalf(
						"position %d = %q, want %q; complete order=%v",
						index,
						got[index],
						tt.expected[index],
						got,
					)
				}
			}
		})
	}
}

func TestSortAssetsOrdersSequenceIndependentlyPerGroupPath(t *testing.T) {
	assets := []Asset{
		{
			LibraryName:  "Anime",
			GroupPath:    "Series/Season 02",
			RelativePath: "Series/Season 02/title_02.mkv",
			FileName:     "title_02.mkv",
		},
		{
			LibraryName:  "Anime",
			GroupPath:    "Series/Season 01",
			RelativePath: "Series/Season 01/10-El Final.mkv",
			FileName:     "10-El Final.mkv",
		},
		{
			LibraryName:  "Anime",
			GroupPath:    "Series/Season 02",
			RelativePath: "Series/Season 02/title.mkv",
			FileName:     "title.mkv",
		},
		{
			LibraryName:  "Anime",
			GroupPath:    "Series/Season 01",
			RelativePath: "Series/Season 01/02-Segundo.mkv",
			FileName:     "02-Segundo.mkv",
		},
		{
			LibraryName:  "Anime",
			GroupPath:    "Series/Season 02",
			RelativePath: "Series/Season 02/title_01.mkv",
			FileName:     "title_01.mkv",
		},
		{
			LibraryName:  "Anime",
			GroupPath:    "Series/Season 01",
			RelativePath: "Series/Season 01/01-Primero.mkv",
			FileName:     "01-Primero.mkv",
		},
	}

	sortAssets(assets)

	expected := []string{
		"Series/Season 01/01-Primero.mkv",
		"Series/Season 01/02-Segundo.mkv",
		"Series/Season 01/10-El Final.mkv",

		"Series/Season 02/title.mkv",
		"Series/Season 02/title_01.mkv",
		"Series/Season 02/title_02.mkv",
	}

	for index := range expected {
		if assets[index].RelativePath != expected[index] {
			t.Fatalf(
				"position %d = %q, want %q; complete order=%v",
				index,
				assets[index].RelativePath,
				expected[index],
				assets,
			)
		}
	}
}

func TestEpisodeSequencePositionsResetPerContainingPath(t *testing.T) {
	sourcePath := "/media/raw/Anime/Show"

	records := []models.AssetRecord{
		{
			Path: filepath.Join(
				sourcePath,
				"Season 01",
				"title_02.mkv",
			),
		},
		{
			Path: filepath.Join(
				sourcePath,
				"Season 02",
				"02-El tercero.mkv",
			),
		},
		{
			Path: filepath.Join(
				sourcePath,
				"Season 01",
				"title.mkv",
			),
		},
		{
			Path: filepath.Join(
				sourcePath,
				"Season 02",
				"00-El primero.mkv",
			),
		},
		{
			Path: filepath.Join(
				sourcePath,
				"Season 01",
				"title_01.mkv",
			),
		},
		{
			Path: filepath.Join(
				sourcePath,
				"Season 02",
				"01-El segundo.mkv",
			),
		},
	}

	positions := episodeSequencePositions(sourcePath, records)

	expected := map[string]int{
		filepath.Join(sourcePath, "Season 01", "title.mkv"):    1,
		filepath.Join(sourcePath, "Season 01", "title_01.mkv"): 2,
		filepath.Join(sourcePath, "Season 01", "title_02.mkv"): 3,

		filepath.Join(sourcePath, "Season 02", "00-El primero.mkv"): 1,
		filepath.Join(sourcePath, "Season 02", "01-El segundo.mkv"): 2,
		filepath.Join(sourcePath, "Season 02", "02-El tercero.mkv"): 3,
	}

	if len(positions) != len(expected) {
		t.Fatalf(
			"positions=%d want=%d: %#v",
			len(positions),
			len(expected),
			positions,
		)
	}

	for path, want := range expected {
		if got := positions[path]; got != want {
			t.Fatalf(
				"episode position for %q = %d, want %d",
				path,
				got,
				want,
			)
		}
	}
}
