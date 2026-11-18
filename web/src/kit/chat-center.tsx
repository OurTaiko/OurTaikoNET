// Messages: players writing to each other, and the site's own notices as one chat among them,
// 系统通知 (./notification-center). The button in the top bar counts what is unread of both and
// lists the latest; /chats is every chat, and /chats/<id> one of them (/chats/system the
// notices).
import { useEffect, useRef, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  ArrowLeftIcon,
  BellSimpleIcon,
  ChatsCircleIcon,
  DotsThreeVerticalIcon,
  PaperPlaneRightIcon,
  ProhibitIcon,
  UserIcon,
} from '@phosphor-icons/react'
import { SITE_API_ROOT } from './api/config'
import { chatPage, chatsPage, maxMessage, setChatSession, systemChat, useChatUnread } from './chat'
import { Button, buttonVariants } from './components/ui/button'
import { Skeleton } from './components/ui/skeleton'
import { Textarea } from './components/ui/textarea'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './components/ui/dropdown-menu'
import { UserAvatar } from './components/user-avatar'
import { RelativeTime } from './comments'
import { formatLocale } from './i18n'
import { cn } from './lib/utils'
import { visit } from './navigation'
import { Notice } from './notifications'
import { useNotification } from './notification-context'
import {
  NoticeLine,
  SystemChat,
  latestNoticeQuery,
  useChatScroll,
  useNoticeCount,
} from './notification-center'
import {
  chatQueries,
  markChatRead,
  openChat,
  readOlderChatMessages,
  sendChatMessage,
  setChatBlocked,
} from './queries/chats'
import type { Chat } from './queries/chats'
import { useSession } from './session'
import { useHideBuddy } from './shell/buddy'

const count99 = (n: number) => (n > 99 ? '99+' : String(n))

function Badge({ count }: { count: number }) {
  return count > 0 ? (
    <span className="min-w-5 shrink-0 rounded-full bg-primary px-1.5 text-center text-[11px] leading-5 font-semibold text-primary-foreground tabular-nums">
      {count99(count)}
    </span>
  ) : null
}

function SystemAvatar({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        'flex shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary [&_svg]:size-5',
        className,
      )}
    >
      <BellSimpleIcon weight="fill" />
    </span>
  )
}

/** What a chat's last message reads like in a list. */
function useLastLine() {
  const { t } = useTranslation()
  const { user } = useSession()
  return (chat: Chat) =>
    chat.lastMessage
      ? chat.lastMessage.senderId === user?.id
        ? t('chat.youSaid', { text: chat.lastMessage.body })
        : chat.lastMessage.body
      : ''
}

