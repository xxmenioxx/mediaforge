package handlers

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	translationProtectedAnchorPrefix = "\ue000MVFORGE-PROTECTED-"
	translationProtectedAnchorSuffix = "\ue001"
	maxTranslationProtectedAnchors   = 256
)

// assDocument is the shared source-preserving ASS/SSA structural boundary.
// Translation-specific adapters use the aliases below; Styling and future
// composition must consume this generic document instead of Translation
// provider models.
type assDocument struct {
	Representation string
	Source         string
	EventFormat    []string
	Styles         []assStyleDefinition
	Events         []assEvent
}

type assEvent struct {
	SourceOrdinal        int
	Start                float64
	End                  float64
	TextStart            int
	TextEnd              int
	StyleStart           int
	StyleEnd             int
	Style                string
	Format               []string
	Fields               []string
	Dialogue             assProtectedDialogue
	SourceIdentity       string
	SourceRevision       string
	SourceRepresentation string
	SemanticRole         SubtitleSemanticRole
	CompositionClass     CompositionCueClass
	Layout               compositionLayoutEvidence
}

// assStyleDefinition is deliberately part of the shared ASS document boundary.
// Composition may namespace these definitions, but it must not parse or
// serialize a second representation of the source document.
type assStyleDefinition struct {
	Section string
	Format  []string
	Name    string
	Fields  []string
}

type assProtectedDialogue struct {
	Parts []assDialoguePart
}

type assDialoguePart struct {
	Protected bool
	Text      string
	Anchor    string
}

// Translation keeps a narrow compatibility adapter over the generic document.
type translationASSDocument = assDocument
type translationASSEvent = assEvent
type translationProtectedDialogue = assProtectedDialogue
type translationDialoguePart = assDialoguePart

func parseASSDocument(representation, source string) (assDocument, error) {
	representation = strings.ToLower(strings.TrimSpace(representation))
	if representation != "ass" && representation != "ssa" {
		return assDocument{}, fmt.Errorf("unsupported ASS/SSA representation %q", representation)
	}
	document := assDocument{Representation: representation, Source: source}
	inEvents := false
	var eventFormat []string
	styleSection := ""
	var styleFormat []string
	lineStart := 0
	dialogueOrdinal := 0
	for lineStart < len(source) {
		lineEnd := lineStart
		for lineEnd < len(source) && source[lineEnd] != '\n' {
			lineEnd++
		}
		line := strings.TrimSuffix(source[lineStart:lineEnd], "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			inEvents = strings.EqualFold(section, "events")
			styleSection = ""
			if strings.EqualFold(section, "v4 styles") || strings.EqualFold(section, "v4+ styles") {
				styleSection = section
			}
		} else if styleSection != "" {
			lower := strings.ToLower(trimmed)
			if strings.HasPrefix(lower, "format:") {
				parsed, err := parseASSFieldFormat(trimmed)
				if err != nil {
					return assDocument{}, err
				}
				styleFormat = parsed
			} else if strings.HasPrefix(lower, "style:") && len(styleFormat) > 0 {
				fields := strings.Split(strings.TrimSpace(trimmed[strings.IndexByte(trimmed, ':')+1:]), ",")
				if len(fields) > 0 {
					document.Styles = append(document.Styles, assStyleDefinition{Section: styleSection, Format: append([]string(nil), styleFormat...), Name: strings.TrimSpace(fields[0]), Fields: fields})
				}
			}
		} else if inEvents {
			if strings.HasPrefix(strings.ToLower(trimmed), "format:") {
				parsed, err := parseTranslationEventFormat(trimmed)
				if err != nil {
					return assDocument{}, err
				}
				eventFormat = parsed
			} else if strings.HasPrefix(strings.ToLower(trimmed), "dialogue:") {
				if len(eventFormat) == 0 {
					return assDocument{}, fmt.Errorf("dialogue event has no active Events Format")
				}
				event, err := parseTranslationASSEvent(line, lineStart, eventFormat, dialogueOrdinal)
				if err != nil {
					return assDocument{}, err
				}
				document.Events = append(document.Events, event)
				dialogueOrdinal++
			}
		}
		if lineEnd < len(source) {
			lineEnd++
		}
		lineStart = lineEnd
	}
	if len(eventFormat) == 0 {
		return assDocument{}, fmt.Errorf("ASS/SSA document has no Events Format")
	}
	document.EventFormat = append([]string(nil), eventFormat...)
	return document, nil
}

