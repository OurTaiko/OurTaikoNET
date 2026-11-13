#!/usr/bin/env python3
"""Asks an old song site and OurTaikoNet the same questions and compares the answers.

Both must be running on the same data: the old site on its own database, OurTaikoNet on a
database imported from it. Every public GET the game and the website make is asked three ways:

  old site                         GET <old>/api/v1/...
  OurTaikoNet, old address         GET <new>/api/v1/...            Host: <legacy host>
  OurTaikoNet, new address         GET <new>/api/v1/sources/<source>/...

and the three bodies must be the same JSON.

  tools/compare-answers.py --old http://127.0.0.1:18081 --new http://127.0.0.1:18090 \\
      --legacy-host database.ourtaiko.org --source official [--charts 200] [--user <id> ...]

Exit status 1 when any answer differs.
"""
import argparse
import json
import sys
import urllib.error
import urllib.request


def get(base, path, host=None):
    request = urllib.request.Request(base + path)
    if host:
        request.add_header("Host", host)
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


IGNORED = set()
# Fields left out of the answers to one question only: {path: {field}}.
IGNORED_IN = {}


def without_ignored(value, fields):
    if isinstance(value, dict):
        return {k: without_ignored(v, fields) for k, v in value.items() if k not in fields}
    if isinstance(value, list):
        return [without_ignored(v, fields) for v in value]
    return value


def parsed(body, path=""):
    try:
        value = json.loads(body)
    except ValueError:
        return body
    # An error names the request it answers with a random id: never the same twice.
    if isinstance(value, dict) and "code" in value:
        value.pop("requestId", None)
    # A field left out of a question is left out of it whatever is asked after its "?".
    return without_ignored(value, IGNORED | IGNORED_IN.get(path, set()) | IGNORED_IN.get(path.split("?")[0], set()))


