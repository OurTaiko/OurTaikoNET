#!/usr/bin/env python3
"""Writes catalog.json for a 3dchara folder (the bucket's 3dchara/ prefix, mirrored locally).

The folder:

    base/animations.glb        the 52 motions (skeleton only)
    base/face.png              the face sheet: square faces stacked top to bottom
    base/face_animations.json  which face shows when, per motion (YataiDON's texture_change entries)
    cos/<id>.glb               whole costumes
    head/<id>.glb, body/<id>.glb
    puchi/<id>.png             puchichara: two frames stacked top to bottom
    puchi/<id>.otanim          or an animated one (spine2otanim, format in SPEC.md, both in the
                               otanim folder beside this repository), best with an icon
                               (spine2otanim -icon) at icons/puchi/<id>.png
    paint/<id>.png             makeup: a face sheet like base/face.png, drawn instead of it on a
                               face plate a third larger (YataiDON's Models/paint/paint_<id>000.png)
    icons/cos/<id>.png, icons/head/<id>.png, icons/body/<id>.png, icons/puchi/<id>.png   (optional)
    names.json                 {"cos": {"12": {"ja": "...", "en": "...", "zh": "..."}}, ...} (optional)

Every file is listed with its path (relative to the catalog), size and SHA-256, so the game can
cache by content. A published file is never replaced: clients and the mirror keep it by its path
alone, so a changed model or picture gets a new path. A new costume is a new file (and a name);
run this again and upload the folder with publish.sh.

Usage: catalog.py <folder>   (writes <folder>/catalog.json)
"""
import hashlib
import json
import os
import re
import sys

KINDS = {"cos": (".glb",), "head": (".glb",), "body": (".glb",), "puchi": (".png", ".otanim"), "paint": (".png",)}
BASE = {"animations": "base/animations.glb", "face": "base/face.png", "faceAnimations": "base/face_animations.json"}
# YataiDON chara_default_color_1/2 and the rim: 1P's Don, and 2P's with the two swapped.
DEFAULT_COLORS = {"p1": ["#68BFC0", "#F94728", "#F9F0E1"], "p2": ["#F94728", "#68BFC0", "#F9F0E1"]}


def describe(root, path):
    full = os.path.join(root, path)
    with open(full, "rb") as f:
        data = f.read()
    return {"path": path, "size": len(data), "sha256": hashlib.sha256(data).hexdigest()}


def main(root):
    names = {}
    names_path = os.path.join(root, "names.json")
    if os.path.exists(names_path):
        with open(names_path, encoding="utf-8") as f:
            names = json.load(f)
    base = {}
    for key, path in BASE.items():
        if not os.path.exists(os.path.join(root, path)):
            sys.exit("missing " + path)
        base[key] = describe(root, path)
    items = {}
    for kind, exts in KINDS.items():
        folder = os.path.join(root, kind)
        found = {}
        for f in sorted(os.listdir(folder) if os.path.isdir(folder) else []):
            m = re.fullmatch(r"(\d+)(" + "|".join(map(re.escape, exts)) + ")", f)
            if not m:
                continue
            i = int(m.group(1))
            if i in found:
                sys.exit(f"{kind}/{i}: both {found[i]} and {f}; keep one")
            found[i] = f
        listed = []
        for i in sorted(found):
            if kind in ("puchi", "paint") and i == 0:
                continue  # 0 is "no puchichara", "no makeup"
            entry = {"id": i, "name": names.get(kind, {}).get(str(i), {}), "file": describe(root, f"{kind}/{found[i]}")}
            icon = f"icons/{kind}/{i}.png"
            if os.path.exists(os.path.join(root, icon)):
                entry["icon"] = describe(root, icon)
            listed.append(entry)
        items[kind] = listed
    catalog = {"version": 1, "base": base, "defaultColors": DEFAULT_COLORS, "items": items}
    with open(os.path.join(root, "catalog.json"), "w", encoding="utf-8") as f:
        json.dump(catalog, f, ensure_ascii=False, separators=(",", ":"))
        f.write("\n")
    print({k: len(v) for k, v in items.items()})


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
