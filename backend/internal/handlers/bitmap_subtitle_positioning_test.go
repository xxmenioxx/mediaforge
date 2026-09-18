package handlers

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePGSSUPGeometryValidatesBoundsAndOrdering(t *testing.T) {
	sup := testPGSSUP(t, []struct{ id, x, y, width, height int }{
		{id: 1, x: 600, y: 80, width: 720, height: 120},
		{id: 2, x: 500, y: 850, width: 920, height: 130},
		{id: 3, x: 1900, y: 10, width: 100, height: 100},
	})
	geometry, err := parsePGSSUPGeometry(sup)
	if err != nil {
		t.Fatal(err)
	}
	if len(geometry) != 2 || geometry[0].BitmapY != 80 || geometry[1].BitmapY != 850 {
		t.Fatalf("geometry=%#v", geometry)
	}
	if _, err := parsePGSSUPGeometry([]byte("PG")); err == nil {
		t.Fatal("truncated SUP was accepted")
	}
}

func TestParsePGSSUPGeometryScopesObjectDimensionsToDisplaySet(t *testing.T) {
	first := testPGSDisplaySet(90000, []struct{ id, x, y, width, height int }{
		{id: 7, x: 100, y: 100, width: 320, height: 80},
	})
	second := testPGSDisplaySet(270000, []struct{ id, x, y, width, height int }{
		{id: 7, x: 200, y: 200, width: 640, height: 160},
	})
	geometry, err := parsePGSSUPGeometry(append(first, second...))
	if err != nil {
		t.Fatal(err)
	}
	if len(geometry) != 2 {
		t.Fatalf("geometry=%#v", geometry)
	}
	if geometry[0].BitmapWidth != 320 || geometry[0].BitmapHeight != 80 || geometry[1].BitmapWidth != 640 || geometry[1].BitmapHeight != 160 {
		t.Fatalf("object dimensions were not scoped to display sets: %#v", geometry)
	}
}

func TestBitmapSubtitleTimingNormalizationUsesProvenance(t *testing.T) {
	if got, ok := normalizeBitmapTiming(1, preparedInputLocal, 42.5); !ok || got != 1 {
		t.Fatalf("prepared-local timing=%v,%t", got, ok)
	}
	if got, ok := normalizeBitmapTiming(43.5, originalSourceAbsolute, 42.5); !ok || got != 1 {
		t.Fatalf("absolute timing=%v,%t", got, ok)
	}
	if got, ok := normalizeBitmapTiming(1, unknownTimingProvenance, 42.5); ok || got != 0 {
		t.Fatalf("unknown timing=%v,%t", got, ok)
	}
	values := normalizeBitmapGeometryTiming([]bitmapCueGeometry{{Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 10, BitmapY: 10, BitmapWidth: 100, BitmapHeight: 50}}, preparedInputLocal, 42.5)
	if len(values) != 1 || values[0].Start != 1 {
		t.Fatalf("prepared geometry was double-adjusted: %#v", values)
	}
	localCue := canonicalBitmapSubtitleCue{Start: 1, End: 2, Text: "local", SourceOrdinal: 7}
	localGeometry := bitmapCueGeometry{Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 10, BitmapY: 10, BitmapWidth: 100, BitmapHeight: 50, SourceOrdinal: 7, TimingProvenance: preparedInputLocal}
	matched := correlateBitmapSubtitleCuesWithTiming([]canonicalBitmapSubtitleCue{localCue}, []bitmapCueGeometry{localGeometry}, preparedInputLocal, 42.5)
	if matched[0].Geometry == nil || matched[0].Start != 1 {
		t.Fatalf("prepared-local correlation was offset: %#v", matched)
	}
	absCue := canonicalBitmapSubtitleCue{Start: 43.5, End: 44.5, Text: "absolute", SourceOrdinal: 8}
	absGeometry := localGeometry
	absGeometry.Start, absGeometry.End, absGeometry.SourceOrdinal = 43.5, 44.5, 8
	absGeometry.TimingProvenance = originalSourceAbsolute
	matched = correlateBitmapSubtitleCuesWithTiming([]canonicalBitmapSubtitleCue{absCue}, []bitmapCueGeometry{absGeometry}, originalSourceAbsolute, 42.5)
	if matched[0].Geometry == nil || matched[0].Start != 43.5 {
		t.Fatalf("absolute correlation did not translate only its comparison copy: %#v", matched)
	}
	unknownCue := canonicalBitmapSubtitleCue{Start: 1, End: 2, Text: "unknown", SourceOrdinal: 9}
	unknownGeometry := localGeometry
	unknownGeometry.SourceOrdinal = 9
	unknownGeometry.TimingProvenance = unknownTimingProvenance
	matched = correlateBitmapSubtitleCuesWithTiming([]canonicalBitmapSubtitleCue{unknownCue}, []bitmapCueGeometry{unknownGeometry}, unknownTimingProvenance, 42.5)
	if matched[0].Geometry != nil {
		t.Fatalf("unknown timing provenance invented a geometry match: %#v", matched)
	}
}