def where(a, b, path=""):
    """The first place two JSON values differ, for the report."""
    if type(a) is not type(b):
        return f"{path or '/'}: {type(a).__name__} against {type(b).__name__}"
    if isinstance(a, dict):
        for key in sorted(set(a) | set(b)):
            if key not in a or key not in b:
                return f"{path}/{key}: only in {'old' if key in a else 'new'}"
            found = where(a[key], b[key], f"{path}/{key}")
            if found:
                return found
        return None
    if isinstance(a, list):
        if len(a) != len(b):
            return f"{path or '/'}: {len(a)} items against {len(b)}"
        for index, (x, y) in enumerate(zip(a, b)):
            found = where(x, y, f"{path}/{index}")
            if found:
                return found
        return None
    return None if a == b else f"{path or '/'}: {str(a)[:60]!r} against {str(b)[:60]!r}"


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--old", required=True)
    p.add_argument("--new", required=True)
    p.add_argument("--legacy-host", required=True)
    p.add_argument("--source", help="the song source the old site is, or")
    p.add_argument("--players", action="store_true", help="the old site is the players' site (OurTaikoProfile)")
    p.add_argument("--charts", type=int, default=200, help="how many charts to ask about one by one (0 = all)")
    p.add_argument("--user", action="append", default=[], help="a player whose scores to compare")
    p.add_argument("--show", type=int, default=40, help="how many differences to list")
    p.add_argument("--skip", action="append", default=[], metavar="PATH",
                   help="a question not asked, because its answer differs on purpose")
    p.add_argument("--skip-ending", action="append", default=[], metavar="END",
                   help="leave out every question whose path ends so")
    p.add_argument("--ignore", action="append", default=[], metavar="FIELD",
                   help="a field left out of the comparison wherever it appears, because it differs on purpose")
    p.add_argument("--ignore-in", action="append", default=[], metavar="PATH:FIELD",
                   help="a field left out of the answers to one question only, because it differs on purpose")
    args = p.parse_args()

    if bool(args.source) == args.players:
        p.error("give either --source or --players")
    IGNORED.update(args.ignore)
    for item in args.ignore_in:
        path, _, field = item.rpartition(":")
        IGNORED_IN.setdefault(path, set()).add(field)
    on_purpose = ", ".join(sorted(IGNORED) + args.skip + ["…" + end for end in args.skip_ending] + [f"{f} of {q}" for q in sorted(IGNORED_IN) for f in sorted(IGNORED_IN[q])])
    # A source's routes are under its name at the new address; players' routes are not under any.
    new_prefix = "/api/v1" if args.players else f"/api/v1/sources/{args.source}"
    asked = 0
    differing = []

    def ask(path):
        nonlocal asked
        if path in args.skip or any(path.endswith(end) for end in args.skip_ending):
            return 0, None
        asked += 1
        old_status, old_body = get(args.old, "/api/v1" + path)
        answers = {
            "old address": get(args.new, "/api/v1" + path, args.legacy_host),
            "new address": get(args.new, new_prefix + path),
        }
        old = parsed(old_body, path)
        for name, (status, body) in answers.items():
            new = parsed(body, path)
            if status != old_status:
                differing.append(f"{path} [{name}]: status {old_status} against {status}")
            elif old != new:
                differing.append(f"{path} [{name}]: {where(old, new) if not isinstance(old, bytes) and not isinstance(new, bytes) else 'bodies differ'}")
        return old_status, old

    if args.players:
        ask("/titles")
        ask("/users/00000000000000000000000000000000")
        ask("/users/00000000000000000000000000000000/nameplate")
        for start in range(0, len(args.user), 100):
            ask("/nameplates?ids=" + ",".join(args.user[start:start + 100]))
        for user in args.user:
            for path in ("", "/nameplate", "/titles", "/dans", "/scores", "/scores?view=all", "/scores?view=all&sort=score",
                         "/scores?view=best&source=fanmade", "/scores?view=all&source=official", "/scores?view=all&page=2"):
                ask(f"/users/{user}{path}")
        print(f"players: {len(args.user)} players, {asked} questions asked three ways, {len(differing)} differences"
              + (f" (left out as different on purpose: {on_purpose})" if on_purpose else ""))
        for line in differing[: args.show]:
            print("  " + line)
        if len(differing) > args.show:
            print(f"  … and {len(differing) - args.show} more")
        sys.exit(1 if differing else 0)

    status, catalog = ask("/game/catalog")
    if status != 200 or not isinstance(catalog, dict):
        sys.exit(f"the old site's catalog answered {status}")
    charts = [c["id"] for c in catalog.get("charts", [])]
    for path in ["/game/dans", "/dans", "/categories", "/playlists", "/playlists/system", "/upload-rules", "/users",
                 "/charts", "/charts?page=2", "/charts?pageSize=100&page=3", "/game/search?q=a", "/game/search?q=%E3%81%AE&order=level",
                 "/charts/00000000000000000000000000000000", "/users/00000000000000000000000000000000/scores"]:
        ask(path)

    def items(answer, *keys):
        """The list an answer carries under one of the given names."""
        if isinstance(answer, list):
            return answer
        if isinstance(answer, dict):
            for key in keys:
                if isinstance(answer.get(key), list):
                    return answer[key]
        return []

    _, dans = ask("/dans")
    for dan in items(dans, "items", "dans", "courses"):
        if isinstance(dan, dict) and dan.get("id"):
            ask(f"/dans/{dan['id']}")
            ask(f"/dans/{dan['id']}/comments")
        if isinstance(dan, dict) and dan.get("code"):
            ask(f"/game/dans/{dan['code']}")
    _, playlists = ask("/playlists")
    for playlist in items(playlists, "items", "playlists"):
        if isinstance(playlist, dict) and playlist.get("id"):
            ask(f"/playlists/{playlist['id']}")
    # Every page of the public replay list, and each recording on it.
    page = 1
    while page <= 200:
        _, replays = ask(f"/replays?page={page}")
        found = items(replays, "items", "replays")
        for replay in found:
            if isinstance(replay, dict) and replay.get("id"):
                ask(f"/scores/{replay['id']}/replay")
        if not found:
            break
        page += 1
    sample = charts if args.charts == 0 else charts[: args.charts]
    for chart in sample:
        ask(f"/charts/{chart}?by=id")
        ask(f"/charts/{chart}/resources")
        ask(f"/charts/{chart}/leaderboard")
        ask(f"/charts/{chart}/comments")
    for user in args.user:
        for view in ("best", "all"):
            ask(f"/users/{user}/scores?view={view}")
        ask(f"/users/{user}")
        ask(f"/users/{user}/comments")

    print(f"{args.source}: {len(charts)} charts in the catalog, {asked} questions asked three ways, {len(differing)} differences"
          + (f" (left out as different on purpose: {on_purpose})" if on_purpose else ""))
    for line in differing[: args.show]:
        print("  " + line)
    if len(differing) > args.show:
        print(f"  … and {len(differing) - args.show} more")
    sys.exit(1 if differing else 0)


if __name__ == "__main__":
    main()
