package handlers

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

func restyleSourcePreservingASS(source []byte, representation string, policy SubtitleStylePolicy, role SubtitleSemanticRole, identity string) ([]byte, error) {
	document, err := parseASSDocument(representation, string(source))
	if err != nil {
		return nil, err
	}
	if !policy.Settings.Enabled || !policy.Settings.OverrideExistingASSStyles {
		return append([]byte(nil), source...), nil
	}
	styleIDs := sourceASSStyleNames(document.Source)
	for _, event := range document.Events {
		if event.Style != "" {
			styleIDs[event.Style] = struct{}{}
		}
	}
	assigned := map[string]string{}
	result := document.Source
	for index := len(document.Events) - 1; index >= 0; index-- {
		event := document.Events[index]
		if event.StyleStart < 0 || event.StyleEnd < event.StyleStart {
			continue
		}
		resolution := resolveSubtitleStyle(policy, subtitleStyleResolutionInput{Role: role})
		name, exists := assigned[resolution.StyleID]
		if !exists {
			name = generatedSubtitleStyleName(resolution.Style, styleIDs, fmt.Sprintf("%s:%d", identity, index))
			assigned[resolution.StyleID] = name
			styleIDs[name] = struct{}{}
		}
		result = result[:event.StyleStart] + name + result[event.StyleEnd:]
	}
	if len(assigned) == 0 {
		return []byte(document.Source), nil
	}
	result, err = appendGeneratedASSStyles(result, document.Representation, policy, assigned)
	if err != nil {
		return nil, err
	}
	return []byte(result), nil
}

func sourceASSStyleNames(source string) map[string]struct{} {
	names := map[string]struct{}{}
	section := ""
	for _, raw := range strings.Split(source, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if section != "v4+ styles" && section != "v4 styles" {
			continue
		}
		lower := strings.ToLower(line)
		if !strings.HasPrefix(lower, "style:") {
			continue
		}
		value := strings.TrimSpace(line[strings.IndexByte(line, ':')+1:])
		if comma := strings.IndexByte(value, ','); comma >= 0 {
			value = strings.TrimSpace(value[:comma])
		}
		if value != "" {
			names[value] = struct{}{}
		}
	}
	return names
}

func appendGeneratedASSStyles(source, representation string, policy SubtitleStylePolicy, assigned map[string]string) (string, error) {
	lines := strings.SplitAfter(source, "\n")
	section := ""
	styleSection := -1
	format := []string(nil)
	styleNames := map[string]struct{}{}
	insertAt := len(lines)
	for index, raw := range lines {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\n"))
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section == "v4+ styles" || section == "v4 styles" {
				styleSection = index
				continue
			}
			if styleSection >= 0 && insertAt == len(lines) {
				insertAt = index
			}
		}
		if styleSection >= 0 && index > styleSection && (section == "v4+ styles" || section == "v4 styles") {
			lower := strings.ToLower(line)
			if strings.HasPrefix(lower, "format:") {
				format = strings.Split(line[strings.IndexByte(line, ':')+1:], ",")
				for fieldIndex := range format {
					format[fieldIndex] = strings.ToLower(strings.TrimSpace(format[fieldIndex]))
				}
			}
			if strings.HasPrefix(lower, "style:") {
				value := strings.TrimSpace(line[strings.IndexByte(line, ':')+1:])
				if comma := strings.IndexByte(value, ','); comma >= 0 {
					styleNames[strings.TrimSpace(value[:comma])] = struct{}{}
				} else if value != "" {
					styleNames[value] = struct{}{}
				}
			}
		}
	}
	if styleSection < 0 {
		return source, errors.New("ASS/SSA source has no style section")
	}
	if insertAt == len(lines) {
		insertAt = len(lines)
	}
	if len(format) == 0 {
		return source, errors.New("ASS/SSA style section has no Format")
	}
	definitions := make([]string, 0, len(assigned))
	ids := make([]string, 0, len(assigned))
	for id := range assigned {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, exists := styleNames[assigned[id]]; exists {
			continue
		}
		preset, ok := subtitleStylePresetForID(policy, id)
		if !ok {
			continue
		}
		values := make([]string, len(format))
		for index, field := range format {
			values[index] = subtitleASSStyleField(field, preset, assigned[id])
		}
		definitions = append(definitions, "Style: "+strings.Join(values, ",")+"\n")
	}
	if len(definitions) == 0 {
		return source, nil
	}
	lines = append(lines[:insertAt], append(definitions, lines[insertAt:]...)...)
	return strings.Join(lines, ""), nil
}

func subtitleASSStyleField(field string, preset SubtitleStylePreset, generatedName string) string {
	primary, _ := subtitleASSColor(preset.PrimaryColor, preset.Opacity)
	outline, _ := subtitleASSColor(preset.OutlineColor, preset.Opacity)
	shadow, _ := subtitleASSColor(preset.ShadowBackColor, preset.Opacity)
	fontSize := preset.FontSize
	if fontSize == 0 || preset.AutoFontSize {
		fontSize = 20
	}
	bold := "0"
	if preset.Bold {
		bold = "-1"
	}
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "name":
		return generatedName
	case "fontname":
		return fallback(preset.FontFamily, "Arial")
	case "fontsize":
		return fmt.Sprintf("%d", fontSize)
	case "primarycolour", "primarycolor", "textcolour", "textcolor":
		return primary
	case "secondarycolour", "secondarycolor":
		return primary
	case "tertiarycolour", "tertiarycolor", "outlinecolour", "outlinecolor":
		return outline
	case "backcolour", "backcolor":
		return shadow
	case "bold":
		return bold
	case "italic", "underline", "strikeout":
		return "0"
	case "borderstyle":
		return "1"
	case "outline":
		return fmt.Sprintf("%d", preset.OutlineWidth)
	case "shadow":
		return fmt.Sprintf("%d", preset.ShadowDepth)
	case "alignment":
		return fmt.Sprintf("%d", subtitleAlignmentForBehavior(preset.AlignmentBehavior))
	case "marginl", "marginr", "marginv", "alphalevel", "encoding", "scalex", "scaley", "spacing", "angle":
		return "0"
	default:
		return "0"
	}
}
