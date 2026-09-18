package handlers

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

type subtitleTimingProvenance string

const (
	preparedInputLocal      subtitleTimingProvenance = "prepared-input-local"
	originalSourceAbsolute  subtitleTimingProvenance = "original-source-absolute"
	unknownTimingProvenance subtitleTimingProvenance = "unknown"
)

type bitmapCueGeometry struct {
	Start            float64
	End              float64
	CanvasWidth      int
	CanvasHeight     int
	BitmapX          int
	BitmapY          int
	BitmapWidth      int
	BitmapHeight     int
	SourceOrdinal    int
	TimingProvenance subtitleTimingProvenance
}

type canonicalBitmapSubtitleCue struct {
	Start            float64
	End              float64
	Text             string
	SourceOrdinal    int
	TimingProvenance subtitleTimingProvenance
	Geometry         *bitmapCueGeometry
}

func normalizeBitmapTiming(value float64, provenance subtitleTimingProvenance, preparationStart float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	switch provenance {
	case preparedInputLocal:
		return value, true
	case originalSourceAbsolute:
		translated := value - preparationStart
		if math.IsNaN(translated) || math.IsInf(translated, 0) {
			return 0, false
		}
		return translated, true
	default:
		return 0, false
	}
}

func normalizeBitmapGeometryTiming(values []bitmapCueGeometry, provenance subtitleTimingProvenance, preparationStart float64) []bitmapCueGeometry {
	result := make([]bitmapCueGeometry, 0, len(values))
	for _, value := range values {
		start, startOK := normalizeBitmapTiming(value.Start, provenance, preparationStart)
		end, endOK := normalizeBitmapTiming(value.End, provenance, preparationStart)
		if !startOK || !endOK || end < start || value.CanvasWidth <= 0 || value.CanvasHeight <= 0 || value.BitmapWidth <= 0 || value.BitmapHeight <= 0 || value.BitmapX < 0 || value.BitmapY < 0 || value.BitmapX+value.BitmapWidth > value.CanvasWidth || value.BitmapY+value.BitmapHeight > value.CanvasHeight {
			continue
		}
		value.Start, value.End = start, end
		value.TimingProvenance = preparedInputLocal
		result = append(result, value)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Start == result[j].Start {
			return result[i].SourceOrdinal < result[j].SourceOrdinal
		}
		return result[i].Start < result[j].Start
	})
	return result
}

func normalizeBitmapCueTiming(values []canonicalBitmapSubtitleCue, provenance subtitleTimingProvenance, preparationStart float64) []canonicalBitmapSubtitleCue {
	result := make([]canonicalBitmapSubtitleCue, 0, len(values))
	for _, value := range values {
		start, startOK := normalizeBitmapTiming(value.Start, provenance, preparationStart)
		end, endOK := normalizeBitmapTiming(value.End, provenance, preparationStart)
		if !startOK || !endOK || end < start {
			continue
		}
		value.Start, value.End = start, end
		value.TimingProvenance = preparedInputLocal
		result = append(result, value)
	}
	return result
}

func correlateBitmapSubtitleCuesWithTiming(cues []canonicalBitmapSubtitleCue, geometry []bitmapCueGeometry, provenance subtitleTimingProvenance, preparationStart float64) []canonicalBitmapSubtitleCue {
	normalizedGeometry := make([]bitmapCueGeometry, 0, len(geometry))
	for _, value := range geometry {
		valueProvenance := value.TimingProvenance
		if valueProvenance == "" {
			valueProvenance = unknownTimingProvenance
		}
		normalizedGeometry = append(normalizedGeometry, normalizeBitmapGeometryTiming([]bitmapCueGeometry{value}, valueProvenance, preparationStart)...)
	}
	normalized := correlateBitmapSubtitleCues(normalizeBitmapCueTiming(cues, provenance, preparationStart), normalizedGeometry)
	geometryByOrdinal := make(map[int]*bitmapCueGeometry, len(normalized))
	for _, cue := range normalized {
		if cue.Geometry != nil {
			value := *cue.Geometry
			geometryByOrdinal[cue.SourceOrdinal] = &value
		}
	}
	result := append([]canonicalBitmapSubtitleCue(nil), cues...)
	for index := range result {
		result[index].Geometry = geometryByOrdinal[result[index].SourceOrdinal]
	}
	return result
}

