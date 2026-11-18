import base64
from concurrent.futures import ThreadPoolExecutor
import configparser
import hashlib
import json
import mimetypes
from pathlib import Path
from tempfile import NamedTemporaryFile
import threading
import time

import oss2
import requests

from .catalogs import objects, player_objects


class Publisher:
    def __init__(self, config, credentials, scratch, deadline=840, max_download=2 * 1024**3):
        self.config = config['oss']
        self.scratch = Path(scratch)
        self.scratch.mkdir(parents=True, exist_ok=True)
        self.deadline = time.monotonic() + deadline
        self.maximum = max_download
        self.downloaded = 0
        self.local = threading.local()
        self.lock = threading.Lock()
        ini = configparser.ConfigParser()
        if not ini.read(credentials):
            raise ValueError('OSS credential file unavailable')
        section = next(ini[name] for name in ini.sections() if 'accesskeyid' in ini[name])
        self.auth = oss2.Auth(section['accesskeyid'], section['accesskeysecret'])
        self.results = {}

    def budget(self, count=0):
        with self.lock:
            self.downloaded += count
            if time.monotonic() > self.deadline or self.downloaded > self.maximum:
                raise TimeoutError('Mirror synchronization budget reached')

    def bucket(self):
        if not hasattr(self.local, 'bucket'):
            self.local.bucket = oss2.Bucket(self.auth, self.config['endpoint'], self.config['bucket'], connect_timeout=20)
        return self.local.bucket

    def session(self):
        if not hasattr(self.local, 'session'):
            self.local.session = requests.Session()
            self.local.session.trust_env = False
            # Catalog SHA-256 values are of the stored bytes. CloudFront compresses .js/.html
            # on the fly for gzip/br requests, which would change the raw bytes read below;
            # identity returns the object as stored (with its own Content-Encoding, if any).
            self.local.session.headers['Accept-Encoding'] = 'identity'
            # Operator-selected proxy is confined to this sync worker.
            import os
            if os.environ.get('OURTAIKO_MIRROR_PROXY'):
                self.local.session.proxies = {'https': os.environ['OURTAIKO_MIRROR_PROXY']}
        return self.local.session

    def catalog(self, path):
        self.budget()
        with self.session().get(self.config['originBaseUrl'].rstrip('/') + '/' + path,
                                timeout=(8, 30), stream=True) as response:
            response.raise_for_status()
            parts, size = [], 0
            for part in response.iter_content(65536):
                self.budget(len(part))
                size += len(part)
                if size > 8 * 1024**2:
                    raise ValueError('Distribution catalog exceeds limit')
                parts.append(part)
            body = b''.join(parts)
        return body, json.loads(body)

    @staticmethod
    def headers(path, digest=None, encoding=None):
        media = mimetypes.guess_type(path)[0] or 'application/octet-stream'
        if path.endswith('.wasm'): media = 'application/wasm'
        if path.endswith('.unityweb'): media = 'application/octet-stream'
        result = {'Content-Type': media, 'Cache-Control': 'public, max-age=31536000, no-transform'}
        if digest: result['x-oss-meta-sha256'] = digest
        if encoding: result['Content-Encoding'] = encoding
        return result

    def ensure(self, path, size, digest):
        self.budget()
        key = self.config['prefix'] + path
        bucket = self.bucket()
        head = None
        try:
            head = bucket.head_object(key)
            if (size is None or head.content_length == size) and head.headers.get('x-oss-meta-sha256') == digest:
                return
        except oss2.exceptions.NoSuchKey:
            pass
        # HEAD may be satisfied by mirror back-to-origin before the object is
        # persisted. Publish unmarked objects ourselves after verifying source
        # bytes, rather than racing an asynchronous mirror save with metadata COPY.
        url = self.config['originBaseUrl'].rstrip('/') + '/' + path
        with NamedTemporaryFile(dir=self.scratch) as temporary:
            sha, md5, length = hashlib.sha256(), hashlib.md5(), 0
            with self.session().get(url, timeout=(8, 45), stream=True) as response:
                response.raise_for_status()
                encoding = response.headers.get('Content-Encoding')
                for part in response.raw.stream(256 * 1024, decode_content=False):
                    self.budget(len(part))
                    length += len(part)
                    if length > (size or 512 * 1024**2): raise ValueError('Source object exceeds declared size')
                    sha.update(part); md5.update(part); temporary.write(part)
            if (size is not None and length != size) or sha.hexdigest() != digest:
                raise ValueError('Source SHA-256 does not match the published catalog')
            temporary.flush()
            headers = self.headers(path, digest, encoding)
            headers['Content-MD5'] = base64.b64encode(md5.digest()).decode()
            bucket.put_object_from_file(key, temporary.name, headers=headers)
            uploaded = bucket.head_object(key)
            if (uploaded.content_length != length or uploaded.etag.lower() != md5.hexdigest()
                    or uploaded.headers.get('x-oss-meta-sha256') != digest):
                raise ValueError('OSS upload verification failed')

    def publish(self, collection):
        manifest = 'player-build.json' if collection == 'player' else collection + '/catalog.json'
        raw, catalog = self.catalog(manifest)
        files = player_objects(catalog) if collection == 'player' else {
            collection + '/' + name: ref for name, ref in objects(collection, catalog).items()
        }
        # executor.map keeps only a fixed small set of active transfers.
        with ThreadPoolExecutor(max_workers=2) as executor:
            for _ in executor.map(lambda entry: self.ensure(entry[0], *entry[1]), files.items()):
                pass
        latest, _ = self.catalog(manifest)
        if latest != raw:
            raise ValueError('Canonical catalog changed during synchronization; retry next run')
        self.bucket().put_object(self.config['prefix'] + manifest, raw, headers={
            'Content-Type': 'application/json', 'Cache-Control': 'public, max-age=60, no-transform',
            'Content-MD5': base64.b64encode(hashlib.md5(raw).digest()).decode(),
        })
        self.results[collection] = len(files)
        print(f'{collection}: published catalog after verifying {len(files)} objects', flush=True)

    def copy_json(self, path):
        """Copies one JSON file the origin may change (it is not in a catalog) as it is now.

        OSS back-to-origin keeps the first copy it fetched and never fetches it again, so every
        JSON file on the mirror is rewritten here; the files catalogs list are kept by publish.
        """
        raw, _ = self.catalog(path)
        self.bucket().put_object(self.config['prefix'] + path, raw, headers={
            'Content-Type': 'application/json', 'Cache-Control': 'public, max-age=60, no-transform',
            'Content-MD5': base64.b64encode(hashlib.md5(raw).digest()).decode(),
        })
        self.results[path] = 1
        print(f'{path}: copied', flush=True)

    def sync(self, collections, json_files=()):
        for collection in collections:
            self.publish(collection)
        for path in json_files:
            self.copy_json(path)
        return {'collections': self.results, 'verifiedBytesRead': self.downloaded}
