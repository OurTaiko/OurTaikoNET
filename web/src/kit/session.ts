// Who is signed in: one store for the whole site, outside React, so that it stays as it is when
// a link crosses from one area to another (src/root.tsx starts each area's pages afresh).
import { useEffect, useMemo, useSyncExternalStore } from 'react'
import { api, jsonRequest } from './api/client'
import { SITE_API_ROOT } from './api/config'
import { endpoints } from './api/endpoints'
import type { Session, UserBase } from './api/types'
import { AREA, type SourceId } from './source'

/** The signed-in player as the site describes them, whichever area is shown. */
export type Account = UserBase & {
  /** The account center's administrator of the site. */
  siteAdmin?: boolean
  /** The user groups the player is in. */
  groups?: string[]
}

export type SessionState<U extends UserBase = UserBase> = Session<U> & {
  loading: boolean
  error: string
  logout: () => Promise<void>
}

type State = { session: Session<Account>; loading: boolean; error: string }

const nobody: Session<Account> = { user: null, csrfToken: '' }
let state: State = { session: nobody, loading: true, error: '' }

const watchers = new Set<() => void>()
const watch = (watcher: () => void) => {
  watchers.add(watcher)
  return () => void watchers.delete(watcher)
}
function set(next: State) {
  state = next
  watchers.forEach((watcher) => watcher())
}

let request: AbortController | undefined
let requested = false
// Goes up on signing out: an answer to a question asked before that says nothing any more.
let revision = 0

/** Asks the site who is signed in; what is known stays until the answer comes. */
export function refreshSession() {
  // A background tab still loads its session once; only re-checks wait until it is visible.
  if (requested && document.visibilityState === 'hidden') return
  requested = true
  request?.abort()
  const current = new AbortController()
  request = current
  const startedAt = revision
  const fresh = () => !current.signal.aborted && startedAt === revision
  api<Session<Account>>(`${SITE_API_ROOT}/me`, { signal: current.signal })
    .then((session) => {
      if (fresh()) set({ session, loading: false, error: '' })
    })
    .catch((e: Error) => {
      if (fresh()) set({ ...state, loading: false, error: e.message })
    })
}

/** What the store holds now, outside React. */
export const currentSession = () => state

/** Signs out of the whole site. */
export async function signOutOfSite() {
  await api(endpoints.logout, jsonRequest('POST', {}, state.session.csrfToken))
  revision++
  request?.abort()
  set({ session: nobody, loading: false, error: '' })
}

// Who manages a source is a matter of these groups, on the server as here.
const managers: Record<SourceId, string> = {
  fanmade: 'FANMADE_MANAGER',
  official: 'OFFICIAL_MANAGER',
}

/** The player as an area's pages know them: isAdmin is whether they manage what is shown there. */
export function seenIn<U extends Account>(user: U, area: SourceId | ''): U {
  return area ? { ...user, isAdmin: !!user.groups?.includes(managers[area]) } : user
}

/** Who is signed in, as the pages of the area being shown know them. */
export function useSession<U extends UserBase = UserBase>() {
  const { session, loading, error } = useSyncExternalStore(watch, () => state)
  const area = AREA
  return useMemo(
    () =>
      ({
        user: session.user && seenIn(session.user, area),
        csrfToken: session.csrfToken,
        loading,
        error,
        logout: signOutOfSite,
      }) as SessionState<U>,
    [session, loading, error, area],
  )
}

/** The signed-in player as the site describes them: for what is drawn above the areas. */
export const useAccount = () => useSyncExternalStore(watch, () => state.session.user)

/** In the site's frame: asks again on each page shown and when the visitor comes back. */
export function useSessionRefresh(page: string) {
  useEffect(() => {
    refreshSession()
  }, [page])
  useEffect(() => {
    window.addEventListener('focus', refreshSession)
    document.addEventListener('visibilitychange', refreshSession)
    return () => {
      window.removeEventListener('focus', refreshSession)
      document.removeEventListener('visibilitychange', refreshSession)
    }
  }, [])
}
