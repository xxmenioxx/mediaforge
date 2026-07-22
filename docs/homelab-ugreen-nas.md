# MVForge V1 Home Lab / UGreen NAS Setup

This guide is for running MVForge against persistent NAS folders while keeping reports and logs safe across app updates.

## Folder Layout

The standard standalone installation lives under the NAS Docker applications directory:

```text
/volume1/docker/mvforge/
  compose.yml
  .env
  config/
    mvforge.db
    backups/
  reports/
    as-is/
    results/
    logs/
  data/
    raw/
    staging/

/volume2/media/
  movies/
  anime-movies/
  series/
  anime/
  music/
  mvforge/
    originals_archive/
```

Use the real UGreen path exposed by your NAS. Examples may look like `/volume1/...`, `/mnt/ugreen/...`, or another host-mounted path depending on your setup.

## Recommended Mounts

Map the NAS folders into the containers like this:

```yaml
services:
  backend:
    volumes:
      - /volume1/docker/mvforge/config:/app/data
      - /volume1/docker/mvforge/data/raw:/media/raw
      - /volume2/media:/media/library
      - /volume1/docker/mvforge/data/staging:/media/staging
      - /volume2/media/mvforge/originals_archive:/media/originals_archive
      - /volume1/docker/mvforge/reports:/media/reports
```

Keep `/media/reports` persistent. It stores:

- AS-IS snapshots before conversion
- final result reports
- job logs and diagnostics

Those files are useful for debugging and later AI/Copilot analysis.

## First Production-Like Test

1. Place a small real asset under `raw/`.
2. Start MVForge.
3. Create destination libraries that point under `/media/library`.
4. Run `Analysis` on the asset.
5. Preview in `Profile Lab`.
6. Queue one job only.
7. Confirm the worker completes.
8. Validate the output.
9. Publish.
10. Confirm the original moved into `/media/originals_archive`.
11. Confirm reports were written under `/media/reports`.

## V1 Default Video Profiles

A clean V1 database seeds these profiles:

- `DVD Archive x265 Main10`
- `Anime DVD x265 Main10`
- `Series Balanced x265 Main10`

All are FFmpeg-based MKV/x265 Main10 profiles and preserve subtitles and chapters.

## V1 Default Audio Profiles

The app seeds conservative audio profiles for:

- gentle normalization
- dialogue clarity
- old source cleanup
- mono cleanup / dual mono
- light experimental mono-to-stereo

For production batches, preview audio in `Profile Lab` before using restoration profiles broadly.

## Reset Local V1 State While Preserving Reports

For local/dev cleanup, use:

```bash
CONFIRM_MVFORGE_RESET=YES sh scripts/reset-v1-preserve-reports.sh
```

This removes the local SQLite Docker volume and clears local working media folders, but preserves `media/reports`.

Do not run this against production NAS folders unless you have intentionally backed up or isolated the folders.

## Production Safety Notes

- Keep `autoPublisherEnabled` disabled until several real jobs are validated.
- Keep `dryRunOnly` enabled until conversion commands look correct.
- Start with one worker and one job at a time.
- Keep originals for at least 30 days while testing.
- Register only the intended destination folders under `/media/library`; do not register downloads or the MVForge work folders as published libraries.
