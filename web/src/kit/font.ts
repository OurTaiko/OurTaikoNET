// The site's typeface. The default is the one bundled with the site (Roboto Flex, then the
// system's own CJK fonts); a player can choose one of the built-in fonts below instead, kept in
// this browser like the theme. A built-in font is a file on the distribution root (fonts/…, the
// mirror for mainland visitors, see distribution.ts), fetched when it is chosen and on later
// page loads (the browser keeps it: the file never changes). The page goes on in the font it has
// until the new one has arrived and been read, then takes it at once; a font that fails to
// arrive leaves the page as it was. styles.css puts --font-chosen first in the site's stack.
import { useSyncExternalStore } from 'react'
import { fetchFile } from './distribution'

export type FontId = 'default' | 'misans' | 'noto-sans-sc'

export type BuiltInFont = {
  id: Exclude<FontId, 'default'>
  /** Its name, as shown in every language. */
  name: string
  /** The name the page knows it by (FontFace), not one the visitor's system could have. */
  family: string
  /** Its path under the distribution root. */
  path: string
  /** Its variable weight axis, as FontFace's weight descriptor. */
  weight: string
  /** What is downloaded (Brotli), in megabytes, as the settings sheet says. */
  megabytes: number
}

// Uploaded Brotli-compressed (Content-Encoding: br) and never replaced in place: a new version of
// a font goes in a new folder (README, "Built-in fonts").
export const builtInFonts: readonly BuiltInFont[] = [
  {
    id: 'misans',
    name: 'MiSans',
    family: 'OurTaiko MiSans',
    path: 'fonts/misans/4.003/MiSans-VF.ttf',
    weight: '150 700',
    megabytes: 12.7,
  },
  {
    id: 'noto-sans-sc',
    name: 'Noto Sans SC',
    family: 'OurTaiko Noto Sans SC',
    path: 'fonts/noto-sans-sc/a85815a/NotoSansSC-VF.ttf',
    weight: '100 900',
    megabytes: 8.5,
  },
]

const storageKey = 'ourtaiko-net-font'
const fontOf = (id: FontId) => builtInFonts.find((font) => font.id === id)

/** The font the player chose; the default for anything not built in. */
export function chosenFont(): FontId {
  try {
    const saved = localStorage.getItem(storageKey)
    return saved && fontOf(saved as FontId) ? (saved as FontId) : 'default'
  } catch {
    return 'default'
  }
}

export type FontState = {
  /** What the player chose. */
  chosen: FontId
  /** What the page is shown in. */
  shown: FontId
  /** The chosen font is on its way. */
  loading: boolean
  /** The chosen font could not be fetched or read; the page stays in `shown`. */
  failed: boolean
}

let state: FontState = { chosen: 'default', shown: 'default', loading: false, failed: false }
/** The chosen font and how far the page has come to it. */
export const fontState = () => state
const watchers = new Set<() => void>()
function set(next: Partial<FontState>) {
  state = { ...state, ...next }
  watchers.forEach((watcher) => watcher())
}

// One fetch per font and page load; a failed one is tried again when the font is chosen again.
const loads = new Map<FontId, Promise<void>>()
function load(font: BuiltInFont): Promise<void> {
  let loading = loads.get(font.id)
  if (!loading) {
    loading = fetchFile(font.path)
      .then((response) => {
        if (!response.ok) throw new Error(`${font.path}: ${response.status}`)
        return response.arrayBuffer()
      })
      .then((data) => new FontFace(font.family, data, { weight: font.weight }).load())
      .then((face) => void document.fonts.add(face))
    loading.catch(() => loads.delete(font.id))
    loads.set(font.id, loading)
  }
  return loading
}

function show(id: FontId) {
  const font = fontOf(id)
  const style = document.documentElement.style
  if (font) style.setProperty('--font-chosen', `'${font.family}'`)
  else style.removeProperty('--font-chosen')
  set({ shown: id, loading: false, failed: false })
}

// Shows `id`: the default at once, a built-in font once it has arrived. Only the latest choice is
// shown: one made while this one was loading wins.
function apply(id: FontId): Promise<void> {
  const font = fontOf(id)
  if (!font) {
    set({ chosen: id })
    show(id)
    return Promise.resolve()
  }
  set({ chosen: id, loading: true, failed: false })
  return load(font).then(
    () => void (state.chosen === id && show(id)),
    () => void (state.chosen === id && set({ loading: false, failed: true })),
  )
}

/** Shows the font the player chose (at startup, and when another tab changes it). */
export const applyFont = () => apply(chosenFont())

/** Keeps the player's choice and shows it, without holding up the page while it arrives. */
export function chooseFont(id: FontId): Promise<void> {
  try {
    localStorage.setItem(storageKey, id)
  } catch {
    // The choice then lasts for this page load only.
  }
  return apply(id)
}

function watch(watcher: () => void) {
  watchers.add(watcher)
  // Another tab changed it.
  const other = (event: StorageEvent) => event.key === storageKey && void applyFont()
  window.addEventListener('storage', other)
  return () => {
    watchers.delete(watcher)
    window.removeEventListener('storage', other)
  }
}
/** The chosen font and how far the page has come to it, kept up to date. */
export const useFontState = () => useSyncExternalStore(watch, fontState)
