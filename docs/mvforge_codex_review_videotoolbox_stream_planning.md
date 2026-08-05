# MVForge – Review and Fix FFmpeg Stream Planning for VideoToolbox Jobs

## Objective

Review and correct the FFmpeg command generation used by MVForge for VideoToolbox conversion jobs.

Use the following real command as the reference case:

```bash
ffmpeg -hide_banner -y \
  -i "/Users/anuelvs/Documents/MediaForge/media/staging/job-25/_input/El increible Castillo Vagabundo (2004).mkv" \
  -map 0:0 \
  -map 0:1 \
  -map 0:4 \
  -map 0:5 \
  -map 0:1 \
  -c:v copy \
  -c:a copy \
  -c:s copy \
  -c:d copy \
  -c:t copy \
  -c:v hevc_videotoolbox \
  -b:v 1.75M \
  -maxrate 2.63M \
  -bufsize 4.38M \
  -g 120 \
  -bf 3 \
  -power_efficient 1 \
  -profile:v main10 \
  -pix_fmt p010le \
  -vf "fieldmatch=order=bff,decimate,setfield=prog,crop=672:438:24:20,colorspace=ispace=smpte170m:iprimaries=smpte170m:itrc=smpte170m:irange=tv:space=bt709:primaries=bt709:trc=bt709:range=tv,setsar=2522880/2903040,setdar=5760/4320" \
  -colorspace bt709 \
  -color_trc bt709 \
  -color_primaries bt709 \
  -color_range tv \
  -c:a:2 aac \
  -b:a:2 192k \
  -ac:a:2 2 \
  -metadata:s:a:2 "title=AAC Stereo (MVForge)" \
  -disposition:a:2 default \
  -metadata:s:a:0 "title=Original Spanish Stereo" \
  -metadata:s:a:0 language=spa \
  -disposition:a:0 0 \
  -metadata:s:a:1 "title=Original Japanese Surround" \
  -metadata:s:a:1 language=jpn \
  -disposition:a:1 0 \
  -map_chapters 0 \
  "/Users/anuelvs/Documents/MediaForge/media/staging/job-25/El increible Castillo Vagabundo (2004).mkv"
```

The goal is not only to fix this one command. Fix the backend planning and command-building logic so future jobs generate correct, explicit and validated FFmpeg mappings.

---

# Problems to Review

## 1. Ambiguous and duplicated video codec options

The command contains both:

```bash
-c:v copy
```

and:

```bash
-c:v hevc_videotoolbox
```

FFmpeg uses the last matching option, but the command is ambiguous and may emit warnings.

### Required change

Do not use broad stream-type defaults that are later overridden when explicit per-type behavior is already known.

Prefer:

```bash
-c:v hevc_videotoolbox
-c:a copy
-c:s copy
-c:t copy
-c:d copy
```

Only emit codec flags for stream types actually mapped.

---

## 2. Stream mapping and output audio indexing may be wrong

The command maps:

```bash
-map 0:0
-map 0:1
-map 0:4
-map 0:5
-map 0:1
```

Then applies:

```bash
-c:a:2 aac
```

This is only correct if the resolved output contains three audio streams and the duplicated `0:1` becomes output audio index `a:2`.

The command builder must not assume that input stream index, map order and output audio index are interchangeable.

### Required change

Introduce or fix a stream planner that explicitly resolves:

- input stream index;
- input stream type;
- selected output order;
- output type-relative index;
- codec action;
- metadata;
- language;
- title;
- disposition;
- source relationship when duplicating a stream.

Example resolved plan:

```text
Output video v:0  <- input 0:v:0
Output audio a:0  <- input 0:a:0, copy
Output audio a:1  <- input 0:a:1, copy
Output subtitle s:0 <- input 0:s:0, copy
Output subtitle s:1 <- input 0:s:1, copy
Output audio a:2  <- input 0:a:0, transcode AAC 192k stereo
```

The command builder should operate from this resolved plan, not from raw input indexes.

---

## 3. Use type-aware mappings where possible

Avoid depending on absolute stream positions such as:

```bash
-map 0:0
-map 0:1
-map 0:4
-map 0:5
```

when semantic mappings are available.

Prefer internally resolving to forms such as:

```bash
-map 0:v:0
-map 0:a:0
-map 0:a:1
-map 0:s:0?
-map 0:s:1?
-map 0:a:0
```

Absolute indexes may still be used when the user explicitly selected exact streams, but MVForge must retain the stream type and validate the selection.

---

## 4. Attachments and data streams are not necessarily mapped

The command declares:

```bash
-c:t copy
-c:d copy
```

but does not map attachments or data streams.

### Required change

Only emit these codec options when corresponding streams are mapped.

If the profile preserves them, add optional maps:

```bash
-map 0:t?
-map 0:d?
```

Do not emit meaningless codec options for unmapped stream types.

---

## 5. SAR and DAR handling is overly complex

The filter contains:

```text
setsar=2522880/2903040,setdar=5760/4320
```

These ratios reduce approximately to:

```text
SAR 73:84
DAR 4:3
```

