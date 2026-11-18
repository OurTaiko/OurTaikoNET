// Players writing to each other: the API's addresses and answers, and one live connection per
// page that tells everyone who listens what has been written (the bell, the chat page).
//
// The server answers /chats/updates as soon as something is newer than the cursor given, or
// waits up to 25 seconds for it. Only one such request is open per page, and only while the
// page is shown: a browser opens few connections to one site, and hidden tabs holding them
// would starve the visible one. A hidden page reads its count once a minute instead.
import { useSyncExternalStore } from 'react'
import { api, ApiError } from './api/client'
import { SITE_API_ROOT } from './api/config'

export type ChatPerson = { id: string; nickname: string; avatarPath?: string }
export type ChatMessage = {
  seq: number
  chatId: string
  senderId: string
  body: string
  createdAt: string
}
export type Chat = {
  id: string
  peer: ChatPerson
  lastMessage: ChatMessage | null
  /** The peer's messages after readSeq. */
  unread: number
  readSeq: number
  /** How far the peer has read. */
  peerReadSeq: number
  /** You have blocked the peer. */
  blocked: boolean
  /** Neither has blocked the other. */
  canWrite: boolean
  createdAt: string
}
export type ChatUpdate = { messages: ChatMessage[]; cursor: number; changed: boolean }

const root = `${SITE_API_ROOT}/chats`
export const chatAddresses = {
  list: root,
  open: root,
  unread: `${root}/unread`,
  updates: (after: number) => `${root}/updates?after=${after}`,
  one: (id: string) => `${root}/${encodeURIComponent(id)}`,
  messages: (id: string, before?: number) =>
    `${root}/${encodeURIComponent(id)}/messages${before ? `?before=${before}` : ''}`,
  read: (id: string) => `${root}/${encodeURIComponent(id)}/read`,
  block: (id: string) => `${root}/${encodeURIComponent(id)}/block`,
}
/** The longest message, in characters (as the server counts them). */
export const maxMessage = 2000
/** The id of the site's own notices among the chats; they are kept by the song sources. */
export const systemChat = 'system'
export const chatsPage = '/chats'
export const chatPage = (id: string) => `${chatsPage}/${encodeURIComponent(id)}`

type Listener = (update: ChatUpdate) => void
const listeners = new Set<Listener>()
const countListeners = new Set<() => void>()
let unread = 0
let running = false
let signedIn = false
let controller: AbortController | null = null

const setUnread = (count: number) => {
  if (count === unread) return
  unread = count
  for (const listener of countListeners) listener()
}

/** Reads the count of unread messages again; gives the cursor to wait from. */
export async function refreshChatUnread() {
  const answer = await api<{ count: number; cursor: number }>(chatAddresses.unread)
  setUnread(answer.count)
  return answer.cursor
}

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
        for (const listener of listeners) listener(update)
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
    setUnread(0)
    return
  }
  watchVisibility()
  start()
}

/** Calls listener with whatever is written or read in the signed-in player's chats. */
export function onChatUpdate(listener: Listener) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/** How many messages the signed-in player has not read. */
export function useChatUnread() {
  return useSyncExternalStore(
    (listener) => {
      countListeners.add(listener)
      return () => countListeners.delete(listener)
    },
    () => unread,
  )
}
