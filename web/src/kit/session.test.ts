import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { seenIn, type Account } from './session'

const player: Account = {
  id: 'a',
  username: 'a',
  nickname: 'A',
  emailVerified: true,
  // On the pages of no source: keeps the title catalog.
  isAdmin: true,
  groups: ['OFFICIAL_MANAGER'],
}

describe('the player as an area knows them', () => {
  it('manages a source only by being in its group', () => {
    expect(seenIn(player, 'official').isAdmin).toBe(true)
    expect(seenIn(player, 'fanmade').isAdmin).toBe(false)
    expect(seenIn({ ...player, groups: undefined }, 'official').isAdmin).toBe(false)
  })

  it('keeps what the site says on the pages of no source', () => {
    expect(seenIn({ ...player, groups: [] }, '').isAdmin).toBe(true)
  })
})

describe('the session store', () => {
  const answer = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  const signedIn = { user: player, csrfToken: 'token' }
  const settle = () => new Promise((resolve) => setTimeout(resolve))

  beforeEach(() => {
    vi.resetModules()
    vi.stubGlobal('document', { visibilityState: 'visible' })
  })
  afterEach(() => vi.unstubAllGlobals())

  it('asks the one address of the site and keeps the answer', async () => {
    const fetch = vi.fn(async () => answer(signedIn))
    vi.stubGlobal('fetch', fetch)
    const { refreshSession, currentSession } = await import('./session')
    expect(currentSession().loading).toBe(true)
    refreshSession()
    await settle()
    expect(fetch.mock.calls.map((call) => (call as unknown[])[0])).toEqual(['/api/v2/me'])
    expect(currentSession()).toMatchObject({ session: signedIn, loading: false, error: '' })
  })

  it('still knows the player when asking again fails', async () => {
    const fetch = vi.fn(async () => answer(signedIn))
    vi.stubGlobal('fetch', fetch)
    const { refreshSession, currentSession } = await import('./session')
    refreshSession()
    await settle()
    fetch.mockImplementation(async () => answer({ message: 'down' }, 503))
    refreshSession()
    await settle()
    expect(currentSession().session.user?.id).toBe('a')
    expect(currentSession().error).not.toBe('')
  })

  it('keeps the same player when the answer says the same', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => answer(signedIn)),
    )
    const { refreshSession, currentSession } = await import('./session')
    refreshSession()
    await settle()
    const before = currentSession().session.user
    refreshSession()
    await settle()
    expect(currentSession().session.user).toBe(before)
  })

  it('forgets what the server said to the player before', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => answer(signedIn)),
    )
    const { refreshSession, signOutOfSite } = await import('./session')
    const { queryClient } = await import('./query/client')
    refreshSession()
    await settle()
    queryClient.setQueryData(['fanmade', 'charts', 'detail', 'a'], { myVote: 1 })
    await signOutOfSite()
    await settle()
    expect(queryClient.getQueryData(['fanmade', 'charts', 'detail', 'a'])).toBeUndefined()
  })

  it('does not let an answer from before signing out bring the player back', async () => {
    let late: (response: Response) => void = () => {}
    const fetch = vi.fn(async (url: string) =>
      url.endsWith('/me') ? answer(signedIn) : answer({}),
    )
    vi.stubGlobal('fetch', fetch)
    const { refreshSession, currentSession, signOutOfSite } = await import('./session')
    refreshSession()
    await settle()
    fetch.mockImplementationOnce(() => new Promise((resolve) => (late = resolve)))
    refreshSession()
    await signOutOfSite()
    late(answer(signedIn))
    await settle()
    expect(currentSession().session.user).toBeNull()
  })
})
