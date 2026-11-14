# The OurTaikoNet website

The website of OurTaiko at `net.ourtaiko.org`: one app for both song sources and the players' pages. Its backend is the rest of this repository. Until 2026-11-14 this was a repository of its own, OurTaikoNetWeb; its history is kept here with the same commit hashes.

Status: in production since 2026-11-14; see "Releasing" below.

## How it is put together

The site has three areas, told apart by how the address starts: the two song sources, `/fanmade/…` and `/official/…`, and the pages that belong to no source (home, players, titles), at the top. Each has a router and pages of its own, fetched the first time one of its pages is opened; `src/root.tsx` shows one area at a time and moves between them without loading the page anew. What depends on the area (`SOURCE`, `BASE`, `AREA` in `src/kit/source.ts`, the API addresses in `src/kit/api/endpoints.ts`, the words) follows the area being shown, so code reads these when it needs them and never keeps a copy at module level. An address outside all three goes to the source used last.

| Folder | What it is |
|---|---|
| `src/kit` | What both sources share: pages, components, the API client. From OurTaikoSiteKit's `web/src` |
| `src/sources/fanmade`, `src/sources/official` | Each source's own parts: its chart card and search, upload form, metadata editor, text. From the two old sites' `web/src`. Loaded only on that source's pages |
| `src/players` | The pages about players, whichever source they play: scores on both, titles, dan badges, nameplate, and for the site's administrator the manage panel (user groups). From OurTaikoProfile's `web/src` |
| `src/TJARenderer` | The chart renderer, another project's source kept as it came |
| `../contracts/validation.json` | Upload rules the backend runs too: one file for both, at the repository's root |

The API of a source is under `/api/v1/sources/<source>/`. Signing in is the whole site's: `/api/v1/auth/…`.

## Running

```sh
pnpm install
pnpm dev          # http://127.0.0.1:5173, /api proxied to the backend on 127.0.0.1:8090 (VITE_API_TARGET)
pnpm test         # unit tests
pnpm lint && pnpm format:check && pnpm build
```

## Releasing

`pnpm build` writes the site to `dist/`. The backend serves it: its `WEB_DIR` is a directory of
this build, and every address that is not the API's and not a file gets `index.html`.

A push to `main` that changes anything here runs the repository's
`.github/workflows/deploy.yml` for the website: lint, formatting, tests, build, then the tested
build is uploaded and `../deploy/publish-web.sh` makes it the current one on the server
(`/opt/ourtaiko-net/web/current`, a link to `web/releases/<commit>`). Nothing restarts. When
the same push changes the backend too, the backend is published first. The scripts and styles
of the build before, which pages already open still ask for, are kept beside the new ones for
two weeks. `rollback.yml`, with the part `website`, points the link back at one of the five
builds the server keeps. See the repository's `deploy/README.md`.

## Not here yet

- The browser chart editor's build (431 MB in the old Fanmade site's `public/edit`): where it is to be hosted is undecided, so the editor page has nothing to load.
- The old sites' browser tests (Playwright).
