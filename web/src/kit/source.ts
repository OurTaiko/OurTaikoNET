// The site has two song sources. A page of a source has the source first in its address:
// /fanmade/charts/… or /official/charts/…. Every other page (home, players, titles) belongs to
// no source. The three areas are shown one at a time by src/root.tsx, which moves between them
// without loading the page anew: what is exported here as SOURCE, BASE and AREA follows the
// area being shown (setArea), so read them when they are needed and never keep a copy.
export const sources = [
  { id: 'fanmade', name: 'Fanmade' },
  { id: 'official', name: 'Database' },
] as const
export type SourceId = (typeof sources)[number]['id']

export function sourceOf(pathname: string): SourceId | null {
  const first = pathname.split('/')[1]
  return sources.find((source) => source.id === first)?.id ?? null
}

const first = typeof location === 'undefined' ? null : sourceOf(location.pathname)
// Outside a browser (tests) there is no address; fanmade stands in.
/** The source whose songs are being shown; the last one shown on a page of no source. */
export let SOURCE: SourceId = first ?? 'fanmade'
/** Where this source's pages start; its router's basename. */
export let BASE = `/${SOURCE}`
/** The area being shown: a source, or '' for the pages that belong to none. */
export let AREA: SourceId | '' = first ?? ''

const followers: (() => void)[] = []
/** Runs `follow` each time the area changes: for values worked out from it. */
export function onAreaChange(follow: () => void) {
  followers.push(follow)
}
/** Turns everything here to another area. Only the root does this, as it swaps the pages. */
export function setArea(area: SourceId | '') {
  AREA = area
  if (area) {
    SOURCE = area
    BASE = `/${area}`
  }
  for (const follow of followers) follow()
}

/** Where the pages about players are: their scores on every source, titles, nameplate. */
export const PLAYERS_BASE = '/players'
/** The first part of the addresses that belong to no source. */
export const sitePages = ['', 'players', 'titles', 'notifications', 'admin', 'manage', 'visits']
/** The area an address is in; null when it is no page of this site as it is now. */
export function areaOf(pathname: string): SourceId | '' | null {
  return sourceOf(pathname) ?? (sitePages.includes(pathname.split('/')[1]) ? '' : null)
}
