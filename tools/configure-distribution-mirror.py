#!/usr/bin/env python3
"""Add only the configured public OSS mirror scopes, preserving all other rules."""
import argparse
import configparser
import copy
import json
from pathlib import Path
import xml.etree.ElementTree as ET
import oss2


def canonical(data):
    if not data: return None
    root = ET.fromstring(data)
    for node in root.iter():
        if node.text is not None and not node.text.strip(): node.text = None
        if node.tail is not None and not node.tail.strip(): node.tail = None
    return ET.tostring(root)


def desired(current, config):
    root = ET.fromstring(current) if current else ET.Element('WebsiteConfiguration')
    rules = root.find('RoutingRules')
    if rules is None: rules = ET.SubElement(root, 'RoutingRules')
    existing = {(rule.findtext('Condition/KeyPrefixEquals'), rule.findtext('Condition/KeySuffixEquals')): rule for rule in rules}
    collections = [{'collection': c, 'suffixes': config['mirrorSuffixes']} for c in config['mirrorCollections']]
    collections += config.get('additionalMirrorCollections', [])
    number = max((int(rule.findtext('RuleNumber', '0')) for rule in rules), default=0)
    for collection in collections:
        for suffix in collection['suffixes']:
            # An empty collection is the whole prefix: one rule for every path under it.
            folder = collection['collection'] + '/' if collection['collection'] else ''
            prefix = config['prefix'] + folder
            origin = config['originBaseUrl'] + folder
            old = existing.get((prefix, suffix or None))
            if old is not None:
                if old.findtext('Condition/HttpErrorCodeReturnedEquals') != '404' or old.findtext('Redirect/MirrorURL') != origin:
                    raise ValueError('Existing mirror scope has different behavior; review before updating')
                continue
            number += 1
            rule = ET.SubElement(rules, 'RoutingRule')
            ET.SubElement(rule, 'RuleNumber').text = str(number)
            condition = ET.SubElement(rule, 'Condition')
            values = [('KeyPrefixEquals', prefix), ('HttpErrorCodeReturnedEquals', '404')]
            if suffix: values.insert(1, ('KeySuffixEquals', suffix))
            for key, value in values:
                ET.SubElement(condition, key).text = value
            redirect = ET.SubElement(rule, 'Redirect')
            for key, value in [('RedirectType', 'Mirror'), ('MirrorURL', origin), ('EnableReplacePrefix', 'true'),
                               ('MirrorSNI', 'true'), ('MirrorAllowHeadObject', 'true'), ('MirrorFollowRedirect', 'false'),
                               ('MirrorPassQueryString', 'false'), ('MirrorCheckMd5', 'true')]:
                ET.SubElement(redirect, key).text = value
            ET.SubElement(ET.SubElement(redirect, 'MirrorHeaders'), 'PassAll').text = 'false'
    return ET.tostring(root)


def retained(actual, expected):
    # OSS reorders Redirect children and inserts default flags on PUT.
    # Verify every requested field, while accepting those server-added defaults.
    if actual.tag != expected.tag or (expected.text or '').strip() != (actual.text or '').strip():
        return False
    for tag in {child.tag for child in expected}:
        wanted = expected.findall(tag)
        found = actual.findall(tag)
        if len(found) != len(wanted) or any(not retained(a, b) for a, b in zip(found, wanted)):
            return False
    return True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', type=Path, default=Path(__file__).resolve().parents[1] / 'deploy/aliyun-mirror.json')
    parser.add_argument('--credentials', default='/root/.ossutilconfig')
    parser.add_argument('--backup', type=Path, required=True)
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    config = json.loads(args.config.read_text())['oss']
    ini = configparser.ConfigParser(); ini.read(args.credentials)
    section = next(ini[s] for s in ini.sections() if 'accesskeyid' in ini[s])
    bucket = oss2.Bucket(oss2.Auth(section['accesskeyid'], section['accesskeysecret']), config['endpoint'], config['bucket'])
    def read():
        try: return bucket._Bucket__do_bucket('GET', params={'website': ''}).read(262144)
        except oss2.exceptions.NoSuchWebsite: return None
    before = read()
    args.backup.write_bytes(before or b'')
    after = desired(before, config)
    print('Configured public mirror rules:', len(ET.fromstring(after).find('RoutingRules')))
    if args.apply and canonical(before) != canonical(after):
        if read() != before: raise ValueError('OSS website configuration changed concurrently')
        bucket._Bucket__do_bucket('PUT', params={'website': ''}, data=after, headers={'Content-MD5': oss2.utils.content_md5(after)})
        actual = read()
        args.backup.with_suffix('.after.xml').write_bytes(actual)
        if not retained(ET.fromstring(actual), ET.fromstring(after)):
            raise ValueError('OSS did not retain the desired mirror rules')
        print('Public mirror scopes updated; existing rules preserved')


if __name__ == '__main__': main()
