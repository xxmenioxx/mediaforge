# MediaForge - Project Context

Version: 0.1

Author: Manuel Villaseñor

Status: Greenfield Project

---

# Vision

MediaForge is an open-source, self-hosted media processing platform designed to manage, optimize and validate personal media libraries before they are published into media servers such as Jellyfin, Plex or Emby.

MediaForge is **not** a media server.

It is the orchestration layer between raw media files and the final media library.

The project focuses on **manual control**, **high-quality conversions**, **intelligent recommendations**, and **transparent workflows**, avoiding aggressive automation.

The philosophy is:

> Give users complete visibility and control over every conversion.

---

# Design Principles

MediaForge must be:

* Self-hosted
* Open Source
* Docker-first
* API-first
* AI-ready
* Modular
* Extensible
* Hardware-aware
* Easy to use
* Safe by default

Users should always understand what the system is doing.

Nothing happens automatically without user approval (unless explicitly configured).

---

# Primary Goals

Provide a modern interface for media conversion workflows.

Replace shell scripts with a maintainable backend.

Manage conversion profiles.

Manage media libraries.

Manage conversion queues.

Validate converted files.

Publish validated files.

Integrate with media servers using their APIs.

Generate analytics.

Support future AI assistance.

---

# Product Philosophy

Unlike Tdarr, FileFlows or Unmanic, MediaForge should prioritize **decision support** instead of **automation**.

Every conversion should answer one question:

"Is this conversion actually worth doing?"

The software should help users make better decisions, not simply execute workflows.

---

# Target Users

* Jellyfin users
* Plex users
* Emby users
* NAS owners
* Homelab enthusiasts
* Digital archivists
* Physical media collectors
* Power users

---

# Architecture

React (TypeScript)

↓

REST API

↓

Go Services

↓

Workers

↓

Media Tools

↓

Storage

↓

Media Server APIs

---

# Technology Stack

Backend

* Go
* Gin
* GORM
* SQLite (initial)
* PostgreSQL (future)

Frontend

* React
* Vite
* TypeScript
* Material UI
* TanStack Query
* React Router

Infrastructure

* Docker
* Docker Compose

Future

* Kubernetes

---

# UI / UX

The user interface should feel like a natural companion to Jellyfin.

Do **not** clone Jellyfin.

Instead, follow the same usability principles:

* Dark-first interface
* Left navigation
* Card-based layouts
* Modern dashboard
* Large media artwork
* Minimal clicks
* Fast navigation

Users familiar with Jellyfin should immediately understand MediaForge.

The application should feel like it belongs in the same ecosystem while maintaining its own identity.

---

# Core Modules

Dashboard

Libraries

Profiles

Scanner

Queue

Workers

History

Validation

Publisher

Settings

AI Copilot

---

# Libraries

Libraries define destination locations.

Examples:

Movies

TV Shows

Anime

Music

Concerts

Home Videos

Each library stores:

* Name
* Source Path
* Destination Path
* Library Type
* Validation Rules
* Default Profile

---

# Profiles

Profiles define conversion behavior.

Examples:

DVD Archive

Blu-ray Archive

Streaming

TV Shows

Anime

Music Videos

Profiles should be editable using forms.

No YAML editing required.

Profiles should be exportable/importable.

Profiles should use the ideas and configuration model discussed in the
`media worker pipeline` project as their foundation.

MediaForge should treat that prior pipeline work as the baseline for:

* Conversion presets
* Worker-compatible encoding options
* Audio and subtitle handling
* Validation expectations
* Profile import/export structure

The user interface should expose these options in a friendly form-based way,
while preserving enough structure for workers to execute profiles deterministically.

---

# Scanner

The scanner never converts automatically.

It only discovers media.

For every file it should detect:

* Codec
* Resolution
* Bitrate
* HDR
* Audio Tracks
* Subtitle Tracks
* Chapters
* Duration
* Container
* Size

---

# Queue

Users manually select media.

Choose:

Profile

Library

Priority

Press Convert.

Workers process the queue.

---

# Workers

Workers execute:

HandBrakeCLI

FFmpeg

ffprobe

MediaInfo

MKVToolNix

No shell scripts.

Workers should be implemented in Go.

---

# Validation

Every conversion is validated.

Checks include:

Duration

Video

Audio

Subtitles

Chapters

Container

Metadata

Validation produces a score.

---

# Publisher

Only validated media can be published.

Publisher moves files.

Refreshes Jellyfin.

Future support:

Plex

Emby

---

# Intelligent Conversion Advisor

MediaForge should implement a pre-conversion analysis engine.

Before creating a job, the system should estimate:

* Final size
* Encoding duration
* Space savings
* CPU/GPU usage
* Storage savings
* Compatibility
* HDR preservation
* Audio compatibility
* Subtitle preservation
* Metadata preservation
* Whether conversion is worthwhile

