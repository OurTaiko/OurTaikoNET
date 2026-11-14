import { endpoints } from './api/endpoints'
import { useLocation } from 'react-router-dom'
import { useEffect, useRef, useState } from 'react'
import { SessionContext, type SessionState } from './session-context'
import type { ReactNode } from 'react'
import { api, jsonRequest } from './api/client'
import type { Session } from './api/types'
// What each address last said, so an area shown again (or another area asking the same
// address) starts from it instead of from nobody; it is asked again all the same.
const known = new Map<string, Session>()

/** me: where the signed-in player is described; a song source's own by default. */
export function SessionProvider({
  children,
  me = endpoints.me,
}: {
  children: ReactNode
  me?: string
}) {
  const { pathname } = useLocation()
  const [session, setSession] = useState<Session>(
    () => known.get(me) ?? { user: null, csrfToken: '' },
  )
  const [loading, setLoading] = useState(() => !known.has(me)),
    [error, setError] = useState('')
  const revision = useRef(0)
  const requested = useRef(false)
  useEffect(() => {
    let controller: AbortController | undefined
    const refresh = () => {
      // A background tab still loads its session once; only re-checks wait
      // until it is visible.
      if (requested.current && document.visibilityState === 'hidden') return
      requested.current = true
      controller?.abort()
      const current = new AbortController()
      controller = current
      const startedAt = revision.current
      api<Session>(me, { signal: current.signal })
        .then((next) => {
          if (!current.signal.aborted && startedAt === revision.current) {
            known.set(me, next)
            setSession(next)
            setError('')
          }
        })
        .catch((e) => {
          if (!current.signal.aborted && startedAt === revision.current) setError(e.message)
        })
        .finally(() => {
          if (!current.signal.aborted && startedAt === revision.current) setLoading(false)
        })
    }
    refresh()
    window.addEventListener('focus', refresh)
    document.addEventListener('visibilitychange', refresh)
    return () => {
      controller?.abort()
      window.removeEventListener('focus', refresh)
      document.removeEventListener('visibilitychange', refresh)
    }
  }, [pathname, me])
  const logout = async () => {
    await api(endpoints.logout, jsonRequest('POST', {}, session.csrfToken))
    revision.current++
    // One sign-in for the whole site: every area's copy goes.
    known.clear()
    setError('')
    setSession({ user: null, csrfToken: '' })
  }
  const value: SessionState = { ...session, loading, error, setSession, logout }
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
}
