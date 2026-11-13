# OurTaikoNetWeb

The website of OurTaiko at `net.ourtaiko.org`: one app for both song sources. Its backend is the `OurTaikoNet` repository.

Status: the two song sources' pages run in one app with a switch between them, against a local backend. Not deployed. The players' pages (titles, nameplates, combined scores) and the manage panel are not here yet.

## How it is put together

A page belongs to the source its address starts with: `/fanmade/…` or `/official/…`. Each source's pages are a single-page app under that prefix; the switch in the header loads the other source's pages anew, with its own charts, text and colours. An address outside both goes to the source used last.

| Folder | What it is |
|---|---|
| `src/kit` | What both sources share: pages, components, the API client. From OurTaikoSiteKit's `web/src` |
| `src/sources/fanmade`, `src/sources/official` | Each source's own parts: its chart card and search, upload form, metadata editor, text. From the two old sites' `web/src`. Loaded only on that source's pages |
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

- The players' pages and the manage panel for user groups.
- The browser chart editor's build (431 MB in the old Fanmade site's `public/edit`): where it is to be hosted is undecided, so the editor page has nothing to load.
- The old sites' browser tests (Playwright).
- How the built site is served and deployed.