The recommendation must always explain *why*.

---

# Hardware Awareness

MediaForge should detect available hardware.

Examples:

Intel Quick Sync

NVIDIA NVENC

AMD AMF

CPU x264

CPU x265

Future AV1

Recommendations should adapt automatically.

---

# AI Copilot

MediaForge should be designed to support local AI models.

Supported providers (future):

* Ollama
* llama.cpp
* LocalAI
* Open WebUI
* OpenAI-compatible APIs

The AI is optional.

The application must work perfectly without AI.

---

# AI Philosophy

The AI never replaces the user.

It acts as an expert advisor.

Its purpose is to explain recommendations.

Users always have final control.

---

# Learning Engine

Future versions should learn from:

Past conversions

Encoding times

Storage savings

User profile selections

Validation results

Hardware performance

This allows recommendations to improve over time.

---

# Future MCP Integration

The AI should use tools instead of guessing.

Examples:

Analyze Library

Analyze Movie

Estimate Encoding

Check Hardware

Recommend Profile

Calculate Savings

Explain Failure

This makes the AI deterministic and trustworthy.

---

# Experimental AI Subtitle Translation

MediaForge should eventually explore an optional AI-assisted workflow for films
or episodes that do not include Spanish subtitles.

The first target languages should be:

* Spanish Latin America
* Spanish Castilian

This module is experimental and must remain opt-in.

It should never overwrite existing subtitles without user approval.

## Translation Goals

The goal is not literal word-by-word translation.

The system should try to preserve:

* Natural dialogue
* Character intention
* Tone
* Humor when possible
* Cultural context
* Timing and readability
* Speaker meaning over exact phrasing

The workflow should support human review before publishing.

## Proposed Phases

Phase A: Subtitle Gap Detection

* Detect assets without Spanish subtitles
* Distinguish Latin American Spanish from Castilian Spanish when metadata allows
* Show subtitle availability in Asset Snapshot and grouped Assets
* Prioritize candidates by language gaps, popularity, or user selection

Phase B: Audio Transcription

* Extract audio safely from selected media
* Run speech-to-text on the source language
* Preserve timestamps at subtitle-friendly segment lengths
* Keep confidence scores and uncertain segments for review
* Support local transcription engines first where possible

Phase C: Natural Translation

* Translate transcript segments into Spanish Latin America or Castilian Spanish
* Preserve intent, tone, and context instead of literal phrasing
* Keep timing constraints readable for subtitles
* Flag idioms, jokes, songs, overlapping speech, or unclear lines

Phase D: Subtitle Review Workspace

* Provide side-by-side original transcript and translated subtitle
* Allow editing line text and timing
* Show warnings for long lines, fast reading speed, or low confidence
* Preview subtitles over the video player before export

Phase E: Subtitle Export and Publishing

* Export `.srt` initially
* Consider `.ass` later for styling and positioning
* Save generated subtitles as external sidecar files when selected
* Optionally mux subtitles into MKV only after explicit approval
* Optionally publish or export subtitles to Jellyfin when that integration is configured
* Optionally publish or export subtitles to Plex when that integration is configured
* Trigger media-server refreshes only after user approval or an explicit publishing setting
* Store provenance: model, language, date, confidence, and reviewer

Phase F: Assisted Quality Review

* Compare generated subtitles with audio/transcript
* Detect obvious mistranslations or timing drift
* Let the AI explain risky lines instead of silently changing them
* Keep the user in control of final acceptance

## Possible Tools

Candidate components may include:

* Whisper-compatible transcription engines
* FFmpeg audio extraction
* Subtitle parsers and validators
* Local LLMs through Ollama, llama.cpp, LocalAI, or OpenAI-compatible APIs
* External APIs only when explicitly configured by the user

This feature should be designed as a pipeline with cached intermediate artifacts:

* extracted audio
* source transcript
* translated draft
* reviewed subtitle
* exported sidecar

---

# Future Profile Lab / Comparison Lab

MediaForge should include a dedicated Lab workspace for testing and comparing
audio and video profiles before queueing full conversions.

The current Audio Test Bench is the first small step in this direction, but the
future Lab should support both audio and video profile design without forcing
users to work blindly.

The Lab should support:

* Selecting a raw asset from the configured source root
* Choosing a start timestamp and sample duration
* Previewing the original asset sample
* Previewing a sample with a selected video profile
* Previewing a sample with a selected audio profile
* Comparing estimated output size
* Comparing codecs before and after
* Comparing container, bitrate, resolution, HDR, audio layout, languages, and subtitles
* Generating screenshots or frame samples for video comparison
* Showing audio sample previews for A/B listening
* Showing effective FFmpeg or worker commands
* Saving a profile when the result is good enough
* Forking an existing profile into a tuned variant for a specific source type

