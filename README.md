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
| `internal/multiplay`, `proto/` | Multi-Enso, the game's online rooms: a gRPC service in the same program on a port of its own, off unless `MULTIPLAY_LISTEN_ADDR` is set. See "Multi-Enso" below |
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
scripts/proto.sh                         # regenerate internal/multiplay/multiplaypb after changing proto/
go run ./tools/multiplay-smoke           # a player of Multi-Enso for trying it by hand; see "Local development"
```

## Multi-Enso (online rooms over gRPC)

OurTaikoPlay's online multiplayer (マルチ演奏). The protocol is [`proto/ourtaiko/multiplay/v1/multiplay.proto`](proto/ourtaiko/multiplay/v1/multiplay.proto): one bidirectional stream per player, `ourtaiko.multiplay.v1.MultiEnso/Connect`. The first message is `join`; from then on the server sends the whole room (`room`) every time anything in it changes. A player is who the game's session says: metadata `authorization: Bearer <game session>`, the session the game sends to `/api/v1/…/game/…`.

- **Process.** The service runs inside this program (`internal/multiplay`), on its own port: `MULTIPLAY_LISTEN_ADDR` (off when empty; the production Compose file has it, as `0.0.0.0:8081` mapped to `127.0.0.1:8097` on the server, commented out until the server is ready for it). It speaks gRPC in clear text (h2c); the reverse proxy ends TLS and passes `/ourtaiko.multiplay.v1.MultiEnso/` on with `grpc_pass` (`deploy/README.md`, "Multi-Enso (gRPC)"). The game asks `GET /api/v1/game/multiplay` before it connects: `{"url": …, "protocol": 1, "maxMembers": 5}`, where `url` is `MULTIPLAY_PUBLIC_URL` (default `APP_ORIGIN`), or 404 `MULTIPLAY_UNAVAILABLE` where the service is off. Rooms are in memory: a restart closes them all (on SIGTERM each player is told `ROOM_CLOSED`), and only one instance can run.
- **Rooms.** Up to 5 players, a 5-digit number. `quick_match` enters the fullest public room that is waiting, or opens one; `create_private` opens a room only its number can enter (`room_code`), which its host can make public for good (`open_room`). The first player is the host; when the host leaves, the longest-standing connected member is. The host locks the members to begin a round (`lock_members`), with at least `MULTIPLAY_MIN_PLAYERS` present (default 2); a public room locks by itself 30 s after it has that many, and at once when full. The host can remove a member while the room waits (`kick`). An account is in one room at a time.
- **A round.** Song select, 90 s: each player picks a chart of either source (`{source, chart_id}`: published, with files) or leaves it to chance (おまかせ), and names the course they would play; what was picked stays hidden until the draw → one pick is drawn at random (おまかせ: any chart the game's catalog lists, each with the same chance), shown for 4 s → course, 15 s: each player picks one of the drawn chart's courses and is ready; the default is the course they asked for, else the one they played last round, else Oni, when the chart has it, otherwise the nearest easier course, otherwise the nearest harder one → loading, up to 90 s: the game downloads the chart (it is described in the room snapshot exactly as `GET /charts/{id}` of its source describes it) and reports `loaded`; `start_play` is sent once everyone has, and whoever has not in time sits the round out → the play: each player reports score, combo, 良/可/不可, 連打 and the soul gauge about 5 times a second, and the server sends everyone's every 250 ms → the result, once everyone has finished or 30 s after the first one did (no play lasts more than 15 minutes), shown for 10 s → back to the lobby with the same members. A phase that is left with fewer connected players than a round needs goes back to the lobby (`NOT_ENOUGH_PLAYERS`).
- **The result.** Players who finished first, by score, then 良, max combo and gauge. The top finisher is the `MVP`, or the `SUPER_STAR` when at least 30 % ahead of the second; with fewer than two finishers, or a top score of zero, nobody is. `team_score` is the sum. `full_combo`: no 不可; `all_good`: only 良. `cleared` is what the game reported. Scores are the game's word: the server ranks them and does not check them, and each player's own score is uploaded by the game as after any play.
- **Losing the connection.** A stream that ends without `leave` keeps its place for 10 s; `join` with the room's number takes it back, in any phase. A player who drops during a play stays in the result as `disconnected`, as does one still playing when the round is ended. A second connection of the same account to the same room replaces the first (`REPLACED`).
- **Stamps.** `stamp_id` 0–15, one a second per player, none from a player in the middle of a play. There is no text chat.
- **Limits.** 40 messages a second from a client on average (and 40 at once), 64 KiB a message, `join` within 15 s, 2000 rooms. Everything a client reports is clamped to a sane range. A client that does not read what it is sent is dropped.
- **Codes.** A request the room refuses is answered with `notice` and the stream stays open; `closed` ends it. Both carry a `code` for the game to show in the player's language and a `message` in English for logs: `ROOM_NOT_FOUND`, `BAD_ROOM_CODE`, `ROOM_FULL`, `ROOM_BUSY`, `ROOM_CLOSED`, `SERVER_FULL`, `NOT_HOST`, `BAD_PHASE`, `BAD_REQUEST`, `NOT_ENOUGH_PLAYERS`, `CHART_NOT_FOUND`, `NO_CHARTS`, `LOAD_FAILED`, `SERVER_ERROR`, `LEFT`, `KICKED`, `REPLACED`, `NOT_IN_ROOM`, `UPDATE_REQUIRED` (the client's protocol is newer than the server's). A stream that cannot be served at all ends with a gRPC status instead: `UNAUTHENTICATED` `AUTH_REQUIRED` (no session, or one the account center refuses), `UNAVAILABLE` `AUTH_UNAVAILABLE` (the account center could not be asked), `FAILED_PRECONDITION` (the first message was not `join`), `DEADLINE_EXCEEDED` (no `join` in time), `RESOURCE_EXHAUSTED` (`TOO_MANY_MESSAGES`, `SLOW_CONSUMER`, or a message over the size limit).

The state machine (`room.go`, `manager.go`) does no I/O and is tested on a clock the tests move; `server.go` is the gRPC service around it, tested over a loopback connection.

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

Multi-Enso is off unless these are set as well, in `.env` or before the command:

| Variable | For local development |
|---|---|
| `MULTIPLAY_LISTEN_ADDR` | Where the gRPC service listens, for example `127.0.0.1:50061`. With `DEV_USER_ID` only a loopback address is accepted |
| `MULTIPLAY_PUBLIC_URL` | What the game is told to dial: `http://127.0.0.1:50061` (clear text). Needed locally, as the default is `APP_ORIGIN`, the website |
| `MULTIPLAY_MIN_PLAYERS` | `1` to play a round alone; the default is 2 |

