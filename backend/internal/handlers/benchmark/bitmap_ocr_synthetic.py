#!/usr/bin/env python3
"""Run a small, deterministic Tesseract benchmark on generated PGM subtitle cues.

This is a pipeline smoke benchmark, not a substitute for labeled PGS/VobSub
captures. It uses only Python's standard library and a locally installed
Tesseract executable; it never reads media or writes outside --workdir.
"""

from __future__ import annotations

import argparse
import json
import os
import resource
import statistics
import subprocess
import time
from pathlib import Path


FONT = {
    "I": ("11111", "00100", "00100", "00100", "00100", "00100", "11111"),
    "l": ("10000", "10000", "10000", "10000", "10000", "10000", "11111"),
    "i": ("00100", "00000", "01100", "00100", "00100", "00100", "01110"),
    "k": ("10000", "10010", "10100", "11000", "10100", "10010", "10001"),
    "e": ("00000", "01110", "10001", "11111", "10000", "10001", "01110"),
    "1": ("00100", "01100", "00100", "00100", "00100", "00100", "01110"),
    "|": ("00100", "00100", "00100", "00100", "00100", "00100", "00100"),
}
CASES = (("I l 1 |",), ("I like 1 l |",), ("I l |", "1 like I"))


def render(lines: tuple[str, ...], scale: int, shadow: bool) -> tuple[int, int, bytearray]:
    char_w, char_h, gap = 5, 7, 2
    width = max(sum((char_w + gap) if c != " " else 4 for c in line) - gap for line in lines)
    width = (width + 16) * scale
    height = (len(lines) * (char_h + 5) + 10) * scale
    pixels = bytearray(width * height)
    for row, line in enumerate(lines):
        x = 8
        y = 5 + row * (char_h + 5)
        for char in line:
            if char == " ":
                x += 4
                continue
            glyph = FONT[char]
            for gy, bits in enumerate(glyph):
                for gx, bit in enumerate(bits):
                    if bit == "1":
                        for sy in range(scale):
                            for sx in range(scale):
                                px, py = (x + gx) * scale + sx, (y + gy) * scale + sy
                                if shadow and px + scale < width and py + scale < height:
                                    shadow_index = (py + scale) * width + px + scale
                                    if pixels[shadow_index] == 0:
                                        pixels[shadow_index] = 90
                                pixels[py * width + px] = 255
            x += char_w + gap
    return width, height, pixels


def write_pgm(path: Path, width: int, height: int, pixels: bytearray) -> None:
    path.write_bytes(f"P5\n{width} {height}\n255\n".encode("ascii") + pixels)


def normalize(text: str) -> list[str]:
    return text.casefold().split()


def edit_distance(left: list[str], right: list[str]) -> int:
    prev = list(range(len(right) + 1))
    for i, a in enumerate(left, 1):
        cur = [i]
        for j, b in enumerate(right, 1):
            cur.append(min(cur[-1] + 1, prev[j] + 1, prev[j - 1] + (a != b)))
        prev = cur
    return prev[-1]


def scores(expected: str, actual: str) -> tuple[float, float]:
    expected_chars, actual_chars = list(expected), list(actual)
    expected_words, actual_words = normalize(expected), normalize(actual)
    return (
        edit_distance(expected_chars, actual_chars) / max(1, len(expected_chars)),
        edit_distance(expected_words, actual_words) / max(1, len(expected_words)),
    )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--workdir", type=Path, required=True)
    parser.add_argument("--tesseract", default="tesseract")
    parser.add_argument("--repeats", type=int, default=3)
    args = parser.parse_args()
    root = args.workdir.resolve()
    root.mkdir(parents=True, exist_ok=True)
    version = subprocess.run([args.tesseract, "--version"], check=True, capture_output=True, text=True).stdout.splitlines()[0]
    rows = []
    for case_index, lines in enumerate(CASES):
        expected = "\n".join(lines)
        for shadow in (False, True):
            for scale in (1, 2, 3):
                width, height, pixels = render(lines, scale, shadow)
                image_path = root / f"cue-{case_index}-shadow-{int(shadow)}-scale-{scale}.pgm"
                write_pgm(image_path, width, height, pixels)
                for psm in (6, 7):
                    for dawg_enabled in (True, False):
                        elapsed, cpu, peak_rss, cer, wer, recognized = [], [], [], [], [], []
                        for repeat in range(args.repeats):
                            before = resource.getrusage(resource.RUSAGE_CHILDREN)
                            started = time.perf_counter()
                            result = subprocess.run(
                                [
                                    args.tesseract, str(image_path), "stdout", "-l", "eng", "--psm", str(psm),
                                    "-c", f"load_system_dawg={int(dawg_enabled)}",
                                    "-c", f"load_freq_dawg={int(dawg_enabled)}",
                                ],
                                check=False, capture_output=True, text=True,
                            )
                            elapsed.append(time.perf_counter() - started)
                            after = resource.getrusage(resource.RUSAGE_CHILDREN)
                            cpu.append((after.ru_utime + after.ru_stime) - (before.ru_utime + before.ru_stime))
                            peak_rss.append(after.ru_maxrss)
                            output = result.stdout.rstrip("\n") if result.returncode == 0 else ""
                            char_error, word_error = scores(expected, output)
                            cer.append(char_error)
                            wer.append(word_error)
                            recognized.append(output)
                        rows.append({
                            "case": case_index, "lines": len(lines), "shadow": shadow, "input_scale": scale,
                            "psm": psm, "dawg_enabled": dawg_enabled, "repeats": args.repeats,
                            "median_elapsed_seconds": statistics.median(elapsed),
                            "median_child_cpu_seconds": statistics.median(cpu),
                            "observed_child_peak_rss_bytes": max(peak_rss),
                            "mean_cer": statistics.mean(cer), "mean_wer": statistics.mean(wer),
                            "recognized_line_count": len(recognized[0].splitlines()),
                            "line_count_delta": abs(len(recognized[0].splitlines()) - len(lines)),
                            "recognized": recognized[0],
                        })
    report = {
        "tesseract_version": version,
        "method": "synthetic 5x7 bitmap glyphs rendered as PGM; direct Tesseract CLI, English, PSM 6/7, DAWG enabled/disabled; no SeConv or PGS/VobSub container path",
        "fixtures": [{"case": i, "ground_truth": "\n".join(lines)} for i, lines in enumerate(CASES)],
        "rows": rows,
        "limitations": [
            "Synthetic block glyphs are not representative of broadcast subtitle fonts, alpha, color palettes, outlines, or segmentation.",
            "Direct Tesseract measurements do not represent MVForge SeConv behavior.",
            "Correction false-positive/false-approval metrics and subtitle artifact integrity are not measured.",
            "Peak RSS is the child-process high-water mark observed by getrusage and may be cumulative on some platforms.",
        ],
    }
    (root / "report.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"report": str(root / "report.json"), "tesseract_version": version, "rows": len(rows)}))


if __name__ == "__main__":
    main()
