# Database

One PostgreSQL database, `ourtaiko_net` (PostgreSQL 18), made from the databases of Fanmade, OurTaiko Database and OurTaikoProfile.

## Layout

| Schema | Holds |
|---|---|
| `fanmade` | Everything Fanmade's database had except users and sign-in |
| `official` | The same tables from OurTaiko Database |
| `net` | `users` (with each player's groups, and a copy of the nickname and avatar the account center publishes), `group_changes`, `sessions`, `oidc_flows`, Profile's `titles`, `awards`, `user_titles`, `user_dans`, `nameplates`, and the parents `scores` and `dan_attempts` |

Rules, all checked by `check.sql`:

- The two sources have the same set of tables with the same columns. Four columns differ in what they allow: `charts.audio_file` and `tja_file` may be empty in `official`, and `category_flags` and `playlists.category_flag` are narrower there. `charts.official_id` is on both so that the same statements read both; only the official source fills it (`build/alike.sql`).
- Each source queues the files its charts and playlists stop referring to (`retired_files`), by trigger, in the transaction that replaces or deletes the row. Fanmade's old database did this; the official source was given the same two triggers here (`build/alike.sql`).
- Each source retires a chart's scores (`is_latest`) when its audio is replaced by one of other content, by trigger (`charts_retire_scores`). OurTaiko Database's old database did this for any change of the audio's record; Fanmade's program did it itself. Both have the one trigger now, also from `build/alike.sql`.
- A connection that works on one source sets `search_path` to `<source>, net`. SQL written without schema names then finds the source's tables and the shared ones.
- Every function of a source is pinned to that same `search_path`, so it works whoever fires it.
- `fanmade.scores` and `official.scores` are partitions of `net.scores`, split by `source`; the same for `dan_attempts`. Each keeps its primary key on `id` and its foreign key to its own `chart_difficulties`. Across sources a score is named by `(source, id)`.
- Slot ids (`chart_difficulties.id`) are per source: both count from 1.

## Files

| File | What it does |
|---|---|
| `build.sh`, `build/` | Builds the empty schema in an empty database from the two song databases' schemas (`adopt.sql`), gives both sources what only one did by itself (`alike.sql`), adds what the site holds once (`net.sql`), and writes `baseline.sql` |
| `baseline.sql` | The result, generated. Do not edit by hand |
| `import.sh`, `import/` | Replaces all song and player data with the three old databases' current contents, in one transaction, and compares every table with its origin |
| `check.sql` | The structural rules above, and every foreign key by query |
| `test/behaviour.sql` | What the database must do once it holds data |
| `dev/` | Test rows and the local test run |

## Changing the schema

The baseline is version 1 and is not edited any more: the database it describes is in production. A change is a file `migrations/<version>_<name>.sql`, versions counting on from 2 without gaps. The server applies the changes a database lacks as it starts, each in a transaction of its own and recorded in `net.schema_migrations` (`internal/netdb`, `Migrate`), so a release that needs a change carries it. It refuses a database newer than itself. Tests run on the baseline with every change applied.

A change runs as the application's role, which owns the tables, while the previous release may still be answering: write changes the release before can live with (add a column with a default; do not rename or drop what it reads).

## Where the baseline comes from

`baseline.sql` is built from the schemas of the production databases, taken from their nightly backups (`dev/pull-backups.sh`, then `dev/test.sh` or `build.sh`). Do not build it from freshly migrated databases: Fanmade does not reach its production layout from migrations alone (a fresh migration leaves everything in `public`, with old columns on `users`).

## Importing

```sh
db/import.sh postgres:///ourtaiko_net_dev <fanmade db> <official db> <profile db>
```

The three old databases are on one server (`SRC_HOST`, `SRC_PORT`, `SRC_USER`, `SRC_PASSWORD`) and are only read. An import can be repeated: each run replaces song and player data, keeps `net.sessions` and every player's groups, and only adds to `net.users`. It refuses to finish if a source has a table or column this database does not know, or if any table's row count or content differs from its origin. Each finished run is recorded in `net.import_runs`.

OurTaikoProfile's awards filed under `database` are filed under `official` here.

This is for development copies only. Production was imported for the last time at the switch on 2026-11-14 and its three old databases were dropped the same day; an import there would replace what players have done since.

## Testing locally

```sh
db/dev/test.sh <fanmade db> <official db> <profile db>
```

The three names are local databases in the old layout. The test copies them, adds test rows to the copies, builds, imports twice, runs `test/behaviour.sql`, and checks that `baseline.sql` rebuilds the same schema. It works in databases named `otn_test_*`, which it removes, and rewrites `baseline.sql` from the sources' schemas.

With AWS credentials for the private bucket, `db/dev/pull-backups.sh` fills the three local databases from last night's production backups (it drops and restores them; the dumps stay in `.data/backups`, which git ignores).

Making the three local databases by hand (names are examples):

- **OurTaiko Database**: any local database its server has migrated, for example a copy of `ourtaiko_database`.
- **OurTaikoProfile**: an empty database with its two migration files applied by `psql` (`internal/database/001_schema.sql`, `002_user_titles.sql`).
- **Fanmade**: a database in the production layout (`public`, `auth`, `internal`) brought to the current schema by Fanmade's own `database.Migrate`. A small `main` that calls `database.Open` and `database.Migrate` can be run inside the Fanmade module without adding a file to it: `go run -overlay <overlay.json> ./cmd/<name>`.
