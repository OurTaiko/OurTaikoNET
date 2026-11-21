import re

PATH = re.compile(r"[a-zA-Z0-9_-][a-zA-Z0-9_.-]*(/[a-zA-Z0-9_-][a-zA-Z0-9_.-]*)*")
DIGEST = re.compile(r"[a-f0-9]{64}")


def valid_path(path):
    return isinstance(path, str) and PATH.fullmatch(path) and all(not p.endswith('.') for p in path.split('/'))


def references(collection, catalog):
    if type(catalog['version']) is not int or catalog['version'] != 1:
        raise ValueError(f'unsupported {collection} catalog version')
    if collection == 'nameplate':
        yield from catalog['files'].values()
        return
    yield from catalog['base'].values()
    for items in catalog['items'].values():
        for item in items:
            yield item['file']
            if 'icon' in item:
                yield item['icon']


def objects(collection, catalog):
    result = {}
    for entry in references(collection, catalog):
        path, size, digest = entry['path'], entry['size'], entry['sha256']
        if (not valid_path(path) or path.lower() == 'catalog.json' or type(size) is not int
                or not 0 < size <= 256 * 1024**2 or not isinstance(digest, str) or not DIGEST.fullmatch(digest)):
            raise ValueError(f'invalid {collection} catalog entry')
        if result.setdefault(path, (size, digest)) != (size, digest):
            raise ValueError(f'conflicting {collection} paths')
    if not result or len(result) > 20000:
        raise ValueError(f'empty or oversized {collection} catalog')
    return result


# Web builds published as a never-changed folder named by a manifest: the web player
# (OurTaikoView_Web, player-build.json) and OurTaikoEdit's web version (edit-nightly.json),
# both {"path": "/<root>/<16 hex>/index.html", "files": {name: sha256}}.
BUILDS = {'player': ('player-build.json', 'player'), 'edit': ('edit-nightly.json', 'edit')}


def build_objects(manifest, root):
    path = manifest.get('path')
    files = manifest.get('files')
    if (not isinstance(path, str) or not re.fullmatch(rf'/{root}/[a-f0-9]{{16}}/index\.html', path)
            or not isinstance(files, dict) or not 0 < len(files) <= 4096 or 'index.html' not in files):
        raise ValueError(f'invalid {root} manifest')
    base = path.removesuffix('index.html').lstrip('/')
    result = {}
    for name, digest in files.items():
        if not valid_path(name) or not isinstance(digest, str) or not DIGEST.fullmatch(digest):
            raise ValueError(f'invalid {root} file')
        result[base + name] = (None, digest)
    return result


def player_objects(manifest):
    return build_objects(manifest, 'player')
