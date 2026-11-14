# OurTaikoNet

The one backend and website of OurTaiko at `net.ourtaiko.org`. It is being built to replace three services (OurTaiko Fanmade, OurTaiko Database and OurTaikoProfile) with one service on one database. Those three stay untouched until this one is shown to work.

Status: the database layer and a first backend exist and pass their tests locally. On a copy of production data the import matches table for table, and the two sources and the players' pages answer every public question as the three old sites do. It is in production since 2026-11-14; how it is published is in [deploy/README.md](deploy/README.md). The website is [`web/`](web/README.md), a React app this server serves (`WEB_DIR`) in front of the API. It was a repository of its own, OurTaikoNetWeb, until 2026-11-14; its history is kept under `web/` with the same commit hashes.

| Part | State |
|---|---|
| `db/` | One database, `ourtaiko_net`, built from the three old ones; importer; tests |
| Backend (Go, this repository) | One program on the one database. `internal/songs` serves a song source, and the program runs it twice: once for Fanmade and once for the official source (`sources.go` holds what each has of its own). `internal/profile` serves the players' pages. `internal/signin` is the website's sign-in and sessions, which all of them use |
| `internal/site` | The new address: each source under `/api/v1/sources/<source>/`, players' routes (titles, nameplates, all their scores, the site's latest activity) under `/api/v1/`, and the website's build (`WEB_DIR`) for every other address |
| `internal/legacy` | The old host names: their APIs answered as the old sites answered, their pages sent to the same pages of the new site. The only place that knows about them |
| `internal/groups` | User groups, one bit each on a player's row: who may do the work each part reserves for its managers. The account center only says who the site's administrator is; the administrator assigns groups under `/api/v1/admin/` |
| `internal/inproc` | The one call the parts still make over HTTP inside the program: the sources reading the title catalog. A stage: it is to become a direct call |
| `web/` | The website (React): both song sources and the players' pages in one app; see `web/README.md` |
| `contracts/` | Cases both sides run: `validation.json`, the upload rules of the Go package `tja` and of the website's `src/kit/tja.ts` |
| `Dockerfile`, `deploy/`, `.github/workflows/` | The image, how the backend and the website are published to the server and rolled back; see `deploy/README.md` |

## Running

```sh
scripts/test.sh                          # every Go test; see the script for what it needs
go run ./cmd/server -init                # create the schema in an empty database
go run ./cmd/server                      # DATABASE_URL, LISTEN_ADDR, STORAGE_DIR, TJAPARSER_BIN, …
db/dev/pull-backups.sh                   # fill the local source databases from production backups (aws login first)
tools/compare-local.sh <3 local dbs>     # run the old sites and this one side by side and compare their answers
```

## Local development

The backend and the website run side by side on this machine:

```sh
scripts/dev.sh                # the backend on http://127.0.0.1:8090
(cd web && pnpm dev)          # the website on http://127.0.0.1:5173, passing /api on
```

Open <http://127.0.0.1:5173> (that address, not `localhost`: the backend accepts changes only from the origin it is told, `APP_ORIGIN`).

`scripts/dev.sh` reads `.env` in this directory, which git ignores. What it needs:

| Variable | For local development |
|---|---|
| `DATABASE_URL` | The local database, for example `postgres:///ourtaiko_net_dev?host=/tmp&sslmode=disable` (made by `db/import.sh`, or empty after `-init`) |
| `LISTEN_ADDR`, `APP_ORIGIN` | `127.0.0.1:8090` and `http://127.0.0.1:5173` |
| `STORAGE_DIR` | Where uploaded files go, for example `.data` |
| `TJAPARSER_BIN` | A build of TJAParser `v0.13.0`, for example kept under `.data/tools/` |
| `DEV_USER_ID` | 32 lowercase hex digits. With no account center configured, signing in signs in as this player, who may manage everything. Only accepted on a loopback address |

The script starts the local PostgreSQL if it is not running and puts ffmpeg on `PATH`. A database filled from production holds the songs but not their files, which are in the bucket: covers and audio of those songs do not load locally, while anything uploaded locally does.


The plan, the decisions behind it and the work log are in the private notes repository, under `OurTaikoNet/`.

## The database in one paragraph

`ourtaiko_net` has three schemas. `fanmade` and `official` each hold one song source: its charts, files, dan courses, comments, playlists, scores, exactly the tables the old sites had. `net` holds what the site has once: players, sign-in, roles, titles and nameplates. `net.scores` and `net.dan_attempts` are parents over the two sources' tables, so a player's plays across both sources are one query while each source's scores keep their foreign key to that source's charts. See [db/README.md](db/README.md).