func parseASSFieldFormat(line string) ([]string, error) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return nil, fmt.Errorf("invalid ASS section Format")
	}
	fields := strings.Split(line[colon+1:], ",")
	seen := make(map[string]bool, len(fields))
	for index := range fields {
		fields[index] = strings.ToLower(strings.TrimSpace(fields[index]))
		if fields[index] == "" || seen[fields[index]] {
			return nil, fmt.Errorf("invalid ASS section Format field")
		}
		seen[fields[index]] = true
	}
	return fields, nil
}

func parseTranslationASSDocument(representation, source string) (translationASSDocument, error) {
	return parseASSDocument(representation, source)
}

func parseTranslationEventFormat(line string) ([]string, error) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return nil, fmt.Errorf("invalid Events Format")
	}
	fields := strings.Split(line[colon+1:], ",")
	seen := make(map[string]bool, len(fields))
	for index := range fields {
		fields[index] = strings.ToLower(strings.TrimSpace(fields[index]))
		if fields[index] == "" || seen[fields[index]] {
			return nil, fmt.Errorf("invalid Events Format field")
		}
		seen[fields[index]] = true
	}
	textIndex := -1
	for index, field := range fields {
		if field == "text" {
			textIndex = index
		}
	}
	if textIndex < 0 || textIndex != len(fields)-1 {
		return nil, fmt.Errorf("Events Format must contain Text as its final field")
	}
	return fields, nil
}

func parseTranslationASSEvent(line string, lineStart int, format []string, sourceOrdinal int) (translationASSEvent, error) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return translationASSEvent{}, fmt.Errorf("invalid Dialogue event")
	}
	valueStart := colon + 1
	for valueStart < len(line) && (line[valueStart] == ' ' || line[valueStart] == '\t') {
		valueStart++
	}
	textStart, fields, err := splitTranslationEventFields(line[valueStart:], valueStart, len(format))
	if err != nil {
		return translationASSEvent{}, err
	}
	fieldIndex := make(map[string]int, len(format))
	for index, name := range format {
		fieldIndex[name] = index
	}
	startIndex, startOK := fieldIndex["start"]
	endIndex, endOK := fieldIndex["end"]
	textIndex, textOK := fieldIndex["text"]
	if !startOK || !endOK || !textOK {
		return translationASSEvent{}, fmt.Errorf("Events Format must contain Start, End, and Text fields")
	}
	if textIndex != len(format)-1 {
		return translationASSEvent{}, fmt.Errorf("Events Format Text field must be final")
	}
	start, err := parseTranslationASSTimestamp(fields[startIndex])
	if err != nil {
		return translationASSEvent{}, fmt.Errorf("invalid Dialogue start: %w", err)
	}
	end, err := parseTranslationASSTimestamp(fields[endIndex])
	if err != nil || end < start {
		return translationASSEvent{}, fmt.Errorf("invalid Dialogue end")
	}
	dialogue, err := parseTranslationProtectedDialogue(fields[len(fields)-1])
	if err != nil {
		return translationASSEvent{}, err
	}
	styleStart, styleEnd := -1, -1
	styleValue := ""
	if styleIndex, exists := fieldIndex["style"]; exists {
		styleStart, styleEnd = assEventFieldRange(line[valueStart:], valueStart, styleIndex, len(format))
		styleValue = fields[styleIndex]
	}
	return translationASSEvent{
		SourceOrdinal: sourceOrdinal,
		Start:         start,
		End:           end,
		TextStart:     lineStart + textStart,
		TextEnd:       lineStart + len(line),
		StyleStart:    lineStart + styleStart,
		StyleEnd:      lineStart + styleEnd,
		Style:         styleValue,
		Format:        append([]string(nil), format...),
		Fields:        append([]string(nil), fields...),
		Dialogue:      dialogue,
	}, nil
}

func assEventFieldRange(value string, valueOffset, fieldIndex, fieldCount int) (int, int) {
	if fieldIndex < 0 || fieldIndex >= fieldCount {
		return -1, -1
	}
	start := 0
	current := 0
	for index := 0; index <= len(value); index++ {
		if index < len(value) && value[index] != ',' && index != len(value) {
			continue
		}
		if current == fieldIndex {
			end := index
			for start < end && (value[start] == ' ' || value[start] == '\t') {
				start++
			}
			for end > start && (value[end-1] == ' ' || value[end-1] == '\t') {
				end--
			}
			return valueOffset + start, valueOffset + end
		}
		current++
		start = index + 1
	}
	return -1, -1
}

