# Deploying OurTaikoNet

Status: published for the first time on 2026-11-14, this repository and then OurTaikoNetWeb,
both by their workflows. The container runs on the server at `127.0.0.1:8096` on a database
imported once from the three old ones, and serves the website. Since the same day it is
public at <https://net.ourtaiko.org> as a preview beside the three old sites. **It has no
account center application yet, so nobody can sign in** (the sign-in link comes back to the
front page), its data is the one import and does not follow the old sites, and it keeps files
locally, so covers and audio answer 503 (see "Files before the switch"). `rollback.yml` has
not been run.

One container, `ourtaiko-net`, on the server that runs the three old sites, beside them until
the switch. It answers `net.ourtaiko.org`, and from the switch on the three old host names as
well (`internal/legacy`). The website is the other repository, OurTaikoNetWeb: its builds are
put in a directory on the server and this container serves the current one.

```
/opt/ourtaiko-net/
  compose.production.yml     put here by every release of this repository
  incoming/<commit>/         an uploaded image, removed once published
  incoming-web/<commit>/     an uploaded website build, removed once published
  web/releases/<commit>/     website builds, the five newest
  web/current -> releases/<commit>
/etc/ourtaiko-net/docker/
  api.env, storage.env       settings and secrets; see api.env.example
```

## A release

Push to `main`. `.github/workflows/deploy.yml` runs every test in the Dockerfile's `test` stage
against a throwaway PostgreSQL 18, builds the image, keeps it in `ourtaiko-private`
(`net/images/<commit>.tar.gz`), uploads it and runs `deploy/publish.sh` on the server, which
loads it, replaces the container and waits for `/readyz`.

The website is released the same way from its own repository; that changes a link on the
server and restarts nothing. When a change needs both, release this repository first.

`rollback.yml` republishes an archived image by commit. It does not touch the database.

Publishing happens only while the repository variable `DEPLOY_ENABLED` is `true` (Settings >
Variables); otherwise a push runs the tests and stops. The same holds for OurTaikoNetWeb,
which stops after its build. Both are `true` since 2026-11-14: **a push to `main` of either
repository publishes to the server.**

Secrets: the server's address is the organization's `SERVER_ADDR`. Each of the two
repositories has `DEPLOY_USER` (`ourtaiko-ci`), `DEPLOY_FINGERPRINT` (the server's SSH host key)
and `DEPLOY_KEY`, a private key whose public half is in that user's `authorized_keys`: signing in
is by key only. Optionally `DEPLOY_PORT` and `DEPLOY_BASE_PATH`. This repository also
needs the organization's `PRIVATE_AWS_ACCESS_KEY_ID` and `PRIVATE_AWS_SECRET_ACCESS_KEY`, which
are given to selected repositories; this one was added on 2026-11-14.

## The account the workflows sign in as

`ourtaiko-ci` on the server (`deploy/server/create-ci-user.sh`, run on 2026-11-14) is used by
nothing else. It has no password and one key, which forwards nothing; it owns
`/opt/ourtaiko-net`; it cannot read the settings files; and it may run as root only the docker
commands `publish.sh` runs, on images named `ourtaiko-net` and the one Compose file
(`/etc/sudoers.d/ourtaiko-ci`). The private key exists only as the `DEPLOY_KEY` secret of the two
repositories: to replace it, make a new pair, put the new public half in the account's
`authorized_keys` and set the secret again.

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
  -e WEB_DIR=/srv/web -v <OurTaikoNetWeb>/dist:/srv/web:ro \
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
3. **Account center.** Register one Application for `net.ourtaiko.org` and put its client and
   service credentials in `api.env`.
4. **Directories.** `/opt/ourtaiko-net/{incoming,incoming-web,web/releases}`, owned by
   `ourtaiko-ci`; the container reads `web/` as user 10001. `/etc/ourtaiko-net/docker`, root
   only. **Done on 2026-11-14.**
5. **Proxy.** In 1Panel, a reverse proxy website for `net.ourtaiko.org` to
   `http://127.0.0.1:8096` with a Let's Encrypt certificate; then `net.locations.conf` is
   installed as `/opt/1panel/www/sites/net.ourtaiko.org/proxy/root.conf` in place of the file
   1Panel wrote (kept beside it as `root.conf.before-net-locations`), and nginx reloaded
   (`nginx -t`, `nginx -s reload` in the OpenResty container). **Done on 2026-11-14.**
6. **Release** this repository, then OurTaikoNetWeb. **Done on 2026-11-14**, before steps 3 and 5.

## Files before the switch

Until the switch the database here is a copy of the old ones, refreshed by the import, while
the old sites go on changing theirs. The server does not only read files: in the background it
deletes the files its database says nothing refers to any more, moves files to new names, and
writes previews. Run on a copy that is hours old, with the right to write where the old sites
keep their files, it would delete or move files the old sites still use.

So before the switch the container gets no credentials for the files at all:
`STORAGE_BACKEND=local` in `storage.env`. Covers, audio and chart files are then not served on
the preview (the pages show their placeholders), and nothing can be uploaded. To see files on
the preview without that risk it needs either credentials that can only read, or a way to turn
the background jobs off; neither exists yet.

## The switch

In one maintenance window, after the proof (PLAN.md, section 5):

1. Stop the three old containers. Released games queue their uploads and retry.
2. Back up the old databases; run `db/import.sh` a last time; it must finish clean.
3. Set the three `LEGACY_*_HOST` in `api.env`, set `storage.env` to `s3` with the real bucket,
   prefixes and credentials, and recreate the container.
4. In the server blocks of `fanmade.ourtaiko.org`, `database.ourtaiko.org` and
   `profile.ourtaiko.org`, replace what they proxy to with `net.locations.conf`. From then on
   their APIs are answered as before and their pages are redirected to the new site.
5. Smoke test: sign in on the website, an upload, a released game from start to score.

Going back within the agreed window: restore the old proxy rules and start the old containers,
which were stopped with their databases untouched. Anything written on the new site since the
switch is lost unless it is copied back. After the window the old containers and, after a last
dump, the old databases are removed.
