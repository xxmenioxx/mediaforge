package handlers

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxTranslationContextSource = 256

type translationContextStatus string

const (
	translationContextUnavailable translationContextStatus = "unavailable"
	translationContextAvailable   translationContextStatus = "available"
)

// translationMediaContextInput is operation-local metadata already resolved
// by a caller. It does not fetch metadata and is never part of cue identity.
type translationMediaContextInput struct {
	Media  TranslationMediaContext
	Source string
}

type translationContextMetadata struct {
	Status translationContextStatus
	Source string
}

type normalizedTranslationMediaContext struct {
	Media    TranslationMediaContext
	Metadata translationContextMetadata
}

func translationBatchesForCuesWithMediaContext(sourceLanguage, targetLocale string, cues []TranslationCue, input translationMediaContextInput, settings translationSettings) ([]TranslationBatch, translationContextMetadata, error) {
	normalized, err := normalizeTranslationMediaContext(input, settings)
	if err != nil {
		return nil, translationContextMetadata{}, err
	}
	batches, err := translationBatchesForCuesWithContextBuilder(sourceLanguage, targetLocale, cues, func(start, end int, allCues []TranslationCue, sourceLanguage, targetLocale string, settings translationSettings) (*TranslationContext, error) {
		return boundedTranslationContextForRange(start, end, allCues, normalized.Media, sourceLanguage, targetLocale, settings)
	}, settings)
	if err != nil {
		return nil, normalized.Metadata, err
	}
	return batches, normalized.Metadata, nil
}

func normalizeTranslationMediaContext(input translationMediaContextInput, settings translationSettings) (normalizedTranslationMediaContext, error) {
	if settings.MaxContextChars < 0 {
		return normalizedTranslationMediaContext{}, fmt.Errorf("translation context character limit must not be negative")
	}
	source := strings.TrimSpace(input.Source)
	if len(source) > maxTranslationContextSource {
		return normalizedTranslationMediaContext{}, fmt.Errorf("translation context source exceeds %d characters", maxTranslationContextSource)
	}
	media := TranslationMediaContext{
		Title:          strings.TrimSpace(input.Media.Title),
		Year:           strings.TrimSpace(input.Media.Year),
		MovieSeries:    strings.TrimSpace(input.Media.MovieSeries),
		Season:         strings.TrimSpace(input.Media.Season),
		Episode:        strings.TrimSpace(input.Media.Episode),
		Synopsis:       strings.TrimSpace(input.Media.Synopsis),
		CharacterNames: normalizedTranslationCharacterNames(input.Media.CharacterNames),
	}
	highPrioritySize := translationMediaContextSize(TranslationMediaContext{
		Title:          media.Title,
		Year:           media.Year,
		MovieSeries:    media.MovieSeries,
		Season:         media.Season,
		Episode:        media.Episode,
		CharacterNames: media.CharacterNames,
	})
	if highPrioritySize > settings.MaxContextChars {
		return normalizedTranslationMediaContext{}, fmt.Errorf("required translation media context exceeds %d characters", settings.MaxContextChars)
	}
	media.Synopsis = truncateTranslationUTF8(media.Synopsis, settings.MaxContextChars-highPrioritySize)
	available := highPrioritySize > 0 || media.Synopsis != ""
	status := translationContextUnavailable
	if available {
		status = translationContextAvailable
	}
	return normalizedTranslationMediaContext{
		Media: media,
		Metadata: translationContextMetadata{
			Status: status,
			Source: source,
		},
	}, nil
}

func normalizedTranslationCharacterNames(names []string) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			result = append(result, name)
		}
	}
	return result
}

type translationContextCandidate struct {
	Index    int
	Distance int
}