func splitTranslationEventFields(value string, valueOffset, fieldCount int) (int, []string, error) {
	if fieldCount <= 0 {
		return 0, nil, fmt.Errorf("empty Events Format")
	}
	fields := make([]string, 0, fieldCount)
	fieldStart := 0
	commas := 0
	for index := 0; index < len(value); index++ {
		if value[index] != ',' || len(fields) == fieldCount-1 {
			continue
		}
		fields = append(fields, value[fieldStart:index])
		fieldStart = index + 1
		commas++
	}
	if commas != fieldCount-1 {
		return 0, nil, fmt.Errorf("Dialogue event has fewer fields than its active Format")
	}
	fields = append(fields, value[fieldStart:])
	return valueOffset + fieldStart, fields, nil
}

func parseTranslationASSTimestamp(value string) (float64, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid timestamp %q", value)
	}
	hours, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	minutes, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || minutes < 0 || minutes >= 60 {
		return 0, fmt.Errorf("invalid minute field")
	}
	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil || seconds < 0 || seconds >= 60 {
		return 0, fmt.Errorf("invalid second field")
	}
	return hours*3600 + minutes*60 + seconds, nil
}

func parseTranslationProtectedDialogue(source string) (translationProtectedDialogue, error) {
	if strings.Contains(source, translationProtectedAnchorPrefix) || strings.Contains(source, translationProtectedAnchorSuffix) {
		return translationProtectedDialogue{}, fmt.Errorf("dialogue contains reserved translation anchor marker")
	}
	result := translationProtectedDialogue{}
	textStart := 0
	drawingMode := false
	flushText := func(end int) {
		if end > textStart {
			result.Parts = append(result.Parts, translationDialoguePart{Text: source[textStart:end]})
		}
	}
	for index := 0; index < len(source); {
		if drawingMode {
			if source[index] == '{' {
				closeIndex := strings.IndexByte(source[index:], '}')
				if closeIndex < 0 {
					return translationProtectedDialogue{}, fmt.Errorf("unterminated override block in drawing dialogue")
				}
				closeIndex += index
				if index > textStart {
					result.Parts = append(result.Parts, translationDialoguePart{Protected: true, Text: source[textStart:index]})
				}
				block := source[index : closeIndex+1]
				result.Parts = append(result.Parts, translationDialoguePart{Protected: true, Text: block})
				updateTranslationDrawingMode(block, &drawingMode)
				index = closeIndex + 1
				textStart = index
				continue
			}
			index++
			continue
		}
		if source[index] == '{' {
			closeIndex := strings.IndexByte(source[index:], '}')
			if closeIndex < 0 {
				return translationProtectedDialogue{}, fmt.Errorf("unterminated override block")
			}
			closeIndex += index
			flushText(index)
			block := source[index : closeIndex+1]
			result.Parts = append(result.Parts, translationDialoguePart{Protected: true, Text: block})
			updateTranslationDrawingMode(block, &drawingMode)
			index = closeIndex + 1
			textStart = index
			continue
		}
		if source[index] == '\\' && index+1 < len(source) && (source[index+1] == 'N' || source[index+1] == 'n' || source[index+1] == 'h') {
			flushText(index)
			result.Parts = append(result.Parts, translationDialoguePart{Protected: true, Text: source[index : index+2]})
			index += 2
			textStart = index
			continue
		}
		index++
	}
	if drawingMode {
		if len(source) > textStart {
			result.Parts = append(result.Parts, translationDialoguePart{Protected: true, Text: source[textStart:]})
		}
	} else {
		flushText(len(source))
	}
	protectedCount := 0
	for index := range result.Parts {
		if !result.Parts[index].Protected {
			continue
		}
		protectedCount++
		if protectedCount > maxTranslationProtectedAnchors {
			return translationProtectedDialogue{}, fmt.Errorf("dialogue contains too many protected anchors")
		}
		result.Parts[index].Anchor = translationProtectedAnchor(protectedCount)
	}
	return result, nil
}

func translationProtectedAnchor(index int) string {
	return fmt.Sprintf("%s%04d%s", translationProtectedAnchorPrefix, index, translationProtectedAnchorSuffix)
}

func updateTranslationDrawingMode(block string, drawingMode *bool) {
	for index := 0; index+2 < len(block); index++ {
		if block[index] != '\\' || block[index+1] != 'p' {
			continue
		}
		end := index + 2
		for end < len(block) && block[end] >= '0' && block[end] <= '9' {
			end++
		}
		if end == index+2 {
			continue
		}
		value, err := strconv.Atoi(block[index+2 : end])
		if err == nil {
			*drawingMode = value > 0
		}
		index = end - 1
	}
}

