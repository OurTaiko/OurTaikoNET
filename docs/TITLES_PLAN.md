# Titles: plan

What the title system does today, and what is planned for it. Written 2026-11-16 after a review of the system; the order is the order proposed for the work, not a commitment. How the rules work in detail is in the README, "Titles by rule".

## Where it stands

A title is private or public (2026-11-25, migration `0023_title_review`; the README's "Titles: private and public" says how). A player makes private titles, theirs alone to hold and wear; a review by the title managers makes one public, and only a public title is given to others. It comes to a player in three ways, all of them set by the title managers:

1. **A 段位 course** made official names it as a reward (`internal/profile/httpapi/awards.go`, `grant`). A course that is not official gives no title.
2. **A title manager** gives it by hand (`POST /api/v1/admin/grants`).
3. **A rule** gives it by itself (`internal/rewards/rules`, table `net.reward_rules`, called `net.title_rules` before migration `0025`): clearing a chart, clearing charts of a category, being seen signed in on the website or in the game, having uploaded a number of charts.

Every title given is a row of `net.user_titles` pointing at the row of `net.awards` that says why (a maker's own title has none). A player wears one of the titles they hold on their nameplate.

Decisions the rules were built on, all open to change:

- A rule says for itself what "cleared" is: the lowest crown (cleared, full combo, all good) and the lowest course.
- "Every chart of a category" is as many as there are when the player is judged. A title given stays given.
- A new rule gives its title at once to everyone who meets it already.
- Only public titles take rules, and only TITLE_MANAGER makes them.
- A rule gives a player its title once. After an administrator takes it back, that rule does not give it again.
- Only scores that still stand (`is_latest`) count. Uploads are counted for every owner of a fanmade chart, collaborators included (until 2026-11-25, only the first owner, of either source).

## Stage 1: players notice

The rules work, but nothing tells a player that they got a title or how far they are from one.

### 1.1 Rewards, told the same way for every request

**Done on `feat/titles` (2026-11-16)**, as described below, with one change: the number is counted from `net.rewards` on every signed-in answer (an index on the unseen rows makes it one short index scan, usually of nothing), not kept on `net.users`. A counter there would have tied `users` to rewards and had to be kept right by every path that gives or marks; counting keeps the package to its own table. Revisit if the count shows up in the database's load. The README's "Rewards" says how it works.

A reward can come from any request: a score, an upload, or any request of the game at all (`game_login` is judged where a request's activity is written). So no single answer can be where a client learns of it, and none is: not even the score's. There are no exceptions.

- **The server keeps what was not seen yet.** A table of the site's own, `net.rewards (id, user_id, kind, data, created_at, seen_at)`, written in the same statement or transaction that gives the thing: a title from a rule, a 段位 course or an administrator; a 段位 badge; later, anything else given. It is one list for the website and the game.
- **Every answer to a signed-in request carries a header**, `X-Pending-Rewards: <n>`, the number not seen yet. One middleware sets it for every route of both sources and of the players' part, file downloads included; no answer's body changes. Absent, or 0, means nothing to fetch.
- **The client fetches when it sees more than 0**: `GET /api/v1/me/rewards` gives the ones not seen, oldest first, each with what is needed to show it (a title with its band). When it has shown them, `POST /api/v1/me/rewards/seen {"ids": […]}`; the header goes back to 0. The game does this on its result screen after a score as anywhere else: the score's answer comes back with the header like every other.
- **The number must be cheap.** (Planned as a counter on `net.users`; done as an indexed count instead, see above.)
- A response lost to a timeout or a retry loses nothing: the next answer carries the header again.
- The website's bell can show the same number, and the inbox can list the rewards; whether rewards and the sources' notifications become one inbox is to be decided when this is built.

### 1.2 The game

**Built in the game on OurTaikoPlay's branch `feat/rewards` (2026-11-16), not committed yet**: the game's notes `OurTaikoLogs/OurTaikoPlay/Documentation/Rewards.md` say how. The 段位 result claims the badge it celebrates itself, so it is not shown twice.

OurTaikoPlay reads `X-Pending-Rewards` on every answer, fetches and shows the rewards (the title band on the result screen, a toast elsewhere) and marks them seen. A change in the game's repository, after 1.1. The server's side is ready: the game reads the header on any answer and uses `GET /api/v1/game/rewards` and `POST /api/v1/game/rewards/seen` with its token.

### 1.3 and 1.4

| # | What | Notes |
|---|---|---|
| 1.3 | **Progress** towards the rules that count | For the signed-in player, `GET /api/v1/titles` (or a route of its own) gives `{have, need}` for each `category_clear` and `charts_uploaded` rule they have not met. The counting is the rule's own query without its `HAVING`. Shown on `/titles` as "7 / 10" |
| 1.4 | **Pick the player by name** when granting by hand | Done for titles (2026-11-25): `/manage/titles` searches players by nickname, as the co-creator picker does. The Don's parts still ask for a 32-digit ID |

## Done since: parts of the Don to earn (2026-11-17)

A per-player ownership table for the Don's parts, the way titles have one; rules give a title or a part; administrators lock, give and take back parts. README: "Parts of the Don to earn". Also fixed then: the game drops a signed-out player's queue unmarked, a server without a site gives its own rewards, seen rewards are pruned, counts are kept while the database notifies, and an answer with nothing written carries the header.

## Stage 2: more to give titles for

Each new kind is one query for "who meets it" in `rules.met` (`internal/rewards/rules`), a value in the table's `kind` check, and its fields in the form.

| # | What | Notes |
|---|---|---|
| 2.1 | **Rules with a start and an end** | `starts_at`, `ends_at` on `net.reward_rules`. A score counts when it was submitted inside the window; the sign-in rules only give inside it. Event titles |
| 2.2 | **Signed in on N days** | Needs something kept: `net.user_days (user_id, day, site, game)`, written where the activity is written now (at most once a minute per player). Gives `site_login` and `game_login` a `count`. (Where a player was last seen is kept since 2026-11-25, `users.seen_on_site_at` and `seen_in_game_at`, which made the sign-in rules' backfill exact) |
| 2.3 | **More kinds** | Reaching a 段位 (`net.user_dans`), a number of plays, likes received on one's charts. To be chosen as they are wanted, not all at once |
| 2.4 | **Hidden titles** | `hidden` on the rule (or the title): the rule is left out of the public catalog until the player holds the title. The administrator's view shows all |
| 2.5 | **Change a rule** | Done (2026-11-25): `PUT /api/v1/admin/rules/{id}` changes the conditions in place (not the item) and judges everyone again; what the rule gave once it does not give again |

## Stage 3: trust

The counts are the game's word; the server works the score and its rank out of them by the chart's scoring (migration 0021) and does not check the counts. A title for a hard feat can be had with made-up counts.

| # | What | Notes |
|---|---|---|
| 3.1 | **Rules that ask for a replay** | A flag on the rule: only scores with `replay_data` count. Cheap, and raises the cost of forging |
| 3.2 | **Rules an administrator confirms** | A flag on the rule: meeting it files a request instead of giving the title, and TITLE_MANAGER approves it in `/manage/titles` (which has a queue since 2026-11-25, for the reviews of players' titles). For the few titles that should mean something |

Neither stops a determined forger. Checking a replay against the chart on the server would, and is a project of its own.

## Stage 4: the catalog and its upkeep

| # | What | Notes |
|---|---|---|
| 4.1 | **Split the catalog page** | Done (2026-11-25): `/titles` is the public titles, with a search; a player's own are at `/me/titles`. By page once there are many |
| 4.2 | **How many hold each title** | Done (2026-11-25): `holders` in the catalog and in the managers' list |
| 4.3 | **Rules whose chart is gone** | They stay and nobody can meet them. Mark them in `/manage/titles` so they get cleaned up |
| 4.4 | **Categories from the backend** | Done (2026-11-25): `GET /api/v1/titles` gives `categories`, and the rule form takes its IDs from there. Their names in four languages are still the website's (`rules.categories.*`); an ID without one shows as it is |

## Stage 5: what the old Profile service left behind

No behaviour changes; goes with any release of the backend.

| # | What | Notes |
|---|---|---|
| 5.1 | **Read the title catalog directly** | The sources still read it over HTTP inside the program (`internal/inproc`, `awards.Client.Titles`). Make it a call, as awards already are |
| 5.2 | **Remove the service interface** | `POST /internal/v1/awards`, `RegisterService`, the `services` table's keys and `awards.Client.Award` have no caller left |
| 5.3 | **"OurTaiko Profile" in the website's words** | About twenty strings still name the old site (`dans.titleCatalog`, `dans.titleMissing`, `dans.titlesUnavailable`, `ourtaikoProfile.view`, and others) |

## Releasing `feat/titles`

- It carries migrations `0009_title_rules`, `0010_rewards` and `0011_don_items`. Once released, an image from before it refuses to start on the database: there is no rolling back past it without restoring.
- Releasing the backend closes every Multi-Enso room.
- `feat/story` has a migration numbered `0003`, which `main` has since used. Whichever of the two branches is merged second renumbers its migration (this branch moved its own from 0006 to 0008 when it took in `main`, and to 0009–0011 when `main` took 0008 for the visits' times).
- After release, make the first rules by hand in `/manage/titles` and check the number the page reports for each against what is expected.
