# Synthetic French and Korean PGS fixtures

All media in this directory is generated from the PNGs beside it; there are no real or NAS assets. The fixtures contain two bitmap text cues per language at 1.0–2.5 s and 3.0–4.5 s on a 1280×720 canvas. The PNG canvas is 640×120; the generator centers it at x=320 and places it 10 px above the bottom edge (y=590).

`french.sup` and `korean.sup` were generated with `dam-cav/img-to-pgs-sup` at commit `f595c0b83cdb27fffb652140fc9501661acfb155` (MIT). Its temporary isolated environment used the pinned requirements from that commit: numpy 1.26.3, opencv-python 4.9.0.80, Pillow 10.2.0, pytesseract 0.3.10. No generator code or dependencies were added to MVForge. The alternate `cubicibo/SUPer` was not used: its upstream is GPL-3.0-or-later, expects BDN XML plus PNG, and has a substantially larger dependency set (including OpenCV, PyOpenCL, SSIM-PIL, GUI and git dependencies). The MIT generator was the simpler disposable-fixture path.

The generated SUPs were independently opened by ffprobe as `hdmv_pgs_subtitle`, 1280×720, with cue boundaries at 1.0/2.5 and 3.0/4.5 s. They were muxed into `two-bitmap-tracks.mkv` using FFV1 video from a synthetic black lavfi source and FFmpeg `-copyts`. This preserves source subtitle intervals 1.0–2.5 s and 3.0–4.5 s in the MKV and backend-extracted SUP; without `-copyts`, the mux shifted cue starts to 0.0 and 2.0 s. ffprobe reports subtitle stream indexes 1 (`fra`, `synthetic-french`) and 2 (`kor`, `synthetic-korean`). The MKV SHA-256 is recorded in `manifest.json`.

A direct SeConv 5.1 runtime probe in the supported MVForge image successfully emitted French SRT text `Bonjour, à bientôt!` and `Déjà vu!`. Korean produced `안녕하세요` for cue 1 and the partial/segmented result `고도 스크` / `밥습니다` for cue 2. This is fixture-specific capability evidence only; it is not a general accuracy claim. The backend end-to-end smoke checks effective language, selected stream identity, cue timing, geometry and valid SRT/ASS sidecars, while requiring only non-empty OCR text so it does not encode perfect recognition as a contract.

See `manifest.json` for source command, generator pins, image metadata and SHA-256 hashes. Run the backend/container smoke from the repository root under the shared-validation lock:

```bash
python3 .agents/skills/dupla/scripts/dupla.py lock --scope shared-validation -- python3 scripts/e2e/subtitle_ocr_runtime_e2e.py --image mvforge-backend:issue-419-ocr-fra-kor
```