type pgsObjectDimension struct {
	width  int
	height int
}

type pgsCompositionObject struct {
	id      uint16
	x       int
	y       int
	ordinal int
}

type pgsComposition struct {
	start   float64
	width   int
	height  int
	objects []pgsCompositionObject
}

type pgsDisplaySet struct {
	compositions []pgsComposition
	dimensions   map[uint16]pgsObjectDimension
}

func parsePGSSUPGeometry(data []byte) ([]bitmapCueGeometry, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("PGS SUP is too short")
	}
	displaySets := []pgsDisplaySet{}
	current := pgsDisplaySet{dimensions: map[uint16]pgsObjectDimension{}}
	for offset, segmentCount := 0, 0; offset < len(data); segmentCount++ {
		if segmentCount >= 10000 {
			return nil, fmt.Errorf("PGS SUP contains too many segments")
		}
		if len(data)-offset < 13 || data[offset] != 'P' || data[offset+1] != 'G' {
			return nil, fmt.Errorf("invalid PGS SUP segment at offset %d", offset)
		}
		pts := binary.BigEndian.Uint32(data[offset+2 : offset+6])
		segmentType := data[offset+10]
		payloadLength := int(binary.BigEndian.Uint16(data[offset+11 : offset+13]))
		payloadStart := offset + 13
		payloadEnd := payloadStart + payloadLength
		if payloadEnd > len(data) {
			return nil, fmt.Errorf("truncated PGS SUP segment at offset %d", offset)
		}
		payload := data[payloadStart:payloadEnd]
		switch segmentType {
		case 0x14:
			// PDS: palette id/version followed by five-byte entries.
			if len(payload) < 2 || (len(payload)-2)%5 != 0 {
				return nil, fmt.Errorf("invalid PGS palette definition")
			}
		case 0x15:
			// ODS: object id/version/sequence, then dimensions on the first
			// fragment. Dimensions belong to this display set, not globally to
			// the object id, because PGS may reuse ids with different sizes.
			if len(payload) < 4 {
				return nil, fmt.Errorf("invalid PGS object definition")
			}
			objectID := binary.BigEndian.Uint16(payload[0:2])
			sequence := payload[3]
			if sequence&0x80 != 0 {
				if len(payload) < 11 {
					return nil, fmt.Errorf("truncated PGS object dimensions")
				}
				width := int(binary.BigEndian.Uint16(payload[7:9]))
				height := int(binary.BigEndian.Uint16(payload[9:11]))
				if width <= 0 || height <= 0 {
					return nil, fmt.Errorf("invalid PGS object dimensions")
				}
				current.dimensions[objectID] = pgsObjectDimension{width: width, height: height}
			}
		case 0x16:
			composition, err := parsePGSComposition(pts, payload)
			if err != nil {
				return nil, err
			}
			current.compositions = append(current.compositions, composition)
		case 0x17:
			// WDS: window count followed by nine-byte window definitions.
			if len(payload) < 1 || (len(payload)-1)%9 != 0 {
				return nil, fmt.Errorf("invalid PGS window definition")
			}
		case 0x80:
			if len(current.compositions) > 0 {
				displaySets = append(displaySets, current)
			}
			current = pgsDisplaySet{dimensions: map[uint16]pgsObjectDimension{}}
		}
		offset = payloadEnd
	}
	if len(current.compositions) > 0 {
		displaySets = append(displaySets, current)
	}
	compositions := make([]pgsComposition, 0)
	dimensionsByComposition := make([]map[uint16]pgsObjectDimension, 0)
	for _, displaySet := range displaySets {
		for _, composition := range displaySet.compositions {
			compositions = append(compositions, composition)
			dimensionsByComposition = append(dimensionsByComposition, displaySet.dimensions)
		}
	}
	result := make([]bitmapCueGeometry, 0)
	ordinal := 0
	for index, composition := range compositions {
		end := composition.start + 10
		if index+1 < len(compositions) && compositions[index+1].start > composition.start {
			end = compositions[index+1].start
		}
		for _, object := range composition.objects {
			dimension, exists := dimensionsByComposition[index][object.id]
			if !exists {
				continue
			}
			value := bitmapCueGeometry{
				Start: composition.start, End: end, CanvasWidth: composition.width, CanvasHeight: composition.height,
				BitmapX: object.x, BitmapY: object.y, BitmapWidth: dimension.width, BitmapHeight: dimension.height,
				SourceOrdinal: ordinal, TimingProvenance: originalSourceAbsolute,
			}
			ordinal++
			if value.BitmapX >= 0 && value.BitmapY >= 0 && value.BitmapX+value.BitmapWidth <= value.CanvasWidth && value.BitmapY+value.BitmapHeight <= value.CanvasHeight {
				result = append(result, value)
			}
		}
	}
	return result, nil
}

