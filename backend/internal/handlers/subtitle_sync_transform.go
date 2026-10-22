package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type subtitleSyncReplacement struct {
	start int
	end   int
	value string
}

type subtitleSyncRange struct {
	start int
	end   int
}

const maxSubtitleSyncTimelineMs = int64(99 * 60 * 60 * 1000)

func validateSubtitleSyncOffset(representation string, offsetMilliseconds int64) error {
	representation = strings.ToLower(strings.TrimSpace(representation))
	if representation != "srt" && representation != "ass" && representation != "ssa" {
		return fmt.Errorf("subtitle synchronization does not support %q", representation)
	}
	if offsetMilliseconds < -maxSubtitleSyncOffsetMs || offsetMilliseconds > maxSubtitleSyncOffsetMs {
		return errors.New("fixed offset must be within twelve hours")
	}
	if representation != "srt" && offsetMilliseconds%10 != 0 {
		return errors.New("ASS and SSA offsets must use 10 ms increments")
	}
	return nil
}

func applySubtitleSyncTransform(representation string, source []byte, offsetMilliseconds int64) ([]byte, []SubtitleSyncTiming, error) {
	representation = strings.ToLower(strings.TrimSpace(representation))
	if len(source) == 0 || int64(len(source)) > maxPublishedSubtitleBytes {
		return nil, nil, errors.New("subtitle is empty or exceeds the supported size")
	}
	if err := validateSubtitleSyncOffset(representation, offsetMilliseconds); err != nil {
		return nil, nil, err
	}
	switch representation {
	case "srt":
		return applySRTSubtitleSync(source, offsetMilliseconds)
	case "ass", "ssa":
		return applyASSubtitleSync(representation, source, offsetMilliseconds)
	default:
		return nil, nil, fmt.Errorf("subtitle synchronization does not support %q", representation)
	}
}

