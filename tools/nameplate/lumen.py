#!/usr/bin/env python3
"""Writes the layered nameplate set for the bucket's nameplate/ prefix: lumen.json and lumen/.

The set is the one OurTaikoPlay carries in Assets/OurTaiko/Resources/Nameplate (made there by
Tools/nameplate-samples/lumen from the arcade nameplate's timeline): every layer a title can be
made of, as .otanim drawn on the whole 408x96 plate canvas, and the plate's own parts.

    band/<name>.otanim         a title's band
    effect/<name>.otanim       what is drawn over a band (glints, sweeps)
    icon/<name>.otanim         the icon on a band's end (characters, crowns, rings)
    composition/<name>.otanim  the plate itself: plates (one clip a layout), badges, blank and
                               the 段位 emblems dan-clear, dan-full, dan-donfull
    composition/catalog.json   the game's catalog: the layouts (states, each an ordered list of
                               what to draw and where the text goes), the titles and the layers
                               each is made of, and the effects that may leave the band

Each .otanim is written to lumen/<folder>/<name>-<first 12 hex of its SHA-256>.otanim, so a
changed file gets a new name: a published file is never replaced (the mirror keeps the first copy
it reads). lumen.json is

    {"version": 1, "catalog": <the game's catalog>,
     "files": {"<folder>/<name>": {"path": "lumen/...", "size": n, "sha256": "..."}, ...}}

with each path relative to the nameplate folder, as catalog.json's are. A title's layers are
named by their keys in files ("band/lumen-title-gold"), the ids titles keep in the database.

Usage: lumen.py <Resources/Nameplate folder> <output folder>
"""
import hashlib
import json
import os
import re
import shutil
import sys

FOLDERS = ("band", "effect", "icon", "composition")
NAME = re.compile(r"^[a-z0-9][a-z0-9_.-]{0,63}$")


def main(source, output):
    with open(os.path.join(source, "composition", "catalog.json"), encoding="utf-8") as f:
        catalog = json.load(f)
    files = {}
    for folder in FOLDERS:
        for entry in sorted(os.listdir(os.path.join(source, folder))):
            if not entry.endswith(".otanim"):
                continue
            name = entry[: -len(".otanim")]
            if not NAME.match(name):
                sys.exit(f"{folder}/{entry}: not a name the catalog can carry")
            with open(os.path.join(source, folder, entry), "rb") as f:
                data = f.read()
            digest = hashlib.sha256(data).hexdigest()
            path = f"lumen/{folder}/{name}-{digest[:12]}.otanim"
            os.makedirs(os.path.dirname(os.path.join(output, path)), exist_ok=True)
            shutil.copyfile(os.path.join(source, folder, entry), os.path.join(output, path))
            files[f"{folder}/{name}"] = {"path": path, "size": len(data), "sha256": digest}

    # Everything the catalog names is in the set.
    named = set()
    for titles in (catalog["titles"], catalog["aiTitles"]):
        for label, parts in titles.items():
            if "band" not in parts:
                sys.exit(f"title {label} has no band")
            named.update(parts.values())
    named.update(catalog.get("unclipped", []))
    named.update(f"composition/{n}" for n in ("plates", "badges", "blank", "dan-clear", "dan-full", "dan-donfull"))
    missing = sorted(named - files.keys())
    if missing:
        sys.exit("missing " + ", ".join(missing))

    with open(os.path.join(output, "lumen.json"), "w", encoding="utf-8") as f:
        json.dump({"version": 1, "catalog": catalog, "files": files}, f, ensure_ascii=False, separators=(",", ":"))
        f.write("\n")
    print(f"{len(files)} files, {len(catalog['titles'])} titles, {len(catalog['aiTitles'])} AI titles")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2])
