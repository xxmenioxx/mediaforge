package handlers

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func assetSequenceLess(left, right string) bool {
	leftName := filepath.Base(filepath.Clean(left))
	rightName := filepath.Base(filepath.Clean(right))

	comparison := naturalAssetCompare(leftName, rightName)
	if comparison != 0 {
		return comparison < 0
	}

	// Deterministic fallback when the filenames compare equivalently.
	return left < right
}

func naturalAssetCompare(left, right string) int {
	leftRunes := []rune(strings.ToLower(left))
	rightRunes := []rune(strings.ToLower(right))

	leftIndex := 0
	rightIndex := 0

	for leftIndex < len(leftRunes) && rightIndex < len(rightRunes) {
		leftDigit := unicode.IsDigit(leftRunes[leftIndex])
		rightDigit := unicode.IsDigit(rightRunes[rightIndex])

		if leftDigit && rightDigit {
			leftEnd := leftIndex
			for leftEnd < len(leftRunes) && unicode.IsDigit(leftRunes[leftEnd]) {
				leftEnd++
			}

			rightEnd := rightIndex
			for rightEnd < len(rightRunes) && unicode.IsDigit(rightRunes[rightEnd]) {
				rightEnd++
			}

			leftNumber := string(leftRunes[leftIndex:leftEnd])
			rightNumber := string(rightRunes[rightIndex:rightEnd])

			if comparison := compareNumericAssetRuns(leftNumber, rightNumber); comparison != 0 {
				return comparison
			}

			leftIndex = leftEnd
			rightIndex = rightEnd
			continue
		}

		if leftRunes[leftIndex] < rightRunes[rightIndex] {
			return -1
		}
		if leftRunes[leftIndex] > rightRunes[rightIndex] {
			return 1
		}

		leftIndex++
		rightIndex++
	}

	switch {
	case leftIndex < len(leftRunes):
		return 1
	case rightIndex < len(rightRunes):
		return -1
	default:
		return 0
	}
}

func compareNumericAssetRuns(left, right string) int {
	leftTrimmed := strings.TrimLeft(left, "0")
	rightTrimmed := strings.TrimLeft(right, "0")

	if leftTrimmed == "" {
		leftTrimmed = "0"
	}
	if rightTrimmed == "" {
		rightTrimmed = "0"
	}

	// Compare number magnitude without converting to int. This avoids
	// overflow for filenames containing unusually long numeric runs.
	if len(leftTrimmed) < len(rightTrimmed) {
		return -1
	}
	if len(leftTrimmed) > len(rightTrimmed) {
		return 1
	}

	if leftTrimmed < rightTrimmed {
		return -1
	}
	if leftTrimmed > rightTrimmed {
		return 1
	}

	// Same numeric value. Prefer fewer leading zeros:
	// 1 before 01 before 001.
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}

	return 0
}

func episodeSequencePositions(
	sourcePath string,
	records []models.AssetRecord,
) map[string]int {
	sourcePath = filepath.Clean(sourcePath)

	groups := make(map[string][]models.AssetRecord)

	for _, record := range records {
		recordPath := filepath.Clean(record.Path)

		relative, err := filepath.Rel(sourcePath, recordPath)
		if err != nil {
			continue
		}

		if relative == ".." ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}

		groupPath := filepath.Dir(relative)
		if groupPath == "." {
			groupPath = ""
		}

		groups[groupPath] = append(groups[groupPath], record)
	}

	positions := make(map[string]int, len(records))

	for _, groupRecords := range groups {
		sort.SliceStable(groupRecords, func(i, j int) bool {
			return assetSequenceLess(
				groupRecords[i].Path,
				groupRecords[j].Path,
			)
		})

		for index, record := range groupRecords {
			positions[record.Path] = index + 1
		}
	}

	return positions
}