func applySRTSubtitleSync(source []byte, offsetMilliseconds int64) ([]byte, []SubtitleSyncTiming, error) {
	canonicalCues := parseSRTBitmapSubtitleCues(string(source))
	if len(canonicalCues) == 0 {
		return nil, nil, errors.New("SRT has no valid canonical cues")
	}
	text := string(source)
	var replacements []subtitleSyncReplacement
	var originalTimings []SubtitleSyncTiming
	lineStart := 0
	for lineStart < len(text) {
		lineEnd := strings.IndexByte(text[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(text)
		} else {
			lineEnd += lineStart
		}
		line := strings.TrimSuffix(text[lineStart:lineEnd], "\r")
		arrow := strings.Index(line, "-->")
		if arrow >= 0 {
			startRange, endRange, ok := srtSyncTimestampRanges(line, arrow)
			if !ok {
				return nil, nil, errors.New("SRT contains an invalid cue timing line")
			}
			startMs, err := parseSRTSyncTimestamp(line[startRange.start:startRange.end])
			if err != nil {
				return nil, nil, err
			}
			endMs, err := parseSRTSyncTimestamp(line[endRange.start:endRange.end])
			if err != nil || endMs <= startMs {
				return nil, nil, errors.New("SRT cue interval is invalid")
			}
			cueIndex := len(originalTimings)
			if cueIndex >= len(canonicalCues) || math.Abs(canonicalCues[cueIndex].Start*1000-float64(startMs)) > 1 || math.Abs(canonicalCues[cueIndex].End*1000-float64(endMs)) > 1 {
				return nil, nil, errors.New("SRT timing fields do not match canonical cue parsing")
			}
			newStart, newEnd := startMs+offsetMilliseconds, endMs+offsetMilliseconds
			if newStart < 0 || newEnd <= newStart || newEnd > maxSubtitleSyncTimelineMs {
				return nil, nil, errors.New("fixed offset would create an invalid SRT cue interval")
			}
			replacements = append(replacements,
				subtitleSyncReplacement{start: lineStart + startRange.start, end: lineStart + startRange.end, value: formatSRTSyncTimestamp(newStart, line[startRange.start:startRange.end])},
				subtitleSyncReplacement{start: lineStart + endRange.start, end: lineStart + endRange.end, value: formatSRTSyncTimestamp(newEnd, line[endRange.start:endRange.end])},
			)
			originalTimings = append(originalTimings, SubtitleSyncTiming{StartMilliseconds: startMs, EndMilliseconds: endMs})
		}
		if lineEnd == len(text) {
			break
		}
		lineStart = lineEnd + 1
	}
	if len(originalTimings) != len(canonicalCues) {
		return nil, nil, errors.New("SRT cue count does not match canonical parsing")
	}
	output, err := applySubtitleSyncReplacements(source, replacements)
	if err != nil {
		return nil, nil, err
	}
	updatedCues := parseSRTBitmapSubtitleCues(string(output))
	if !canonicalCueTextValuesEqual(canonicalCues, updatedCues) || len(updatedCues) != len(originalTimings) {
		return nil, nil, errors.New("synchronized SRT changed canonical cue content")
	}
	updatedTimings := make([]SubtitleSyncTiming, len(originalTimings))
	for index, previous := range originalTimings {
		updatedTimings[index] = SubtitleSyncTiming{StartMilliseconds: previous.StartMilliseconds + offsetMilliseconds, EndMilliseconds: previous.EndMilliseconds + offsetMilliseconds}
	}
	if !validTranslatedSubtitleSidecar("srt", output) {
		return nil, nil, errors.New("synchronized SRT failed canonical validation")
	}
	return output, updatedTimings, nil
}

func srtSyncTimestampRanges(line string, arrow int) (subtitleSyncRange, subtitleSyncRange, bool) {
	left := line[:arrow]
	right := line[arrow+len("-->"):]
	startToken := strings.TrimSpace(left)
	endTrimmed := strings.TrimLeft(right, " \t")
	if startToken == "" || endTrimmed == "" {
		return subtitleSyncRange{}, subtitleSyncRange{}, false
	}
	startAt := strings.Index(left, startToken)
	endAt := len(right) - len(endTrimmed)
	endToken := strings.Fields(endTrimmed)
	if startAt < 0 || len(endToken) == 0 {
		return subtitleSyncRange{}, subtitleSyncRange{}, false
	}
	return subtitleSyncRange{start: startAt, end: startAt + len(startToken)}, subtitleSyncRange{start: arrow + len("-->") + endAt, end: arrow + len("-->") + endAt + len(endToken[0])}, true
}

func parseSRTSyncTimestamp(value string) (int64, error) {
	separator := strings.LastIndexAny(value, ",.")
	if separator < 0 || separator == len(value)-1 {
		return 0, errors.New("SRT timestamp must include milliseconds")
	}
	clock, fraction := value[:separator], value[separator+1:]
	if len(fraction) < 1 || len(fraction) > 3 || !allASCIIDigits(fraction) {
		return 0, errors.New("SRT timestamp milliseconds are invalid")
	}
	parts := strings.Split(clock, ":")
	if len(parts) != 3 {
		return 0, errors.New("SRT timestamp clock is invalid")
	}
	hours, hourErr := strconv.ParseInt(parts[0], 10, 64)
	minutes, minuteErr := strconv.ParseInt(parts[1], 10, 64)
	seconds, secondErr := strconv.ParseInt(parts[2], 10, 64)
	if hourErr != nil || minuteErr != nil || secondErr != nil || hours < 0 || minutes < 0 || minutes >= 60 || seconds < 0 || seconds >= 60 {
		return 0, errors.New("SRT timestamp components are invalid")
	}
	for len(fraction) < 3 {
		fraction += "0"
	}
	milliseconds, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, errors.New("SRT timestamp milliseconds are invalid")
	}
	total := ((hours*60+minutes)*60+seconds)*1000 + milliseconds
	if total > maxSubtitleSyncTimelineMs {
		return 0, errors.New("SRT timestamp exceeds the supported timeline")
	}
	return total, nil
}