func boundedTranslationContextForRange(start, end int, cues []TranslationCue, media TranslationMediaContext, sourceLanguage, targetLocale string, settings translationSettings) (*TranslationContext, error) {
	if settings.MaxContextCues < 0 || settings.MaxContextChars < 0 {
		return nil, fmt.Errorf("translation context limits must not be negative")
	}
	highPriorityMedia := media
	highPriorityMedia.Synopsis = ""
	highPrioritySize := translationMediaContextSize(highPriorityMedia)
	if highPrioritySize > settings.MaxContextChars {
		return nil, fmt.Errorf("required translation media context exceeds %d characters", settings.MaxContextChars)
	}

	candidates := translationContextCandidates(start, end, len(cues), settings.MaxContextCues)
	for {
		neighboring := make([]TranslationContextCue, 0, len(candidates))
		neighborSize := 0
		for _, candidate := range candidates {
			cue := cues[candidate.Index]
			neighboring = append(neighboring, TranslationContextCue{ID: cue.ID, Text: cue.Text})
			neighborSize += len(cue.ID) + len(cue.Text)
		}
		remaining := settings.MaxContextChars - highPrioritySize - neighborSize
		if remaining >= 0 {
			boundedMedia := highPriorityMedia
			boundedMedia.Synopsis = truncateTranslationUTF8(media.Synopsis, remaining)
			context := translationContextWithCandidates(boundedMedia, neighboring, candidates)
			if context == nil {
				return nil, nil
			}
			if translationContextFitsRequest(start, end, cues, sourceLanguage, targetLocale, context, settings.MaxRequestBytes) {
				return context, nil
			}
			if boundedMedia.Synopsis != "" {
				boundedMedia.Synopsis = largestTranslationSynopsisForRequest(start, end, cues, sourceLanguage, targetLocale, boundedMedia, neighboring, candidates, settings.MaxRequestBytes)
				context = translationContextWithCandidates(boundedMedia, neighboring, candidates)
				if context != nil && translationContextFitsRequest(start, end, cues, sourceLanguage, targetLocale, context, settings.MaxRequestBytes) {
					return context, nil
				}
			}
			if len(candidates) == 0 {
				return context, nil
			}
			candidates = candidates[:len(candidates)-1]
			continue
		}
		if len(candidates) == 0 {
			return nil, fmt.Errorf("translation media context cannot fit within %d characters", settings.MaxContextChars)
		}
		candidates = candidates[:len(candidates)-1]
	}
}

func translationContextWithCandidates(media TranslationMediaContext, neighboring []TranslationContextCue, candidates []translationContextCandidate) *TranslationContext {
	context := &TranslationContext{NeighboringCues: translationContextCuesInSourceOrder(neighboring, candidates), Media: media}
	if len(context.NeighboringCues) == 0 && translationMediaContextSize(context.Media) == 0 {
		return nil
	}
	return context
}

func translationContextFitsRequest(start, end int, cues []TranslationCue, sourceLanguage, targetLocale string, context *TranslationContext, maximumBytes int) bool {
	batch := TranslationBatch{
		ContractVersion: translationProviderContractVersion,
		SourceLanguage:  sourceLanguage,
		TargetLocale:    targetLocale,
		Cues:            cues[start:end],
		Context:         context,
	}
	encoded, err := json.Marshal(batch)
	return err == nil && len(encoded) <= maximumBytes
}

func largestTranslationSynopsisForRequest(start, end int, cues []TranslationCue, sourceLanguage, targetLocale string, media TranslationMediaContext, neighboring []TranslationContextCue, candidates []translationContextCandidate, maximumBytes int) string {
	low, high := 0, len(media.Synopsis)
	best := ""
	for low <= high {
		middle := low + (high-low)/2
		candidateMedia := media
		candidateMedia.Synopsis = truncateTranslationUTF8(media.Synopsis, middle)
		context := translationContextWithCandidates(candidateMedia, neighboring, candidates)
		if context != nil && translationContextFitsRequest(start, end, cues, sourceLanguage, targetLocale, context, maximumBytes) {
			best = candidateMedia.Synopsis
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	return best
}

func translationContextCandidates(start, end, cueCount, maximum int) []translationContextCandidate {
	if maximum == 0 {
		return nil
	}
	candidates := make([]translationContextCandidate, 0, maximum)
	for distance := 1; len(candidates) < maximum; distance++ {
		if previous := start - distance; previous >= 0 {
			candidates = append(candidates, translationContextCandidate{Index: previous, Distance: distance})
			if len(candidates) == maximum {
				break
			}
		}
		if following := end - 1 + distance; following < cueCount {
			candidates = append(candidates, translationContextCandidate{Index: following, Distance: distance})
		}
		if start-distance < 0 && end-1+distance >= cueCount {
			break
		}
	}
	return candidates
}

func translationContextCuesInSourceOrder(cues []TranslationContextCue, candidates []translationContextCandidate) []TranslationContextCue {
	ordered := append([]translationContextCandidate(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Index < ordered[j].Index })
	result := make([]TranslationContextCue, 0, len(ordered))
	for _, candidate := range ordered {
		for index, original := range candidates {
			if original.Index == candidate.Index {
				result = append(result, cues[index])
				break
			}
		}
	}
	return result
}

func translationMediaContextSize(media TranslationMediaContext) int {
	size := len(media.Title) + len(media.Year) + len(media.MovieSeries) + len(media.Season) + len(media.Episode) + len(media.Synopsis)
	for _, name := range media.CharacterNames {
		size += len(name)
	}
	return size
}

func truncateTranslationUTF8(value string, maximumBytes int) string {
	if maximumBytes <= 0 {
		return ""
	}
	if len(value) <= maximumBytes {
		return value
	}
	value = value[:maximumBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
