// The site has three areas, told apart by how the address starts: the two song sources,
// /fanmade/… and /official/…, and the pages that belong to no source (home, players, titles),
// at the top. Each has a router and pages of its own, fetched the first time one of its
// pages is opened; this is what shows one of them at a time and moves from one to another
// without loading the page anew. All three are shown in the same frame (@sitekit/shell), drawn
// once here.
import { StrictMode, useSyncExternalStore } from 'react'
import type { ComponentType } from 'react'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'
import { IconContext } from '@phosphor-icons/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { applyTone } from '@sitekit/accent'
import { i18n, setAreaCatalogues, type Language } from '@sitekit/i18n'
import { moveInArea, setVisitor } from '@sitekit/navigation'
import { queryClient } from '@sitekit/query/client'
import { FrameBuddy } from '@sitekit/shell/buddy'
import { Frame } from '@sitekit/shell/shell'
import { rememberSource, rememberedSource } from '@sitekit/shell/site'
import { areaOf, setArea, type SourceId } from '@sitekit/source'
import { ThemeProvider } from '@sitekit/theme'
import './styles.css'

type Area = SourceId | ''
/** An area's pages: its own words, and what to render. */
type Part = { catalogues: Record<Language, object>; App: ComponentType }

const fetchers: Record<Area, () => Promise<Part>> = {
  '': () => import('@players/app').then((module) => module.players),
  fanmade: () => import('./songs').then((module) => module.songs('fanmade')),
  official: () => import('./songs').then((module) => module.songs('official')),
}
const fetched = new Map<Area, Part>()
const fetching = new Map<Area, Promise<Part>>()
function fetchPart(area: Area) {
  let part = fetching.get(area)
  if (!part) {
    part = fetchers[area]().then((value) => {
      fetched.set(area, value)
      return value
    })
    // A failed fetch (offline, a new release) is tried again on the next visit.
    part.catch(() => fetching.delete(area))
    fetching.set(area, part)
  }
  return part
}

// What is on screen. null while the first area, or one arrived at by the back button, is
// being fetched.
let shown: { area: Area; part: Part } | null = null
const watchers = new Set<() => void>()
const watch = (watcher: () => void) => {
  watchers.add(watcher)
  return () => void watchers.delete(watcher)
}
function show(next: typeof shown, now: boolean) {
  shown = next
  const tell = () => watchers.forEach((watcher) => watcher())
  // After the back button the address has already left the old area, whose router must be
  // gone before it looks at it.
  if (now) flushSync(tell)
  else tell()
}

/** Turns everything to an area: the addresses its pages ask, the words, the accent. */
function turnTo(area: Area, part: Part, now: boolean) {
  setArea(area)
  setAreaCatalogues(part.catalogues)
  if (area) {
    document.documentElement.dataset.source = area
    rememberSource(area)
  } else delete document.documentElement.dataset.source
  applyTone()
  show({ area, part }, now)
}

let turn = 0
/**
 * Shows the page at an address. `how` says what becomes of the browser's history: a new
 * entry ('push'), the current one rewritten ('replace'), or nothing because the browser has
 * already moved ('arrive': the first page, the back and forward buttons).
 */
async function go(address: string, how: 'push' | 'replace' | 'arrive') {
  const target = new URL(address, location.origin)
  let area = areaOf(target.pathname)
  if (area === null) {
    // An address from before the sources were told apart (/charts/x) goes to the source used
    // last, Fanmade the first time, keeping the rest of the address.
    area = rememberedSource()
    target.pathname = `/${area}${target.pathname}`
    if (how === 'arrive') how = 'replace'
  }
  const path = target.pathname + target.search + target.hash
  if (shown?.area === area) {
    // The same area: its own router moves, and hears the back button itself.
    if (how !== 'arrive')
      moveInArea(area ? path.slice(area.length + 1) || '/' : path, how === 'replace')
    return
  }
  const mine = ++turn
  let part = fetched.get(area)
  if (!part) {
    if (how === 'arrive' && shown) show(null, true)
    try {
      part = await fetchPart(area)
    } catch {
      // The area's pages could not be fetched: let the browser load the address whole.
      if (how === 'arrive') location.reload()
      else location.assign(path)
      return
    }
    if (mine !== turn) return
  }
  if (how === 'push') history.pushState(null, '', path)
  else if (how === 'replace') history.replaceState(null, '', path)
  turnTo(area, part, how === 'arrive')
  if (how !== 'arrive') window.scrollTo(0, 0)
}

function Root() {
  const current = useSyncExternalStore(watch, () => shown)
  return (
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <IconContext.Provider value={{ weight: 'bold' }}>
            <ThemeProvider storageKey="ourtaiko-net-theme">
              {/* The frame: one for the whole site, so it stays put when the area changes. */}
              <Frame>
                {/* A new key for each area: its router and every page under it start afresh. */}
                {current && <current.part.App key={current.area} />}
              </Frame>
              {/* The player's Don: one for the whole site, so it is not drawn anew with an area. */}
              <FrameBuddy />
            </ThemeProvider>
          </IconContext.Provider>
        </I18nextProvider>
      </QueryClientProvider>
    </StrictMode>
  )
}

/**
 * A plain link to a page of this site is followed in place. Links the routers made have
 * been dealt with by then; links to anything else (the API, files, other sites, a new tab)
 * are left to the browser.
 */
function followLink(event: MouseEvent) {
  if (event.defaultPrevented || event.button !== 0) return
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
  const link = event.target instanceof Element ? event.target.closest('a[href]') : null
  if (!(link instanceof HTMLAnchorElement)) return
  if ((link.target && link.target !== '_self') || link.hasAttribute('download')) return
  if (link.getAttribute('href')?.startsWith('#')) return
  const to = new URL(link.href)
  if (to.origin !== location.origin || areaOf(to.pathname) === null) return
  event.preventDefault()
  void go(to.pathname + to.search + to.hash, 'push')
}

export function start() {
  // Before anything is rendered, so that this hears the back button ahead of the routers.
  window.addEventListener('popstate', () => {
    void go(location.pathname + location.search + location.hash, 'arrive')
  })
  document.addEventListener('click', followLink)
  setVisitor((address, replace) => void go(address, replace ? 'replace' : 'push'))
  createRoot(document.getElementById('root')!).render(<Root />)
  void go(location.pathname + location.search + location.hash, 'arrive')
}
