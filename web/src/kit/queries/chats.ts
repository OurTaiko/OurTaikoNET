// Players writing to each other: the API's addresses and answers, what the cache holds of them,
// and what changes them. The live connection (../chat) writes what it hears here too.
import { queryOptions } from '@tanstack/react-query'
import { api, jsonRequest } from '../api/client'
import { SITE_API_ROOT } from '../api/config'
import { queryClient } from '../query/client'

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
/** The messages of a chat read so far, oldest first; more says that older ones exist. */
export type ChatMessages = { items: ChatMessage[]; more: boolean }

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

export const chatKeys = {
  unread: () => ['chats', 'unread'] as const,
  list: () => ['chats', 'list'] as const,
  ones: () => ['chats', 'one'] as const,
  one: (id: string) => [...chatKeys.ones(), id] as const,
  messages: (id: string) => ['chats', 'messages', id] as const,
}

/** Puts new messages after the ones there, each once. */
function addMessages(list: ChatMessage[], more: ChatMessage[]) {
  const seen = new Set(list.map((m) => m.seq))
  const added = more.filter((m) => !seen.has(m.seq))
  return added.length ? [...list, ...added].sort((a, b) => a.seq - b.seq) : list
}

/** The newest page of a chat, put together with what is already held of it. */
function withNewest(known: ChatMessages | undefined, page: ChatMessages): ChatMessages {
  const newest = known?.items.at(-1)
  // More was written than a page holds: what lies between is not known, so it starts again here.
  if (!known || !newest || (page.more && page.items[0].seq > newest.seq)) return page
  return { items: addMessages(known.items, page.items), more: known.more }
}

export const chatQueries = {
  // Read again when the live connection says so, not by age.
  unread: queryOptions({
    queryKey: chatKeys.unread(),
    queryFn: ({ signal }) =>
      api<{ count: number; cursor: number }>(chatAddresses.unread, { signal }),
    staleTime: Infinity,
  }),
  // These three are read each time they are opened: a hidden page hears nothing of what is written.
  list: queryOptions({
    queryKey: chatKeys.list(),
    queryFn: ({ signal }) => api<{ items: Chat[] }>(chatAddresses.list, { signal }),
    select: (answer) => answer.items,
    staleTime: 0,
  }),
  one: (id: string) =>
    queryOptions({
      queryKey: chatKeys.one(id),
      queryFn: ({ signal }) => api<Chat>(chatAddresses.one(id), { signal }),
      staleTime: 0,
    }),
  messages: (id: string) =>
    queryOptions({
      queryKey: chatKeys.messages(id),
      queryFn: async ({ signal }) => {
        const page = await api<ChatMessages>(chatAddresses.messages(id), { signal })
        // Taken after the answer: what came by the live connection meanwhile stays, and so do
        // the older pages already read.
        return withNewest(queryClient.getQueryData<ChatMessages>(chatKeys.messages(id)), page)
      },
      staleTime: 0,
    }),
}

/** Reads the count of unread messages again; gives the cursor to wait from. */
export async function refreshChatUnread() {
  return (await queryClient.fetchQuery({ ...chatQueries.unread, staleTime: 0 })).cursor
}

const invalidateChatList = () => queryClient.invalidateQueries({ queryKey: chatKeys.list() })

function addChatMessages(id: string, messages: ChatMessage[]) {
  queryClient.setQueryData<ChatMessages>(
    chatKeys.messages(id),
    (known) => known && { ...known, items: addMessages(known.items, messages) },
  )
}

let listTimer = 0
/** Takes in what the live connection heard: messages as they are, the rest by asking again. */
export function feedChatUpdate(update: ChatUpdate) {
  const written = new Set(update.messages.map((m) => m.chatId))
  for (const id of written)
    addChatMessages(
      id,
      update.messages.filter((m) => m.chatId === id),
    )
  // What is written here arrives by the live connection; a read or a block, as a change.
  if (update.changed) void queryClient.invalidateQueries({ queryKey: chatKeys.ones() })
  else for (const id of written) void queryClient.invalidateQueries({ queryKey: chatKeys.one(id) })
  // The list is read at most once in a short while.
  window.clearTimeout(listTimer)
  listTimer = window.setTimeout(() => void invalidateChatList(), 150)
}

/** Opens the chat with a player, new or not. */
export const openChat = (userId: string, csrf: string) =>
  api<Chat>(chatAddresses.open, jsonRequest('POST', { userId }, csrf))

/** Reads the page of messages before the oldest one held. */
export async function readOlderChatMessages(id: string, before: number) {
  const page = await api<ChatMessages>(chatAddresses.messages(id, before))
  queryClient.setQueryData<ChatMessages>(chatKeys.messages(id), (known) => ({
    items: addMessages(known?.items ?? [], page.items),
    more: page.more,
  }))
}

/** Says how far the player has read a chat. */
export async function markChatRead(id: string, seq: number, csrf: string) {
  queryClient.setQueryData<Chat>(
    chatKeys.one(id),
    (chat) => chat && { ...chat, readSeq: seq, unread: 0 },
  )
  await api(chatAddresses.read(id), jsonRequest('POST', { seq }, csrf))
  void invalidateChatList()
  await refreshChatUnread()
}

export async function sendChatMessage(id: string, body: string, csrf: string) {
  const sent = await api<ChatMessage>(
    chatAddresses.messages(id),
    jsonRequest('POST', { body }, csrf),
  )
  addChatMessages(id, [sent])
  queryClient.setQueryData<Chat>(
    chatKeys.one(id),
    (chat) => chat && { ...chat, lastMessage: sent, readSeq: sent.seq },
  )
  void invalidateChatList()
}

export async function setChatBlocked(id: string, blocked: boolean, csrf: string) {
  const next = await api<Chat>(chatAddresses.block(id), jsonRequest('PUT', { blocked }, csrf))
  queryClient.setQueryData(chatKeys.one(id), next)
  void invalidateChatList()
}