The Lab should remain review-first and temporary by default. Generated samples
should live inside MediaForge-controlled staging or cache paths and should be
cleanable without touching originals or published libraries.

In the future, this Lab can become part of AI Copilot:

* Copilot can inspect source metadata, screenshots, short audio samples, and
  profile settings
* Copilot can suggest profile changes for difficult assets
* Copilot can explain tradeoffs such as quality, compatibility, size, HDR,
  subtitle preservation, and audio clarity
* Copilot can propose profile variants, but the user must approve changes

Profiles should also be designed with future sharing in mind.

Shared community profiles could help users learn from each other and avoid
recreating common presets for DVDs, Blu-rays, anime, TV recordings, concerts,
old mono/stereo sources, and device-specific playback.

Community sharing should include:

* Exportable/importable profile bundles
* Human-readable descriptions and intended use cases
* Required tools and codec support
* Sample before/after metadata when available
* Compatibility notes for Jellyfin, Plex, browsers, TVs, and mobile devices
* Versioning and provenance
* Safety warnings for destructive, lossy, or experimental settings

Shared profiles should never execute automatically on import. They should be
reviewed, tested in the Lab, and explicitly enabled by the user.

---

# Future Pipeline Map / Stage Inspector

MediaForge should include a visual process map for every queued job and worker
execution.

The goal is to show exactly where a conversion or media workflow is in the
pipeline, what each stage is doing, and why it passed or failed.

This should support normal conversion jobs, publishing jobs, validation jobs,
and future AI subtitle workflows.

Example conversion flow:

* Input path
* Preflight checks
* Probe / snapshot
* Advisor
* Profile resolution
* Conversion plan
* Transcode or remux
* Output validation
* Publish / move
* Jellyfin or Plex refresh
* Final path

Example subtitle flow:

* Detect missing subtitles
* Extract audio
* Transcribe
* Translate
* Review
* Export sidecar or mux
* Publish to Jellyfin or Plex

Each stage should expose:

* Status: pending, running, completed, failed, skipped
* Worker name
* Start and finish time
* Duration
* Inputs and outputs
* Effective configuration
* Generated command, when applicable
* Logs for that stage
* Warnings and errors
* Retry attempts
* Artifacts generated by the stage

The UI should show clickable boxes for stages.

Clicking a stage should open details with logs, commands, configuration,
artifacts, warnings, and recovery hints.

The map should be visible from:

* Workers, for live execution
* Queue, for job progress
* History, for completed jobs
* Assets, for the latest run attached to a file or grouped path

This feature should be added before or alongside real destructive conversions,
so FFmpeg and HandBrake execution remains transparent and easy to debug.

---

# Future Queue Management Controls

MediaForge should add explicit queue-management actions before enabling real
destructive conversion execution.

The queue should support:

* Cancel job
* Delete or remove job from queue
* Edit job priority
* Move job up or down
* Retry failed job
* Requeue completed job with confirmation

For grouped folder batches, the queue should support:

* Cancel batch
* Delete batch from queue
* Edit batch priority
* Retry failed jobs in batch
* Remove one file from a batch
* Add notes to a batch

Cancellation and cleanup must remain separate concepts.

Canceling a job should preserve logs, diagnostics, notes, commands, snapshots,
and metadata by default.

Cleanup should be explicit and should only remove artifacts inside
MediaForge-controlled paths.

Suggested settings:

* Keep logs and diagnostics
* Delete generated files and temporary artifacts
* Delete partial output from staging

Suggested API actions:

* Cancel job
* Update priority
* Delete job
* Cleanup job artifacts

Future queue metadata:

* canceledAt
* canceledReason
* retryOfJobId
* attempt
* lockedByWorker
* lastHeartbeatAt
* batchPriority

---

# Future Browser And Push Notifications

MediaForge should add optional browser or push notifications for long-running
workflows so users do not need to keep the Queue or Workers page open.

Notifications should be opt-in and user-controlled.

The first version should support browser notifications for:

* Job completed
* Job failed
* Batch completed
* Batch has failures
* Worker requires attention
* Validation failed or produced warnings
* Publisher completed or failed

The notification system should include:

* A Settings section for enabling notifications
* Browser permission request flow
* Per-event toggles
* Quiet hours or do-not-disturb window
* In-app notification center as a fallback when browser notifications are disabled
* Links from notifications to the relevant Queue job, Worker view, Validation result, or Logs file

Later versions can add push notifications through a self-hosted notification
gateway, mobile app integration, ntfy, Gotify, Apprise, or webhook providers.

---

# Manual Analysis And Profile Lab

MediaForge should separate quick filesystem scanning from deeper manual
analysis and profile experimentation.

