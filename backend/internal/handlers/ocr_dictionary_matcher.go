package handlers

import (
	"sort"
	"strings"
	"unicode"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

type ocrDictionaryTextMatch struct {
	Start        int      `json:"start"`
	End          int      `json:"end"`
	Before       string   `json:"before"`
	After        string   `json:"after"`
	RuleIDs      []string `json:"ruleIds"`
	RuleVersions []int    `json:"ruleVersions"`
	Priority     int      `json:"priority"`
}

type ocrDictionaryAmbiguity struct {
	Start   int      `json:"start"`
	End     int      `json:"end"`
	RuleIDs []string `json:"ruleIds"`
}

type ocrDictionaryMatchResult struct {
	Text      string                   `json:"text"`
	Matches   []ocrDictionaryTextMatch `json:"matches"`
	Ambiguous []ocrDictionaryAmbiguity `json:"ambiguous,omitempty"`
}

type ocrDictionaryTextCandidate struct {
	start int
	end   int
	rule  models.OCRDictionaryEntry
}

type ocrDictionaryCandidateKey struct {
	id    string
	start int
}

func applyOCRDictionaryRules(text, language string, rules []models.OCRDictionaryEntry) (string, int) {
	result := matchOCRDictionaryText(text, language, rules)
	return result.Text, len(result.Matches)
}

func matchOCRDictionaryText(text, language string, rules []models.OCRDictionaryEntry) ocrDictionaryMatchResult {
	result := ocrDictionaryMatchResult{Text: text, Matches: []ocrDictionaryTextMatch{}}
	canonicalLanguage, ok := strictOCRDictionaryLanguage(language)
	if !ok || text == "" || len(text) > 64*1024 || len(rules) == 0 || len(rules) > ocrDictionaryMaxEntries {
		return result
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return result
	}
	byPrefix := make(map[rune][]models.OCRDictionaryEntry)
	for _, rule := range rules {
		if !rule.Enabled || rule.Language != canonicalLanguage || rule.MatchMode != "literal" || rule.BoundaryPolicy != "word" || len(rule.Pattern) == 0 {
			continue
		}
		pattern := []rune(rule.Pattern)
		if len(pattern) == 0 || (rule.CasePolicy != "exact" && rule.CasePolicy != "insensitive") {
			continue
		}
		prefix := pattern[0]
		byPrefix[prefix] = append(byPrefix[prefix], rule)
		if rule.CasePolicy == "insensitive" {
			lower := unicode.ToLower(prefix)
			if lower != prefix {
				byPrefix[lower] = append(byPrefix[lower], rule)
			}
		}
	}
	if len(byPrefix) == 0 {
		return result
	}
	structural := ocrStructuralRuneMask(runes)
	candidates := make([]ocrDictionaryTextCandidate, 0)
	seen := make(map[ocrDictionaryCandidateKey]struct{})
	for start, current := range runes {
		candidateRules := byPrefix[current]
		lowerRules := byPrefix[unicode.ToLower(current)]
		for _, rule := range lowerRules {
			if rule.CasePolicy == "insensitive" {
				candidateRules = append(candidateRules, rule)
			}
		}
		for _, rule := range candidateRules {
			pattern := []rune(rule.Pattern)
			end := start + len(pattern)
			if end > len(runes) || !ocrDictionaryLiteralMatches(runes[start:end], pattern, rule.CasePolicy) {
				continue
			}
			key := ocrDictionaryCandidateKey{id: rule.ID, start: start}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			if !ocrDictionaryBoundaryMatches(runes, start, end) || !ocrDictionaryContextMatches(runes, start, end, rule) || !ocrDictionaryBuiltInEnglishContextMatches(runes, structural, start, end, rule) {
				continue
			}
			structuralHit := false
			for index := start; index < end; index++ {
				if structural[index] {
					structuralHit = true
					break
				}
			}
			if structuralHit {
				continue
			}
			candidates = append(candidates, ocrDictionaryTextCandidate{start: start, end: end, rule: rule})
		}
	}
	if len(candidates) == 0 {
		return result
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].start != candidates[j].start {
			return candidates[i].start < candidates[j].start
		}
		if candidates[i].end != candidates[j].end {
			return candidates[i].end < candidates[j].end
		}
		if candidates[i].rule.Priority != candidates[j].rule.Priority {
			return candidates[i].rule.Priority > candidates[j].rule.Priority
		}
		return candidates[i].rule.ID < candidates[j].rule.ID
	})

	selected := make([]ocrDictionaryTextCandidate, 0, len(candidates))
	for first := 0; first < len(candidates); {
		last := first + 1
		componentEnd := candidates[first].end
		for last < len(candidates) && candidates[last].start < componentEnd {
			if candidates[last].end > componentEnd {
				componentEnd = candidates[last].end
			}
			last++
		}
		component := candidates[first:last]
		highest := component[0].rule.Priority
		for _, candidate := range component[1:] {
			if candidate.rule.Priority > highest {
				highest = candidate.rule.Priority
			}
		}
		top := make([]ocrDictionaryTextCandidate, 0, len(component))
		for _, candidate := range component {
			if candidate.rule.Priority == highest {
				top = append(top, candidate)
			}
		}
		if sameOCRDictionaryMatch(top) {
			selected = append(selected, top[0])
			if len(top) > 1 {
				ids, versions := ocrDictionaryRuleEvidence(top)
				result.Matches = append(result.Matches, ocrDictionaryTextMatch{
					Start: top[0].start, End: top[0].end,
					Before: string(runes[top[0].start:top[0].end]), After: top[0].rule.Replacement,
					RuleIDs: ids, RuleVersions: versions, Priority: highest,
				})
			}
		} else if len(top) == 1 {
			selected = append(selected, top[0])
		} else {
			ids := make([]string, 0, len(top))
			start, end := top[0].start, top[0].end
			for _, candidate := range top {
				ids = append(ids, candidate.rule.ID)
				if candidate.start < start {
					start = candidate.start
				}
				if candidate.end > end {
					end = candidate.end
				}
			}
			sort.Strings(ids)
			result.Ambiguous = append(result.Ambiguous, ocrDictionaryAmbiguity{Start: start, End: end, RuleIDs: ids})
		}
		first = last
	}

	// Components above are sorted and disjoint. Build output once so all text
	// outside each literal match remains byte-for-byte equivalent in UTF-8.
	var output strings.Builder
	cursor := 0
	for _, match := range selected {
		if match.start < cursor {
			continue
		}
		output.WriteString(string(runes[cursor:match.start]))
		output.WriteString(match.rule.Replacement)
		cursor = match.end
		if len(match.rule.ID) > 0 {
			alreadyRecorded := false
			for _, recorded := range result.Matches {
				if recorded.Start == match.start && recorded.End == match.end {
					alreadyRecorded = true
					break
				}
			}
			if !alreadyRecorded {
				result.Matches = append(result.Matches, ocrDictionaryTextMatch{
					Start: match.start, End: match.end,
					Before: string(runes[match.start:match.end]), After: match.rule.Replacement,
					RuleIDs: []string{match.rule.ID}, RuleVersions: []int{match.rule.RuleVersion},
					Priority: match.rule.Priority,
				})
			}
		}
	}
	output.WriteString(string(runes[cursor:]))
	if output.Len() > 0 {
		result.Text = output.String()
	}
	sort.Slice(result.Matches, func(i, j int) bool { return result.Matches[i].Start < result.Matches[j].Start })
	return result
}

