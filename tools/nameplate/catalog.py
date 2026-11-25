#!/usr/bin/env python3
"""Writes catalog.json for a nameplate folder (the bucket's nameplate/ prefix, mirrored locally).

The folder holds the nameplate art (the Nijiiro skin's global/nameplate plus OurTaikoPlay's own
段位 rings and texts), all drawn on the 408x96 plate canvas. Both the website and the game read it
from the bucket; the game keeps no copy and caches what it fetches at sign-in:

    shadow.png, outline.png        the plate and the band's outline, 408x96
    frame_top.png                  the 5 title bands, 408x56 each, stacked top to bottom
    frame_top_rainbow.png          the rainbow band's 6 frames, 408x56 each, top to bottom
    dan_emblem_bg.png              the 段位 chip's plate, 160x48
    dan_ring_{silver,gold,rainbow}.png   the 段位 ring by the pass's combo, 72x40
    dan_text.png, dan_text_gold.png      the 25 段位 names (0 初級 .. 24 達人), 72x40 cells,
                                   5 to a row, left to right then top to bottom (0..5 are blank)
    1p.png, 2p.png                 the seat badges, 88x88

Every file is listed with its path (relative to the catalog), size and SHA-256. A published file
is never replaced: clients and the mirror keep it by its path alone, so changed art gets a new
file name.

Released game versions read this catalog; the layered set newer ones and the website draw is
published beside it by lumen.py (lumen.json and lumen/).

Usage: catalog.py <folder>   (writes <folder>/catalog.json)
"""
import hashlib
import json
import os
import sys

FILES = {
    "shadow": "shadow.png",
    "outline": "outline.png",
    "bands": "frame_top.png",
    "rainbow": "frame_top_rainbow.png",
    "danBackground": "dan_emblem_bg.png",
    "danRingSilver": "dan_ring_silver.png",
    "danRingGold": "dan_ring_gold.png",
    "danRingRainbow": "dan_ring_rainbow.png",
    "danText": "dan_text.png",
    "danTextGold": "dan_text_gold.png",
    "badge1p": "1p.png",
    "badge2p": "2p.png",
}


def describe(root, path):
    with open(os.path.join(root, path), "rb") as f:
        data = f.read()
    return {"path": path, "size": len(data), "sha256": hashlib.sha256(data).hexdigest()}


def main(root):
    missing = [p for p in FILES.values() if not os.path.exists(os.path.join(root, p))]
    if missing:
        sys.exit("missing " + ", ".join(missing))
    catalog = {"version": 1, "files": {key: describe(root, path) for key, path in FILES.items()}}
    with open(os.path.join(root, "catalog.json"), "w", encoding="utf-8") as f:
        json.dump(catalog, f, ensure_ascii=False, separators=(",", ":"))
        f.write("\n")
    print(f"{len(FILES)} files")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
