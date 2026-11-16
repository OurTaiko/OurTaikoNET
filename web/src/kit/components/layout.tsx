import { useState } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { Notice } from '../notifications'
import { ChatBell } from '../chat-center'
import { useSession } from '../session-context'
import { FrameBell, useTellFrame } from '../shell/frame'
import { useSite } from '../site'
import { SOURCE } from '../source'

/** A page of a song source, inside the site's frame (drawn by src/root.tsx). */
export function Layout() {
  const session = useSession()
  const site = useSite()
  const { pathname } = useLocation()
  const [error, setError] = useState('')
  useTellFrame({
    area: SOURCE,
    path: pathname,
    user: session.user,
    loading: session.loading,
    uploadsHere: site.canUpload(session.user),
    signOut: () => {
      setError('')
      void session.logout().catch((e) => setError(e.message))
    },
  })
  return (
    <>
      {session.user && (
        <FrameBell>
          <ChatBell />
        </FrameBell>
      )}
      {(session.error || error) && <Notice>{session.error || error}</Notice>}
      <Outlet />
    </>
  )
}
