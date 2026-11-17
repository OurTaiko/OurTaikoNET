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

- **Process.** The service runs inside this program (`internal/multiplay`), on its own port: `MULTIPLAY_LISTEN_ADDR` (off when empty; the production Compose file sets it to `0.0.0.0:8081`, mapped to `127.0.0.1:8097` on the server, since 2026-11-14). It speaks gRPC in clear text (h2c); the reverse proxy ends TLS and passes `/ourtaiko.multiplay.v1.MultiEnso/` on with `grpc_pass` (`deploy/README.md`, "Multi-Enso (gRPC)"). The game asks `GET /api/v1/game/multiplay` before it connects: `{"url": …, "protocol": 1, "maxProtocol": 2, "maxMembers": 5}`, where `url` is `MULTIPLAY_PUBLIC_URL` (default `APP_ORIGIN`), `protocol` the oldest version of the protocol a game may speak (an older game asks its player to update) and `maxProtocol` the newest the server speaks (a newer game joins with that one), or 404 `MULTIPLAY_UNAVAILABLE` where the service is off. Rooms are in memory: a restart closes them all (on SIGTERM each player is told `ROOM_CLOSED`), and only one instance can run.
- **Rooms.** Up to 5 players, a 5-digit number. `quick_match` enters the fullest public room that is waiting, or opens one; `create_private` opens a room only its number can enter (`room_code`), which its host can make public for good (`open_room`). The first player is the host; when the host leaves, the longest-standing connected member is. A round begins in one of two ways only, in a public room as in a private one: the host locks the members (`lock_members`) with at least `MULTIPLAY_MIN_PLAYERS` of them in the room (default 2), or the room is full and every member is in it, when it begins by itself. No room begins a round for having waited. "In the room" is connected and not away at a play or its result (see "Coming back after a play"). The host can remove a member while the room waits (`kick`). An account is in one room at a time.
- **A round.** Song select, 90 s: each player picks a chart of either source (`{source, chart_id}`: published, with files) or leaves it to chance (おまかせ), and names the course they would play; what was picked stays hidden until the draw → a chart somebody picked always goes before chance: one of the charts that were picked is drawn, each with the same chance, and only when everyone left it to chance is any chart drawn (any the game's catalog lists, each with the same chance); it is shown for 4 s → course, 15 s: each player picks one of the drawn chart's courses and is ready; the default is the course they asked for, else the one they played last round, else Oni, when the chart has it, otherwise the nearest easier course, otherwise the nearest harder one → loading, up to 90 s: the game downloads the chart (it is described in the room snapshot exactly as `GET /charts/{id}` of its source describes it) and reports `loaded`; `start_play` is sent once everyone has, and whoever has not in time sits the round out → the play: each player reports score, combo, 良/可/不可, 連打 and the soul gauge about 5 times a second, and the server sends everyone's every 250 ms → the result, once everyone has finished or 30 s after the first one did (no play lasts more than 15 minutes), shown for 10 s → back to the lobby with the same members. A phase that is left with fewer connected players than a round needs goes back to the lobby (`NOT_ENOUGH_PLAYERS`).
- **The result.** Players who finished first, by score, then 良, max combo and gauge. The top finisher is the `MVP`, or the `SUPER_STAR` when at least 30 % ahead of the second; with fewer than two finishers, or a top score of zero, nobody is. `team_score` is the sum. `full_combo`: no 不可; `all_good`: only 良. `cleared` is what the game reported. Scores are the game's word: the server ranks them and does not check them, and each player's own score is uploaded by the game as after any play.
- **Coming back after a play (protocol 2).** A play is the game's own play scene, and after it the player's own result screen, so a round's players are away from the room's screen for a while. From `start_play` each of them is `away` in the room snapshot until their game says `returned`, which it does while the room's screen is up and the snapshot still has the player away (about once a second, so a message lost with a connection is said again). A member who is away is not in the room: a full room does not begin its next round until every member is back, and the host cannot start with fewer than the minimum in the room (`NOT_ENOUGH_PLAYERS`). When the host does start, the members who are still away are left behind: they are taken out of the room (`closed` `LEFT_BEHIND`) and may come in again by its number once it is back in its lobby. Nothing begins or ends by itself for a player staying away. A member whose game joined with protocol 1 is never away: those games do not say when they are back.
- **Losing the connection.** A stream nothing has come from for 40 s is ended (`DEADLINE_EXCEEDED`, `SILENT`): the game pings every 15 s, and a connection that died without a word is found by its silence. A stream that ends without `leave` keeps its place for 10 s; `join` with the room's number takes it back, in any phase. A player who drops during a play stays in the result as `disconnected`, as does one still playing when the round is ended. A second connection of the same account to the same room replaces the first (`REPLACED`).
- **Stamps.** `stamp_id` 0–15, one a second per player, none from a player in the middle of a play. There is no text chat.
- **Limits.** 40 messages a second from a client on average (and 40 at once), 64 KiB a message, `join` within 15 s, 2000 rooms. Everything a client reports is clamped to a sane range. A client that does not read what it is sent is dropped.
- **Codes.** A request the room refuses is answered with `notice` and the stream stays open; `closed` ends it. Both carry a `code` for the game to show in the player's language and a `message` in English for logs: `ROOM_NOT_FOUND`, `BAD_ROOM_CODE`, `ROOM_FULL`, `ROOM_BUSY`, `ROOM_CLOSED`, `SERVER_FULL`, `NOT_HOST`, `BAD_PHASE`, `BAD_REQUEST`, `NOT_ENOUGH_PLAYERS`, `CHART_NOT_FOUND`, `NO_CHARTS`, `LOAD_FAILED`, `SERVER_ERROR`, `LEFT`, `KICKED`, `REPLACED`, `NOT_IN_ROOM`, `UPDATE_REQUIRED` (the client's protocol is newer than the server's). A stream that cannot be served at all ends with a gRPC status instead: `UNAUTHENTICATED` `AUTH_REQUIRED` (no session, or one the account center refuses), `UNAVAILABLE` `AUTH_UNAVAILABLE` (the account center could not be asked), `FAILED_PRECONDITION` (the first message was not `join`), `DEADLINE_EXCEEDED` (no `join` in time), `RESOURCE_EXHAUSTED` (`TOO_MANY_MESSAGES`, `SLOW_CONSUMER`, or a message over the size limit).

The state machine (`room.go`, `manager.go`) does no I/O and is tested on a clock the tests move; `server.go` is the gRPC service around it, tested over a loopback connection.

## Visits

How much the website is visited (`internal/visits`, migration `0005_visits`). The website tells the server each page it shows (`web/src/kit/shell/visits.ts`, from the frame, once a page has stayed 0.6 s); nothing is counted from the requests themselves, so the game and programs reading the API are not visitors.

- **What is kept.** `net.visit_days (day, views, visitors, salt)`, `net.visit_pages (day, page, views)` and, for the day that is still open, `net.visit_visitors (day, visitor)`. A day is a day of UTC+8. A page is an address with every part that is not a plain word replaced by `:id` (`/fanmade/charts/:id`); a day has at most 300 pages, the rest go to `(other)`.
- **Visitors.** No cookie and no stored address: a visitor is `sha256(the day's random salt || IP and User-Agent)`, so the same person is one visitor a day and cannot be followed from one day to the next. Every hour the days that are over are closed: the hashes are counted into `visit_days.visitors` and deleted with the salt. Over several days "visitors" is the sum of each day's.
- `POST /api/v1/visits` `{"path": "/fanmade/charts/…"}` → 204, with or without a session. Not counted, and still 204: a User-Agent that is a program's or missing, more than 60 a minute from one address, a path that is no address of the site. It does not count against the 40 writes a minute.
- `GET /api/v1/visits`, public → `{"total":{"views","visitors"},"today":{…},"since":"YYYY-MM-DD"}`, kept in memory for 30 s. The footer of every page shows it.
- `GET /api/v1/admin/visits?days=30` (1–366), the site's administrator → the same, plus `days` (each day, oldest first, with the days nobody came) and `pages` (the 50 seen most in the period). The website shows it at `/manage/visits` (`web/src/players/pages/visits.tsx`), in the menu for the administrator.

## Messages

Players writing to each other (`internal/chat`, migration `0006_chats`; the routes in `internal/profile/httpapi/chats.go`). The website's notification center became a messages page (`/chats`, `web/src/kit/chat-center.tsx`): the chats with other players, and the site's own notices as one chat among them, 系统通知 (`/chats/system`). The notices are not stored as chats: each source still keeps its own (`notifications`, `/api/v1/sources/<source>/me/notifications…`), and the website asks both and shows them together, as it did before (`web/src/kit/notification-center.tsx`). `/notifications` goes to `/chats/system`.

- **What is kept.** `net.chats (id, pair, created_at, last_message_at)`: one chat for each two players (`pair` is their ids, the smaller first, joined by `:`); `net.chat_members (chat_id, user_id, read_seq)`; `net.chat_messages (seq, chat_id, sender_id, body, created_at)`, `seq` counting over all chats; `net.chat_blocks (user_id, blocked_id)`. A message is 1–2000 characters, without the blank around it. Nothing is deleted or edited, and nobody but the two members reads a chat through the API.
- **Blocking.** While either of two players has blocked the other, neither can write in their chat (403 `CHAT_BLOCKED`). The blocked one is not told why.
- **Routes**, all for the signed-in player: `GET /api/v1/chats` (the chats written in, the latest first, at most 200, and `cursor`), `POST /api/v1/chats {"userId"}` (the chat with a player who has been here, made if there is none; 404 `USER_NOT_FOUND`, 400 `CHAT_SELF`), `GET /api/v1/chats/unread` (`{"count","chats","cursor"}`), `GET /api/v1/chats/{id}`, `GET …/{id}/messages?before=<seq>&limit=<1–100>` (oldest first, and `more`), `POST …/{id}/messages {"body"}` (201), `POST …/{id}/read {"seq"}`, `PUT …/{id}/block {"blocked"}`. A chat is `{"id","peer":{"id","nickname","avatarUrl"},"lastMessage","unread","readSeq","peerReadSeq","blocked","canWrite","createdAt"}`. Marking read does not count against the 40 writes a minute.
- **Live.** `GET /api/v1/chats/updates?after=<cursor>` answers at once with the messages after the cursor, or waits up to 25 s for one or for something else in the player's chats (a read, a block: `"changed": true`): `{"messages","cursor","changed"}`. The waiting is in this process (`chat.Hub`), so it holds while the site runs as one instance. The website keeps one such request open per page, only while the page is shown (`web/src/kit/chat.ts`); a hidden page reads its count once a minute.

## The 3D Don

OurTaikoPlay draws each player's 3D Don (its Chara3D, ported from YataiDON). The models are not in this repository or the game: they are in the public bucket under `3dchara/` (`https://distrib.ourtaiko.org/3dchara/`), listed by `3dchara/catalog.json`, which `tools/3dchara/catalog.py` writes for a local copy of that folder and `tools/3dchara/publish.sh` uploads (files first, catalog last). A new costume is a new file there (and its name in `names.json`).

- **Catalog.** `{"version":1, "base":{"animations","face","faceAnimations"}, "defaultColors":{"p1","p2"}, "items":{"cos","head","body","puchi","paint":[{"id","name":{"ja",…},"file":{"path","size","sha256"},"icon"?}]}}`; paths are relative to the catalog. A puchichara's file is a two-frame PNG or an animated `.otanim` (made with spine2otanim; format and tool in the `otanim` folder beside this repository, `SPEC.md`), told apart by its first bytes; an `.otanim` comes with an icon (`icons/puchi/<id>.png`), which the dressing room shows instead of the file. The server reads it (`DON_CATALOG_URL`, default the bucket's) and keeps it 5 minutes; when a read fails the last good one stays.
- **Makeup** (`paint`, YataiDON's `chara_paint_index`, the cabinet's メイク): `paint/<id>.png` is a face sheet like `base/face.png` (YataiDON's `Models/paint/paint_<id>000.png`, ids 1–57; 0 is no makeup and has no file). The Don draws it instead of the plain sheet, on face plates a third larger (YataiDON `PAINT_PLANE_SCALE`); a head or costume with no face plate of its own shows no makeup.
- **Data.** `net.don_looks (user_id, costume, cos_id, head_id, body_id, puchi_id, paint_id, colors, updated_at)`, migrations `0003_don_looks` and `0007_don_paint` (`paint_id`, default 0). No row is the plain Don (costume 0); `colors` null is the seat's colours. (`feat/story`'s migration was also numbered 0003: whichever of the two is deployed second must be renumbered, since a database only runs versions above its newest.)
- `GET /api/v1/don/catalog` → the catalog with `baseUrl`, the address its paths are relative to; 503 `DON_CATALOG_UNAVAILABLE` while it has never been read. Readable from any website.
- `GET /api/v1/users/{id}/don` (the game, no session) and `GET /api/v1/me/don` (session) → `{"look":{"costume","cos","head","body","puchi","paint","colors":["#RRGGBB"×3]|null},"files":[{"kind","id","url","size","sha256"}]}`: the files the look needs (the costume, or the body and the head, the puchichara and the makeup), with `?v=<hash>` in the address. Without a catalog `files` is empty.
- **The website.** Each player's page has a Don tab (`web/src/players/pages/user-don.tsx`, loaded on demand): a live 3D preview for everyone, and for the player themself the dressing room in three tabs: the outfit (a whole costume, or a head and a body, switched inside the tab), the puchichara, and the makeup (the tab shows only when the catalog has makeup; each sheet's first face on the Don's face colour), each an icon list; below the tabs the three colours, and Save. The preview is `web/src/kit/don3d` (`@sitekit/don3d`), the game's Chara3D ported to three.js; `web/dev/don3d.html` (not in the build; `pnpm dev`, then `/dev/don3d.html?grid=1`) draws the game's preview shots for comparing with the game.
- `PUT /api/v1/me/don` (session and CSRF) with a look (`paint` left out is 0): every number must be in the catalog, also the ones not worn now (so switching between a costume and a head and body keeps both), colours `#RRGGBB` (stored upper case) or null; 422 `DON_LOOK_INVALID`; answers as GET.

