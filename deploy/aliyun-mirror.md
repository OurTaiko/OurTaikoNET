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
APIs. Current regional Don support does not consume these song inventories. Covers, preview
audio and videos are outside this snapshot's scope.

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

In OSS, Bucket `sigaer`, Data Management > Mirroring-based back-to-origin, configure six
rules: each combination of prefix `ourtaiko/fanmade/` or `ourtaiko/database/` and suffix
`.tja`, `.ogg` or `.mp3`, with HTTP error condition `404`. Strip the matching OSS prefix and
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
| Fanmade/database `.tja`, `.ogg`, `.mp3` with content-hash filenames | 1 year | 1 year |
| Don/nameplate model or image requests with a `v` parameter | 1 year | 1 year |
| Catalogs and unversioned resource requests | 60 seconds | 60 seconds |

Retain all query parameters in the cache key, including `v`. Cache 4xx and 5xx responses for
zero seconds. Keep Range support enabled and allow browser CORS GET from the NET website.
Only content-addressed song files belong under the long-cache song prefixes. New extension
types require an explicit policy review.

## Publication and regional rollout

After a canonical publication succeeds:

1. Read the authoritative catalogs or song resource manifests; compare paths, sizes and SHA-256.
2. Upload changed mutable art, and prefetch missing immutable song objects through OSS using
   full GET. Verify every newly published object's body and persistence before advertising it.
3. Publish each mirror catalog last. Preserve the canonical art catalog format; set the song
   inventory `baseUrl` to its permanent public collection URL.
4. Use the new SHA-based `v` value for changed art. Refresh previously cached unversioned art
   URLs if immediate visibility is required; otherwise their maximum TTL is 60 seconds.

This hook is still to be integrated with the upstream publishing workflow; cloud mirroring
does not automatically update inventories. No recurring synchronization job is installed by
these cloud settings.

Once regional Don routing is deployed, keep `DON_CATALOG_URL` canonical and set:

```dotenv
DON_MIRROR_CATALOG_URL=https://static.sakura-bot.cn/ourtaiko/3dchara/catalog.json
```

The GeoIP database and trusted proxy settings are also required. Song resource manifests and
file redirects still need regional selection; do not globally replace the canonical S3
public bases to simulate it. Nameplate clients with the hardcoded canonical hostname need
regional routing under that hostname or a client update.

Before rollout, test mainland and overseas clients, a cold HEAD followed by full GET, audio
Range 206, CORS, SHA-256 and mirror failure behavior. The former
`ourtaiko-mirror-staging/1.0.1-525/` objects are retained temporarily until callers have
switched and access logs show they are unused. They are not a second publication target;
remove that old prefix after this condition is met, without rebuilding the permanent mirror.
