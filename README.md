# OurTaikoNetWeb

The website of OurTaiko at `net.ourtaiko.org`: one app for both song sources. Its backend is the `OurTaikoNet` repository.

Status: the two song sources' pages and the players' pages run in one app, against a local backend. Not deployed.

## How it is put together

The site has three areas, told apart by how the address starts: the two song sources, `/fanmade/…` and `/official/…`, and the pages about players, `/players/…`. Each is a single-page app under its prefix, loaded only when one of its pages is opened; going from one area to another loads the page anew. An address outside all three goes to the source used last.

| Folder | What it is |
|---|---|
| `src/kit` | What both sources share: pages, components, the API client. From OurTaikoSiteKit's `web/src` |
| `src/sources/fanmade`, `src/sources/official` | Each source's own parts: its chart card and search, upload form, metadata editor, text. From the two old sites' `web/src`. Loaded only on that source's pages |
| `src/players` | The pages about players, whichever source they play: scores on both, titles, dan badges, nameplate, and for the site's administrator the manage panel (user groups). From OurTaikoProfile's `web/src` |
| `src/TJARenderer` | The chart renderer, another project's source kept as it came |
| `contracts/validation.json` | Upload rules shared with the backend, which has the same file |

The API of a source is under `/api/v1/sources/<source>/`. Signing in is the whole site's: `/api/v1/auth/…`.

## Running

```sh
pnpm install
pnpm dev          # http://127.0.0.1:5173, /api proxied to the backend on 127.0.0.1:8090 (VITE_API_TARGET)
pnpm test         # unit tests
pnpm lint && pnpm format:check && pnpm build
```

## Not here yet

- The players' pages keep the look they had on the old site (their own header, font and components), not the song pages' look.
- The browser chart editor's build (431 MB in the old Fanmade site's `public/edit`): where it is to be hosted is undecided, so the editor page has nothing to load.
- The old sites' browser tests (Playwright).
- How the built site is served and deployed.
