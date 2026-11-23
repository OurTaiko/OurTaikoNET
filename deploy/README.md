# Deploying OurTaikoNet

Status: **in production since the switch on 2026-11-14, 04:06 UTC.** The one container
`ourtaiko-net` (`127.0.0.1:8096`) answers <https://net.ourtaiko.org>. The three old host
names `fanmade.ourtaiko.org`, `database.ourtaiko.org` and `profile.ourtaiko.org` are no longer
served by it: until 2026-11-23 it answered their old APIs and sent their pages on; since then
their pages are redirected to the new site by rules at Cloudflare, and their old APIs are gone
(the code and the `LEGACY_*_HOST` settings were removed). Its
database `ourtaiko_net` is the only one written; files are in the bucket the old sites used,
each source signing with its old site's key. The three old containers, their images and
their deploy folders were removed on 2026-11-14, 07:30 UTC: there is no going back to them.
Their databases were dropped on 2026-11-14, 08:57 UTC, after every row of them was found in
`ourtaiko_net`; a last dump of each is kept. A push to
`main` of this repository publishes to production. Nobody is the site's
administrator until the role is given in the account center's admin (Client roles), and
`rollback.yml` has not been run.

One container, `ourtaiko-net`, on the server that runs the three old sites, beside them until
the switch. It answers `net.ourtaiko.org` (and, from the switch until 2026-11-23, the three old
host names). The website is `web/` in this repository: its builds are put in a
directory on the server and this container serves the current one. Until 2026-11-14 the
website was a repository of its own, OurTaikoNetWeb (archived on 2026-11-14); its history is kept here,
under `web/`, with the same commit hashes.

```
/opt/ourtaiko-net/
  compose.production.yml     put here by every release of the backend
  incoming/<commit>/         an uploaded image, removed once published
  incoming-web/<commit>/     an uploaded website build, removed once published
  web/releases/<commit>/     website builds, the five newest
  web/current -> releases/<commit>
/etc/ourtaiko-net/docker/
  api.env, storage.env       settings and secrets; see api.env.example
```

## A release

Push to `main`. `.github/workflows/deploy.yml` first looks at which part the push changed:
anything under `web/` (and `deploy/publish-web.sh`) is the website, everything else but
Markdown is the backend, and `contracts/` and the workflow itself are both. A run started by
hand does both. Then, for each part changed:

- **Backend.** Every test in the Dockerfile's `test` stage against a throwaway PostgreSQL 18;
  the image is built, kept in `ourtaiko-private` (`net/images/<commit>.tar.gz`) and uploaded,
  and `deploy/publish.sh` on the server loads it, replaces the container and waits for
  `/readyz`.
- **Website.** Lint, formatting, unit tests and the build in `web/`; the tested build is
  uploaded and `deploy/publish-web.sh` makes it the current one
  (`web/releases/<commit>`). That changes a link on the server and restarts nothing.

Nothing is published unless every test that ran passed. When a push changes both, the backend
is published first and the website only after it succeeded.

`rollback.yml` puts back one part, chosen when it is started. The backend: an archived image
by commit. It does not touch the database. A release that changes the schema applies the
change as it starts (`db/README.md`, "Changing the schema"), and an image from before that
change refuses the newer database: to go back past one, delete its row from
`net.schema_migrations` first (the older program does not mind the columns it does not
know). The website: one of the five builds the server keeps, by commit; builds published
before the website moved into this repository are named by OurTaikoNetWeb's commits.

Publishing happens only while the repository variable `DEPLOY_ENABLED` is `true` (Settings >
Variables); otherwise a push runs the tests and stops. It is `true` since 2026-11-14: **a push
to `main` publishes to the server.**

