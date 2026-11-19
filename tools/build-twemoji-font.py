# /// script
# requires-python = ">=3.12"
# dependencies = [
#   "nanoemoji==0.16.0",
#   "fonttools[woff]==4.68.0",
# ]
# ///
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later
"""Build the Twemoji color font that the frontend bundles.

The frontend commits the result, so normal builds and CI never run this
script. Run it only to change the Twemoji release or the build steps:

    mise update-twemoji-font

The script downloads one pinned jdecked/twemoji release, checks its SHA-256
checksum, and builds a COLRv0 font with nanoemoji. COLRv0 renders in
Chromium, Firefox, and Safari. The steps below are necessary for correct
rendering:

- The SVG file names give the code point sequence of each glyph. The script
  removes U+FE0F from these sequences. HarfBuzz ignores U+FE0F when it matches
  ligatures, so a ligature that contains U+FE0F never matches.
- The script removes U+0020 from the character map. nanoemoji adds it with the
  width of an emoji.
- The script declares the emoji presentation sequence (base + U+FE0F) of each
  mapped character in a format 14 character map.
"""

import hashlib
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
from pathlib import Path

from fontTools.ttLib import TTFont
from fontTools.ttLib.tables._c_m_a_p import CmapSubtable

TWEMOJI_VERSION = "17.0.3"
TWEMOJI_URL = f"https://github.com/jdecked/twemoji/archive/refs/tags/v{TWEMOJI_VERSION}.tar.gz"
TWEMOJI_SHA256 = "a0855654b633045ae2337537e77f1bb4361162f7fcd910e613eaab1d6d9c5fca"

FAMILY = "Twemoji"
REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
OUTPUT = REPOSITORY_ROOT / "apps/frontend/src/lib/assets/fonts/twemoji.woff2"

# A fixed timestamp keeps the output identical across builds of the same input.
# 2026-01-01T00:00:00Z, in seconds since 1904-01-01 (the OpenType epoch).
FIXED_TIMESTAMP = 3850070400

COPYRIGHT = "Copyright 2014-2021 Twitter; Copyright 2022-present Jason Sofonia & Justine De Caires"
LICENSE = "Graphics licensed under CC-BY 4.0: https://creativecommons.org/licenses/by/4.0/"


def download(destination: Path) -> None:
    """Download the pinned Twemoji release and stop if its checksum differs."""
    with urllib.request.urlopen(TWEMOJI_URL) as response:
        data = response.read()
    digest = hashlib.sha256(data).hexdigest()
    if digest != TWEMOJI_SHA256:
        sys.exit(f"Checksum mismatch for {TWEMOJI_URL}: got {digest}")
    destination.write_bytes(data)


def collect_svgs(archive: Path, work: Path) -> list[Path]:
    """Extract the SVGs and give them the file names that nanoemoji reads.

    nanoemoji reads the code points from names such as `emoji_u1f3f3_200d_1f308.svg`.
    """
    extracted = work / "extracted"
    with tarfile.open(archive) as tar:
        tar.extractall(extracted, filter="data")
    sources = sorted(extracted.glob("*/assets/svg/*.svg"))
    if not sources:
        sys.exit("The Twemoji archive contains no SVG files.")

    renamed = work / "svg"
    renamed.mkdir()
    result = []
    for source in sources:
        codepoints = [cp for cp in source.stem.split("-") if cp != "fe0f"]
        target = renamed / f"emoji_u{'_'.join(codepoints)}.svg"
        if target.exists():
            sys.exit(f"Two Twemoji files map to the same sequence: {source.name}")
        shutil.copyfile(source, target)
        result.append(target)
    return result


def build_ttf(svgs: list[Path], work: Path) -> Path:
    """Build a COLRv0 TrueType font from the SVGs."""
    nanoemoji = Path(sys.executable).parent / "nanoemoji"
    build_dir = work / "build"
    subprocess.run(
        [
            str(nanoemoji),
            "--family", FAMILY,
            "--color_format", "glyf_colr_0",
            "--build_dir", str(build_dir),
            "--output_file", "Twemoji.ttf",
            *map(str, svgs),
        ],
        check=True,
    )
    return build_dir / "Twemoji.ttf"


def finish(ttf: Path) -> None:
    """Correct the character map and metadata, then write the WOFF2 file."""
    font = TTFont(ttf)
    cmap = font["cmap"]
    for table in cmap.tables:
        if hasattr(table, "cmap"):
            table.cmap.pop(0x20, None)

    mapped = font.getBestCmap()
    sequences = CmapSubtable.newSubtable(14)
    sequences.platformID, sequences.platEncID, sequences.language = 0, 5, 0
    sequences.cmap = {}
    sequences.uvsDict = {
        0xFE0F: [(cp, None) for cp in sorted(mapped) if cp not in (0x200D, 0xFE0F)]
    }
    cmap.tables.append(sequences)

    name = font["name"]
    name.setName(COPYRIGHT, 0, 3, 1, 0x409)
    name.setName(f"Version {TWEMOJI_VERSION}", 5, 3, 1, 0x409)
    name.setName(LICENSE, 13, 3, 1, 0x409)
    name.setName("https://creativecommons.org/licenses/by/4.0/", 14, 3, 1, 0x409)

    font["head"].created = FIXED_TIMESTAMP
    font["head"].modified = FIXED_TIMESTAMP
    font.recalcTimestamp = False
    font.flavor = "woff2"
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    font.save(OUTPUT)


def main() -> None:
    with tempfile.TemporaryDirectory() as directory:
        work = Path(directory)
        archive = work / "twemoji.tar.gz"
        download(archive)
        svgs = collect_svgs(archive, work)
        finish(build_ttf(svgs, work))
    print(f"Wrote {OUTPUT.relative_to(REPOSITORY_ROOT)} ({OUTPUT.stat().st_size} bytes)")


if __name__ == "__main__":
    main()