func parsePGSComposition(pts uint32, payload []byte) (pgsComposition, error) {
	if len(payload) < 11 {
		return pgsComposition{}, fmt.Errorf("PGS presentation composition is too short")
	}
	width := int(binary.BigEndian.Uint16(payload[0:2]))
	height := int(binary.BigEndian.Uint16(payload[2:4]))
	objectCount := int(payload[10])
	if width <= 0 || height <= 0 || objectCount > 255 {
		return pgsComposition{}, fmt.Errorf("invalid PGS presentation composition dimensions")
	}
	composition := pgsComposition{start: float64(pts) / 90000, width: width, height: height}
	offset := 11
	for ordinal := 0; ordinal < objectCount; ordinal++ {
		if len(payload)-offset < 8 {
			return pgsComposition{}, fmt.Errorf("truncated PGS composition object")
		}
		objectID := binary.BigEndian.Uint16(payload[offset : offset+2])
		cropped := payload[offset+3]&0x40 != 0
		x := int(binary.BigEndian.Uint16(payload[offset+4 : offset+6]))
		y := int(binary.BigEndian.Uint16(payload[offset+6 : offset+8]))
		if x < 0 || y < 0 {
			return pgsComposition{}, fmt.Errorf("invalid PGS composition object position")
		}
		// The object header is eight bytes when the x/y coordinates are present.
		offset += 8
		if cropped {
			if len(payload)-offset < 8 {
				return pgsComposition{}, fmt.Errorf("truncated PGS cropped object")
			}
			offset += 8
		}
		composition.objects = append(composition.objects, pgsCompositionObject{id: objectID, x: x, y: y, ordinal: ordinal})
	}
	return composition, nil
}

func bitmapSubtitleGeometryForPath(inputPath, codec string, preparedLocal bool, preparationStart float64) []bitmapCueGeometry {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "hdmv_pgs_subtitle" || codec == "pgssub" || strings.HasSuffix(strings.ToLower(inputPath), ".sup") {
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return nil
		}
		values, err := parsePGSSUPGeometry(data)
		if err != nil {
			return nil
		}
		provenance := originalSourceAbsolute
		if preparedLocal {
			provenance = preparedInputLocal
		}
		return normalizeBitmapGeometryTiming(values, provenance, preparationStart)
	}
	if codec == "dvd_subtitle" || codec == "vobsub" {
		return parseVobSubGeometry(inputPath, preparedLocal, preparationStart)
	}
	return nil
}

func parseVobSubGeometry(inputPath string, preparedLocal bool, preparationStart float64) []bitmapCueGeometry {
	// VobSub filepos values point into an MPEG Program Stream/PES container,
	// not directly at an SPU packet. Until a bounded, structure-validated
	// PS/PES-to-SPU extractor is available, fail closed so callers retain the
	// deterministic bottom-center fallback rather than trusting byte scans.
	return nil
}

func parseCanonicalBitmapSubtitleCues(format string, content []byte) []canonicalBitmapSubtitleCue {
	if strings.EqualFold(strings.TrimSpace(format), "ass") {
		return parseASSBitmapSubtitleCues(string(content))
	}
	return parseSRTBitmapSubtitleCues(string(content))
}

