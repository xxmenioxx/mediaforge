# MVForge V2 - Product Vision

Version: 2.0 Draft

---

# Overview

MVForge is evolving from a media transcoding application into a complete **Media Processing Platform**.

Its mission is to provide a reproducible, modular and user-friendly workspace where multimedia assets can be analyzed, enhanced, converted, validated and published.

MVForge does **not** replace Jellyfin, Plex, Radarr or Sonarr.

Instead, it fills the gap between media acquisition and media consumption.

```
Acquisition
(Radarr / Sonarr / Manual)

        │

        ▼

     MVForge

        │

        ▼

 Jellyfin / Plex / Archive
```

---

# Product Philosophy

Every feature added to MVForge should answer one question:

> Does this improve the quality, reproducibility or confidence of media processing?

If not, it probably belongs in another application.

MVForge is a **Media Processing Platform**, not a media server.

---

# Inspiration

MVForge is inspired by professional photography workflows.

Just as photographers prepare images before publishing them, MVForge prepares multimedia assets before they are consumed.

The project can be thought of as:

- a Digital Darkroom for Multimedia
- a Media Processing Platform
- a reproducible multimedia workspace

Internally we often compare the workflow to Lightroom, but MVForge extends beyond image editing by covering the entire media processing lifecycle.

---

# Long-Term Vision

MVForge should eventually become the operating system for multimedia processing.

Every media asset follows the same pipeline.

```
Discover

↓

Analyze

↓

Plan

↓

Enhance

↓

Convert

↓

Validate

↓

Publish

↓

History

↓

Knowledge Base
```

Every stage generates structured metadata.

Every decision is reproducible.

Nothing happens as a black box.

---

# Design Principles

## User First

Users should think about media quality.

Not FFmpeg.

Advanced options remain available through Advanced Mode.

---

## Non-destructive

Original media is never modified.

Processing always occurs on copies.

---

## Reproducibility

Every pipeline should produce deterministic results.

---

## Transparency

MVForge should always explain:

- what happened
- why it happened
- which profile was used
- which plugins executed
- validation results
- warnings

---

## AI Ready

Every job generates structured reports.

No AI automation initially.

Collect high-quality data first.

---

# Evolution of the Processing Pipeline

Current architecture:

```
Assets

↓

Queue

↓

Workers

↓

Validation

↓

Publisher
```

Target architecture:

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

---

# Processing Stages

## Discovery

Locate media assets.

No processing.

---

## Analysis

Extract complete technical metadata.

Examples:

- codecs
- bitrate
- HDR
- subtitles
- chapters
- languages
- frame rate
- duration
- color information
- audio streams

The Analysis stage should not modify media.

---

## Planning

Generate an execution plan.

Examples:

Recommended Profile

Estimated Processing Time

Expected Size

Expected Savings

Quality Confidence

Potential Warnings

---

## Enhancement

Improve media before encoding.

Enhancement should be optional.

Two independent pipelines exist:

- Video Enhancement
- Audio Enhancement

---

## Conversion

Execute encoding.

Uses configurable processors.

---

## Validation

Compare input and output.

Validate:

- duration
- streams
- codecs
- size
- warnings
- quality score

Generate reports.

---

## Publishing

Move completed assets to destination libraries.

Archive originals.

Never delete outside managed paths.

---

# Phase 2 - Media Enhancement

MVForge evolves beyond transcoding.

The goal becomes improving media.

---

# Video Enhancement Studio

New workspace dedicated to image restoration.

Capabilities include:

- brightness
- contrast
- gamma
- saturation
- hue
- white balance
- color temperature
- crop
- resize
- sharpen
- denoise
- deblock
- artifact removal
- deinterlace
- aspect ratio correction
- SDR/HDR conversion
- grain preservation

Future capabilities:

- Anime4K
- Real-ESRGAN
- VapourSynth
- AI restoration
- frame interpolation
- dead pixel repair

---

# Audio Restoration Studio

Visual controls.

Examples:

- loudness normalization
- true peak
- equalizer
- denoise
- hum removal
- mono repair
- stereo widening
- clipping detection
- silence detection

Future integrations:

- DeepFilterNet
- Demucs
- SoX
- Whisper

---

# The Forge

The Profile Lab evolves into **Forge**.

Forge becomes the central workspace of MVForge.

Instead of directly converting files, users experiment safely.

Workflow:

```
Asset

↓

Pipeline

↓

Preview Generation

↓

Compare

↓

Adjust

↓

Save Pipeline

↓

Queue
```

The goal is eliminating blind conversions.

---

# Pipeline-Based Architecture

The Forge executes Pipelines.

Not FFmpeg.

A pipeline consists of stages.

Example:

```
Analyze

↓

Crop

↓

Color Correction

↓

Denoise

↓

Sharpen

↓

Audio Restoration

↓

Encoding

↓

Validation
```

Stages are configurable.

Stages may be enabled or disabled independently.

---

# Plugin Architecture

One of the core architectural goals of V2 is complete decoupling between MVForge Core and processing tools.

MVForge should never directly depend on FFmpeg.

Instead:

```
MVForge Core

↓

Pipeline Engine

↓

Stages

↓

Plugin Registry

↓

Plugins
```

Examples:

Video Plugins

- FFmpeg
- VapourSynth
- Real-ESRGAN
- Anime4K

Audio Plugins

- FFmpeg
- DeepFilterNet
- Demucs
- SoX

Metadata Plugins

- ffprobe
- MediaInfo

Container Plugins

- FFmpeg
- MKVToolNix

Validation Plugins

- FFprobe
- MediaInfo
- Custom Validators

AI Plugins

- Ollama
- Whisper
- Future Models

---

# Stage Abstraction

The UI never knows which plugin is executing.

Users configure:

Color Correction

Not:

FFmpeg eq filter

Plugins translate stage settings into implementation-specific commands.

Example:

```
Stage

Color Correction

↓

Plugin

FFmpeg

↓

-vf eq=...
```

Tomorrow:

```
Stage

Color Correction

↓

Plugin

VapourSynth

↓

.vpy script
```

The UI remains unchanged.

---

# Profiles Become Pipelines

Instead of storing FFmpeg parameters, V2 stores Pipelines.

Example:

Anime DVD Restoration

Contains:

- Analysis
- Crop
- Color Correction
- Denoise
- Audio Restoration
- Encoding
- Validation

Each stage selects its own processor.

---

# Intelligent Automation

After the manual workflow is mature:

Rule Engine

Examples:

IF

Anime

AND

DVD

↓

Recommend Anime DVD Restoration

Another example

IF

HEVC Main10

↓

Skip Video Encoding

---

# Knowledge Base

Every job stores:

Analysis

Pipeline

Plugin versions

Parameters

Execution

Validation

Logs

Reports

Output metadata

This becomes the foundation for future AI.

---

# AI Roadmap

The first AI should recommend.

Not automate.

Example:

"This asset is 97% similar to 420 successful conversions.

Recommended Pipeline:

Anime DVD Restoration v5"

---

# Community

Future versions may support:

Pipeline Sharing

Community Profiles

Validation Templates

Best Practices

Shared Enhancement Chains

---

# Long-Term Goal

MVForge should become the definitive self-hosted platform for media processing.

Not because it has the most filters.

Not because it wraps FFmpeg.

But because it provides the safest, most reproducible and most intuitive workflow for preparing multimedia assets.

Every feature should move MVForge closer to answering one question:

> How can I produce the best possible version of this media asset?