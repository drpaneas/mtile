#!/usr/bin/env python3
"""Convert every PICO-8 cart PNG in a corpus to deterministic text carts."""

from __future__ import annotations

import argparse
import json
import shutil
import struct
import subprocess
from pathlib import Path


PICO_CART_SIZE = (160, 205)


def png_size(path: Path) -> tuple[int, int] | None:
    with path.open("rb") as source:
        header = source.read(24)
    if len(header) != 24 or header[:8] != b"\x89PNG\r\n\x1a\n" or header[12:16] != b"IHDR":
        return None
    return struct.unpack(">II", header[16:24])


def cart_pngs(corpus: Path) -> list[Path]:
    return sorted(
        path
        for path in corpus.rglob("*.png")
        if path.is_file() and png_size(path) == PICO_CART_SIZE
    )


def convert(corpus: Path, output_dir: Path, pico8: Path) -> dict[str, object]:
    corpus = corpus.resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    records: list[dict[str, str]] = []
    sources = cart_pngs(corpus)
    for index, source in enumerate(sources):
        case_dir = output_dir / f"{index:03d}"
        case_dir.mkdir(parents=True, exist_ok=True)
        copied = case_dir / "source.p8.png"
        target = case_dir / "converted.p8"
        if not target.is_file() or target.stat().st_size == 0:
            shutil.copy2(source, copied)
            result = subprocess.run(
                [
                    str(pico8),
                    "-root_path",
                    str(case_dir),
                    "-desktop",
                    str(case_dir),
                    copied.name,
                    "-export",
                    target.name,
                ],
                cwd=case_dir,
                check=False,
                capture_output=True,
                text=True,
                timeout=30,
            )
            if result.returncode != 0 or not target.is_file() or target.stat().st_size == 0:
                raise RuntimeError(
                    f"PICO-8 failed to convert {source}: status={result.returncode}\n"
                    f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
                )
        records.append(
            {
                "source": source.relative_to(corpus).as_posix(),
                "cart": target.relative_to(output_dir).as_posix(),
            }
        )
        print(f"[{index + 1}/{len(sources)}] {source.relative_to(corpus)}")
    manifest: dict[str, object] = {"version": 1, "carts": records}
    (output_dir / "manifest.json").write_text(
        json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )
    return manifest


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("corpus", type=Path)
    parser.add_argument("output_dir", type=Path)
    parser.add_argument(
        "--pico8",
        type=Path,
        default=Path("/Applications/PICO-8.app/Contents/MacOS/pico8"),
    )
    args = parser.parse_args()
    convert(args.corpus, args.output_dir.resolve(), args.pico8.resolve())


if __name__ == "__main__":
    main()