func parseSRTBitmapSubtitleCues(content string) []canonicalBitmapSubtitleCue {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	result := make([]canonicalBitmapSubtitleCue, 0)
	for cueLine := 0; cueLine < len(lines); cueLine++ {
		if !strings.Contains(lines[cueLine], "-->") {
			continue
		}
		times := strings.SplitN(lines[cueLine], "-->", 2)
		if len(times) != 2 {
			continue
		}
		start, startOK := parseSubtitleTimestamp(strings.TrimSpace(times[0]))
		end, endOK := parseSubtitleTimestamp(strings.TrimSpace(strings.Fields(times[1])[0]))
		if !startOK || !endOK || end < start {
			continue
		}
		textEnd := cueLine + 1
		for textEnd < len(lines) && !strings.Contains(lines[textEnd], "-->") {
			textEnd++
		}
		textLines := append([]string(nil), lines[cueLine+1:textEnd]...)
		if len(textLines) > 0 && strings.TrimSpace(textLines[0]) == strconv.Itoa(len(result)+1) {
			textLines = textLines[1:]
		}
		if len(textLines) > 0 && strings.TrimSpace(textLines[len(textLines)-1]) == strconv.Itoa(len(result)+2) {
			textLines = textLines[:len(textLines)-1]
		}
		text := strings.TrimSpace(strings.Join(textLines, "\n"))
		if strings.TrimSpace(text) == "" {
			continue
		}
		result = append(result, canonicalBitmapSubtitleCue{Start: start, End: end, Text: text, SourceOrdinal: len(result)})
	}
	return result
}

func parseASSBitmapSubtitleCues(content string) []canonicalBitmapSubtitleCue {
	result := []canonicalBitmapSubtitleCue{}
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "dialogue:") {
			continue
		}
		fields := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Dialogue:")), ",", 10)
		if len(fields) < 4 {
			continue
		}
		start, startOK := parseSubtitleTimestamp(strings.TrimSpace(fields[1]))
		end, endOK := parseSubtitleTimestamp(strings.TrimSpace(fields[2]))
		textField := fields[len(fields)-1]
		if !startOK || !endOK || end < start || strings.TrimSpace(textField) == "" {
			continue
		}
		result = append(result, canonicalBitmapSubtitleCue{Start: start, End: end, Text: strings.ReplaceAll(textField, "\\N", "\n"), SourceOrdinal: len(result)})
	}
	return result
}

func parseSubtitleTimestamp(value string) (float64, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, false
	}
	hour, hourErr := strconv.ParseFloat(parts[0], 64)
	minute, minuteErr := strconv.ParseFloat(parts[1], 64)
	second, secondErr := strconv.ParseFloat(parts[2], 64)
	if hourErr != nil || minuteErr != nil || secondErr != nil || minute < 0 || minute >= 60 || second < 0 || second >= 60 {
		return 0, false
	}
	return hour*3600 + minute*60 + second, true
}

func correlateBitmapSubtitleCues(cues []canonicalBitmapSubtitleCue, geometry []bitmapCueGeometry) []canonicalBitmapSubtitleCue {
	result := append([]canonicalBitmapSubtitleCue(nil), cues...)
	orderedGeometry := append([]bitmapCueGeometry(nil), geometry...)
	sort.SliceStable(orderedGeometry, func(i, j int) bool {
		if orderedGeometry[i].Start == orderedGeometry[j].Start {
			return orderedGeometry[i].SourceOrdinal < orderedGeometry[j].SourceOrdinal
		}
		return orderedGeometry[i].Start < orderedGeometry[j].Start
	})
	used := make([]bool, len(orderedGeometry))
	for cueIndex := range result {
		candidates := []int{}
		for geometryIndex, value := range orderedGeometry {
			if used[geometryIndex] || value.TimingProvenance != "" && value.TimingProvenance != preparedInputLocal || result[cueIndex].TimingProvenance != "" && result[cueIndex].TimingProvenance != preparedInputLocal || !subtitleCueIntervalsOverlap(result[cueIndex].Start, result[cueIndex].End, value.Start, value.End) {
				continue
			}
			candidates = append(candidates, geometryIndex)
		}
		if len(candidates) == 0 {
			continue
		}
		// Source order is the deterministic tie-breaker for simultaneous cues.
		geometryIndex := candidates[0]
		used[geometryIndex] = true
		value := orderedGeometry[geometryIndex]
		result[cueIndex].Geometry = &value
	}
	return result
}