func TestCorrelateBitmapSubtitleCuesKeepsSimultaneousRegionsSeparate(t *testing.T) {
	cues := parseSRTBitmapSubtitleCues("1\n00:00:01,000 --> 00:00:02,000\nTop\n\n2\n00:00:01,000 --> 00:00:02,000\nBottom\n")
	geometry := []bitmapCueGeometry{
		{Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 850, BitmapWidth: 720, BitmapHeight: 130, SourceOrdinal: 2, TimingProvenance: preparedInputLocal},
		{Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 80, BitmapWidth: 720, BitmapHeight: 120, SourceOrdinal: 1, TimingProvenance: preparedInputLocal},
	}
	for index := range cues {
		cues[index].TimingProvenance = preparedInputLocal
	}
	correlated := correlateBitmapSubtitleCues(cues, geometry)
	if len(correlated) != 2 || correlated[0].Geometry == nil || correlated[1].Geometry == nil {
		t.Fatalf("correlated=%#v", correlated)
	}
	if bitmapASSAlignment(correlated[0]) != 8 || bitmapASSAlignment(correlated[1]) != 2 {
		t.Fatalf("simultaneous alignments=%d,%d", bitmapASSAlignment(correlated[0]), bitmapASSAlignment(correlated[1]))
	}
}

func TestCorrelateBitmapSubtitleCuesLeavesAmbiguousOverlapUnpositioned(t *testing.T) {
	t.Run("one unambiguous match", func(t *testing.T) {
		cues := []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "Unique", TimingProvenance: preparedInputLocal}}
		geometry := []bitmapCueGeometry{{Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 80, BitmapWidth: 720, BitmapHeight: 120, SourceOrdinal: 1, TimingProvenance: preparedInputLocal}}
		correlated := correlateBitmapSubtitleCues(cues, geometry)
		if correlated[0].Geometry == nil || bitmapASSAlignment(correlated[0]) != 8 {
			t.Fatalf("unambiguous match=%#v", correlated)
		}
	})

	t.Run("equal timing uses source order", func(t *testing.T) {
		cues := []canonicalBitmapSubtitleCue{
			{Start: 1, End: 2, Text: "First", SourceOrdinal: 10, TimingProvenance: preparedInputLocal},
			{Start: 1, End: 2, Text: "Second", SourceOrdinal: 20, TimingProvenance: preparedInputLocal},
		}
		geometry := []bitmapCueGeometry{
			{Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 850, BitmapWidth: 720, BitmapHeight: 130, SourceOrdinal: 2, TimingProvenance: preparedInputLocal},
			{Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 80, BitmapWidth: 720, BitmapHeight: 120, SourceOrdinal: 1, TimingProvenance: preparedInputLocal},
		}
		correlated := correlateBitmapSubtitleCues(cues, geometry)
		if correlated[0].Geometry == nil || correlated[1].Geometry == nil || bitmapASSAlignment(correlated[0]) != 8 || bitmapASSAlignment(correlated[1]) != 2 {
			t.Fatalf("equal-timing source order=%#v", correlated)
		}
	})

	t.Run("non-equivalent overlap falls back", func(t *testing.T) {
		cues := []canonicalBitmapSubtitleCue{{Start: 1, End: 3, Text: "Ambiguous", TimingProvenance: preparedInputLocal}}
		geometry := []bitmapCueGeometry{
			{Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 80, BitmapWidth: 720, BitmapHeight: 120, SourceOrdinal: 1, TimingProvenance: preparedInputLocal},
			{Start: 2, End: 4, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 850, BitmapWidth: 720, BitmapHeight: 130, SourceOrdinal: 2, TimingProvenance: preparedInputLocal},
		}
		correlated := correlateBitmapSubtitleCues(cues, geometry)
		if correlated[0].Geometry != nil {
			t.Fatalf("ambiguous overlap was positioned: %#v", correlated)
		}
		ass := string(renderBitmapASSCues(correlated))
		if !strings.Contains(ass, "{\\an2}Ambiguous") {
			t.Fatalf("ambiguous overlap did not use bottom-center fallback: %q", ass)
		}
	})
}

