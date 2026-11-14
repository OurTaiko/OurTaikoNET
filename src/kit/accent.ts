// The site's two accents are the drum's two hits: Don (red) and Ka (teal). A song source's
// pages wear the source's own: Official is Don, Fanmade is Ka. The pages that belong to no
// source (home, players, titles) wear the one the player chose, Don until they choose.
// The choice is kept in this browser, like the light or dark theme.
import { useSyncExternalStore } from 'react'
import { AREA } from './source'

export type Tone = 'don' | 'ka'
const storageKey = 'ourtaiko-net-accent'

/** The accent the player chose for the pages of no source. */
export function chosenTone(): Tone {
  try {
    return localStorage.getItem(storageKey) === 'ka' ? 'ka' : 'don'
  } catch {
    return 'don'
  }
}

/** Puts the accent of the area being shown on the page (styles.css reads data-tone). */
export function applyTone() {
  document.documentElement.dataset.tone =
    AREA === 'fanmade' ? 'ka' : AREA === 'official' ? 'don' : chosenTone()
}

const watchers = new Set<() => void>()
const tell = () => {
  applyTone()
  watchers.forEach((watcher) => watcher())
}
export function chooseTone(tone: Tone) {
  try {
    localStorage.setItem(storageKey, tone)
  } catch {
    // The choice then lasts only as long as nothing else asks for it.
  }
  tell()
}
function watch(watcher: () => void) {
  watchers.add(watcher)
  // Another tab changed it.
  const other = (event: StorageEvent) => event.key === storageKey && tell()
  window.addEventListener('storage', other)
  return () => {
    watchers.delete(watcher)
    window.removeEventListener('storage', other)
  }
}
/** The player's chosen accent, kept up to date. */
export const useChosenTone = () => useSyncExternalStore(watch, chosenTone)