func ocrDictionaryLiteralMatches(text, pattern []rune, casePolicy string) bool {
	if len(text) != len(pattern) {
		return false
	}
	if casePolicy == "exact" {
		for index := range text {
			if text[index] != pattern[index] {
				return false
			}
		}
		return true
	}
	return strings.EqualFold(string(text), string(pattern))
}

func ocrDictionaryBoundaryMatches(text []rune, start, end int) bool {
	return (start == 0 || !ocrDictionaryWordRune(text[start-1])) && (end >= len(text) || !ocrDictionaryWordRune(text[end]))
}

func ocrDictionaryWordRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_' || value == '\'' || value == '’'
}

func ocrDictionaryContextMatches(text []rune, start, end int, rule models.OCRDictionaryEntry) bool {
	left := []rune(rule.LeftContext)
	right := []rune(rule.RightContext)
	if len(left) > start || end+len(right) > len(text) {
		return false
	}
	if len(left) > 0 && !ocrDictionaryLiteralMatches(text[start-len(left):start], left, rule.CasePolicy) {
		return false
	}
	if len(right) > 0 && !ocrDictionaryLiteralMatches(text[end:end+len(right)], right, rule.CasePolicy) {
		return false
	}
	return true
}

func ocrDictionaryBuiltInEnglishContextMatches(text []rune, structural []bool, start, end int, rule models.OCRDictionaryEntry) bool {
	if rule.Source != "builtin" || rule.Language != "eng" || !strings.Contains(rule.Pattern, "|") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(rule.Pattern), "am |") && strings.EqualFold(strings.TrimSpace(rule.Replacement), "am I") {
		suffix := strings.TrimSpace(ocrDictionaryVisibleTextSpan(text, structural, end, len(text)))
		if suffix == "" {
			return true
		}
		suffixRunes := []rune(suffix)
		first := suffixRunes[0]
		return strings.ContainsRune(".,;:!?…", first)
	}

	replacement := []rune(strings.TrimSpace(rule.Replacement))
	if len(replacement) == 0 || unicode.ToLower(replacement[0]) != 'i' {
		return true
	}
	prefix := strings.ToLower(strings.TrimSpace(ocrDictionaryVisibleTextSpan(text, structural, 0, start)))
	if prefix == "" {
		return true
	}
	switch prefix {
	case "well,", "no,", "yes,", "oh,", "ah,":
		return true
	default:
		return false
	}
}

