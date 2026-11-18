// Players writing to each other: one live connection per page, which puts what has been written
// into the cache that the bell and the chat page read (./queries/chats).
//
// The server answers /chats/updates as soon as something is newer than the cursor given, or
// waits up to 25 seconds for it. Only one such request is open per page, and only while the
// page is shown: a browser opens few connections to one site, and hidden tabs holding them
// would starve the visible one. A hidden page reads its count once a minute instead.
import { useQuery } from '@tanstack/react-query'
import { api, ApiError } from './api/client'
import { chatAddresses, chatQueries, feedChatUpdate, refreshChatUnread } from './queries/chats'
import type { ChatUpdate } from './queries/chats'
import { useAccount } from './session'

/** The longest message, in characters (as the server counts them). */
export const maxMessage = 2000
/** The id of the site's own notices among the chats; they are kept by the song sources. */
export const systemChat = 'system'
export const chatsPage = '/chats'
export const chatPage = (id: string) => `${chatsPage}/${encodeURIComponent(id)}`

let running = false
let signedIn = false
let controller: AbortController | null = null

const pause = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve) => {
    const timer = window.setTimeout(resolve, ms)
    signal.addEventListener('abort', () => {
      window.clearTimeout(timer)
      resolve()
    })
  })

async function run(signal: AbortSignal) {
  let cursor = -1
  let failures = 0
  while (!signal.aborted) {
    try {
      if (cursor < 0) cursor = await refreshChatUnread()
      const update = await api<ChatUpdate>(chatAddresses.updates(cursor), { signal })
      failures = 0
      cursor = update.cursor
      if (update.messages.length || update.changed) {
        feedChatUpdate(update)
        void refreshChatUnread().catch(() => {})
      }
    } catch (e) {
      if (signal.aborted) break
      // Signed out elsewhere: nothing to wait for.
      if (e instanceof ApiError && e.status === 401) break
      failures++
      cursor = -1
      await pause(Math.min(60000, 2000 * 2 ** Math.min(failures, 5)), signal)
    }
  }
}

function start() {
  if (running || !signedIn || document.visibilityState !== 'visible') return
  running = true
  controller = new AbortController()
  const mine = controller
  void run(mine.signal).finally(() => {
    if (controller === mine) {
      running = false
      controller = null
    }
  })
}
function stop() {
  controller?.abort()
  controller = null
  running = false
}

let watching = false
function watchVisibility() {
  if (watching) return
  watching = true
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') start()
    else stop()
  })
  // A hidden page still learns of new messages, more slowly.
  window.setInterval(() => {
    if (signedIn && document.visibilityState !== 'visible') void refreshChatUnread().catch(() => {})
  }, 60000)
}

/** Whether someone is signed in on this page: the live connection runs only then. */
export function setChatSession(user: string | null) {
  signedIn = !!user
  if (!signedIn) {
    stop()
    return
  }
  watchVisibility()
  start()
}

/** How many messages the signed-in player has not read. */
export function useChatUnread() {
  const signedIn = !!useAccount()
  const { data } = useQuery({
    ...chatQueries.unread,
    enabled: signedIn,
    select: (answer) => answer.count,
  })
  return signedIn ? (data ?? 0) : 0
}
