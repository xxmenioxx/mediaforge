# Library Path Rename

**Status:** Draft / SDD  
**Scope:** Assets → configured Library paths.

## Goal

Repair already-published Library paths so they match existing canonical Library naming, without re-encoding.

Example:

```text
Doctor Who/Season2/DR_WHO_COMPLETE_SERIES_2018_D03_Ttitle_t02.mkv
→
Doctor Who/Season 02/Doctor Who - S02E01.mkv
```

## User flow

```text
Rename Path
→ Preview
→ Review
→ Apply
```

Preview is read-only. Apply uses the reviewed plan after revalidation.

## Naming

Use the existing canonical naming logic.

Required baseline:

```text
Season2 + Doctor Who hierarchy
→ Doctor Who / Season 02 / Doctor Who - S02E##
```

Episode numbering must remain based on the complete asset inventory, including partial-selection cases.

## Sidecars

Move recognized external subtitle sidecars owned by the media asset:

```text
.srt
.ass
```

Preserve suffixes:

```text
old.eng.srt → Doctor Who - S02E01.eng.srt
old.spa.ass → Doctor Who - S02E01.spa.ass
```

Unrelated files remain untouched.

## Apply safeguards

Block Apply when:

```text
source missing
target exists
duplicate target
plan changed after Preview
asset has active Queue/maintenance work
source/target escapes the Library
canonical naming cannot resolve safely
```

Do not generate `.mvf` fallback names for collisions.

## State

After success:

- new Library paths become current;
- current publication references follow the move;
- per-asset path keyed state follows the move;
- historical publication/provenance remains historical.

## V1 boundaries

Included:

```text
Library paths
path-level repair
canonical episode naming
Preview + Apply
media + SRT/ASS sidecars
current-state reconciliation
```

Excluded:

```text
Raw
Archive
manual arbitrary rename
global mass rename
artwork
OCR/subtitle conversion
re-encode
new naming rules
```

## Acceptance

| Case | Expected |
|---|---|
| Preview | no mutation |
| Doctor Who Season2 | canonical Season 02 / S02E## |
| partial inventory | canonical episode ordinals preserved |
| already canonical | no-op |
| SRT/ASS | follow media stem |
| unrelated files | untouched |
| collision/stale/active | Apply blocked |
| successful Apply | filesystem + current MVForge state agree |
| historical paths | preserved as history |