Applying both `setsar` and `setdar` is redundant because `setdar` changes SAR again to reach the target DAR.

### Required change

Use one geometry policy.

Preferred behavior:

```text
Preserve original DAR
```

For this specific output:

```bash
setsar=73/84
```

Then validate the result with ffprobe and confirm:

```text
DAR = 4:3
```

Alternatively, use only:

```bash
setdar=4/3
```

Do not emit both unless a documented case requires it.

Store reduced rational values instead of large unreduced fractions.

---

## 6. Color conversion must be asset-derived

The command uses:

```text
ispace=smpte170m
iprimaries=smpte170m
itrc=smpte170m
irange=tv
```

This is correct only if the source analysis actually reports those values or if MVForge intentionally normalizes from a known source interpretation.

Previous assets may report other valid legacy combinations, such as:

```text
matrix: bt470bg
primaries: bt470m
transfer: bt470m
range: tv
```

### Required change

Build color conversion from a normalized source color descriptor.

Example:

```go
type SourceColorCharacteristics struct {
    Matrix       string
    Primaries    string
    Transfer     string
    Range        string
    Chroma       string
    Confidence   float64
    Inferred     bool
    InferenceReason string
}
```

Do not use a fixed SMPTE 170M input template for all DVD or SD assets.

If source fields are missing:

1. Use the analyzer’s explicit inference policy.
2. Record that values were inferred.
3. Store the inference reason.
4. Show a warning in effective configuration.
5. Do not silently relabel pixels.

---

## 7. VideoToolbox bitrate values must retain precision

The command uses:

```bash
-b:v 1.75M
-maxrate 2.63M
-bufsize 4.38M
```

Decimal Mbps values may work, but MVForge should internally store bitrate in integer bps or kbps and generate exact `k` values.

Preferred:

```bash
-b:v 1750k
-maxrate 2630k
-bufsize 4380k
```

For calculated values such as:

```text
Target: 2337 kbps
Maxrate: 3505 kbps
Buffer: 5842 kbps
```

generate exactly:

```bash
-b:v 2337k
-maxrate 3505k
-bufsize 5842k
```

Do not round to whole megabits.

Round only for UI presentation.

---

## 8. Offline VideoToolbox jobs must set realtime explicitly

The command does not include:

```bash
-realtime 0
```

For normal conversion jobs, add it explicitly.

Use:

```bash
-realtime 0
```

Only use `-realtime 1` for preview or realtime-specific jobs.

---

## 9. Validate VideoToolbox profile and pixel format

The pair in this command is valid:

```text
-profile:v main10
-pix_fmt p010le
```

The builder must validate:

```text
Main     <-> 8-bit yuv420p/nv12-compatible path
Main10   <-> p010le
```

Do not allow invalid combinations.

Store both requested and effective values.

---

## 10. Verify fieldmatch/decimate assumptions

The filter uses:

```text
fieldmatch=order=bff,decimate,setfield=prog
```

This is appropriate only if analysis determined:

- telecine or cadence suitable for IVTC;
- bottom-field-first order;
- output should be 23.976p.

### Required change

Require analysis evidence before emitting this chain.

Store:

```text
declared field order
detected cadence
cadence confidence
selected restoration policy
effective output frame rate
```

If cadence confidence is low, generate a warning or require review.

Do not apply `fieldmatch,decimate` solely because the source is 29.97i.

---

# Proposed Stream Planning Model

Create or adapt a model similar to:

```go
type PlannedStream struct {
    InputIndex       int
    InputType        string
    InputTypeIndex   int

    OutputIndex      int
    OutputTypeIndex  int

    Action           string
    Codec            string

    Language         string
    Title            string
    Default          bool
    Forced           bool

    DuplicateOfInput *int
    Optional         bool
}

type StreamPlan struct {
    Video       []PlannedStream
    Audio       []PlannedStream
    Subtitles   []PlannedStream
    Attachments []PlannedStream
    Data        []PlannedStream
}
```

The plan must be finalized before generating any:

```text
-map
-c:<type>:<index>
-metadata:s:<type>:<index>
-disposition:<type>:<index>
```

---

# Expected Command Structure

The exact source stream indexes must come from ffprobe and user selection.

A structurally correct result should resemble:

```bash
ffmpeg -hide_banner -y \
  -i "/path/to/input.mkv" \
  -map 0:v:0 \
  -map 0:a:0 \
  -map 0:a:1 \
  -map 0:s:0? \
  -map 0:s:1? \
  -map 0:a:0 \
  -map 0:t? \
  -map 0:d? \
  -c:v hevc_videotoolbox \
  -b:v 1750k \
  -maxrate 2630k \
  -bufsize 4380k \
  -g 120 \
  -bf 3 \
  -realtime 0 \
  -power_efficient 1 \
  -profile:v main10 \
  -pix_fmt p010le \
  -vf "fieldmatch=order=bff,decimate,setfield=prog,crop=672:438:24:20,colorspace=ispace=smpte170m:iprimaries=smpte170m:itrc=smpte170m:irange=tv:space=bt709:primaries=bt709:trc=bt709:range=tv,setsar=73/84" \
  -colorspace bt709 \
  -color_trc bt709 \
  -color_primaries bt709 \
  -color_range tv \
  -c:a:0 copy \
  -c:a:1 copy \
  -c:a:2 aac \
  -b:a:2 192k \
  -ac:a:2 2 \
  -metadata:s:a:0 "title=Original Spanish Stereo" \
  -metadata:s:a:0 language=spa \
  -disposition:a:0 0 \
  -metadata:s:a:1 "title=Original Japanese Surround" \
  -metadata:s:a:1 language=jpn \
  -disposition:a:1 0 \
  -metadata:s:a:2 "title=AAC Stereo (MVForge)" \
  -metadata:s:a:2 language=spa \
  -disposition:a:2 default \
  -c:s copy \
  -c:t copy \
  -c:d copy \
  -map_chapters 0 \
  "/path/to/output.mkv"
```