func TestBitmapSubtitleSerializersKeepGeometryOutOfSRT(t *testing.T) {
	cues := []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "Top", TimingProvenance: preparedInputLocal, Geometry: &bitmapCueGeometry{CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 80, BitmapWidth: 720, BitmapHeight: 120}}}
	srt := string(renderBitmapSRTCues(cues))
	ass := string(renderBitmapASSCues(cues))
	if strings.Contains(srt, "\\an") || strings.Contains(srt, "pos(") || !strings.Contains(srt, "Top") {
		t.Fatalf("SRT serialization leaked geometry: %q", srt)
	}
	if !strings.Contains(ass, "{\\an8}Top") {
		t.Fatalf("ASS serialization did not apply upper layout: %q", ass)
	}
}

func TestBitmapASSAlignmentUsesCoarseNineRegionGrid(t *testing.T) {
	for _, test := range []struct {
		name                      string
		x, y, width, height, want int
	}{
		{name: "upper-left", x: 0, y: 0, width: 100, height: 100, want: 7},
		{name: "middle-center", x: 960, y: 500, width: 100, height: 100, want: 5},
		{name: "lower-right", x: 1700, y: 900, width: 100, height: 100, want: 3},
		{name: "vertical-one-third-boundary", x: 960, y: 310, width: 100, height: 100, want: 5},
		{name: "vertical-two-thirds-boundary", x: 960, y: 670, width: 100, height: 100, want: 2},
		{name: "horizontal-one-third-boundary", x: 590, y: 500, width: 100, height: 100, want: 5},
		{name: "horizontal-two-thirds-boundary", x: 1230, y: 500, width: 100, height: 100, want: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			cue := canonicalBitmapSubtitleCue{Geometry: &bitmapCueGeometry{CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: test.x, BitmapY: test.y, BitmapWidth: test.width, BitmapHeight: test.height}}
			if got := bitmapASSAlignment(cue); got != test.want {
				t.Fatalf("alignment=%d want %d", got, test.want)
			}
		})
	}
	if got := bitmapASSAlignment(canonicalBitmapSubtitleCue{}); got != 2 {
		t.Fatalf("missing geometry fallback=%d", got)
	}
}

