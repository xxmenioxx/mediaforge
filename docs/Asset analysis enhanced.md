# MVForge vNext - Enhanced Asset Analysis

## Goal

Transform the current Asset Analysis from a metadata viewer into an intelligent media analyzer.

The analyzer should answer questions such as:

- Will this DirectPlay?
- Why will Jellyfin transcode it?
- Is this a good conversion candidate?
- Is this DVD actually progressive or telecined?
- Is HDR metadata correct?
- Is the color metadata preserved?
- Is this compatible with Apple TV / Chrome / Android TV?
- Will converting improve compatibility?

This analysis must happen BEFORE any conversion.

---

# Analysis Pipeline

Instead of only reading ffprobe output, analysis should consist of multiple stages.

```
Container Analysis
        │
        ▼
Stream Analysis
        │
        ▼
Video Analysis
        │
        ▼
Audio Analysis
        │
        ▼
Subtitle Analysis
        │
        ▼
Compatibility Analysis
        │
        ▼
Conversion Advisor
```

---

# 1. Container Analysis

Current

- container
- duration
- bitrate
- size

New

- container format
- muxing application
- writing library
- chapter count
- attachment count
- menu information
- timestamps

Example

```
Container

MKV

Duration
02:17:53

Writing Library
libmatroska

Attachments
2

Chapters
37
```

---

# 2. Video Analysis

Current

Codec
Resolution
Bitrate
Range

Needs to include

## Codec

Codec
Profile
Level

Example

```
HEVC Main
Level 4.1
```

---

## Pixel Information

Pixel Format

Example

```
yuv420p

nv12

p010
```

Bit Depth

```
8-bit

10-bit
```

Chroma

```
4:2:0

4:2:2

4:4:4
```

---

## Aspect Information

Current

720x480

Needs

Stored Resolution

Display Resolution

Sample Aspect Ratio

Display Aspect Ratio

Example

```
Stored Resolution

720x480

Display Resolution

854x480

SAR

32:27

DAR

16:9

Square Pixel Equivalent

854x480
```

This information is critical because Jellyfin may transcode anamorphic video.

---

## Scan Detection

Current

Progressive

Needs

Declared Scan Type

Detected Scan Type

Confidence

Example

```
Declared

Interlaced

Detected

Progressive

Confidence

96%
```

This allows detection of incorrect metadata.

---

## Cadence Detection

Detect

Progressive

Interlaced

Telecine

Hybrid

Mixed Cadence

Unknown

Confidence score.

Example

```
Cadence

Telecine

Confidence

91%
```

---

## Motion Analysis

Already partially implemented.

Expand to include

Fast motion

Slow motion

High grain

Static scenes

Credits

Animation

Film

Video

Noise estimation

Useful later for AI profile recommendations.

---

## Color Analysis

Current

SDR

Needs

Color Primaries

Transfer Characteristics

Matrix

Color Range

Mastering Display Metadata

MaxCLL

MaxFALL

HDR Type

Example

```
Primaries

BT.709

Transfer

BT.709

Matrix

SMPTE170M

Range

Limited
```

Highlight suspicious combinations.

Example

```
720x480

BT709

Warning

Possible incorrect color metadata.
```

---

# 3. Audio Analysis

Per audio stream

Codec

Channels

Bitrate

Sample Rate

Bit Depth

Language

Title

Default

Forced

Atmos

DTS-HD

TrueHD

PCM

AAC LC

HE-AAC

Normalize loudness estimation

Dynamic range estimation

Clipping estimation

Dialogue detection (future)

---

# 4. Subtitle Analysis

Per subtitle stream

Codec

Language

Forced

Default

PGS

DVD Subtitle

SRT

ASS

SSA

Text

Image-based

OCR Candidate

Burn-in Required

Compatibility

Example

```
PGS

Chrome

Not Supported

Will Require Burn-In
```

---

# 5. Compatibility Analysis

This is one of the biggest features.

Instead of only showing metadata, calculate compatibility.

Targets

Jellyfin

Plex

Chrome

Safari

Android TV

Apple TV

Fire TV

Roku

LG WebOS

Samsung Tizen

Kodi

MPV

VLC

For each platform

Direct Play

Direct Stream

Transcode

Unknown

Example

```
Chrome

Video

Direct Play

Audio

Transcode

Subtitle

Burn-in

Overall

Transcode Required
```

---

# 6. Transcode Reason Analyzer

Simulate Jellyfin logic.

Instead of users discovering it later, explain beforehand.

Example

```
Reasons

Image subtitles

Audio codec unsupported

Video anamorphic

Interlaced video

High profile

Unsupported level

HDR unsupported

Dolby Vision unsupported
```

---

# 7. Conversion Advisor

New AI-assisted recommendations.

Example

```
Compatibility Score

72%

Recommended Actions

✔ Convert AC3 → AAC

✔ OCR subtitles

✔ Remove anamorphic SAR

✔ Preserve chapters

✔ Keep HEVC

Estimated DirectPlay Score

96%
```

---

# 8. Quality Analyzer

Estimate

Compression artifacts

Blocking

Banding

Noise

Grain

Oversharpening

Edge enhancement

Interlacing artifacts

Telecine artifacts

Dropped frames

Duplicate frames

Future

AI video restoration recommendations.

---

# 9. DirectPlay Score

Introduce a score.

Example

```
Overall

84%

Video

100%

Audio

65%

Subtitles

30%

Container

100%
```

---

# 10. Color Integrity

Very important for MVForge.

Compare

Original

Converted

Report

Pixel format

Matrix

Primaries

Transfer

Range

Bit depth

HDR metadata

Warn if conversion changes color unexpectedly.

---

# 11. Validation Comparison

Instead of only validating output exists.

Compare

Original

↓

Converted

Detect

Codec changes

Bit depth changes

SAR changes

DAR changes

Color metadata changes

Subtitle losses

Chapter losses

Audio losses

Generate a human-readable report.

---

# 12. Future AI Integration

Store all analysis results.

Future AI agents should learn from

Original Analysis

↓

Profile Used

↓

Conversion Settings

↓

Result Analysis

↓

Validation

↓

User Rating

This dataset becomes the foundation for future AI recommendation systems.

---

# UI Improvements

Current

Metadata list

Future

Analysis Dashboard

Sections

Video

Audio

Subtitles

Compatibility

Quality

Color

Conversion Advice

Validation

Each section should include

Status

Warnings

Recommendations

Confidence

Expandable technical details

The interface should answer:

"What do I have?"

"What problems exist?"

"What will happen if I convert?"

"Will this DirectPlay?"

"What profile should I use?"

instead of only displaying ffprobe information.