// The site has two song sources. A page of a source has the source first in its address:
// /fanmade/charts/… or /official/charts/…. Each source's pages are a single-page app of their
// own under that prefix; going from one source to the other loads the page anew. Every other
// page (home, players, titles) belongs to no source and is a third app, at the top.
export const sources = [
  { id: 'fanmade', name: 'Fanmade' },
  { id: 'official', name: 'Database' },
] as const
export type SourceId = (typeof sources)[number]['id']

export function sourceOf(pathname: string): SourceId | null {
  const first = pathname.split('/')[1]
  return sources.find((source) => source.id === first)?.id ?? null
}

// Outside a browser (tests) there is no address; fanmade stands in.
export const SOURCE: SourceId =
  (typeof location === 'undefined' ? null : sourceOf(location.pathname)) ?? 'fanmade'
/** Where this source's pages start; the router's basename. */
export const BASE = `/${SOURCE}`

/** The area this page is in: a source, or '' for the pages that belong to none. */
export const AREA: SourceId | '' =
  (typeof location === 'undefined' ? null : sourceOf(location.pathname)) ?? ''

/** Where the pages about players are: their scores on every source, titles, nameplate. */
export const PLAYERS_BASE = '/players'
/** The first part of the addresses that belong to no source. */
export const sitePages = ['', 'players', 'titles', 'admin', 'manage']
