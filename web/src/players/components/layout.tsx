import { useState } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { FrameBell, useTellFrame } from '@sitekit/shell/frame'
import { NotificationBell } from '@sitekit/notification-center'
import { useSession } from '@players/session-context'
import { Notice } from '@players/notice'

/** A page that belongs to no song source, inside the site's frame (drawn by src/root.tsx). */
export function Layout() {
  const session = useSession()
  const { pathname } = useLocation()
  const [error, setError] = useState('')
  useTellFrame({
    area: '',
    path: pathname,
    user: session.user,
    loading: session.loading,
    // No source of its own: the frame asks the site's groups (see @sitekit/shell/shell).
    uploadsHere: false,
    signOut: () => {
      setError('')
      void session.logout().catch((e) => setError(e.message))
    },
  })
  return (
    <>
      {session.user && (
        <FrameBell>
          <NotificationBell />
        </FrameBell>
      )}
      {(session.error || error) && <Notice>{session.error || error}</Notice>}
      <Outlet />
    </>
  )
}
