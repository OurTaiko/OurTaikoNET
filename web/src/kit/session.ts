// Who is signed in: one query for the whole site, so that it stays as it is when a link crosses
// from one area to another (src/root.tsx starts each area's pages afresh).
import { useEffect, useMemo } from 'react'
import { QueryObserver, focusManager, queryOptions, useQuery } from '@tanstack/react-query'
import { api, jsonRequest } from './api/client'
import { SITE_API_ROOT } from './api/config'
import { endpoints } from './api/endpoints'
import type { Session, UserBase } from './api/types'
import { queryClient } from './query/client'
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

const nobody: Session<Account> = { user: null, csrfToken: '' }

const sessionKey = ['session'] as const
// Asked again only by refreshSession, whatever reads it.
const sessionQuery = queryOptions({
  queryKey: sessionKey,
  queryFn: ({ signal }) => api<Session<Account>>(`${SITE_API_ROOT}/me`, { signal }),
  staleTime: Infinity,
  gcTime: Infinity,
  retry: false,
})

// What the server says depends on who asks (my vote, my playlists): when that changes, nothing
// it said before holds any more.
let viewer: string | null | undefined
new QueryObserver(queryClient, { ...sessionQuery, enabled: false }).subscribe(({ data }) => {
  if (!data) return
  const now = data.user?.id ?? null
  if (viewer !== undefined && viewer !== now)
    void queryClient.resetQueries({ predicate: (query) => query.queryKey[0] !== sessionKey[0] })
  viewer = now
})

let requested = false

/** Asks the site who is signed in; what is known stays until the answer comes. */
export function refreshSession() {
  // A background tab still loads its session once; only re-checks wait until it is visible.
  if (requested && document.visibilityState === 'hidden') return
  requested = true
  queryClient.fetchQuery({ ...sessionQuery, staleTime: 0 }).catch(() => {})
}

/** What is known now, outside React. */
export function currentSession() {
  const state = queryClient.getQueryState<Session<Account>>(sessionKey)
  return {
    session: state?.data ?? nobody,
    loading: !state || state.status === 'pending',
    error: state?.error?.message ?? '',
  }
}

/** Signs out of the whole site. */
export async function signOutOfSite() {
  await api(endpoints.logout, jsonRequest('POST', {}, currentSession().session.csrfToken))
  // An answer to a question asked before this says nothing any more.
  await queryClient.cancelQueries({ queryKey: sessionKey })
  queryClient.setQueryData(sessionKey, nobody)
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
  const { data, isPending, error } = useQuery(sessionQuery)
  const session = data ?? nobody
  const message = error?.message ?? ''
  const area = AREA
  return useMemo(
    () =>
      ({
        user: session.user && seenIn(session.user, area),
        csrfToken: session.csrfToken,
        loading: isPending,
        error: message,
        logout: signOutOfSite,
      }) as SessionState<U>,
    [session, isPending, message, area],
  )
}

/** The signed-in player as the site describes them: for what is drawn above the areas. */
export const useAccount = () =>
  useQuery({ ...sessionQuery, select: (session) => session.user }).data ?? null

/** In the site's frame: asks again on each page shown and when the visitor comes back. */
export function useSessionRefresh(page: string) {
  useEffect(() => {
    refreshSession()
  }, [page])
  useEffect(() => focusManager.subscribe((focused) => void (focused && refreshSession())), [])
}