Scanner remains the fast inventory layer:

* Discover assets
* Read basic container, codec, track, language, size, and path metadata
* Persist AS IS snapshots for later comparison

Analysis is the manual evidence layer:

* Review one asset at a time
* Save AS IS snapshots
* Add human notes
* Record review decisions
* Preserve enough information for future agents to learn from real human choices

Profile Lab is the manual A/B experimentation layer:

* Pick a raw asset
* Pick a start time and duration
* Compare original video/audio samples against profile drafts
* Tune video quality, codec, output, and size intent
* Tune audio loudness, true peak, filters, denoise, EQ, and output codec
* Save derived video and audio profiles for repeatable sources such as series,
  anime batches, DVD sets, TV recordings, or recurring source families

DeepFilterNet should be considered as a future advanced audio restoration
option for speech-focused denoise. It should be opt-in and tested in Profile
Lab before batch use because neural denoise can remove ambience, music texture,
or source character when applied too aggressively.

This should remain manual-first until the user explicitly enables agent-driven
recommendations or corrections.

Future agents should consume:

* AS IS snapshots
* Profile draft settings
* Effective FFmpeg commands
* Preview decisions
* Validation results
* Human notes
* Accepted, rejected, retried, and published outcomes

---

# Future Episode Splitter

MediaForge should include an optional splitter workflow for series, anime, or
DVD/Blu-ray sources where multiple episodes are contained inside a single MKV.

This feature should be review-first and should not cut or overwrite files
without user approval.

The splitter should support:

* Detecting candidate multi-episode MKV files
* Reading chapters, durations, titles, and metadata
* Suggesting episode boundaries from chapters when available
* Allowing manual boundary editing when automatic detection is uncertain
* Previewing split points before writing output files
* Naming output episodes using user-provided season and episode metadata
* Preserving selected audio tracks, subtitle tracks, chapters, and metadata
* Writing outputs to staging before publishing
* Validating every split output before it can be moved to the final library

Possible split strategies:

* Chapter-based split
* Manual timestamp split
* Duration pattern split
* Future AI-assisted scene or intro/outro detection

Example workflow:

* Select source MKV
* Scan chapters and streams
* Suggest episode boundaries
* User reviews and edits boundaries
* Generate split plan
* Dry-run output names and commands
* Split into staged MKV files
* Validate each episode
* Queue conversion or publish staged episodes

The splitter should integrate with grouped Assets and Queue batches, so one
source MKV can produce multiple episode jobs while remaining traceable to the
original file.

---

# Future Phase: Observability With Prometheus And Grafana

MediaForge should include an optional observability stack for technical users
who want to monitor server load and media pipeline behavior over time.

Suggested stack:

* Prometheus for metrics collection
* Grafana for dashboards
* Node exporter or container metrics for CPU, memory, disk, and IO
* MediaForge application metrics exposed from the backend

System metrics:

* CPU usage
* Memory usage
* Disk usage by controlled paths
* Disk IO during conversions
* Container/service health
* Worker resource consumption

MediaForge metrics:

* Jobs queued, running, completed, failed, canceled
* Job duration by profile, library, asset type, and worker
* Average validation score
* Failure rate by profile and worker
* Most-used video profiles
* Most-used audio profiles
* Queue depth by batch and status
* Conversion throughput over time
* Advisor decisions and confidence score distribution
* Publisher success/failure counts

Dashboards should help answer:

* Is the server saturated?
* Which profiles are slow or failure-prone?
* Which workers are healthy?
* How many jobs remain in the current batch?
* Are validation scores improving or degrading?
* Which libraries are generating the most work?

This should be optional and self-hosted. MediaForge should still work without
Grafana or Prometheus.

---

# Product Roadmap

Version 1

Manual workflows

Profiles

Libraries

Queue

Dashboard

Validation

Jellyfin integration

Version 2

Rules Engine

Multiple Publishers

Scheduler

Notifications

Version 3

Distributed Workers

Remote Agents

Cluster Support

Pipeline Map / Stage Inspector

Episode Splitter

Version 4

AI Copilot

Learning Engine

Media Knowledge Engine

Natural Language Queries

Experimental Subtitle Translation

Profile Lab / AI-assisted profile tuning

Version 5

Community Plugins

Marketplace

Community Profile Sharing

Custom Validators

Custom Publishers

---

# Long-Term Vision

MediaForge should become the central operating system for self-hosted media processing.

Jellyfin, Plex and Emby remain responsible for playback.

MediaForge becomes responsible for everything that happens before a media file reaches those servers.

The ultimate goal is to create an intelligent media pipeline that combines high-quality conversion, validation, analytics, hardware optimization and AI-assisted decision making while remaining transparent, modular and completely self-hosted.
