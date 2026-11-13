# OurTaikoNet

The one backend and website of OurTaiko at `net.ourtaiko.org`. It is being built to replace three services (OurTaiko Fanmade, OurTaiko Database and OurTaikoProfile) with one service on one database. Those three stay untouched until this one is shown to work.

Status: the database layer and a first backend exist and pass their tests locally. On a copy of production data the import matches table for table, and the two sources and the players' pages answer every public question as the three old sites do. Nothing is deployed, and there is no website here: the React frontend will be a repository of its own.

| Part | State |
|---|---|
| `db/` | One database, `ourtaiko_net`, built from the three old ones; importer; tests |
| Backend (Go, this repository) | One program on the one database. `internal/songs` serves a song source, and the program runs it twice: once for Fanmade and once for the official source (`sources.go` holds what each has of its own). `internal/profile` serves the players' pages. `internal/signin` is the website's sign-in and sessions, which all of them use |
| `internal/site` | The new address: each source under `/api/v1/sources/<source>/`, players' routes (titles, nameplates, all their scores) under `/api/v1/` |
| `internal/legacy` | The old host names, answered as the old sites answered. The only place that knows about them |
| `internal/groups` | User groups, one bit each on a player's row: who may do the work each part reserves for its managers. The account center only says who the site's administrator is; the administrator assigns groups under `/api/v1/admin/` |
| `internal/inproc` | The one call the parts still make over HTTP inside the program: the sources reading the title catalog. A stage: it is to become a direct call |
| Website | Not started; separate repository |

## Running

```sh
scripts/test.sh                          # every Go test; see the script for what it needs
go run ./cmd/server -init                # create the schema in an empty database
go run ./cmd/server                      # DATABASE_URL, LISTEN_ADDR, STORAGE_DIR, TJAPARSER_BIN, …
db/dev/pull-backups.sh                   # fill the local source databases from production backups (aws login first)
tools/compare-local.sh <3 local dbs>     # run the old sites and this one side by side and compare their answers
```

The plan, the decisions behind it and the work log are in the private notes repository, under `OurTaikoNet/`.

## The database in one paragraph

`ourtaiko_net` has three schemas. `fanmade` and `official` each hold one song source: its charts, files, dan courses, comments, playlists, scores, exactly the tables the old sites had. `net` holds what the site has once: players, sign-in, roles, titles and nameplates. `net.scores` and `net.dan_attempts` are parents over the two sources' tables, so a player's plays across both sources are one query while each source's scores keep their foreign key to that source's charts. See [db/README.md](db/README.md).
