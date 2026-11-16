#!/usr/bin/env python3
"""Stage the published 3dchara and nameplate catalog objects for a distribution mirror."""

import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
from http.client import HTTPException
import json
from pathlib import Path
import re
import sys
from tempfile import NamedTemporaryFile
import time
from urllib.request import urlopen


DISTRIB = "https://distrib.ourtaiko.org"
COLLECTIONS = ("3dchara", "nameplate")


def fetch(url, consume):
    for attempt in range(3):
        try:
            with urlopen(url, timeout=25) as response:
                return consume(response)
        except (HTTPException, OSError, ValueError) as error:
            if attempt == 2:
                raise RuntimeError(f"{url}: {error}") from error
            time.sleep(attempt + 1)


def references(collection, catalog):
    if type(catalog["version"]) is not int or catalog["version"] != 1:
        raise ValueError(f"unsupported {collection} catalog version")
    if collection == "nameplate":
        yield from catalog["files"].values()
        return
    yield from catalog["base"].values()
    for items in catalog["items"].values():
        for item in items:
            yield item["file"]
            if "icon" in item:
                yield item["icon"]


def objects(collection, catalog):
    result = {}
    for entry in references(collection, catalog):
        path, size, digest = entry["path"], entry["size"], entry["sha256"]
        if (not isinstance(path, str) or path.lower() == "catalog.json"
                or not all(re.fullmatch(r"[a-zA-Z0-9_-][a-zA-Z0-9_.-]*", part) and not part.endswith(".")
                           for part in path.split("/"))
                or type(size) is not int or size < 0
                or not isinstance(digest, str) or not re.fullmatch(r"[0-9a-f]{64}", digest)):
            raise ValueError(f"invalid {collection} catalog entry: {entry!r}")
        if result.setdefault(path, (size, digest)) != (size, digest):
            raise ValueError(f"conflicting catalog entries for {collection}/{path}")
    if not result:
        raise ValueError(f"empty {collection} catalog")
    return result


def matches(path, size, digest):
    if not path.is_file() or path.stat().st_size != size:
        return False
    with path.open("rb") as file:
        return hashlib.file_digest(file, "sha256").hexdigest() == digest


def stage_object(base, root, path, size, digest):
    target = root / path
    if matches(target, size, digest):
        return
    target.parent.mkdir(parents=True, exist_ok=True)

    def write(response):
        temporary = None
        try:
            with NamedTemporaryFile(dir=target.parent, delete=False) as file:
                temporary = Path(file.name)
                checksum = hashlib.sha256()
                length = 0
                while chunk := response.read(min(1024 * 1024, size - length + 1)):
                    length += len(chunk)
                    if length > size:
                        raise ValueError(f"expected {size} bytes")
                    checksum.update(chunk)
                    file.write(chunk)
            if length != size or checksum.hexdigest() != digest:
                raise ValueError("size or SHA-256 does not match the catalog")
            temporary.replace(target)
        finally:
            if temporary is not None:
                temporary.unlink(missing_ok=True)

    fetch(f"{base}/{path}?v={digest[:12]}", write)


def stage(collection, destination):
    base = f"{DISTRIB}/{collection}"
    root = destination / collection
    catalog = fetch(f"{base}/catalog.json", lambda response: response.read())
    files = objects(collection, json.loads(catalog))
    with ThreadPoolExecutor(max_workers=8) as pool:
        jobs = [pool.submit(stage_object, base, root, path, *metadata)
                for path, metadata in files.items()]
        for job in jobs:
            job.result()

    temporary = None
    try:
        with NamedTemporaryFile(dir=root, delete=False) as file:
            temporary = Path(file.name)
            file.write(catalog)
        temporary.replace(root / "catalog.json")
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)
    print(f"{collection}: verified {len(files)} objects", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("destination", type=Path)
    parser.add_argument("collection", nargs="?", choices=COLLECTIONS)
    args = parser.parse_args()
    try:
        for collection in (args.collection,) if args.collection else COLLECTIONS:
            stage(collection, args.destination)
    except (KeyError, OSError, RuntimeError, TypeError, ValueError) as error:
        sys.exit(str(error))


if __name__ == "__main__":
    main()