This is only an example. Do not hardcode the example stream indexes.

---

# Required Tests

Add targeted tests for the following.

## Stream planning

1. Two original audio tracks plus one derived AAC track produce output indexes `a:0`, `a:1`, `a:2`.
2. Duplicating an input audio stream does not overwrite or mislabel another output stream.
3. Subtitle mappings do not affect audio type-relative indexes.
4. Metadata and dispositions apply to output indexes, not input indexes.
5. Missing optional subtitle, attachment or data streams do not fail the job.
6. Attachments and data are only assigned copy codecs when mapped.
7. User-selected absolute input indexes are validated against stream type.

## Codec generation

8. No command contains both `-c:v copy` and `-c:v hevc_videotoolbox`.
9. Per-stream audio codecs override a clear default without duplicate ambiguity.
10. Unmapped stream types do not receive unnecessary codec flags.

## Geometry

11. Large SAR/DAR fractions are reduced.
12. Preserve-DAR policy emits either `setsar` or `setdar`, not both.
13. The 672×438 output with SAR 73:84 validates as DAR 4:3.
14. Crop changes are reflected in effective output geometry.

## Color

15. Source color characteristics are taken from analysis.
16. BT.470 and SMPTE 170M sources generate different input color parameters.
17. Missing source color values produce recorded inference/warnings.
18. Output BT.709 metadata matches the actual conversion policy.

## VideoToolbox

19. Bitrate calculations retain kbps precision.
20. No whole-Mbps rounding is introduced.
21. Offline jobs emit `-realtime 0`.
22. Preview jobs may emit `-realtime 1`.
23. Main10/P010 is accepted.
24. Invalid profile/pixel-format pairs are rejected before job creation.

## Cadence

25. `fieldmatch,decimate` is emitted only when cadence analysis supports IVTC.
26. Bottom-field-first analysis produces `order=bff`.
27. Low-confidence cadence produces a warning or review requirement.

---

# Runtime Validation

After generating a command:

1. Run `ffmpeg` with a short sample or dry-run equivalent where possible.
2. Capture FFmpeg warnings about:
   - duplicate codec options;
   - invalid stream indexes;
   - unsupported pixel formats;
   - invalid metadata targets;
   - unsupported VideoToolbox options.
3. Run ffprobe on the output.
4. Validate:
   - stream count;
   - stream order;
   - codecs;
   - titles;
   - languages;
   - dispositions;
   - resolution;
   - SAR/DAR;
   - frame rate;
   - field order;
   - color metadata;
   - chapter preservation.

Store requested and effective configuration.

---

# UI and Job Detail Changes

Show a resolved stream plan before starting the job.

Example:

```text
Video
v:0 <- source 0:v:0
HEVC VideoToolbox Main10 / P010

Audio
a:0 <- source Spanish Stereo
Copy

a:1 <- source Japanese Surround
Copy

a:2 <- source Spanish Stereo
AAC 192 kbps stereo
Default

Subtitles
s:0 <- source subtitle 0
Copy

s:1 <- source subtitle 1
Copy
```

Also show:

```text
Requested bitrate
Effective bitrate

Requested geometry
Effective geometry

Requested color policy
Effective color conversion

Requested cadence policy
Effective cadence filter
```

---

# Work Instructions

Before editing:

1. Locate the ffprobe stream model.
2. Locate track selection and duplication logic.
3. Locate output stream index assignment.
4. Locate the FFmpeg command builder.
5. Locate geometry/SAR/DAR handling.
6. Locate color-policy translation.
7. Locate VideoToolbox bitrate formatting.
8. Summarize the current flow in no more than 12 lines.
9. Identify the root cause of each issue listed above.

During implementation:

- Keep the change focused on stream planning and VideoToolbox command generation.
- Do not refactor unrelated encoders.
- Do not hardcode the example asset.
- Preserve existing profile behavior unless it is demonstrably invalid.
- Add small pure functions where possible.
- Keep requested and effective configuration separate.

After implementation:

1. Run targeted unit tests.
2. Generate the command for the reference asset.
3. Show the old and new command.
4. Show the resolved stream plan.
5. Show ffprobe validation of the generated output.
6. List changed files.
7. List known limitations.
8. Report any fallback or inferred metadata.