func subtitleCueIntervalsOverlap(startA, endA, startB, endB float64) bool {
	const tolerance = 0.1
	return startA <= endB+tolerance && startB <= endA+tolerance
}

func bitmapASSAlignment(cue canonicalBitmapSubtitleCue) int {
	if cue.Geometry == nil {
		return 2
	}
	geometry := cue.Geometry
	centerX := float64(geometry.BitmapX+geometry.BitmapWidth/2) / float64(geometry.CanvasWidth)
	centerY := float64(geometry.BitmapY+geometry.BitmapHeight/2) / float64(geometry.CanvasHeight)
	column := 1
	if centerX < 1.0/3.0 {
		column = 0
	} else if centerX >= 2.0/3.0 {
		column = 2
	}
	row := 2
	if centerY < 1.0/3.0 {
		row = 0
	} else if centerY < 2.0/3.0 {
		row = 1
	}
	return [3][3]int{{7, 8, 9}, {4, 5, 6}, {1, 2, 3}}[row][column]
}

func renderBitmapASSCues(cues []canonicalBitmapSubtitleCue) []byte {
	var builder strings.Builder
	builder.WriteString("[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, TertiaryColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, AlphaLevel, Encoding\nStyle: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H64000000,-1,0,1,2,0,2,10,10,10,0,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, cue := range cues {
		text := strings.ReplaceAll(strings.ReplaceAll(cue.Text, "\r\n", "\n"), "\n", "\\N")
		text = strings.ReplaceAll(text, "{\\an1}", "")
		text = strings.ReplaceAll(text, "{\\an2}", "")
		text = strings.ReplaceAll(text, "{\\an3}", "")
		text = strings.ReplaceAll(text, "{\\an4}", "")
		text = strings.ReplaceAll(text, "{\\an5}", "")
		text = strings.ReplaceAll(text, "{\\an6}", "")
		text = strings.ReplaceAll(text, "{\\an7}", "")
		text = strings.ReplaceAll(text, "{\\an8}", "")
		text = strings.ReplaceAll(text, "{\\an9}", "")
		builder.WriteString(fmt.Sprintf("Dialogue: 0,%s,%s,Default,,0,0,0,,{\\an%d}%s\n", formatASSTimestamp(cue.Start), formatASSTimestamp(cue.End), bitmapASSAlignment(cue), text))
	}
	return []byte(builder.String())
}

func formatASSTimestamp(value float64) string {
	if value < 0 {
		value = 0
	}
	hours := int(value / 3600)
	value -= float64(hours * 3600)
	minutes := int(value / 60)
	seconds := value - float64(minutes*60)
	return fmt.Sprintf("%d:%02d:%05.2f", hours, minutes, seconds)
}

func renderBitmapSRTCues(cues []canonicalBitmapSubtitleCue) []byte {
	var builder strings.Builder
	for index, cue := range cues {
		builder.WriteString(strconv.Itoa(index + 1))
		builder.WriteString("\n")
		builder.WriteString(formatSRTTimestamp(cue.Start))
		builder.WriteString(" --> ")
		builder.WriteString(formatSRTTimestamp(cue.End))
		builder.WriteString("\n")
		builder.WriteString(strings.ReplaceAll(cue.Text, "\\N", "\n"))
		builder.WriteString("\n\n")
	}
	return []byte(builder.String())
}

func formatSRTTimestamp(value float64) string {
	if value < 0 {
		value = 0
	}
	milliseconds := int(math.Round(value * 1000))
	hours := milliseconds / 3600000
	milliseconds %= 3600000
	minutes := milliseconds / 60000
	milliseconds %= 60000
	seconds := milliseconds / 1000
	milliseconds %= 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, milliseconds)
}