## The nameplate art

The website and the game draw nameplates, title bands and 段位 emblems from the same art, unchanged: the Nijiiro skin's `global/nameplate` plus OurTaikoPlay's own 段位 rings and rank texts. It lives only in the public bucket under `nameplate/` (the game keeps no copy since 2026-11-15; it fetches the prefix at sign-in and caches it) (`https://distrib.ourtaiko.org/nameplate/`), listed by `nameplate/catalog.json` (`{"version":1,"files":{"shadow","outline","bands","rainbow","danBackground","danRing{Silver,Gold,Rainbow}","danText","danTextGold","badge1p","badge2p":{"path","size","sha256"}}}`). `tools/nameplate/publish.sh <folder of the 12 PNGs>` writes the catalog and uploads (files first, catalog last, then clears `/nameplate/*` on CloudFront). The backend reads it for `GET /api/v1/nameplate-art/catalog`, returning the canonical `files` and `baseUrl`, plus exact-match `mirrorBaseUrl`/`mirrorPaths` for mainland visitors. The website and updated Play clients use these addresses, verify bytes, and fall back to the canonical source on mirror failure.

`web/src/kit/nameplate-art-source.ts` reads the regional catalog once per page load, downloads each file with `?v=<hash>`, verifies its bytes and supplies shared blob URLs to `web/src/kit/components/nameplate-art.tsx`. `NameplateArt` stacks the layers where the game's `NameplateView` puts them on the 408x96 canvas (cropped to the art, x 42–371, y 10–91) and scales with its width; the title and name are text, centred and squeezed into the game's boxes. `TitleBandArt` is the band alone and `DanEmblem` the ring with the rank. The rainbow band cycles its 6 frames every 300 ms like the game (still when motion is reduced). If the catalog cannot be read, the text still shows without the art.

## Regional resource distribution

Public chart resource manifests and GET/HEAD file routes share the regional Don IP policy:
mainland China uses verified available objects under `https://static.sakura-bot.cn/ourtaiko/`,
while other regions use the canonical CDN. TJA, full/preview audio, covers, backgrounds and
MP4s are eligible; signed/private URLs and custom origins are unchanged. A routed manifest
file adds optional `fallbackUrl` for a credential-free, size/SHA-256-verified canonical retry.
File routes accept `?origin=1`. See [regional deployment](deploy/README.md#regional-resource-mirror-rollout)
and [OSS/ESA policy](deploy/aliyun-mirror.md) for configuration, lazy back-to-origin and rollout.

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

## License

OurTaikoNet (the backend and the website under `web/`) is free software under the GNU Affero
General Public License, version 3 or (at your option) any later version; see [LICENSE](LICENSE).
As the AGPL asks of a program used over a network, the website links to this source from its
settings (About).

The website's frame (the top bar, the side menu and the settings sheet) follows the layout of
[Moesekai](https://github.com/StarMoe-org/Moesekai), which is under the AGPL-3.0.
`web/src/TJARenderer` keeps its own MIT license (`web/src/TJARenderer/LICENSE`).
