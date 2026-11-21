# OSS and ESA distribution mirror

The permanent public base is **https://static.sakura-bot.cn/ourtaiko/**. Resource paths keep
the canonical layout; neither a staging label nor a game release number belongs in the
shared resource prefix. Desired cloud settings are recorded in
[aliyun-mirror.json](aliyun-mirror.json), without credentials.

| Collection | OSS prefix in `sigaer` | Public catalog |
| --- | --- | --- |
| Don models, icons and paint | `ourtaiko/3dchara/` | `https://static.sakura-bot.cn/ourtaiko/3dchara/catalog.json` |
| Nameplate art | `ourtaiko/nameplate/` | `https://static.sakura-bot.cn/ourtaiko/nameplate/catalog.json` |
| Fanmade TJA and full audio | `ourtaiko/fanmade/` | `https://static.sakura-bot.cn/ourtaiko/fanmade/catalog.json` |
| Official TJA and full audio | `ourtaiko/database/` | `https://static.sakura-bot.cn/ourtaiko/database/catalog.json` |

The song catalogs are mirror inventories, not a replacement for NET's authoritative source
APIs. Regional song routing uses the visitor's country alone (no HEAD probe) and does not
depend on these inventories. TJA, audio, preview, covers, background images and
MP4 videos share the immutable-file route. Videos are fetched on demand; no bulk video copy
is required.

## Stage Don and nameplate art

Use Python 3.11 or newer to stage both canonical art catalogs and their objects:

```sh
python3 -I tools/mirror-distrib.py /path/to/staging
```

Pass `3dchara` or `nameplate` after the destination to stage one collection. The tool checks
byte lengths and SHA-256, includes Don base files and website icons, reuses verified local
files on retry, and writes each local catalog after all its objects pass. Use a fresh
directory for an exact snapshot; an existing directory can contain older unreferenced files.

Upload the staged objects to the same relative paths under `ourtaiko/3dchara/` and
`ourtaiko/nameplate/`, preserving bytes and content types. Upload each `catalog.json` last,
after its objects are reachable and verified through ESA. Repeat staging and upload after
publishing changed canonical art. This tool stages art only; it does not synchronize song
resources, upload to OSS or install a recurring job.

## Missing-object back-to-origin

In OSS, Bucket `sigaer`, Data Management > Mirroring-based back-to-origin, there is one rule
(since 2026-11-18): prefix `ourtaiko/`, any suffix, HTTP error condition `404`, stripping the
prefix and fetching from `https://distrib.ourtaiko.org/`. HTTPS SNI, HEAD back-to-origin and MD5
checking are on; query-string forwarding, redirect following and arbitrary header forwarding
are off. The bucket has no other routing rules; preserve any added later.

GET of a missing object fetches and stores its body. HEAD can check the upstream object
without proving that its body has been stored. Check persistence with ListObjects and verify
the full GET against the authoritative size and SHA-256 before adding a file to a mirror
inventory. OSS's optional Content-MD5 check does not verify the catalog's SHA-256.

