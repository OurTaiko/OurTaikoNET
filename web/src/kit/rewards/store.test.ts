import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  currentRewards as current,
  dismissRewards,
  noticeRewards,
  resetRewards,
  REWARDS_HEADER,
  type Reward,
} from './store'

const reward = (id: number): Reward => ({ id, kind: 'title', data: {}, createdAt: '' })
const answer = (pending: string | null, body: object = {}, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: pending === null ? {} : { [REWARDS_HEADER]: pending },
  })
const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

describe('rewards', () => {
  let calls: { url: string; init?: RequestInit }[]
  let server: (url: string, init?: RequestInit) => Response
  beforeEach(() => {
    calls = []
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      return server(url, init)
    })
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    resetRewards()
  })

  it('fetches only when an answer says something is waiting, and once', async () => {
    server = (url) =>
      url === '/api/v1/me'
        ? answer('2', { user: { id: 'p1' }, csrfToken: 'c' })
        : answer('2', { rewards: [reward(1), reward(2)] })
    noticeRewards(answer(null))
    noticeRewards(answer('0'))
    await settle()
    expect(calls).toHaveLength(0)
    noticeRewards(answer('2'))
    noticeRewards(answer('2'))
    await settle()
    expect(calls.filter((c) => c.url === '/api/v1/me/rewards')).toHaveLength(1)
    expect(current().rewards.map((r) => r.id)).toEqual([1, 2])
    expect(current().player).toBe('p1')
    // While they are on show, more answers fetch nothing.
    noticeRewards(answer('2'))
    await settle()
    expect(calls.filter((c) => c.url === '/api/v1/me/rewards')).toHaveLength(1)
  })

  it('marks what was shown seen with the CSRF token, and fetches again if more wait', async () => {
    let left = '1'
    let lists = 0
    server = (url) => {
      if (url === '/api/v1/me') return answer(null, { user: { id: 'p1' }, csrfToken: 'token' })
      if (url === '/api/v1/me/rewards/seen') return answer(left, { marked: 1 })
      lists += 1
      return answer('1', { rewards: [reward(lists)] })
    }
    noticeRewards(answer('1'))
    await settle()
    await dismissRewards()
    const seen = calls.find((c) => c.url === '/api/v1/me/rewards/seen')
    expect(seen?.init?.method).toBe('POST')
    expect((seen?.init?.headers as Record<string, string>)['X-CSRF-Token']).toBe('token')
    expect(JSON.parse(String(seen?.init?.body))).toEqual({ ids: [1] })
    // The answer said one more was waiting: it is on show now.
    left = '0'
    await settle()
    expect(current().rewards.map((r) => r.id)).toEqual([2])
    await dismissRewards()
    await settle()
    expect(current().rewards).toEqual([])
  })

  it('does not fetch again while marking seen, so nothing is shown twice', async () => {
    let release = () => {}
    let lists = 0
    server = (url) => {
      if (url === '/api/v1/me') return answer('1', { user: { id: 'p1' }, csrfToken: 't' })
      if (url === '/api/v1/me/rewards/seen') return answer('0', { marked: 1 })
      lists += 1
      return answer('1', { rewards: [reward(7)] })
    }
    noticeRewards(answer('1'))
    await settle()
    expect(lists).toBe(1)
    // The marking waits on the server; an older answer still counting the reward arrives.
    const slow = server
    server = (url, init) => {
      if (url === '/api/v1/me/rewards/seen')
        return new Promise<Response>(
          (resolve) => (release = () => resolve(slow(url, init))),
        ) as never
      return slow(url, init)
    }
    const done = dismissRewards()
    await settle()
    noticeRewards(answer('1'))
    await settle()
    expect(lists).toBe(1)
    expect(current().rewards).toEqual([])
    release()
    await done
    await settle()
    expect(lists).toBe(1)
  })

  it('shows nothing when the fetch fails, and tries again on a later answer', async () => {
    let fail = true
    server = (url) =>
      url === '/api/v1/me/rewards' && fail
        ? answer(null, {}, 500)
        : answer('1', { rewards: [reward(3)], user: { id: 'p1' } })
    noticeRewards(answer('1'))
    await settle()
    expect(current().rewards).toEqual([])
    fail = false
    noticeRewards(answer('1'))
    await settle()
    expect(current().rewards.map((r) => r.id)).toEqual([3])
  })
})
