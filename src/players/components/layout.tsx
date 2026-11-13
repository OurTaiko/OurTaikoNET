import { useState } from 'react'
import { Outlet } from 'react-router-dom'
import { Shell } from '@sitekit/shell/shell'
import { NotificationBell } from '@sitekit/notification-center'
import { useSession } from '@players/session-context'
import { Notice } from '@players/notice'

/** A page that belongs to no song source inside the site's frame. */
export function Layout() {
  const session = useSession()
  const [error, setError] = useState('')
  const groups = session.user?.groups ?? []
  return (
    <Shell
      area=""
      user={session.user}
      loading={session.loading}
      bell={session.user ? <NotificationBell /> : null}
      // Anyone uploads to Fanmade; the official source's songs are its managers'.
      canUpload={(source) => source === 'fanmade' || groups.includes('OFFICIAL_MANAGER')}
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
