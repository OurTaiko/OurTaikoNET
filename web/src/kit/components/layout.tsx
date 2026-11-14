import { useState } from 'react'
import { Outlet } from 'react-router-dom'
import { Notice } from '../notifications'
import { NotificationBell } from '../notification-center'
import { useSession } from '../session-context'
import { Shell } from '../shell/shell'
import { useSite } from '../site'
import { SOURCE } from '../source'

/** A page of a song source inside the site's frame. */
export function Layout() {
  const session = useSession()
  const site = useSite()
  const [error, setError] = useState('')
  return (
    <Shell
      area={SOURCE}
      user={session.user}
      loading={session.loading}
      canUpload={() => site.canUpload(session.user)}
      bell={session.user ? <NotificationBell /> : null}
      onSignOut={() => {
        setError('')
        void session.logout().catch((e) => setError(e.message))
      }}
    >
      {(session.error || error) && <Notice>{session.error || error}</Notice>}
      <Outlet />
    </Shell>
  )
}