func formatSRTSyncTimestamp(milliseconds int64, original string) string {
	separator := ","
	if strings.Contains(original, ".") {
		separator = "."
	}
	hourWidth := len(strings.SplitN(original, ":", 2)[0])
	if hourWidth < 2 {
		hourWidth = 2
	}
	hours := milliseconds / 3_600_000
	remaining := milliseconds % 3_600_000
	minutes := remaining / 60_000
	remaining %= 60_000
	seconds := remaining / 1_000
	millis := remaining % 1_000
	return fmt.Sprintf("%0*d:%02d:%02d%s%03d", hourWidth, hours, minutes, seconds, separator, millis)
}

func applyASSubtitleSync(representation string, source []byte, offsetMilliseconds int64) ([]byte, []SubtitleSyncTiming, error) {
	document, err := parseASSDocument(representation, string(source))
	if err != nil {
		return nil, nil, fmt.Errorf("ASS/SSA source is invalid: %w", err)
	}
	if len(document.Events) == 0 {
		return nil, nil, errors.New("ASS/SSA has no Dialogue events")
	}
	var replacements []subtitleSyncReplacement
	originalTimings := make([]SubtitleSyncTiming, 0, len(document.Events))
	for _, event := range document.Events {
		if event.StartStart < 0 || event.StartEnd <= event.StartStart || event.EndStart < 0 || event.EndEnd <= event.EndStart || event.StartEnd > len(source) || event.EndEnd > len(source) {
			return nil, nil, errors.New("ASS/SSA Dialogue timing fields are unavailable")
		}
		startToken := string(source[event.StartStart:event.StartEnd])
		endToken := string(source[event.EndStart:event.EndEnd])
		startMs, err := parseASSSyncTimestamp(startToken)
		if err != nil || math.Abs(event.Start*1000-float64(startMs)) > 1 {
			return nil, nil, errors.New("ASS/SSA Dialogue start is invalid")
		}
		endMs, err := parseASSSyncTimestamp(endToken)
		if err != nil || math.Abs(event.End*1000-float64(endMs)) > 1 || endMs <= startMs {
			return nil, nil, errors.New("ASS/SSA Dialogue end is invalid")
		}
		newStart, newEnd := startMs+offsetMilliseconds, endMs+offsetMilliseconds
		if newStart < 0 || newEnd <= newStart || newEnd > maxSubtitleSyncTimelineMs {
			return nil, nil, errors.New("fixed offset would create an invalid ASS/SSA Dialogue interval")
		}
		replacements = append(replacements,
			subtitleSyncReplacement{start: event.StartStart, end: event.StartEnd, value: formatASSSyncTimestamp(newStart, startToken)},
			subtitleSyncReplacement{start: event.EndStart, end: event.EndEnd, value: formatASSSyncTimestamp(newEnd, endToken)},
		)
		originalTimings = append(originalTimings, SubtitleSyncTiming{StartMilliseconds: startMs, EndMilliseconds: endMs})
	}
	output, err := applySubtitleSyncReplacements(source, replacements)
	if err != nil {
		return nil, nil, err
	}
	updated, err := parseASSDocument(representation, string(output))
	if err != nil || len(updated.Events) != len(document.Events) {
		return nil, nil, errors.New("synchronized ASS/SSA failed structural validation")
	}
	updatedTimings := make([]SubtitleSyncTiming, len(originalTimings))
	for index, previous := range originalTimings {
		updatedTimings[index] = SubtitleSyncTiming{StartMilliseconds: previous.StartMilliseconds + offsetMilliseconds, EndMilliseconds: previous.EndMilliseconds + offsetMilliseconds}
		if updated.Events[index].TextStart < 0 || updated.Events[index].TextEnd < updated.Events[index].TextStart || string(output[updated.Events[index].TextStart:updated.Events[index].TextEnd]) != string(source[document.Events[index].TextStart:document.Events[index].TextEnd]) {
			return nil, nil, errors.New("synchronized ASS/SSA changed Dialogue text")
		}
	}
	if !validTranslatedSubtitleSidecar(representation, output) {
		return nil, nil, errors.New("synchronized ASS/SSA failed canonical validation")
	}
	return output, updatedTimings, nil
}

