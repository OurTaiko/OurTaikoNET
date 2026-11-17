// What the player was given and has not seen (the backend's internal/rewards). Every answer of
// the site to a signed-in request says how many there are, in a header; whoever reads an answer
// passes it to noticeRewards, and this module does the rest: it fetches them once, keeps them
// for the dialog (./rewards-dialog) and marks them seen when the player has looked.
//
// It knows nothing of what a reward is (the dialog does) nor of the code whose answers it reads.
import { useSyncExternalStore } from 'react'

export const REWARDS_HEADER = 'X-Pending-Rewards'
const address = '/api/v1/me/rewards'

export type Reward = {
  id: number
  kind: string
  data: Record<string, unknown>
  createdAt: string
}
export type RewardsState = {
  rewards: Reward[]
  /** The player they were given to, for links to their page. */
  player: string
}

const empty: RewardsState = { rewards: [], player: '' }
let state = empty
let loading = false
// While rewards are being marked seen, an answer that left before may still count them: wait
// for the marking's own answer instead.
let marking = false
const listeners = new Set<() => void>()

function set(next: RewardsState) {
  state = next
  for (const listener of listeners) listener()
}

const json = (response: Response) => response.json() as Promise<Record<string, unknown>>

/** Reads an answer's header; with rewards waiting and none on show, fetches them. */
export function noticeRewards(response: Response) {
  if (Number(response.headers.get(REWARDS_HEADER)) > 0) void load()
}

async function load() {
  if (loading || marking || state.rewards.length > 0) return
  loading = true
  try {
    const [list, me] = await Promise.all([
      fetch(address, { credentials: 'include' }),
      fetch('/api/v1/me', { credentials: 'include' }),
    ])
    if (!list.ok) return
    const rewards = ((await json(list)).rewards ?? []) as Reward[]
    const user = (me.ok ? (await json(me)).user : null) as { id?: string } | null
    if (rewards.length > 0) set({ rewards, player: user?.id ?? '' })
  } catch {
    // Told again by the next answer.
  } finally {
    loading = false
  }
}

/** The player has looked: the rewards on show are marked seen and go. */
export async function dismissRewards() {
  const ids = state.rewards.map((reward) => reward.id)
  if (ids.length === 0) return
  marking = true
  set(empty)
  let answer: Response | null = null
  try {
    const me = await fetch('/api/v1/me', { credentials: 'include' })
    const csrf = me.ok ? String((await json(me)).csrfToken ?? '') : ''
    answer = await fetch(`${address}/seen`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
      body: JSON.stringify({ ids }),
    })
  } catch {
    // Not marked: they are shown again on a later answer.
  } finally {
    marking = false
  }
  // More than fit one fetch, or given meanwhile.
  if (answer) noticeRewards(answer)
}

/** What is on show now, outside React. */
export const currentRewards = (): RewardsState => state

export function useRewards(): RewardsState {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    () => state,
  )
}

/** For tests: forget everything. */
export function resetRewards() {
  loading = false
  marking = false
  set(empty)
}
