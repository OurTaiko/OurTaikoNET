// What the frame knows of the whole site, whichever area shows it.
import { useEffect, useState } from 'react'
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

export function rememberSource(source: SourceId) {
  try {
    localStorage.setItem(sourceKey, source)
  } catch {
    // The choice is simply not remembered.
  }
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
// The last answer, so the frame of the next area shown starts from it.
let lastRoles: { id: string; roles: SiteRoles } | null = null

/**
 * What the signed-in player may manage on the site. A song source's own session says only
 * whether they manage that source, so the frame asks the site once.
 */
export function useSiteRoles(userId: string | undefined): SiteRoles {
  const [roles, setRoles] = useState(lastRoles)
  useEffect(() => {
    if (!userId) return
    const controller = new AbortController()
    fetch('/api/v1/me', { signal: controller.signal, headers: { Accept: 'application/json' } })
      .then((response) => (response.ok ? response.json() : null))
      .then((body) => {
        const user = body?.user
        if (user?.id !== userId) return
        lastRoles = {
          id: userId,
          roles: {
            siteAdmin: !!user.siteAdmin,
            titleManager: !!user.isAdmin,
            groups: Array.isArray(user.groups) ? user.groups : [],
          },
        }
        setRoles(lastRoles)
      })
      .catch(() => {})
    return () => controller.abort()
  }, [userId])
  return roles && roles.id === userId ? roles.roles : nobody
}
