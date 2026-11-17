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

In OSS, Bucket `sigaer`, Data Management > Mirroring-based back-to-origin, configure sixteen
rules: each combination of prefix `ourtaiko/fanmade/` or `ourtaiko/database/` and suffix
`.tja`, `.ogg`, `.mp3`, `.webp`, `.png`, `.jpg`, `.jpeg` or `.mp4`, with HTTP error condition `404`. Strip the matching OSS prefix and
use the corresponding origin `https://distrib.ourtaiko.org/fanmade/` or
`https://distrib.ourtaiko.org/database/`. Enable HTTPS SNI, HEAD back-to-origin and MD5
checking; disable query-string forwarding, redirect following and arbitrary header
forwarding. Preserve other bucket routing rules if any are added later.

GET of a missing object fetches and stores its body. HEAD can check the upstream object
without proving that its body has been stored. Check persistence with ListObjects and verify
the full GET against the authoritative size and SHA-256 before adding a file to a mirror
inventory. OSS's optional Content-MD5 check does not verify the catalog's SHA-256.

These rules supplement publication, not update it: existing objects are never refreshed by a
missing-object rule. Catalogs, same-path model files and same-path images must be explicitly
published. Do not mirror the entire shared bucket or mirror `catalog.json` on demand.

## ESA cache rules

The three rules in `aliyun-mirror.json` apply only to host `static.sakura-bot.cn` and path
`/ourtaiko/`. Put the content-addressed song and versioned-art rules before the general
OurTaiko rule, and all three before the site's generic PNG/MP3 rule. Preserve the site's
unrelated settings.

| Requests | Edge TTL | Browser TTL |
| --- | --- | --- |
| Fanmade/database `.tja`, `.ogg`, `.mp3`, `.webp`, `.png`, `.jpg`, `.jpeg`, `.mp4` with content-hash filenames | 1 year | 1 year |
| Don/nameplate `.glb`, `.otanim`, `.json`, `.png`, `.jpg`, `.jpeg`, `.webp` with a `v` parameter | 1 year | 1 year |
| Catalogs and unversioned resource requests | 60 seconds | 60 seconds |

Retain all query parameters in the cache key, including `v`. Cache 4xx and 5xx responses for
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
4. Use the new SHA-based `v` value for changed art. Refresh previously cached unversioned art
   URLs if immediate visibility is required; otherwise their maximum TTL is 60 seconds.

`tools/sync-distribution-mirror.py` now refreshes 3D art (including `.otanim` and icons),
nameplate art, and the entire web-player file inventory. It validates source SHA-256 and
uploaded bytes, rechecks that the canonical catalog has not changed, then publishes the
mirror catalog last. A failed transfer leaves the previous catalog in place. Old player
versions and other business data are never deleted.

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
back up existing configuration, and preserve unrelated settings. New OSS mirror scopes are
`sso/avatars/*.webp` and public web-player files under `player/`. The player uses one prefix
rule so the bucket stays within OSS's routing-rule limit (18 rules total). The mutable
`player-build.json` is published only by the verified synchronization worker, never by
unverified mirror back-to-origin. Versioned player files and immutable avatars cache for
one year; catalogs cache for 60 seconds; errors are not cached. Existing CORS rules cover
all these public prefixes, including cached HEAD and Range responses.

The existing ESA rule named `OurTaiko content-addressed songs` also covers immutable
avatar and player static-file extensions. Reusing it keeps the current plan's rule count
unchanged; catalogs and JSON still use the short-lived catch-all policy.

For regional routing, keep `DON_CATALOG_URL` canonical and set:

```dotenv
DON_MIRROR_CATALOG_URL=https://static.sakura-bot.cn/ourtaiko/3dchara/catalog.json
```

The existing GeoIP database and trusted proxy settings apply to every collection, and the
country alone decides: NET never probes mirror objects or compares catalogs. Mainland requests receive ESA song URLs for every public canonical content-hash
file; a missing object is fetched by OSS back-to-origin on first request.
Manifests retain canonical `fallbackUrl`; signed/private/custom-origin URLs are unchanged.
GET/HEAD media redirects use the same policy, and `?origin=1` bypasses the mirror.

`GET /api/v1/nameplate-art/catalog` carries canonical `files`/`baseUrl` and, for mainland
requests, the mirror's own `nameplate/catalog.json` (re-read every 60 seconds, as long as ESA
keeps it) with `mirrorBaseUrl`/`mirrorPaths` listing every file in it; `baseUrl` stays canonical
for fallback. The 3D Don catalog and `/users/{id}/don` do the same with
`3dchara/catalog.json` (an item the mirror's catalog does not list yet keeps its canonical
file). Until a mirror catalog has been read once, the canonical one is answered. The website and updated Play use this API,
verify files, and retry the canonical source on mirror errors. The synchronization worker
publishes each mirror catalog after the files it lists, so a mirror catalog never names a file
the mirror lacks. Older NET servers are supported
by falling back to the canonical nameplate catalog. Deploy NET and its website, then release
Play so nameplate routing and download recovery reach game clients. Do not globally replace
canonical S3 public bases.

`GET /api/v1/distribution/avatar?url=...` accepts only canonical content-addressed SSO
avatar URLs, uses the same country/trusted-proxy policy (no probe), and returns `url` plus the original `fallbackUrl`. `redirect=1` supports ordinary browser images.
Both variants send `Cache-Control: no-store` so region selection is never shared by a CDN.
Play's common account/multiplayer picture loader uses this endpoint, retains the original URL,
and falls back if the mirror is unavailable.

`GET /api/v1/distribution/source` answers `mirror`, `baseUrl` and `canonicalBaseUrl` for the
visitor (same country policy, `no-store`, no probe). The website asks it once per page load,
builds public avatar addresses on that root itself, falls back to the original address when
the mirror copy fails to load, and shows the root in the footer. Signed, private, and
custom-origin pictures keep their original route.

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
