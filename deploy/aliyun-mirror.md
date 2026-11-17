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
APIs. Regional song routing reads the current API resource metadata and probes mirror HEAD;
it does not depend on these inventories. TJA, audio, preview, covers, background images and
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

This hook is still to be integrated with the upstream publishing workflow; cloud mirroring
does not automatically update inventories. No recurring synchronization job is installed by
these cloud settings.

For regional routing, keep `DON_CATALOG_URL` canonical and set:

```dotenv
DON_MIRROR_CATALOG_URL=https://static.sakura-bot.cn/ourtaiko/3dchara/catalog.json
```

The existing GeoIP database and trusted proxy settings apply to every collection. Mainland
requests receive ESA song URLs only for public canonical content-hash files with a successful
HEAD check (2-second shared response budget; cold probes may finish in the background
within 15 seconds; success cached 5 minutes, failure 30 seconds).
Manifests retain canonical `fallbackUrl`; signed/private/custom-origin URLs are unchanged.
GET/HEAD media redirects use the same policy, and `?origin=1` bypasses the mirror.

`GET /api/v1/nameplate-art/catalog` carries canonical `files`/`baseUrl` and, for mainland
requests, exact-match `mirrorBaseUrl`/`mirrorPaths`. The website and updated Play use this API,
verify files, and retry the canonical source on mirror errors. Older NET servers are supported
by falling back to the canonical nameplate catalog. Deploy NET and its website, then release
Play so nameplate routing and download recovery reach game clients. Do not globally replace
canonical S3 public bases.

Before rollout, test mainland and overseas clients, a cold HEAD followed by full GET, audio
Range 206, CORS, SHA-256 and mirror failure behavior. The former
`ourtaiko-mirror-staging/1.0.1-525/` objects are retained temporarily until callers have
switched and access logs show they are unused. They are not a second publication target;
remove that old prefix after this condition is met, without rebuilding the permanent mirror.