// The button in the top bar: how many messages and notices are unread, and a menu of the
// latest chats with the notices first.
export function ChatBell() {
  const { t } = useTranslation()
  const { user } = useSession()
  const chats = useChatUnread()
  const notices = useNoticeCount()
  const lastLine = useLastLine()
  useEffect(() => setChatSession(user?.id ?? null), [user?.id])
  // The menu reads what it lists each time it is opened.
  const [open, setOpen] = useState(false)
  const recentChats = useQuery({
    ...chatQueries.list,
    select: (answer) => answer.items.slice(0, 5),
    enabled: open,
  })
  const recentNotice = useQuery({ ...latestNoticeQuery(notices), enabled: open })
  if (!user) return null
  const count = chats + notices
  return (
    <DropdownMenu onOpenChange={(isOpen) => setOpen(isOpen)}>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            className="relative"
            data-testid="notification-bell"
            aria-label={count ? t('chat.buttonUnread', { count }) : t('chat.title')}
          />
        }
      >
        <ChatsCircleIcon className="size-5" aria-hidden="true" />
        {count > 0 && (
          <span className="absolute -top-0.5 -right-0.5 min-w-4 rounded-full bg-primary px-1 text-center text-[10px] leading-4 font-semibold text-primary-foreground tabular-nums">
            {count99(count)}
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[min(22rem,calc(100vw-2rem))] p-1">
        <div className="flex items-center justify-between px-2 py-1.5">
          <span className="text-sm font-semibold">{t('chat.title')}</span>
          {count > 0 && (
            <span className="text-xs text-muted-foreground">
              {t('notificationCenter.unreadCount', { count })}
            </span>
          )}
        </div>
        <DropdownMenuSeparator />
        {recentChats.isPending || recentNotice.isPending ? (
          <div className="space-y-2 p-2" role="status" aria-label={t('chat.loading')}>
            <Skeleton className="h-12 rounded-xl" />
            <Skeleton className="h-12 rounded-xl" />
          </div>
        ) : (
          <>
            <DropdownMenuItem
              className="items-center gap-3 rounded-xl p-2"
              onClick={() => visit(chatPage(systemChat))}
            >
              <SystemAvatar className="size-9" />
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium">{t('chat.system')}</span>
                <span className="block truncate text-xs text-muted-foreground">
                  {recentNotice.data ? (
                    <NoticeLine n={recentNotice.data} />
                  ) : (
                    t('notificationCenter.empty')
                  )}
                </span>
              </span>
              <Badge count={notices} />
            </DropdownMenuItem>
            {recentChats.data?.map((chat) => (
              <DropdownMenuItem
                key={chat.id}
                className="items-center gap-3 rounded-xl p-2"
                onClick={() => visit(chatPage(chat.id))}
              >
                <UserAvatar
                  nickname={chat.peer.nickname || '?'}
                  avatarPath={chat.peer.avatarPath}
                  className="size-9"
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">
                    {chat.peer.nickname || t('messages.unknownMember')}
                  </span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {lastLine(chat)}
                  </span>
                </span>
                <Badge count={chat.unread} />
              </DropdownMenuItem>
            ))}
          </>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => visit(chatsPage)}>
          <ChatsCircleIcon />
          {t('chat.viewAll')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** On a player's page: opens the chat with them. */
export function MessageButton({ userId }: { userId: string }) {
  const { t } = useTranslation()
  const { user, csrfToken } = useSession()
  const { notify } = useNotification()
  const navigate = useNavigate()
  const [busy, setBusy] = useState(false)
  if (!user || user.id === userId) return null
  return (
    <Button
      variant="outline"
      disabled={busy}
      onClick={() => {
        setBusy(true)
        openChat(userId, csrfToken)
          .then((chat) => navigate(chatPage(chat.id)))
          .catch((e) => notify((e as Error).message, 'error'))
          .finally(() => setBusy(false))
      }}
    >
      <ChatsCircleIcon />
      {t('chat.message')}
    </Button>
  )
}

/** /chats and /chats/:id: the list of chats and, beside it or instead of it on a phone, one. */
export function ChatsPage() {
  const { t } = useTranslation()
  const { id } = useParams()
  const { user, loading } = useSession()
  if (loading) return <p className="text-sm text-muted-foreground">{t('messages.connecting')}</p>
  if (!user)
    return (
      <Notice kind="info" title={t('chat.title')}>
        <a
          href={`${SITE_API_ROOT}/auth/sso/login?returnTo=${encodeURIComponent(id ? chatPage(id) : chatsPage)}`}
          className="text-link hover:underline"
        >
          {t('chat.signIn')}
        </a>
      </Notice>
    )
  return <Chats open={id} />
}

function Chats({ open }: { open?: string }) {
  const { t } = useTranslation()
  // The Don's corner is the send button's.
  useHideBuddy(!!open)
  const notices = useNoticeCount()
  const lastLine = useLastLine()
  const list = useQuery(chatQueries.list)
  const chats = list.data ?? null
  const error = list.error?.message ?? ''
  const { data: notice } = useQuery(latestNoticeQuery(notices))
  const back = (
    <Link
      to={chatsPage}
      className={buttonVariants({ variant: 'ghost', size: 'icon', className: 'lg:hidden' })}
      aria-label={t('chat.back')}
      title={t('chat.back')}
    >
      <ArrowLeftIcon className="size-5" />
    </Link>
  )
  const row = (
    key: string,
    avatar: ReactNode,
    name: ReactNode,
    line: ReactNode,
    when: string | undefined,
    unread: number,
  ) => (
    <li key={key}>
      <Link
        to={chatPage(key)}
        aria-current={open === key ? 'page' : undefined}
        className={cn(
          'flex items-center gap-3 rounded-2xl px-3 py-2.5 outline-none hover:bg-accent focus-visible:ring-3 focus-visible:ring-ring/40',
          open === key && 'bg-secondary text-secondary-foreground hover:bg-secondary',
        )}
      >
        {avatar}
        <span className="min-w-0 flex-1">
          <span className="flex items-baseline gap-2">
            <span className="min-w-0 flex-1 truncate text-sm font-semibold">{name}</span>
            {when && (
              <span className="shrink-0 text-xs text-muted-foreground">
                <RelativeTime value={when} />
              </span>
            )}
          </span>
          <span className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">{line}</span>
            <Badge count={unread} />
          </span>
        </span>
      </Link>
    </li>
  )
  return (
    <section className="flex h-[calc(100svh-11rem)] min-h-[26rem] overflow-hidden rounded-[24px] bg-card shadow-elev-1 lg:h-[calc(100svh-6.5rem)]">
      <aside
        className={cn(
          'flex w-full min-w-0 flex-col lg:w-80 lg:shrink-0 lg:border-r',
          open && 'hidden lg:flex',
        )}
        aria-label={t('chat.list')}
      >
        <h1 className="px-5 pt-5 pb-3 text-2xl font-extrabold tracking-tight">{t('chat.title')}</h1>
        <ul className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
          {row(
            systemChat,
            <SystemAvatar className="size-11" />,
            t('chat.system'),
            notice ? (
              <NoticeLine n={notice} />
            ) : notice === null ? (
              t('notificationCenter.empty')
            ) : (
              ''
            ),
            notice?.createdAt,
            notices,
          )}
          {chats?.map((chat) =>
            row(
              chat.id,
              <UserAvatar
                nickname={chat.peer.nickname || '?'}
                avatarPath={chat.peer.avatarPath}
                className="size-11"
              />,
              <>
                {chat.peer.nickname || t('messages.unknownMember')}
                {chat.blocked && (
                  <ProhibitIcon
                    className="ml-1 inline size-3.5 text-muted-foreground"
                    aria-label={t('chat.blocked')}
                  />
                )}
              </>,
              lastLine(chat),
              chat.lastMessage?.createdAt,
              chat.unread,
            ),
          )}
          {!chats && !error && (
            <li className="space-y-2 p-2" role="status" aria-label={t('chat.loading')}>
              <Skeleton className="h-14 rounded-2xl" />
              <Skeleton className="h-14 rounded-2xl" />
            </li>
          )}
        </ul>
        {error && (
          <div className="p-3">
            <Notice>{error}</Notice>
          </div>
        )}
        {chats?.length === 0 && (
          <p className="px-5 pb-5 text-sm text-muted-foreground">{t('chat.noChats')}</p>
        )}
      </aside>
      <div className={cn('min-w-0 flex-1 flex-col', open ? 'flex' : 'hidden lg:flex')}>
        {open === systemChat ? (
          <SystemChat
            header={
              <>
                {back}
                <SystemAvatar className="size-9" />
                <h2 className="min-w-0 flex-1 truncate font-bold">{t('chat.system')}</h2>
              </>
            }
          />
        ) : open ? (
          <DirectChat key={open} id={open} back={back} />
        ) : (
          <div className="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center text-sm text-muted-foreground">
            <ChatsCircleIcon className="size-12 opacity-40" aria-hidden="true" />
            {t('chat.pick')}
          </div>
        )}
      </div>
    </section>
  )
}

/** One chat with another player. */
function DirectChat({ id, back }: { id: string; back: ReactNode }) {
  const { t, i18n } = useTranslation()
  const { user, csrfToken } = useSession()
  const { notify } = useNotification()
  const navigate = useNavigate()
  const chatRead = useQuery(chatQueries.one(id))
  const messagesRead = useQuery(chatQueries.messages(id))
  // The chat is shown once both have come, and nothing can be sent before that.
  const ready = !!chatRead.data && !!messagesRead.data
  const chat = ready ? chatRead.data! : null
  const messages = ready ? messagesRead.data!.items : null
  const more = ready && messagesRead.data!.more
  // A read that fails behind a chat on show leaves the chat as it is.
  const error =
    (!chatRead.data && chatRead.error?.message) ||
    (!messagesRead.data && messagesRead.error?.message) ||
    ''
  const [older, setOlder] = useState(false)
  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)
  const [visible, setVisible] = useState(document.visibilityState === 'visible')
  const scroller = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLTextAreaElement>(null)
  const me = user?.id ?? ''
  useEffect(() => {
    const watch = () => setVisible(document.visibilityState === 'visible')
    document.addEventListener('visibilitychange', watch)
    return () => document.removeEventListener('visibilitychange', watch)
  }, [])
  // What is shown while the page is in view has been read.
  const last = messages?.at(-1)
  useEffect(() => {
    if (!chat || !last || !visible || last.seq <= chat.readSeq) return
    void markChatRead(id, last.seq, csrfToken).catch(() => {})
  }, [chat, last, visible, id, csrfToken])
  const onScroll = useChatScroll(scroller, messages?.[0]?.seq, messages?.length ?? 0)

  const length = Array.from(draft.trim()).length
  const send = async (event?: FormEvent) => {
    event?.preventDefault()
    if (!length || length > maxMessage || sending) return
    setSending(true)
    try {
      await sendChatMessage(id, draft, csrfToken)
      setDraft('')
    } catch (e) {
      notify((e as Error).message, 'error')
    } finally {
      setSending(false)
      input.current?.focus()
    }
  }
  const loadOlder = () => {
    if (!messages?.length) return
    setOlder(true)
    readOlderChatMessages(id, messages[0].seq)
      .catch((e) => notify((e as Error).message, 'error'))
      .finally(() => setOlder(false))
  }
  const block = (blocked: boolean) =>
    setChatBlocked(id, blocked, csrfToken).catch((e) => notify((e as Error).message, 'error'))

  if (error)
    return (
      <div className="p-4">
        <div className="mb-3 flex items-center gap-2">{back}</div>
        <Notice>{error}</Notice>
      </div>
    )
  const locale = formatLocale(i18n.language)
  const day = new Intl.DateTimeFormat(locale, { dateStyle: 'medium' })
  const time = new Intl.DateTimeFormat(locale, { timeStyle: 'short' })
  const full = new Intl.DateTimeFormat(locale, { dateStyle: 'full', timeStyle: 'short' })
  const lastMine = messages?.filter((m) => m.senderId === me).at(-1)
  const name = chat?.peer.nickname || t('messages.unknownMember')
  return (
    <>
      <div className="flex items-center gap-2 border-b px-3 py-2">
        {back}
        {chat ? (
          <Link
            to={`/players/${encodeURIComponent(chat.peer.id)}`}
            className="flex min-w-0 flex-1 items-center gap-3 rounded-xl outline-none focus-visible:ring-3 focus-visible:ring-ring/40"
          >
            <UserAvatar
              nickname={chat.peer.nickname || '?'}
              avatarPath={chat.peer.avatarPath}
              className="size-9"
            />
            <h2 className="min-w-0 truncate font-bold">{name}</h2>
          </Link>
        ) : (
          <Skeleton className="h-9 flex-1 rounded-xl" />
        )}
        {chat && (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="ghost" size="icon" aria-label={t('chat.more')} />}
            >
              <DotsThreeVerticalIcon className="size-5" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                onClick={() => navigate(`/players/${encodeURIComponent(chat.peer.id)}`)}
              >
                <UserIcon />
                {t('chat.profile')}
              </DropdownMenuItem>
              <DropdownMenuItem
                variant={chat.blocked ? 'default' : 'destructive'}
                onClick={() => void block(!chat.blocked)}
              >
                <ProhibitIcon />
                {chat.blocked ? t('chat.unblock') : t('chat.block')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
      <div
        ref={scroller}
        onScroll={onScroll}
        className="min-h-0 flex-1 overflow-y-auto px-3 py-4 sm:px-5"
        aria-live="polite"
      >
        {more && (
          <div className="mb-3 flex justify-center">
            <Button variant="outline" size="sm" disabled={older} onClick={loadOlder}>
              {t('chat.older')}
            </Button>
          </div>
        )}
        {!messages ? (
          <div className="space-y-3" role="status" aria-label={t('chat.loading')}>
            <Skeleton className="h-10 w-1/2 rounded-[20px]" />
            <Skeleton className="ml-auto h-10 w-1/3 rounded-[20px]" />
          </div>
        ) : messages.length === 0 ? (
          <p className="py-12 text-center text-sm text-muted-foreground">
            {t('chat.empty', { name })}
          </p>
        ) : (
          <ol className="space-y-1">
            {messages.map((m, i) => {
              const mine = m.senderId === me
              const previous = messages[i - 1]
              const next = messages[i + 1]
              const at = new Date(m.createdAt)
              const newDay =
                !previous || day.format(new Date(previous.createdAt)) !== day.format(at)
              // A run is the messages one sender wrote in a row within a few minutes.
              const endsRun =
                !next ||
                next.senderId !== m.senderId ||
                Date.parse(next.createdAt) - at.getTime() > 5 * 60000
              return (
                <li key={m.seq}>
                  {newDay && (
                    <p className="my-4 text-center text-xs text-muted-foreground">
                      {day.format(at)}
                    </p>
                  )}
                  <div className={cn('flex items-end gap-2', mine && 'flex-row-reverse')}>
                    {!mine &&
                      (endsRun && chat ? (
                        <UserAvatar
                          nickname={chat.peer.nickname || '?'}
                          avatarPath={chat.peer.avatarPath}
                          className="size-8"
                        />
                      ) : (
                        <span className="w-8 shrink-0" aria-hidden="true" />
                      ))}
                    <div
                      title={full.format(at)}
                      className={cn(
                        'max-w-[min(32rem,78%)] rounded-[20px] px-3.5 py-2 text-sm leading-relaxed whitespace-pre-wrap wrap-anywhere',
                        mine ? 'bg-primary text-primary-foreground' : 'bg-muted',
                        endsRun && (mine ? 'rounded-br-md' : 'rounded-bl-md'),
                      )}
                    >
                      {m.body}
                    </div>
                  </div>
                  {endsRun && (
                    <p
                      className={cn(
                        'mt-1 mb-2 text-[11px] text-muted-foreground',
                        mine ? 'pr-1 text-right' : 'pl-11',
                      )}
                    >
                      {time.format(at)}
                      {m === lastMine &&
                        chat &&
                        ` · ${chat.peerReadSeq >= m.seq ? t('chat.read') : t('chat.sent')}`}
                    </p>
                  )}
                </li>
              )
            })}
          </ol>
        )}
      </div>
      {chat && !chat.canWrite ? (
        <div className="flex flex-wrap items-center justify-center gap-3 border-t px-4 py-3 text-sm text-muted-foreground">
          {chat.blocked ? t('chat.youBlocked') : t('chat.cannotWrite')}
          {chat.blocked && (
            <Button size="sm" variant="outline" onClick={() => void block(false)}>
              {t('chat.unblock')}
            </Button>
          )}
        </div>
      ) : (
        <form onSubmit={(e) => void send(e)} className="flex items-end gap-2 border-t p-3">
          <div className="min-w-0 flex-1">
            <Textarea
              ref={input}
              value={draft}
              rows={1}
              disabled={!chat}
              aria-label={t('chat.placeholder')}
              placeholder={t('chat.placeholder')}
              aria-invalid={length > maxMessage || undefined}
              className="max-h-40 min-h-11 py-2.5"
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                // Enter sends; Shift+Enter is a new line, and Enter that ends typing in an input
                // method (Chinese, Japanese) only ends it.
                if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                  e.preventDefault()
                  void send()
                }
              }}
            />
            {length > maxMessage - 200 && (
              <p
                className={cn(
                  'mt-1 text-right text-xs tabular-nums text-muted-foreground',
                  length > maxMessage && 'text-destructive',
                )}
              >
                {length} / {maxMessage}
              </p>
            )}
          </div>
          <Button
            type="submit"
            size="icon"
            className="size-11 shrink-0 rounded-full"
            disabled={!chat || !length || length > maxMessage || sending}
            aria-label={t('chat.send')}
            title={t('chat.send')}
          >
            <PaperPlaneRightIcon weight="fill" className="size-5" />
          </Button>
        </form>
      )}
    </>
  )
}
