#!/usr/bin/env python3
"""Verify and publish current art and web-player builds; never delete old objects."""
import argparse
from datetime import datetime, timezone
import fcntl
import json
import os
from pathlib import Path
from distribution_mirror.publisher import Publisher


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', type=Path, default=Path(__file__).resolve().parents[1] / 'deploy/aliyun-mirror.json')
    parser.add_argument('--credentials', default='/root/.ossutilconfig')
    parser.add_argument('--scratch', type=Path, required=True)
    parser.add_argument('--state', type=Path)
    parser.add_argument('--collections', nargs='+', choices=['3dchara', 'nameplate', 'player'], default=['3dchara', 'nameplate', 'player'])
    args = parser.parse_args()
    args.scratch.mkdir(parents=True, exist_ok=True)
    with (args.scratch / 'sync.lock').open('w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        publisher = Publisher(json.loads(args.config.read_text()), args.credentials, args.scratch)
        result = publisher.sync(args.collections)
        result['completedAt'] = datetime.now(timezone.utc).isoformat()
        if args.state:
            args.state.parent.mkdir(parents=True, exist_ok=True)
            temporary = args.state.with_suffix('.tmp')
            temporary.write_text(json.dumps(result, indent=2) + '\n')
            os.replace(temporary, args.state)
        print(json.dumps(result), flush=True)


if __name__ == '__main__':
    main()
