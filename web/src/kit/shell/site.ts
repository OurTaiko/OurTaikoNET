// What the frame knows of the whole site, whichever area shows it.
import { useMemo, useSyncExternalStore } from 'react'
import { useAccount } from '../session'
import { sourceOf, type SourceId } from '../source'

const sourceKey = 'ourtaiko-net-source'

/** The song source used last on this browser; Fanmade the first time. */
export function rememberedSource(): SourceId {
  try {
    return sourceOf(`/${localStorage.getItem(sourceKey)}`) ?? 'fanmade'
  } catch {
    return 'fanmade'
  }
}

const sourceWatchers = new Set<() => void>()
export function rememberSource(source: SourceId) {
  try {
    localStorage.setItem(sourceKey, source)
  } catch {
    // The choice is simply not remembered.
  }
  sourceWatchers.forEach((watcher) => watcher())
}
/**
 * The source the menu's switch is at, kept up to date: what a page of no source shows the
 * songs or players of.
 */
export function useRememberedSource(): SourceId {
  return useSyncExternalStore((watcher) => {
    sourceWatchers.add(watcher)
    return () => void sourceWatchers.delete(watcher)
  }, rememberedSource)
}

export type SiteRoles = {
  /** The site's administrator: manages players' user groups. */
  siteAdmin: boolean
  /** Keeps the title catalog. */
  titleManager: boolean
  /** The user groups the player is in. */
  groups: string[]
}
const nobody: SiteRoles = { siteAdmin: false, titleManager: false, groups: [] }

/** What the signed-in player may manage on the site. */
export function useSiteRoles(userId: string | undefined): SiteRoles {
  const account = useAccount()
  return useMemo(
    () =>
      account && account.id === userId
        ? {
            siteAdmin: !!account.siteAdmin,
            titleManager: !!account.isAdmin,
            groups: account.groups ?? [],
          }
        : nobody,
    [account, userId],
  )
}
