# MediaForge - Product Vision

## Vision

MediaForge is a self-hosted media processing platform designed to help users analyze, enhance, convert, validate, and publish multimedia assets.

Its goal is **not** to replace Jellyfin, Plex, Radarr, Sonarr or similar applications.

Instead, MediaForge complements those tools by providing a complete processing pipeline between media acquisition and media consumption.

```
Acquisition
(Radarr / Sonarr / Manual)

        │

        ▼

    MediaForge

        │

        ▼

Jellyfin / Plex / Archive
```

MediaForge should eventually become the operating system for media processing, where every decision, conversion, restoration, validation and publication is reproducible and traceable.

---

# Product Philosophy

Every feature added to MediaForge should answer one simple question:

> Does this help produce a better version of a media asset?

If the answer is no, it probably belongs in another application.

MediaForge focuses on media processing, not media management.

---

# Design Principles

## User First

Advanced multimedia processing should be accessible to non-technical users.

Complex FFmpeg arguments, filters and codecs should remain hidden unless explicitly requested.

Simple workflows first.

Advanced controls second.

---

## Reproducibility

Every conversion should be reproducible.

Running the same profile over the same asset should generate the same output.

---

## Transparency

MediaForge should never behave like a black box.

Users should always understand:

- what happened
- why it happened
- which profile was used
- which filters were applied
- why validation passed or failed

---

## Non-destructive Processing

Original files should never be modified.

Every operation should work on copies.

Originals can later be archived or removed according to retention policies.

---

## AI Ready

Every operation should generate structured data that can later be used for automation and AI recommendations.

The application should start manual-first while collecting high-quality data.

---

# Long-Term Architecture

MediaForge should evolve into a modular pipeline.

```
Discovery

↓

Analysis

↓

Planning

↓

Enhancement

↓

Conversion

↓

Validation

↓

Publishing

↓

History

↓

Knowledge Base
```

Each stage has a single responsibility.

Each stage produces structured metadata.

---

# Phase 1 - Foundation

Goal:

Create a reliable and reproducible processing pipeline.

Components

- Discovery Engine
- Analysis Engine
- Queue
- Workers
- Validation
- Publisher
- History
- Reports

At the end of this phase MediaForge should reliably process assets from start to finish.

No AI.

No automation.

Everything user controlled.

---

# Phase 2 - Media Enhancement

This phase transforms MediaForge from a transcoding application into a media enhancement platform.

## Video Enhancement Studio

Users should be able to improve video quality before conversion.

Examples:

- brightness
- contrast
- gamma
- saturation
- hue
- white balance
- color temperature
- deinterlace
- crop
- resize
- sharpen
- denoise
- deblock
- artifact reduction
- aspect ratio correction
- HDR/SDR conversion
- grain preservation

The interface should remain user friendly.

Users should not configure FFmpeg filters directly unless they explicitly enable Advanced Mode.

---

## Audio Restoration Studio

Provide visual controls for:

- loudness normalization
- true peak
- equalizer
- dialogue enhancement
- stereo widening
- mono repair
- denoise
- hum removal
- clipping detection
- silence detection
- future DeepFilterNet integration
- future Demucs integration
- future SoX integration

---

## Profile Lab

The Profile Lab becomes the heart of MediaForge.

Instead of converting an entire movie, users generate previews.

```
Original

↓

Profile A

↓

Profile B

↓

Compare

↓

Save Profile
```

The goal is to eliminate blind conversions.

---

# Phase 3 - Intelligent Automation

Automation begins after the manual workflow is mature.

## Rule Engine

Rules automatically recommend profiles.

Example

IF

Anime

AND

DVD

↓

Recommend Anime DVD Profile

Another example

IF

HEVC Main10

↓

Skip video conversion

---

## Auto Queue

Assets without review requirements may automatically enter the queue.

---

## Batch Policies

Examples

- Maximum jobs
- CPU limits
- Night processing
- Pause during business hours
- Per-library limits

---

## Smart Recommendations

MediaForge begins suggesting profiles based on previous successful conversions.

Still user approved.

No automatic AI decisions.

---

# Phase 4 - Knowledge Platform

Every conversion becomes part of a knowledge base.

For every job store:

- Analysis
- Selected profile
- Parameters
- Execution metadata
- Validation results
- Logs
- Final report

This information becomes the foundation for future AI capabilities.

---

## AI Recommendation Engine

The first AI feature should recommend.

Not convert.

Example

"This asset is very similar to 320 previous successful conversions.

Recommended profile:

Anime DVD Restoration v3

Confidence: 97%"

---

## Community Profiles

Allow users to export and import:

- profiles
- presets
- rules
- enhancement chains

Eventually a public repository may exist.

---

# Phase 5 - Multimedia Library

This later phase adds a unified inventory workspace for assets that live across
registered media paths. It is an analysis and decision surface, not a playback
catalog or replacement for Jellyfin, Plex, Radarr, or Sonarr.

## Federated Library Browser

Users should be able to:

- browse one registered library at a time using tabs or a library selector
- browse all registered libraries in one aggregate view
- filter by library, path, media type, container, codec, resolution, size,
  warnings, analysis status, and Advisor recommendation
- search and sort assets without moving or modifying source files
- add other filesystem roots explicitly, with read-only discovery by default

Paths outside configured roots must never be scanned implicitly.

## MediaForge Provenance

Every successfully processed output should have a durable provenance record.
The MediaForge database and immutable job report are authoritative and should
store at least:

- processed by MediaForge
- MediaForge application version
- job ID and processing timestamp
- profile ID and profile version
- source fingerprint and output fingerprint
- effective processing pipeline and validation result

For portability, MediaForge should also write equivalent embedded container
metadata when the format supports safe custom tags. When it does not, it should
write a versioned sidecar manifest. Embedded tags and sidecars are useful hints,
but must not be the only source of truth because files can be remuxed, copied,
renamed, or stripped of metadata.

The UI should distinguish verified provenance, imported/unverified provenance,
and assets with no known MediaForge history.

## Library Advisor

The existing Advisor should be reusable from the Multimedia Library, both per
asset and for filtered batches. It should explain:

- whether conversion is recommended, optional, or should be skipped
- estimated output size and potential space savings
- compatibility improvements
- likely quality, restoration, or normalization benefits
- possible quality loss or processing risks
- confidence and the evidence behind the recommendation
- suggested video and audio profiles

Advisor results should be filterable and sortable so users can find the largest
safe savings or the assets most likely to benefit from improvement. Advice is
non-destructive; queueing and conversion continue to follow explicit approval,
automation, and safety policies.

---

# Future Vision

MediaForge should eventually become much more than a FFmpeg frontend.

It should become a complete Media Processing Platform capable of:

- media analysis
- quality enhancement
- audio restoration
- video restoration
- conversion
- validation
- publishing
- reporting
- AI-assisted recommendations

while remaining fully self-hosted and reproducible.

---

# What MediaForge is NOT

MediaForge is not:

- a media server
- a downloader
- a torrent client
- a metadata scraper
- a Plex replacement
- a Jellyfin replacement
- a Radarr replacement
- a Sonarr replacement

Those applications already solve those problems well.

MediaForge focuses on what happens after the media has been acquired and before it is consumed.

---

# Guiding Principle

MediaForge should help users answer one question with confidence:

> "How can I create the best possible version of this media asset?"

Everything else is secondary.