Secrets: the server's address is the organization's `SERVER_ADDR`. The repository has
`DEPLOY_USER` (`ourtaiko-ci`), `DEPLOY_FINGERPRINT` (the server's SSH host key) and
`DEPLOY_KEY`, a private key whose public half is in that user's `authorized_keys`: signing in is
by key only. Optionally `DEPLOY_PORT` and `DEPLOY_BASE_PATH`. It also needs the organization's
`PRIVATE_AWS_ACCESS_KEY_ID` and `PRIVATE_AWS_SECRET_ACCESS_KEY`, which are given to selected
repositories; this one was added on 2026-11-14.

## The account the workflows sign in as

`ourtaiko-ci` on the server (`deploy/server/create-ci-user.sh`, run on 2026-11-14) is used by
nothing else. It has no password and one key, which forwards nothing; it owns
`/opt/ourtaiko-net`; it cannot read the settings files; and it may run as root only the docker
commands `publish.sh` runs (`deploy/server/update-ci-sudoers.sh` is the list), on images named `ourtaiko-net` and `ourtaiko-net-rooms` and the one Compose file
(`/etc/sudoers.d/ourtaiko-ci`). The private key exists only as the `DEPLOY_KEY` secret of this
repository (and of OurTaikoNetWeb, archived, where no workflow runs): to replace it, make a new pair, put the new
public half in the account's `authorized_keys` and set the secret again.

This narrows what a leaked key can do, but not to nothing: the Compose file that says what the
container may do comes from this repository with every release, so whoever can push to `main`,
or holds the key, can still start a container of their choosing on the server.

## Trying the image on your own machine

```sh
docker build -t ourtaiko-net:local .
docker run --rm -p 127.0.0.1:8091:8080 \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=1g --cap-drop ALL \
  -e DATABASE_URL='postgres://<you>@host.docker.internal:5432/ourtaiko_net_dev?sslmode=disable' \
  -e APP_ORIGIN=http://127.0.0.1:8091 -e STORAGE_DIR=/tmp/staging \
  -e WEB_DIR=/srv/web -v "$PWD/web/dist":/srv/web:ro \
  ourtaiko-net:local
```

Without `SSO_ISSUER` it serves the catalogues and the website; signing in needs the account
center. The whole suite as CI runs it:

```sh
docker build --target test -t ourtaiko-net-test .
docker run --rm -e DATABASE_TEST_URL=postgres://postgres:postgres@<a PostgreSQL 18>/postgres ourtaiko-net-test
```

## The first time (preview at net.ourtaiko.org)

Each step touches production and is done with the owner's go-ahead.

1. **Database.** `deploy/server/create-database.sh` makes the role and the empty database
   `ourtaiko_net` in the shared PostgreSQL and writes the two settings files with a password it
   generates and never prints. `build-schema.sh` builds the schema as that role. `import.sh`
   fills it from the three old databases, which it only reads, and compares every table with
   its origin (see `db/README.md`); it can be repeated, so it can run nightly while the
   preview lasts. Each is copied to the server and run from a file there, beside the SQL
   files it names. **Done on 2026-11-14.**
2. **Settings.** `create-database.sh` wrote `/etc/ourtaiko-net/docker/api.env` (database,
   origin, secure cookies, the proxy's address) and `storage.env` (`STORAGE_BACKEND=local`: the
   preview stays away from the old sites' files, next section). The account center's values
   and the three `LEGACY_*_HOST` are added later; see `api.env.example`. **Done on 2026-11-14.**
3. **Account center.** `deploy/server/register-application.sh` registers the Application
   `OurTaikoNet` (website sign-in, token lookup, profiles, game sessions and account-token
   exchange) and appends its credentials and a session key to `api.env`; then the container is
   recreated. **Done on 2026-11-14**, after step 6.
4. **Directories.** `/opt/ourtaiko-net/{incoming,incoming-web,web/releases}`, owned by
   `ourtaiko-ci`; the container reads `web/` as user 10001. `/etc/ourtaiko-net/docker`, root
   only. **Done on 2026-11-14.**
5. **Proxy.** In 1Panel, a reverse proxy website for `net.ourtaiko.org` to
   `http://127.0.0.1:8096` with a Let's Encrypt certificate; then `net.locations.conf` is
   installed as `/opt/1panel/www/sites/net.ourtaiko.org/proxy/root.conf` in place of the file
   1Panel wrote (kept beside it as `root.conf.before-net-locations`), and nginx reloaded
   (`nginx -t`, `nginx -s reload` in the OpenResty container). **Done on 2026-11-14.**
6. **Release** the backend, then the website (then still OurTaikoNetWeb). **Done on 2026-11-14**,
   before steps 3 and 5.

## The address in mainland China

The site has a second address, **otnet.sakura-bot.cn** (ICP-registered), for visitors in
mainland China whose connection to Los Angeles is unreliable. A server in mainland China runs
nginx with [mainland-proxy.conf](mainland-proxy.conf): it ends the visitor's TLS there and
passes every request, Multi-Enso's gRPC included, to `net.ourtaiko.org` over connections it
keeps open, with the visitor's address in `X-Real-IP` and the host name unchanged. Files do not
come this way: mainland visitors already get them from the mirror (next section).

It is the same site, database and account: the backend lists the address in
`APP_OTHER_ORIGINS` and answers each request in the address it came to (`internal/web`,
`Origins`). A write is accepted only from the pages of its own address, sign-in begins and ends
there (the account center's callback is `SSO_REDIRECT_URL`'s path at that address), and the
full addresses an answer gives (players' score links, the address the game is told to dial for
Multi-Enso when `MULTIPLAY_PUBLIC_URL` is empty) are on it. A browser keeps the sessions of the
two addresses apart, as it keeps their local storage: a visitor signs in at each, and the
chart editor's local projects at one are not seen at the other. Requests to any other host name
(the old sites' included) are answered in `APP_ORIGIN`.

**Live since 2026-11-22.** The mainland server is `sakura` (an SSH alias; Tencent Cloud, BT
Panel's nginx 1.22). Each step was done with the owner's go-ahead, in this order, and is kept
for a server set up anew:

1. **Backend.** Release the version that reads `APP_OTHER_ORIGINS`. Add
   `APP_OTHER_ORIGINS=https://otnet.sakura-bot.cn` to `api.env` and recreate the container; it
   logs `also served at [...]` at start. Leave `MULTIPLAY_PUBLIC_URL` unset so that the game
   that asked at the mainland address dials it.
2. **Los Angeles proxy.** The site's server block
   (`/opt/1panel/www/conf.d/net.ourtaiko.org.conf`) lists `otnet.sakura-bot.cn` in
   `server_name` beside `net.ourtaiko.org` (no certificate is needed for it: the mainland server
   connects under the name `net.ourtaiko.org`), and believes `X-Real-IP` from the mainland
   server's address alone, so that rate limits and region go by the visitor:

   ```
   set_real_ip_from <the mainland server's public address>;
   real_ip_header X-Real-IP;
   ```

   The site's own `proxy_set_header X-Real-IP $remote_addr` then passes the visitor's address
   on. Without these lines every mainland visitor is the mainland server: one rate limit for
   all of them. These were written into the file by hand (the version before is kept beside it
   as `.net.ourtaiko.org.conf.before-otnet-20261122`); check they survive when the site is
   changed in 1Panel. `nginx -t`, then reload, in the OpenResty container.
3. **Account center.** Add `https://otnet.sakura-bot.cn/api/v1/auth/sso/callback` to the
   redirect URIs of the Application `OurTaikoNet`, beside the one there (in the `ourtaiko-sso`
   container's `manage.py shell`, `Application.redirect_uris` is space-separated).
4. **Certificate.** On the mainland server, acme.sh (`/root/.acme.sh`, from its official
   repository; Let's Encrypt, no account email) issued `otnet.sakura-bot.cn` by webroot
   (`/www/wwwroot/otnet.sakura-bot.cn-acme`, which the port-80 server block answers from) and
   installs it as `/www/server/panel/vhost/cert/otnet.sakura-bot.cn/{fullchain,privkey}.pem`,
   reloading nginx; its cron job renews it.
5. **Mainland proxy.** `mainland-proxy.conf` is
   `/www/server/panel/vhost/nginx/otnet.sakura-bot.cn.conf` there, with those paths, without
   the IPv6 `listen` lines (the server has no IPv6 address) and with an `error_log` of its own
   (`/www/wwwlogs/otnet.sakura-bot.cn.error.log`; the server's own is at `crit`). Set the CA
   bundle path to the system's (`/etc/pki/tls/certs/ca-bundle.crt` on CentOS-like systems).
   nginx resolves `net.ourtaiko.org` when it starts: reload it if that address ever changes.
6. **DNS.** `otnet.sakura-bot.cn` points at the mainland server.
7. **Check**: the site loads at `https://otnet.sakura-bot.cn`; sign-in goes to the account
   center with that address's callback; a write from that address's pages is taken and one
   from the other address refused; `GET /api/v1/game/multiplay` there answers that address; a
   gRPC call there reaches the rooms (an unknown method answers `grpc-status: 12`, as at
   `net.ourtaiko.org`); the Los Angeles access log shows the visitor's address, not the
   mainland server's.

To take it away, point DNS elsewhere or stop the mainland server, then remove the value from
`APP_OTHER_ORIGINS`, the redirect URI and the domain.

## Regional resource mirror rollout

Keep `DON_CATALOG_URL` on the canonical CloudFront catalog. Prepare and verify the mirror
using [aliyun-mirror.md](aliyun-mirror.md), which documents the staging tool, permanent
OSS/ESA paths, cache rules and publication order. Publish the mirror catalog only after its
objects are verified. The canonical catalog remains authoritative for every region.

Put a GeoLite2 Country or compatible MMDB file in `/opt/ourtaiko-net/geoip`, readable by the
container. Set these in the server's `api.env` and recreate the container:

```dotenv
DON_GEOIP_DB=/srv/geoip/GeoLite2-Country.mmdb
DON_MIRROR_CATALOG_URL=https://static.sakura-bot.cn/ourtaiko/3dchara/catalog.json
```

With `DON_MIRROR_CATALOG_URL` set, the server refuses to start if it is not a plain HTTPS URL
or the country database cannot be opened; check `docker logs` after recreating.

The reverse proxy must keep overwriting `X-Real-IP` with the actual client IP and
`TRUSTED_PROXY_CIDRS` must identify the proxy. The proxy reaches the published port from the
gateway of the container's gateway network, which the Compose file pins to `ourtaiko-shared`
(`gw_priority`, Docker Engine 28+), so `TRUSTED_PROXY_CIDRS=172.20.0.1/32`; the site logs
`trusted proxies: …` at start. Mainland China (`CN`) requests get mirror URLs
for exact Don/nameplate catalog matches, and for public content-addressed chart media
after a bounded mirror HEAD check. Overseas, Hong Kong, Macao, Taiwan and unknown locations get
canonical URLs. Unlisted or changed art stays canonical; failed refreshes retain the last
good mirror catalog. New public song files can use OSS missing-object back-to-origin.
While both settings are in effect, the per-user Don answer is `private, no-store` and the
website catalog `private, max-age=60`, so shared caches never hand one region's addresses to
another; otherwise the catalog stays `public, max-age=60`.

Check the same outfit from one mainland and one overseas connection. Inspect both
`/api/v1/users/{id}/don` and `/api/v1/don/catalog`, then load the outfit with a fresh game cache
and in the browser. Check `/api/v1/nameplate-art/catalog`, source chart `resources`
responses and GET/HEAD media redirects too. Download from a cold mirror and exercise a failed
mirror (including wrong size/hash): updated web/Play clients retry canonical resource URLs.
File routes accept `?origin=1` for an explicit canonical retry. Include an outfit whose mirror
file is missing or outdated. To disable the
rollout, clear `DON_MIRROR_CATALOG_URL` and recreate the container. Restart after replacing the
country database. Keep Fanmade and official S3 public bases canonical: only mainland requests
are selected for ESA. Signed/private URLs and other CDN origins keep their existing route.

The existing Don mirror/GeoIP/trusted-proxy settings are shared by all collections; no second
IP database or new environment variable is required. The mirror URL must end in
`/3dchara/catalog.json` to infer sibling collections. A nonstandard custom Don catalog remains
Don-only. Release the NET backend and website together, then a Play update: older Play builds
can use routed song URLs, but retain their canonical nameplate source and lack per-file
canonical retry. HTTPS distribution is still served by the upstream CDN outside mainland China.

## Multi-Enso (gRPC)

**On since 2026-11-14.** The steps below were done that day, in the order 2, 3, 1, 4, and
are kept for a server set up anew. Where the service is off the site answers
`404 MULTIPLAY_UNAVAILABLE` at `/api/v1/game/multiplay` and the game says online rooms are
not available.

Multi-Enso (the game's online rooms, `internal/multiplay`; root `README.md`, "Multi-Enso") is
the container `ourtaiko-net-rooms`, the program `ourtaiko-rooms` of the site's image. It
listens on 8081 in clear text (h2c), mapped to `127.0.0.1:8097` on the server, and asks the
site at `http://net:8082` on the private network `rooms`. The game is told to dial `MULTIPLAY_PUBLIC_URL`,
by default the site's address the game asked at (`APP_ORIGIN`, `https://net.ourtaiko.org`, or the
address in mainland China, whose proxy passes the service on in turn): port 443, where the proxy ends TLS and
passes the one path of the service on:

```
location ^~ /ourtaiko.multiplay.v1.MultiEnso/ { grpc_pass grpc://127.0.0.1:8097; … }
```

That location is `deploy/net.multiplay.conf`, a file of its own: `net.locations.conf` has to
stay one location for the panel to read it.

1. **Switch it on and release the backend.** Check first that nothing on the server listens
   on 8097 (`ss -ltn 'sport = :8097'`): a port that is taken stops the container from
   starting. Then, in `compose.production.yml`, set `MULTIPLAY_INTERNAL_ADDR` on `net` and
   keep the service `rooms`, and push to `main`.
   Afterwards `docker port ourtaiko-net-rooms` shows `8081/tcp -> 127.0.0.1:8097`, its log says
   `Multi-Enso (gRPC, h2c) listening on …`, and
   `curl https://net.ourtaiko.org/api/v1/game/multiplay` answers
   `{"url":"https://net.ourtaiko.org","protocol":1,"maxProtocol":2,"maxMembers":5}`. From here on the game
   tries to connect, so do steps 2 and 3 right after (or before: they do no harm while the
   port is closed).
2. **HTTP/2.** gRPC is HTTP/2: the 443 listener of the site `net.ourtaiko.org` must have it
   on (`http2 on;`, or `listen 443 ssl http2;`, in the site's server block).
   `curl -sI --http2 https://net.ourtaiko.org/healthz` must begin with `HTTP/2 200`. Without
   it the website works and the game cannot connect. (On 2026-11-14 the site's
   `/opt/1panel/www/conf.d/net.ourtaiko.org.conf` already had `http2 on;`.)
3. **The location.** Look at the site's server block first
   (`/opt/1panel/www/sites/net.ourtaiko.org/…`, in the panel "configuration file"): it
   should include every file of the site's `proxy/` folder (`include …/proxy/*.conf;`).
   Copy `deploy/net.multiplay.conf` there as `proxy/multiplay.conf`, beside `root.conf`;
   if the block includes `root.conf` by name only, add a line that includes the new file.
   Then `nginx -t` and `nginx -s reload` in the OpenResty container, as for
   `net.locations.conf`. (On 2026-11-14
   the block included `proxy/*.conf`, and the file is there as `proxy/multiplay.conf`. It is
   a copy: a change to `deploy/net.multiplay.conf` has to be copied again by hand.)
4. **Check from outside**, with [grpcurl](https://github.com/fullstorydev/grpcurl) from a
   checkout of this repository (the service has no reflection, so it is given the protocol):

   ```sh
   grpcurl -import-path proto -proto ourtaiko/multiplay/v1/multiplay.proto \
     -d '{"join":{"quick_match":{}}}' net.ourtaiko.org:443 ourtaiko.multiplay.v1.MultiEnso/Connect
   ```

   Without a session it must answer `Code: Unauthenticated`, `Message: AUTH_REQUIRED`. With
   a made-up one (`-H 'authorization: Bearer <64 hex digits>'`) it must answer the same after
   a moment: the container asked the account center. `Unavailable` `AUTH_UNAVAILABLE` there
   means it could not. Any other answer (an unexpected HTTP status or content type, say)
   means the request went to the website's location: step 3 is not in effect. The same
   check without grpcurl: `go run ./tools/multiplay-smoke -do probe -site https://net.ourtaiko.org`.

### Releasing

Rooms are in memory, so their container is replaced only when a push changes `cmd/rooms`,
`internal/multiplay` (tests and Markdown aside) or `deploy/compose.production.yml`; a run of
the deploy workflow by hand does so only with its `rooms` input. Every other release of the
backend leaves rooms playing. When the rooms are replaced, `publish.sh` first runs
`ourtaiko-rooms drain` in the container: rounds under way go on, nothing new begins, and it
goes ahead when none is left or after 20 minutes. Then the rooms still waiting in their lobby
are told `ROOM_CLOSED`. A publish that dies meanwhile does not leave the rooms refusing players:
they take rounds again by themselves 5 minutes after the wait.

`rollback.yml` puts the site back and leaves the rooms as they are. An image from before the
rooms had a container of their own has no `MULTIPLAY_INTERNAL_ADDR`: with it the game can enter
no room.

`MULTIPLAY_PUBLIC_URL` may be set in `api.env`. `MULTIPLAY_MIN_PLAYERS` is set on the service
`rooms` in the Compose file. To switch the service off, comment `MULTIPLAY_INTERNAL_ADDR` out
in the Compose file: the site then answers `404 MULTIPLAY_UNAVAILABLE` and the game does not
try.

### Moving to the container of its own (once)

1. **Before merging,** on the server, as an administrator: copy
   `deploy/server/update-ci-sudoers.sh` there and run it. Without it the publish stops before
   changing anything and says so.
2. If `api.env` sets `MULTIPLAY_MIN_PLAYERS`, move the value to the service `rooms`.
3. Push to `main`. That one release closes the rooms open at the time: the site gives up port
   8097 and then the rooms' container takes it, a few seconds without rooms.
4. Check as in steps 1 and 4 above, then release something of the site alone and see that a
   room stays open through it.

## Files before the switch

Until the switch the database here is a copy of the old ones, refreshed by the import, while
the old sites go on changing theirs. The server does not only read files: in the background it
deletes the files its database says nothing refers to any more, moves files to new names, and
writes previews. Run on a copy that is hours old, with the right to write where the old sites
keep their files, it would delete or move files the old sites still use.

So before the switch the container gets no credentials for the files at all. `storage.env`
says `STORAGE_BACKEND=public`, with each source's prefix and the public address of that prefix
(`FANMADE_S3_PREFIX`, `FANMADE_S3_PUBLIC_BASE_URL`, and the same two for `OFFICIAL`): the old
sites' files are read over the address everyone reads them at, and cannot be written, moved or
deleted, because nothing here could sign such a request. In that mode the server starts none
of the background work above and answers the routes that store a file with 503
`FILES_READ_ONLY`.

What the preview then shows is the old sites' files as its copy of the database names them: a
chart uploaded or replaced there since the last import is missing or shows its old files, and
a file the old site has since deleted answers 403 from the public address.

`STORAGE_BACKEND=local` (the first days of the preview) serves no files at all.

## The switch (done on 2026-11-14)

What was done, in order; the scripts ran on the server from `/tmp` and are described here
because they are not needed again.

Before the window, with the old sites still serving:
1. The three `LEGACY_*_HOST` were set in `api.env` (they matter only to a request that reaches
   the container under one of those names) and the import was repeated.
2. `tools/compare-answers.py` asked the old sites and this server the same questions on the
   server itself (old `127.0.0.1:8080`, `8081`, `8093`; new `127.0.0.1:8096` by old host name
   and by new address). Fanmade: 347 charts, 1644 questions, the differences were `coverHash`
   added to the game's lists (on purpose) and one avatar missed once. Official: 150 charts one
   by one and every list, 1285 questions, `sso.service` (on purpose: the game asks the account
   center for a session of the service the catalogue names) and one nickname missed once.
   Players: 564 questions; links now lead to the new site, and the official source is called
   `official` where the old players' site said `database`.

The window, 04:05:34 to 04:06:05 UTC:
3. `docker stop` of `ourtaiko-fanmade`, `ourtaiko-database`, `ourtaiko-profile`.
4. `deploy/server/import.sh` a last time: 55 tables, check passed, 47,034 rows.
5. `storage.env` set to `s3`: the bucket and region of the old sites, each source's prefix and
   public address, and for each source the access key its old site used
   (`FANMADE_AWS_*`, `OFFICIAL_AWS_*`); the container recreated.
6. `net.locations.conf` installed as the proxy file of the three old sites
   (`fanmade…/proxy/root.conf`, `database…/database.conf`, `profile…/proxy/root.conf`), each
   old file kept beside it as `<name>.before-net`; `nginx -t`, reload.

Afterwards: a last backup of each old database by its own timer's service, and the daily
backup of `ourtaiko_net` (`deploy/server/install-backup.sh`, `cmd/backup`; 04:30 Asia/Tokyo,
30 days kept, uploaded under `net/backups/postgresql/` of the private bucket).

Not done by anyone yet: a released game from sign-in to a score; an upload on the website; a
restore drill of an `ourtaiko_net` backup.

## The database console

CloudBeaver (<https://postgre.ourtaiko.org>, `/opt/ourtaiko-cloudbeaver`) has two connections
to `ourtaiko_net`, with the two accounts it already used for the old databases
(`deploy/server/grant-console-access.sql`, run once on 2026-11-14):

- **OurTaikoNET (read only)**, as `ourtaiko_viewer`: reads all three schemas. Its queries name
  the schema (`fanmade.charts`, `official.charts`, `net.users`).
- **OurTaikoNET official songs (read-write)**, as `ourtaiko_database_editor`: reads all three,
  writes the schema `official` only, and cannot change the schema. A name without a schema is
  the official source's.

Both give up a transaction left open for five minutes, so a forgotten one cannot hold a lock
against a migration. An edit made there goes around the server: the database's own triggers
still apply (replacing a chart's file queues the old one for deletion), the server's checks do
not. The two connections to the old databases were removed with those databases.

## What is left of the old sites

Removed on 2026-11-14: the containers `ourtaiko-fanmade`, `ourtaiko-database` and
`ourtaiko-profile`, their images, `/opt/ourtaiko-{fanmade,database,profile}` and the daily
backup timers of their databases. The switch cannot be undone any more.

Dropped on 2026-11-14, 08:57 UTC: the databases `ourtaiko_fanmade`, `ourtaiko_database` and
`ourtaiko_profile`. Before that each of their tables was compared with its table here by
primary key (54 tables, 46,978 rows; the 56 players by id): nothing was missing but what this
server had removed since the switch — two chart files their authors replaced, and twelve
difficulty constants no chart used even in the old databases. The last dump of each, taken
minutes before, is `/var/backups/ourtaiko-{fanmade,database,profile}/daily/20261114T0856…/`
on the server and under `{fanmade,database,profile}/backups/postgresql/` in the private
bucket; nothing deletes them. `deploy/server/import.sh` has nothing left to read and must not
be pointed at a restored copy: it replaces what players have done since.

Removed after them, the same day: the old databases' roles (`ourtaiko`, `ourtaiko_database`,
`ourtaiko_profile`, `ourtaiko_database_viewer`), which owned nothing any more, and the units
`ourtaiko-{fanmade,database,profile}-backup.{service,timer}`. To read one of the last dumps,
restore it into a scratch database with `--no-owner`.

Still there, on purpose:

- The three old host names, their 1Panel sites and certificates, and `LEGACY_*_HOST` in
  `api.env`: every released game before 2026-11-14 asked the old addresses. Since 2026-11-23
  the server no longer reads `LEGACY_*_HOST` and answers nothing for those names; their
  pages are redirected at Cloudflare. The settings and the 1Panel sites can go.
- The roles `ourtaiko_viewer` and `ourtaiko_database_editor`: the console's accounts, in use.
- `/usr/local/libexec/ourtaiko-fanmade-backup`: the account center's daily backup runs it.
- The two IAM users of the old Fanmade and Database sites: this server signs with their keys
  (`FANMADE_AWS_*`, `OFFICIAL_AWS_*` in `storage.env`, now the only copy on the server).
- The old sites' backups under `/var/backups/ourtaiko-*`, the last dumps among them.

Also removed on 2026-11-14: the old settings `/etc/ourtaiko-{fanmade,database,profile}`
(after the storage keys in them were found equal to this server's own), and the unit
`ourtaiko-fanmade.service` from before the sites ran in containers.

The repositories Fanmade, Database, OurTaikoProfile and OurTaikoSiteKit are archived on GitHub
(2026-11-14), so their workflows cannot run and nothing can publish them again.