```sh
MULTIPLAY_LISTEN_ADDR=127.0.0.1:50061 MULTIPLAY_PUBLIC_URL=http://127.0.0.1:50061 MULTIPLAY_MIN_PLAYERS=1 scripts/dev.sh
curl http://127.0.0.1:8090/api/v1/game/multiplay   # {"url":"http://127.0.0.1:50061","protocol":1,"maxMembers":5}
go run ./tools/multiplay-smoke                     # a whole round alone, every message printed
go run ./tools/multiplay-smoke -do probe           # no session: must answer UNAUTHENTICATED AUTH_REQUIRED
go run ./tools/multiplay-smoke -do token           # the game session of the development user
go run ./tools/multiplay-smoke -do bot -room 12345 -token <another account's session>
```

Without an account center, the game session of `DEV_USER_ID` is the SHA-256, in hex, of `ourtaiko-dev-game:` followed by that ID; it is what `POST /api/v1/sources/fanmade/game/login` gives the game for any name and password, and it opens Multi-Enso as it opens the `/game/` routes. `tools/multiplay-smoke` (see the top of its `main.go`) plays the part of a game: `round` opens a private room and goes through one round alone, which needs `MULTIPLAY_MIN_PLAYERS=1`; `bot` enters a room and plays along in every round, as a second player for a game under test. A second player needs a second account, which a server with only `DEV_USER_ID` does not have: the same account entering a room again replaces its first connection.

The script starts the local PostgreSQL if it is not running and puts ffmpeg on `PATH`. A database filled from production holds the songs but not their files, which are in the bucket: covers and audio of those songs do not load locally, while anything uploaded locally does.


The plan, the decisions behind it and the work log are in the private notes repository, under `OurTaikoNet/`.

## The database in one paragraph

`ourtaiko_net` has three schemas. `fanmade` and `official` each hold one song source: its charts, files, dan courses, comments, playlists, scores, exactly the tables the old sites had. `net` holds what the site has once: players, sign-in, roles, titles and nameplates. `net.scores` and `net.dan_attempts` are parents over the two sources' tables, so a player's plays across both sources are one query while each source's scores keep their foreign key to that source's charts. See [db/README.md](db/README.md).