func TestParseVobSubGeometryFailsClosedForPSWrappedSub(t *testing.T) {
	root := t.TempDir()
	idxPath := filepath.Join(root, "captions.idx")
	subPath := filepath.Join(root, "captions.sub")
	if err := os.WriteFile(idxPath, []byte("size: 1920x1080\ntimestamp: 00:00:01:000, filepos: 000000000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// filepos points into the PS/PES stream. The bytes resembling an SPU
	// control packet are deliberately wrapped and must not be byte-scanned as
	// if the file began with a raw SPU packet.
	spu := make([]byte, 20)
	binary.BigEndian.PutUint16(spu[0:2], uint16(len(spu)))
	binary.BigEndian.PutUint16(spu[2:4], 4)
	spu[8] = 0x05
	spu[9], spu[10], spu[11] = 0, 0xa0, 20
	spu[12], spu[13], spu[14] = 1, 0xe0, 40
	pesPayload := append([]byte{0x80, 0x80, 0x00}, spu...)
	pes := append([]byte{0x00, 0x00, 0x01, 0xbd, byte(len(pesPayload) >> 8), byte(len(pesPayload))}, pesPayload...)
	pack := []byte{0x00, 0x00, 0x01, 0xba, 0x44, 0x00, 0x04, 0x00, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(subPath, append(pack, pes...), 0o644); err != nil {
		t.Fatal(err)
	}
	geometry := parseVobSubGeometry(idxPath, false, 0)
	if geometry != nil {
		t.Fatalf("PS/PES VobSub representation must fail closed: %#v", geometry)
	}
	if got := parseVobSubGeometry(filepath.Join(root, "missing.idx"), false, 0); got != nil {
		t.Fatalf("missing VobSub representation should fall back, got %#v", got)
	}
}

func TestBitmapOCRASSRetainsGeometryThroughRawAndCleanup(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	fixturePath := filepath.Join(root, "input.sup")
	if err := os.WriteFile(fixturePath, testPGSSUP(t, []struct{ id, x, y, width, height int }{
		{id: 1, x: 600, y: 80, width: 720, height: 120},
		{id: 2, x: 500, y: 850, width: 920, height: 130},
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BITMAP_POSITIONING_FIXTURE", fixturePath)
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nprintf '%s\\n' spa\n")
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
output=""
for argument do output="$argument"; done
case "$output" in
  *.sup) cp "$BITMAP_POSITIONING_FIXTURE" "$output" ;;
esac
`)
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
output=""
cleanup=1
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
    --ocr-engine:tesseract) cleanup=0 ;;
  esac
done
if [ "$cleanup" = "0" ]; then
	printf '%s\n' '[Events]' 'Format: Layer, Start, End, Style, Text' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,Top' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,Bottom' > "$output"
else
	printf '%s\n' '[Events]' 'Format: Layer, Start, End, Style, Text' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,Clean Top' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,Clean Bottom' > "$output"
fi
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	stream := FFProbeStream{Index: 4, CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "spa"}}
	outputPath := filepath.Join(root, "captions.ass")
	if _, err := generateBitmapSubtitleAtPathWithResult(context.Background(), filepath.Join(root, "movie.mkv"), stream, "ass", "spa", "clean", outputPath, 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	cleaned, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "captions.raw.ass"))
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"raw": string(raw), "cleaned": string(cleaned)} {
		if !strings.Contains(content, "{\\an8}") || !strings.Contains(content, "{\\an2}") {
			t.Fatalf("%s ASS lost independent geometry: %q", name, content)
		}
	}
	if !strings.Contains(string(cleaned), "Clean Top") || !strings.Contains(string(cleaned), "Clean Bottom") {
		t.Fatalf("cleaned ASS did not use cleaned canonical cues: %q", cleaned)
	}
}

func TestBitmapBothUsesOneOCRAndGeometryAwareASSSerializer(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	fixturePath := filepath.Join(root, "input.sup")
	logPath := filepath.Join(root, "commands.log")
	if err := os.WriteFile(fixturePath, testPGSSUP(t, []struct{ id, x, y, width, height int }{
		{id: 1, x: 600, y: 80, width: 720, height: 120},
		{id: 2, x: 500, y: 850, width: 920, height: 130},
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BITMAP_POSITIONING_FIXTURE", fixturePath)
	t.Setenv("BITMAP_POSITIONING_LOG", logPath)
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nprintf '%s\\n' spa\n")
	writeExecutable(t, filepath.Join(bin, "ffprobe"), `#!/bin/sh
printf '%s' '{"streams":[{"index":4,"codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle","tags":{"language":"spa"}}]}'
`)
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
printf '%s\n' ffmpeg >> "$BITMAP_POSITIONING_LOG"
output=""
for argument do output="$argument"; done
case "$output" in *.sup) cp "$BITMAP_POSITIONING_FIXTURE" "$output" ;; esac
`)
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
printf '%s\n' seconv >> "$BITMAP_POSITIONING_LOG"
output=""
for argument do case "$argument" in --output-filename:*) output="${argument#--output-filename:}" ;; esac; done
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Top' '2' '00:00:01,000 --> 00:00:02,000' 'Bottom' > "$output"
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	output := filepath.Join(root, "movie.mkv")
	plan := MediaJobPlan{InputPath: filepath.Join(root, "movie.mkv"), OutputPath: output, Streams: MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: "hdmv_pgs_subtitle", Language: "spa"}}}, ResolvedTracks: &ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{
		{StreamIndex: 4, Codec: "hdmv_pgs_subtitle", SourceType: "bitmap", Language: "spa", Format: "srt", RequestedFormat: "both", Mode: "converted", OCRRequired: true, OCRLanguage: "spa", OCRMode: "raw", LogicalTransformKey: "subtitle:4:converted:both"},
		{StreamIndex: 4, Codec: "hdmv_pgs_subtitle", SourceType: "bitmap", Language: "spa", Format: "ass", RequestedFormat: "both", Mode: "converted", OCRRequired: true, OCRLanguage: "spa", OCRMode: "raw", LogicalTransformKey: "subtitle:4:converted:both"},
	}}}
	artifacts, err := generateResolvedSubtitleArtifacts(context.Background(), plan, nil)
	if err != nil || len(artifacts) != 2 {
		t.Fatalf("artifacts=%#v err=%v", artifacts, err)
	}
	ass, err := os.ReadFile(artifacts[1].StagedPath)
	if err != nil || !strings.Contains(string(ass), "{\\an8}Top") || !strings.Contains(string(ass), "{\\an2}Bottom") {
		t.Fatalf("both ASS geometry=%q err=%v", ass, err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(log)); got != "ffmpeg\nseconv" {
		t.Fatalf("both should extract once and OCR once without serializer OCR/render: %q", got)
	}
}