func parseASSSyncTimestamp(value string) (int64, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, errors.New("ASS timestamp clock is invalid")
	}
	secondsParts := strings.Split(parts[2], ".")
	if len(secondsParts) != 2 || len(secondsParts[1]) < 1 || len(secondsParts[1]) > 2 || !allASCIIDigits(secondsParts[1]) {
		return 0, errors.New("ASS timestamp centiseconds are invalid")
	}
	hours, hourErr := strconv.ParseInt(parts[0], 10, 64)
	minutes, minuteErr := strconv.ParseInt(parts[1], 10, 64)
	seconds, secondErr := strconv.ParseInt(secondsParts[0], 10, 64)
	centiseconds, fractionErr := strconv.ParseInt(secondsParts[1], 10, 64)
	if hourErr != nil || minuteErr != nil || secondErr != nil || fractionErr != nil || hours < 0 || minutes < 0 || minutes >= 60 || seconds < 0 || seconds >= 60 {
		return 0, errors.New("ASS timestamp components are invalid")
	}
	if len(secondsParts[1]) == 1 {
		centiseconds *= 10
	}
	total := ((hours*60+minutes)*60+seconds)*1000 + centiseconds*10
	if total > maxSubtitleSyncTimelineMs {
		return 0, errors.New("ASS timestamp exceeds the supported timeline")
	}
	return total, nil
}

func formatASSSyncTimestamp(milliseconds int64, original string) string {
	hourWidth := len(strings.SplitN(original, ":", 2)[0])
	if hourWidth < 1 {
		hourWidth = 1
	}
	hours := milliseconds / 3_600_000
	remaining := milliseconds % 3_600_000
	minutes := remaining / 60_000
	remaining %= 60_000
	seconds := remaining / 1_000
	centiseconds := (remaining % 1_000) / 10
	return fmt.Sprintf("%0*d:%02d:%02d.%02d", hourWidth, hours, minutes, seconds, centiseconds)
}

func allASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func applySubtitleSyncReplacements(source []byte, replacements []subtitleSyncReplacement) ([]byte, error) {
	sorted := append([]subtitleSyncReplacement(nil), replacements...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].start < sorted[j].start })
	var output bytes.Buffer
	last := 0
	for _, replacement := range sorted {
		if replacement.start < last || replacement.end <= replacement.start || replacement.end > len(source) {
			return nil, errors.New("subtitle sync timestamp ranges overlap or are invalid")
		}
		output.Write(source[last:replacement.start])
		output.WriteString(replacement.value)
		last = replacement.end
	}
	output.Write(source[last:])
	if output.Len() == 0 || int64(output.Len()) > maxPublishedSubtitleBytes {
		return nil, errors.New("synchronized subtitle is empty or exceeds the supported size")
	}
	return output.Bytes(), nil
}

func subtitleSyncPreview(original, proposed []SubtitleSyncTiming) []SubtitleSyncCuePreview {
	if len(original) != len(proposed) || len(original) == 0 {
		return []SubtitleSyncCuePreview{}
	}
	count := len(original)
	if count > maxSubtitleSyncPreview {
		count = maxSubtitleSyncPreview
	}
	result := make([]SubtitleSyncCuePreview, 0, count)
	for index := 0; index < count; index++ {
		cueIndex := index
		if count > 1 && len(original) > count {
			cueIndex = index * (len(original) - 1) / (count - 1)
		}
		result = append(result, SubtitleSyncCuePreview{CueIndex: cueIndex + 1,
			CurrentStartMs: original[cueIndex].StartMilliseconds, CurrentEndMs: original[cueIndex].EndMilliseconds,
			ProposedStartMs: proposed[cueIndex].StartMilliseconds, ProposedEndMs: proposed[cueIndex].EndMilliseconds})
	}
	return result
}