OSS keeps what it fetched, with the origin's `Cache-Control`, and never fetches a stored object
again: OSS has no pass-through option and does not follow origin updates
([back-to-origin overview](https://www.alibabacloud.com/help/zh/oss/back-to-origin-rules-overview)).
Files other than JSON are never replaced in place, so that is right for them. JSON is not, so
the synchronization worker below rewrites every JSON file of the bucket on the mirror every 15
minutes: `player-build.json`, `edit-nightly.json`, `3dchara/catalog.json` and `nameplate/catalog.json` with the files
they list (which include `3dchara/base/face_animations.json`), and the JSON no catalog lists,
named in `oss.mirrorJson` of `aliyun-mirror.json` (`3dchara/names.json`). A JSON file added to
the bucket outside a catalog must be added there, or the mirror keeps its first copy (delete
the object in OSS to have it fetched again).

## ESA cache rules

One rule (since 2026-11-18), `OurTaiko mirror follows origin`, applies to host
`static.sakura-bot.cn` and path `/ourtaiko/`: edge and browser caching follow the origin's
`Cache-Control`, which OSS stored from `https://distrib.ourtaiko.org/` (or the worker wrote),
and last 60 seconds when there is none. Content-addressed song files and avatars carry one
year from the origin, 3D Don and nameplate files a day, catalogs written by the worker 60
seconds. 4xx and 5xx are not cached by it. The site's own rules after it (`静态缓存`,
`构建文件缓存`) are not OurTaiko's; `静态缓存` also applies to OurTaiko's PNG, MP3, JPEG and
WebP files and caches them a year, which their never-replaced paths allow.

Retain all query parameters in the cache key (older game builds still add `v` to nameplate
art). Cache 4xx and 5xx responses for
zero seconds. Keep Range support enabled. Install the scoped ESA-to-client response header
rule in `responseHeaderRules` as well: **Add** `Access-Control-Allow-Origin: *` and the listed
read methods/exposed headers. ESA's Add operation overwrites an existing header; Modify only
changes an existing one ([official documentation](https://www.alibabacloud.com/help/en/edge-security-acceleration/esa/user-guide/modify-outgoing-response-header)).
Setting CORS only in OSS is insufficient: a cache entry warmed without Origin (for example
by NET's HEAD probe) may otherwise lack CORS on subsequent browser requests. Check already
cached URLs with and without Origin, including Range 206. These are public, credential-free
resource paths; do not attach an allow-credentials header.
Only content-addressed song files belong under the long-cache song prefixes. New extension
types require an explicit policy review.

## Publication and regional rollout

After a canonical publication succeeds:

1. Read the authoritative catalogs or song resource manifests; compare paths, sizes and SHA-256.
2. Upload changed mutable art, and prefetch missing immutable song objects through OSS using
   full GET when prewarming is wanted. Song files may instead be lazily fetched by OSS; NET
   checks HEAD status/size and updated clients verify SHA-256 before caching or packaging.
3. Publish each mirror catalog last. Preserve the canonical art catalog format; set the song
   inventory `baseUrl` to its permanent public collection URL.
4. Files other than JSON are never replaced in place: changed art is published under a new path.
   The mirror keeps the first copy of a path it reads and ESA keeps art for a year, so a file
   replaced in place would stay old there.

`tools/sync-distribution-mirror.py` now refreshes 3D art (including `.otanim` and icons),
nameplate art, the entire web-player file inventory, and OurTaikoEdit's web version (its
`edit-nightly.json` names one never-changed `edit/<build id>/` folder, published by that
repository's `build/publish-web.py` the way the web player is). It validates source SHA-256 and
uploaded bytes, rechecks that the canonical catalog has not changed, then publishes the
mirror catalog last. A failed transfer leaves the previous catalog in place. Old player and
OurTaikoEdit versions and other business data are never deleted.

On the mirror host, Python 3.11+, `requests`, `oss2`, and the existing private ossutil
credential file are required. Run `tools/install-mirror-sync.sh` as root after committing
the reviewed change. It installs an immutable source release under `/opt/ourtaiko-mirror`,
and enables `ourtaiko-mirror-sync.timer` every 15 minutes. The worker has a 256 MiB memory
limit and 15-minute runtime, uses two transfers, and shares the host maintenance lock.
An optional `/etc/ourtaiko-mirror-sync.env` can set `OURTAIKO_MIRROR_PROXY` for this worker.
Inspect `journalctl -u ourtaiko-mirror-sync` and `/var/lib/ourtaiko-mirror/last-success.json`.

Apply cloud scopes with `tools/configure-distribution-mirror.py --backup FILE --apply`
and `tools/configure-esa-mirror.cjs --backup FILE --apply` (the latter needs the Alibaba ESA
Node SDK in the operator tooling or NODE_PATH). Both commands default to read-only plans,
back up existing configuration, and preserve unrelated settings. Existing CORS rules cover
the whole prefix, including cached HEAD and Range responses.

For regional routing, keep `DON_CATALOG_URL` canonical and set:

```dotenv
DON_MIRROR_CATALOG_URL=https://static.sakura-bot.cn/ourtaiko/3dchara/catalog.json
```

The existing GeoIP database and trusted proxy settings apply to every collection, and the
country alone decides: NET never probes mirror objects or reads the mirror's catalogs. Every
file a mainland visitor asks for that the mirror does not hold yet is fetched by OSS
back-to-origin on first request.

API version 2 (`/api/v2/`, the website's) answers every public file as a path under the
distribution root and the same answer for everyone. `GET /api/v2/distribution/source` answers
`mirror`, `baseUrl` and `canonicalBaseUrl` for the visitor (`Cache-Control: no-store`); the
client joins `baseUrl` and each path, and retries `canonicalBaseUrl` with the same path when the
mirror's copy fails. The website asks it once per page load and shows the root in the footer.

API version 1 (`/api/v1/`, released games) makes the choice in its answers: song files in
resource manifests, file redirects and the game's folder covers move to the mirror for a
mainland visitor with the canonical `fallbackUrl`, and `?origin=1` keeps the canonical address;
`/users/{id}/don` files and reward icons are on the mirror's root; the 3D Don and nameplate
catalogs carry `baseUrl` and, for a mainland visitor, `mirrorBaseUrl` with `mirrorPaths` listing
every file. `GET /api/v1/distribution/avatar?url=...` accepts only canonical content-addressed
SSO avatar URLs and returns `url` plus the original `fallbackUrl` (`redirect=1` for a plain
image); `GET /api/v1/distribution/source` is version 2's. Both send `Cache-Control: no-store`.
Play's account and multiplayer picture loader uses the avatar route. Do not globally replace
canonical S3 public bases.

The website reads `player-build.json` from the visitor's root in the source answer (the mirror's
copy for mainland visitors, cached 60 seconds by ESA) and loads that build from the same root,
keeping the OSS prefix; if the mirror's manifest cannot be read, the canonical manifest and root
are used. It retries the
canonical iframe if the mirror reports a startup error or the player is not ready after 90
seconds. Non-mainland visitors always retain the canonical base. Deploy the updated NET backend and
website together; game avatar routing also needs the updated Play build. Syncing the OSS
files alone does not change already released application code.

Before rollout, test mainland and overseas clients, a cold HEAD followed by full GET, audio
Range 206, CORS, SHA-256 and mirror failure behavior. The former
`ourtaiko-mirror-staging/1.0.1-525/` objects are retained temporarily until callers have
switched and access logs show they are unused. They are not a second publication target;
remove that old prefix after this condition is met, without rebuilding the permanent mirror.
