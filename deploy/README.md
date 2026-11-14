# Deploying OurTaikoNet

Status: written and tried on a developer's machine. **Nothing here has run on the server or in
GitHub Actions yet**; the first real run of each step should be watched.

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

The repository needs the secrets Fanmade's has: `DEPLOY_HOST`, `DEPLOY_USER`,
`DEPLOY_PASSWORD`, `DEPLOY_FINGERPRINT` (and optionally `DEPLOY_PORT`, `DEPLOY_BASE_PATH`), and
the organization's `PRIVATE_AWS_ACCESS_KEY_ID` and `PRIVATE_AWS_SECRET_ACCESS_KEY`.

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

1. **Database.** In the shared PostgreSQL create the database `ourtaiko_net` and a role for
   the application. Create the schema with the image: `ourtaiko-net -init` against the empty
   database (`DATABASE_URL`). Fill it with `db/import.sh` (see `db/README.md`): it reads the
   three old databases and compares every table with its origin. It can be repeated, so it can
   run nightly while the preview lasts.
2. **Settings.** Write `/etc/ourtaiko-net/docker/api.env` and `storage.env` from
   `api.env.example`. Leave the three `LEGACY_*_HOST` empty for now. During the preview give
   uploads their own S3 prefixes, so that nothing written there mixes with the old sites' files.
3. **Account center.** Register one Application for `net.ourtaiko.org` and put its client and
   service credentials in `api.env`.
4. **Directories.** `/opt/ourtaiko-net/{incoming,incoming-web,web/releases}`, writable by the
   deploy user; the container reads `web/` as user 10001.
5. **Proxy.** A server block for `net.ourtaiko.org` with its certificate, including
   `net.locations.conf`.
6. **Release** this repository, then OurTaikoNetWeb.

## The switch

In one maintenance window, after the proof (PLAN.md, section 5):

1. Stop the three old containers. Released games queue their uploads and retry.
2. Back up the old databases; run `db/import.sh` a last time; it must finish clean.
3. Set the three `LEGACY_*_HOST` in `api.env`, point uploads at the real S3 prefixes, and
   recreate the container.
4. In the server blocks of `fanmade.ourtaiko.org`, `database.ourtaiko.org` and
   `profile.ourtaiko.org`, replace what they proxy to with `net.locations.conf`. From then on
   their APIs are answered as before and their pages are redirected to the new site.
5. Smoke test: sign in on the website, an upload, a released game from start to score.

Going back within the agreed window: restore the old proxy rules and start the old containers,
which were stopped with their databases untouched. Anything written on the new site since the
switch is lost unless it is copied back. After the window the old containers and, after a last
dump, the old databases are removed.
