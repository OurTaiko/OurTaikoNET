import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
from types import SimpleNamespace
import threading
import time
import unittest

from distribution_mirror.catalogs import objects, player_objects
from distribution_mirror.publisher import Publisher


class CatalogTests(unittest.TestCase):
    def test_otanim_and_icon_are_part_of_the_same_inventory(self):
        ref = lambda path: {'path': path, 'size': 3, 'sha256': 'a' * 64}
        catalog = {'version': 1, 'base': {'face': ref('base/face.png')}, 'items': {
            'puchi': [{'file': ref('puchi/1.otanim'), 'icon': ref('icons/puchi/1.png')}]}}
        self.assertEqual(set(objects('3dchara', catalog)), {'base/face.png', 'puchi/1.otanim', 'icons/puchi/1.png'})

    def test_player_requires_index_and_rejects_escape_paths(self):
        manifest = {'path': '/player/0123456789abcdef/index.html', 'files': {'index.html': 'a' * 64, 'Build/Web.wasm.unityweb': 'b' * 64}}
        self.assertEqual(len(player_objects(manifest)), 2)
        for bad in ['../secret', '/absolute', 'a/../../secret', 'https://other.example/file']:
            with self.subTest(path=bad), self.assertRaises(ValueError):
                player_objects({**manifest, 'files': {**manifest['files'], bad: 'a' * 64}})


class PublicationTests(unittest.TestCase):
    def publisher(self, fail=False, changed=False):
        class Fake(Publisher):
            def __init__(self):
                self.events = []
                self.results = {}
                self.config = {'prefix': 'ourtaiko/'}
                self.reads = 0
            def catalog(self, path):
                self.reads += 1
                manifest = {'path': '/player/0123456789abcdef/index.html', 'files': {'index.html': 'a' * 64, 'Build/Web.wasm.unityweb': 'b' * 64}}
                body = json.dumps(manifest).encode()
                return (body + b' ' if changed and self.reads == 2 else body), manifest
            def ensure(self, path, size, digest):
                if fail and path.endswith('.unityweb'): raise ValueError('SHA mismatch')
                self.events.append(('verified', path))
            def bucket(self): return self
            def put_object(self, key, data, headers): self.events.append(('published', key))
        return Fake()

    def test_catalog_is_published_only_after_every_file_is_verified(self):
        publisher = self.publisher()
        publisher.publish('player')
        self.assertEqual(publisher.events[-1], ('published', 'ourtaiko/player-build.json'))
        self.assertEqual(sum(event[0] == 'verified' for event in publisher.events), 2)

    def test_corruption_or_upstream_update_keeps_the_previous_catalog(self):
        for options in [{'fail': True}, {'changed': True}]:
            with self.subTest(options=options):
                publisher = self.publisher(**options)
                with self.assertRaises(ValueError): publisher.publish('player')
                self.assertFalse(any(event[0] == 'published' for event in publisher.events))

    def test_virtual_mirror_head_requires_verified_upload_before_marking_ready(self):
        for corrupt in (False, True):
            with self.subTest(corrupt=corrupt), tempfile.TemporaryDirectory() as scratch:
                good = b'verified source'
                data = b'wrong source!!' if corrupt else good
                class Cloud:
                    written = None
                    def head_object(self, key):
                        return SimpleNamespace(content_length=len(good), headers=self.written[1] if self.written else {}, etag=hashlib.md5(good).hexdigest())
                    def put_object_from_file(self, key, path, headers):
                        self.written = Path(path).read_bytes(), headers
                class Source:
                    headers = {}
                    def __enter__(self): return self
                    def __exit__(self, *args): pass
                    def raise_for_status(self): pass
                    @property
                    def raw(self): return self
                    def stream(self, *args, **kwargs): yield data
                    def get(self, *args, **kwargs): return self
                cloud = Cloud()
                publisher = Publisher.__new__(Publisher)
                publisher.config = {'prefix': 'ourtaiko/', 'originBaseUrl': 'https://distrib.ourtaiko.org/'}
                publisher.bucket = lambda: cloud
                publisher.session = lambda: Source()
                publisher.scratch = Path(scratch)
                publisher.lock = threading.Lock(); publisher.downloaded = 0
                publisher.deadline = time.monotonic() + 10; publisher.maximum = 100
                digest = hashlib.sha256(good).hexdigest()
                if corrupt:
                    with self.assertRaises(ValueError): publisher.ensure('player/version/index.html', len(good), digest)
                    self.assertIsNone(cloud.written)
                else:
                    publisher.ensure('player/version/index.html', len(good), digest)
                    self.assertEqual(cloud.written[0], good)
                    self.assertEqual(cloud.written[1]['x-oss-meta-sha256'], digest)


class CloudRuleTests(unittest.TestCase):
    def test_scoped_rule_addition_is_idempotent_and_preserves_existing_routes(self):
        spec = importlib.util.spec_from_file_location('configure_mirror', Path(__file__).with_name('configure-distribution-mirror.py'))
        module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)
        config = json.loads((Path(__file__).parents[1] / 'deploy/aliyun-mirror.json').read_text())['oss']
        existing = b'<WebsiteConfiguration><IndexDocument><Suffix>index.html</Suffix></IndexDocument><RoutingRules><RoutingRule><RuleNumber>99</RuleNumber><Condition><KeyPrefixEquals>unrelated/</KeyPrefixEquals></Condition><Redirect><RedirectType>External</RedirectType></Redirect></RoutingRule></RoutingRules></WebsiteConfiguration>'
        desired = module.desired(existing, config)
        self.assertEqual(module.canonical(module.desired(desired, config)), module.canonical(desired))
        root = module.ET.fromstring(desired)
        self.assertEqual(root.findtext('IndexDocument/Suffix'), 'index.html')
        self.assertEqual(root.findtext('RoutingRules/RoutingRule/Condition/KeyPrefixEquals'), 'unrelated/')
        actual = module.ET.fromstring(desired)
        module.ET.SubElement(actual.find('RoutingRules/RoutingRule/Redirect'), 'ServerDefault').text = 'false'
        self.assertTrue(module.retained(actual, root))
        actual.find('RoutingRules/RoutingRule/Condition/KeyPrefixEquals').text = 'changed/'
        self.assertFalse(module.retained(actual, root))


if __name__ == '__main__': unittest.main()