func testPGSSUP(t *testing.T, objects []struct{ id, x, y, width, height int }) []byte {
	t.Helper()
	return testPGSDisplaySet(90000, objects)
}

func testPGSDisplaySet(pts uint32, objects []struct{ id, x, y, width, height int }) []byte {
	pcs := make([]byte, 11+8*len(objects))
	binary.BigEndian.PutUint16(pcs[0:2], 1920)
	binary.BigEndian.PutUint16(pcs[2:4], 1080)
	pcs[10] = byte(len(objects))
	od := make([]byte, 0, len(objects)*24)
	for index, object := range objects {
		offset := 11 + index*8
		binary.BigEndian.PutUint16(pcs[offset:offset+2], uint16(object.id))
		binary.BigEndian.PutUint16(pcs[offset+4:offset+6], uint16(object.x))
		binary.BigEndian.PutUint16(pcs[offset+6:offset+8], uint16(object.y))
		payload := make([]byte, 11)
		binary.BigEndian.PutUint16(payload[0:2], uint16(object.id))
		payload[3] = 0x80
		binary.BigEndian.PutUint16(payload[7:9], uint16(object.width))
		binary.BigEndian.PutUint16(payload[9:11], uint16(object.height))
		od = append(od, pgsTestSegment(pts, 0x15, payload)...)
	}
	ordered := pgsTestSegment(pts, 0x16, pcs)
	ordered = append(ordered, pgsTestSegment(pts, 0x17, []byte{0})...)
	ordered = append(ordered, pgsTestSegment(pts, 0x14, []byte{0, 0})...)
	ordered = append(ordered, od...)
	return append(ordered, pgsTestSegment(pts, 0x80, nil)...)
}

func pgsTestSegment(pts uint32, segmentType byte, payload []byte) []byte {
	segment := make([]byte, 13+len(payload))
	copy(segment[0:2], []byte("PG"))
	binary.BigEndian.PutUint32(segment[2:6], pts)
	segment[10] = segmentType
	binary.BigEndian.PutUint16(segment[11:13], uint16(len(payload)))
	copy(segment[13:], payload)
	return segment
}
