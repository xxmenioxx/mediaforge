# ARBEGAS conversion assessment

Assessment date: 2026-07-22

## Executive conclusion

Converting ARBEGAS is worthwhile only when recovering roughly 5–8 GiB is a meaningful objective. The collection is already a uniform, lossy H.264 encode at a moderate bitrate, so another encode cannot improve source quality and always introduces some generational loss.

Recommended decision: do not queue the complete series immediately. Run a pilot with three representative episodes and proceed only if the measured saving is at least 30% and motion, line art, gradients, and existing compression artifacts remain acceptable.

## Collection inventory

| Measurement | Result |
| --- | ---: |
| Episodes | 45 |
| Total duration | 18.31 hours |
| Total size | 19.63 GiB |
| Average episode size | 446.67 MiB |
| Episode size range | 438.85–450.48 MiB |
| Average container bitrate | 2.558 Mbps |
| Duration range | 24:06–24:27 |

All 45 files share the same essential format:

- Container: MP4
- Video: H.264 Main, 960×720, `yuv420p`, 8-bit
- Frame rate: 24000/1001 (23.976 fps)
- Field order: progressive
- Audio: AAC stereo, approximately 156–165 kbps
- Embedded subtitles: none detected
- HDR: none detected
- Color primaries, transfer, and matrix: not declared

## Motion and interlace inspection

Twenty-second samples from episodes 1, 23, and 45 were inspected with FFmpeg `idet` around the eight-minute mark.

- Multi-frame detection classified the samples as progressive.
- No consistent TFF or BFF cadence was found.
- The isolated TFF detections in episode 23 were not sustained by multi-frame analysis.

Conclusion: do not apply deinterlacing or inverse telecine automatically. Preserve the source frame rate and progressive structure.

## Is another encode beneficial?

### Potential benefit

HEVC/x265 10-bit can compress this type of animation more efficiently than the existing H.264 Main encode. A conservative conversion may reduce storage by approximately 25–40% while keeping the same resolution and copied audio.

Estimated collection outcomes:

| Scenario | Estimated final size | Estimated saving | Risk |
| --- | ---: | ---: | --- |
| Conservative | 14.5–15.0 GiB | 24–26% | Low to moderate |
| Balanced | 12.0–13.5 GiB | 31–39% | Moderate |
| Aggressive | 9.5–11.0 GiB | 44–52% | High |

These are planning ranges, not guarantees. CRF output depends on the actual complexity and existing artifacts in every episode.

### Quality risks

The source is already compressed. Re-encoding can amplify or introduce:

- ringing around dark outlines;
- loss of fine line detail during motion;
- banding in flat gradients;
- smearing in pans and action scenes;
- increased visibility of blocking already present in the H.264 source.

The 10-bit output format can reduce newly introduced banding, but it cannot restore information already lost in the source.

## Suggested MVForge pilot

Use episodes 1, 23, and 45 as the initial comparison set.

- Video codec: `libx265`
- Pixel format: `yuv420p10le`
- Resolution: preserve 960×720
- Frame rate: preserve 24000/1001
- Scan type: preserve progressive; no deinterlace
- Preset: `medium` or `slow`
- Initial CRF: 20
- Secondary comparison: CRF 21
- Audio: copy the existing AAC track
- Subtitles: none to add
- Chapters/metadata: preserve when present
- Avoid sharpening, denoising, scaling, and color-space overrides unless analysis detects a specific defect

Evaluate at least the opening sequence, a fast action scene, a dark scene, gradients, and static line art. Compare the converted samples directly with the originals at 100% scale.

## Acceptance criteria

Process the full collection only when all conditions are met:

1. Average saving across the three pilot episodes is at least 30%.
2. No visible loss appears in moving outlines or detailed backgrounds.
3. No additional banding, ringing, blocking, or motion smearing is observed.
4. DirectPlay compatibility with the intended Jellyfin clients remains acceptable.
5. The original files remain recoverable in Originals Archive.

If the saving is below 25%, retain the current H.264 files. The storage reduction would not justify the generational quality loss and processing time.

## Final recommendation

ARBEGAS is a reasonable candidate for a controlled HEVC 10-bit space-saving conversion, but not for automatic bulk processing without a pilot. The balanced target is approximately 12–14 GiB for the complete collection, saving about 6–8 GiB. Prefer CRF 20 as the starting point and move to CRF 21 only if the visual comparison remains clean.

This assessment used read-only metadata and sample decoding from the NAS. No source assets or NAS configuration were modified.
