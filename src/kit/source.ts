// The site has two song sources. A page belongs to the one its address starts with:
// /fanmade/charts/… or /official/charts/…. Each source's pages are a single-page app of their
// own under that prefix; going from one source to the other loads the page anew.
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