func (dialogue assProtectedDialogue) TranslatableText() string {
	parts := make([]string, 0, len(dialogue.Parts))
	for _, part := range dialogue.Parts {
		if part.Protected {
			parts = append(parts, part.Anchor)
		} else {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "")
}

func (dialogue assProtectedDialogue) HasTranslatableText() bool {
	for _, part := range dialogue.Parts {
		if !part.Protected && strings.TrimSpace(part.Text) != "" {
			return true
		}
	}
	return false
}

func (dialogue assProtectedDialogue) ReconstructProviderText(providerText string) (string, error) {
	expected := make([]string, 0, len(dialogue.Parts))
	for _, part := range dialogue.Parts {
		if part.Protected {
			expected = append(expected, part.Anchor)
		}
	}
	actual := make([]string, 0, len(expected))
	for searchFrom := 0; searchFrom < len(providerText); {
		start := strings.Index(providerText[searchFrom:], translationProtectedAnchorPrefix)
		if start < 0 {
			break
		}
		start += searchFrom
		endOffset := strings.Index(providerText[start:], translationProtectedAnchorSuffix)
		if endOffset < 0 {
			return "", fmt.Errorf("malformed protected translation anchor")
		}
		end := start + endOffset + len(translationProtectedAnchorSuffix)
		actual = append(actual, providerText[start:end])
		searchFrom = end
	}
	if strings.Count(providerText, translationProtectedAnchorSuffix) != len(actual) {
		return "", fmt.Errorf("unexpected protected translation anchor")
	}
	if len(actual) != len(expected) {
		return "", fmt.Errorf("protected translation anchors count mismatch")
	}
	for index := range expected {
		if actual[index] != expected[index] || strings.Count(providerText, expected[index]) != 1 {
			return "", fmt.Errorf("protected translation anchors are missing, duplicated, or reordered")
		}
	}
	providerTextWithoutAnchors := providerText
	for _, anchor := range expected {
		providerTextWithoutAnchors = strings.Replace(providerTextWithoutAnchors, anchor, "", 1)
	}
	for _, forbidden := range []string{"{", "}", "\\N", "\\n", "\\h", "\r", "\n"} {
		if strings.Contains(providerTextWithoutAnchors, forbidden) {
			return "", fmt.Errorf("provider text contains ASS/SSA structure outside protected anchors")
		}
	}
	result := providerText
	for _, part := range dialogue.Parts {
		if part.Protected {
			result = strings.Replace(result, part.Anchor, part.Text, 1)
		}
	}
	return result, nil
}

func (dialogue assProtectedDialogue) OriginalTextSegments() []string {
	segments := make([]string, 0, len(dialogue.Parts))
	for _, part := range dialogue.Parts {
		if !part.Protected {
			segments = append(segments, part.Text)
		}
	}
	return segments
}

func (dialogue assProtectedDialogue) ReconstructText(segments []string) (string, error) {
	textIndex := 0
	var builder strings.Builder
	for _, part := range dialogue.Parts {
		if part.Protected {
			builder.WriteString(part.Text)
			continue
		}
		if textIndex >= len(segments) {
			return "", fmt.Errorf("missing translated dialogue segment")
		}
		builder.WriteString(segments[textIndex])
		textIndex++
	}
	if textIndex != len(segments) {
		return "", fmt.Errorf("unexpected translated dialogue segment")
	}
	return builder.String(), nil
}

func (document assDocument) ReconstructDialogueSegments(replacements map[int][]string) (string, error) {
	result := document.Source
	for index := len(document.Events) - 1; index >= 0; index-- {
		event := document.Events[index]
		segments, replace := replacements[event.SourceOrdinal]
		if !replace {
			continue
		}
		text, err := event.Dialogue.ReconstructText(segments)
		if err != nil {
			return "", err
		}
		result = result[:event.TextStart] + text + result[event.TextEnd:]
	}
	return result, nil
}

func (document assDocument) ReconstructDialogueProviderTexts(replacements map[int]string) (string, error) {
	result := document.Source
	for index := len(document.Events) - 1; index >= 0; index-- {
		event := document.Events[index]
		providerText, replace := replacements[event.SourceOrdinal]
		if !replace {
			continue
		}
		text, err := event.Dialogue.ReconstructProviderText(providerText)
		if err != nil {
			return "", err
		}
		result = result[:event.TextStart] + text + result[event.TextEnd:]
	}
	return result, nil
}

func (document assDocument) TranslationSourceCues(sourceIdentity, logicalTransformKey string) []translationSourceCue {
	result := make([]translationSourceCue, 0, len(document.Events))
	for _, event := range document.Events {
		if !event.Dialogue.HasTranslatableText() {
			continue
		}
		text := event.Dialogue.TranslatableText()
		result = append(result, translationSourceCue{
			SourceOrdinal:    event.SourceOrdinal,
			Text:             text,
			Start:            event.Start,
			End:              event.End,
			TimingProvenance: unknownTimingProvenance,
		})
	}
	return result
}