func ocrDictionaryMatchesUseOnlyBuiltInRules(ruleIDs []string, rules []models.OCRDictionaryEntry) bool {
	if len(ruleIDs) == 0 {
		return false
	}
	required := make(map[string]struct{}, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		if strings.TrimSpace(ruleID) == "" {
			return false
		}
		required[ruleID] = struct{}{}
	}
	found := make(map[string]struct{}, len(required))
	for _, rule := range rules {
		if _, ok := required[rule.ID]; !ok {
			continue
		}
		if rule.Source != "builtin" {
			return false
		}
		found[rule.ID] = struct{}{}
	}
	return len(found) == len(required)
}

func ocrDictionaryVisibleTextSpan(text []rune, structural []bool, start, end int) string {
	if start < 0 || end > len(text) || start > end {
		return ""
	}
	var visible strings.Builder
	for index := start; index < end; index++ {
		if !structural[index] {
			visible.WriteRune(text[index])
		}
	}
	return visible.String()
}

func sameOCRDictionaryMatch(candidates []ocrDictionaryTextCandidate) bool {
	if len(candidates) < 2 {
		return len(candidates) == 1
	}
	first := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.start != first.start || candidate.end != first.end || candidate.rule.Replacement != first.rule.Replacement {
			return false
		}
	}
	return true
}

func ocrDictionaryRuleEvidence(candidates []ocrDictionaryTextCandidate) ([]string, []int) {
	ids := make([]string, 0, len(candidates))
	versions := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.rule.ID)
		versions = append(versions, candidate.rule.RuleVersion)
	}
	return ids, versions
}

func ocrStructuralRuneMask(text []rune) []bool {
	mask := make([]bool, len(text))
	for index := 0; index < len(text); {
		end := -1
		if text[index] == '{' && index+1 < len(text) && text[index+1] == '\\' {
			for candidate := index + 2; candidate < len(text); candidate++ {
				if text[candidate] == '}' {
					end = candidate
					break
				}
			}
		} else if text[index] == '<' {
			for candidate := index + 1; candidate < len(text); candidate++ {
				if text[candidate] == '>' {
					end = candidate
					break
				}
				if text[candidate] == '\n' || text[candidate] == '<' {
					break
				}
			}
		}
		if end >= index {
			for marked := index; marked <= end; marked++ {
				mask[marked] = true
			}
			index = end + 1
			continue
		}
		index++
	}
	return mask
}
