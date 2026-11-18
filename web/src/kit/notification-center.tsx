import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { ReactNode, RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowBendUpLeftIcon,
  ArrowFatUpIcon,
  BellSimpleIcon,
  ChatCircleIcon,
  GearSixIcon,
  ShieldWarningIcon,
  UploadSimpleIcon,
  XIcon,
} from '@phosphor-icons/react'
import { api, jsonRequest } from './api/client'
import { SITE_API_ROOT } from './api/config'
import { withQuery } from './api/endpoints'
import { errorCodeMessage } from './api/error-message'
import type { AppNotification, NotificationPage, NotificationSetting } from './api/types'
import { Button } from './components/ui/button'
import { Checkbox } from './components/ui/checkbox'
import { Skeleton } from './components/ui/skeleton'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './components/ui/dialog'
import { SourceTag } from './components/source-tag'
import { UserAvatar } from './components/user-avatar'
import { RelativeTime, commentPath, danCommentPath } from './comments'
import { danTitle } from './dan-title'
import { cn } from './lib/utils'
import { Notice } from './notifications'
import { useNotification } from './notification-context'
import { useSession } from './session'
import { visit } from './navigation'
import { sources } from './source'
import type { SourceId } from './source'

// The site's own notices: one chat among the player's chats, 系统通知 (./chat-center). Each
// source keeps its own notices, so the chat and its count ask both and put the answers
// together; a notice remembers which source it came from, and everything done to it goes back
// to that source.
export type Sourced = AppNotification & { source: SourceId }
const ids = sources.map((source) => source.id)
const mine = (source: SourceId) => `${SITE_API_ROOT}/sources/${source}/me`
const one = (n: Sourced) => `${mine(n.source)}/notifications/${encodeURIComponent(n.id)}`
const addresses = {
  list: (source: SourceId, query: { filter: 'all' | 'unread'; page: number }) =>
    withQuery(`${mine(source)}/notifications`, query),
  unread: (source: SourceId) => `${mine(source)}/notifications/unread`,
  read: (source: SourceId) => `${mine(source)}/notifications/read`,
  one,
  respond: (n: Sourced) => `${one(n)}/respond`,
  settings: (source: SourceId) => `${mine(source)}/notification-settings`,
}
const when = (n: AppNotification) => Date.parse(n.createdAt)
const newestFirst = (a: AppNotification, b: AppNotification) => when(b) - when(a)

// Any change to read state anywhere refreshes the bell's count.
const changedEvent = 'ourtaiko:notifications-changed'
export const announceChange = () => window.dispatchEvent(new Event(changedEvent))

/** Goes to a page of a source, or of none (''). */
export const useGo = () => (area: SourceId | '', path: string) =>
  visit(`${area && `/${area}`}${path}`)

export function useNotificationText() {
  const { t, i18n } = useTranslation()
  return (n: AppNotification) => {
    const actor = n.actor?.nickname || t('messages.unknownMember')
    // Comment notices name the song or the course the thread is on.
    const chart = n.chart?.title ?? (n.dan ? danTitle(n.dan.title, i18n.resolvedLanguage) : '')
    const count = n.data.upvotes ?? 0
    switch (n.kind) {
      case 'chart_collaboration':
        return t('collaboration.notice', { actor, chart })
      case 'comment_reply':
        return t('notificationCenter.text.comment_reply', { actor, chart })
      case 'chart_comment':
        return t('notificationCenter.text.chart_comment', { actor, chart })
      case 'chart_upvotes':
        return t('notificationCenter.text.chart_upvotes', { chart, count })
      case 'dan_comment':
        return t('notificationCenter.text.dan_comment', { actor, chart })
      case 'dan_upvotes':
        return t('notificationCenter.text.dan_upvotes', { chart, count })
      case 'comment_upvotes':
        return t('notificationCenter.text.comment_upvotes', { chart, count })
      case 'comment_removed':
        return t('notificationCenter.text.comment_removed', { chart })
      case 'playlist_removed':
        return t('notificationCenter.text.playlist_removed', { name: n.data.name ?? '' })
      case 'upload_result': {
        const title = n.data.title || chart
        if (n.data.status === 'failed')
          return t('notificationCenter.text.upload_failed', {
            title,
            reason: errorCodeMessage(n.data.code ?? ''),
          })
        const updated = n.data.status === 'updated'
        const skipped = (n.data.failedFiles ?? []).map(({ file, code }) =>
          t('notificationCenter.text.upload_skipped_file', {
            file:
              file === 'cover'
                ? t('notificationCenter.file.cover')
                : file === 'movie'
                  ? t('notificationCenter.file.movie')
                  : file,
            reason: errorCodeMessage(code),
          }),
        )
        if (!skipped.length)
          return t(`notificationCenter.text.${updated ? 'upload_updated' : 'upload_published'}`, {
            title,
          })
        return t(
          `notificationCenter.text.${updated ? 'upload_updated_partial' : 'upload_published_partial'}`,
          { title, files: new Intl.ListFormat(i18n.resolvedLanguage).format(skipped) },
        )
      }
      default:
        return t('notificationCenter.text.unknown')
    }
  }
}

/** The page a notice is about, inside its source; null when it has none. */
export function target(n: AppNotification) {
  if (n.kind === 'chart_collaboration' && n.data.status === 'pending') return null
  if (n.chart && n.comment) return commentPath(n.chart.id, n.comment.id)
  if (n.chart) return `/charts/${encodeURIComponent(n.chart.id)}`
  if (n.dan && n.comment) return danCommentPath(n.dan.id, n.comment.id)
  if (n.dan) return `/dans/${encodeURIComponent(n.dan.id)}`
  return null
}

const kindIcons: Record<string, ReactNode> = {
  comment_reply: <ArrowBendUpLeftIcon />,
  chart_comment: <ChatCircleIcon />,
  chart_upvotes: <ArrowFatUpIcon weight="fill" className="text-don" />,
  dan_comment: <ChatCircleIcon />,
  dan_upvotes: <ArrowFatUpIcon weight="fill" className="text-don" />,
  comment_upvotes: <ArrowFatUpIcon weight="fill" className="text-don" />,
  comment_removed: <ShieldWarningIcon className="text-destructive" />,
  upload_result: <UploadSimpleIcon />,
  playlist_removed: <ShieldWarningIcon className="text-destructive" />,
}

export function NotificationIcon({ n }: { n: AppNotification }) {
  if (n.actor)
    return (
      <span className="relative shrink-0">
        <UserAvatar
          nickname={n.actor.nickname || '?'}
          avatarPath={n.actor.avatarPath}
          className="size-9"
        />
        <span className="absolute -right-1 -bottom-1 flex size-5 items-center justify-center rounded-full bg-background border border-border [&_svg]:size-3">
          {kindIcons[n.kind] ?? <BellSimpleIcon />}
        </span>
      </span>
    )
  return (
    <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-muted [&_svg]:size-4">
      {kindIcons[n.kind] ?? <BellSimpleIcon />}
    </span>
  )
}

export function NotificationContent({ n }: { n: Sourced }) {
  const text = useNotificationText()
  return (
    <span className="min-w-0 flex-1 space-y-1">
      <span className={cn('block text-sm leading-snug', !n.read && 'font-medium')}>{text(n)}</span>
      {n.comment?.excerpt && (
        <span className="line-clamp-2 block text-sm text-muted-foreground wrap-anywhere">
          {n.comment.excerpt}
        </span>
      )}
      <span className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <SourceTag source={n.source} />
        <RelativeTime value={n.createdAt} />
      </span>
    </span>
  )
}

/** Marks everything read, in each source. */
export const markAllRead = (csrf: string) =>
  Promise.all(ids.map((source) => api(addresses.read(source), jsonRequest('POST', {}, csrf))))

type Loaded = { items: Sourced[]; total: number; unread: number; more: boolean }
/** A source's notices, its first `depth` pages. */
async function loadSource(
  source: SourceId,
  filter: 'all' | 'unread',
  depth: number,
  signal?: AbortSignal,
): Promise<Loaded> {
  const page = (number: number) =>
    api<NotificationPage>(addresses.list(source, { filter, page: number }), { signal })
  const first = await page(1)
  const pages = Math.min(depth, Math.max(1, Math.ceil(first.total / first.pageSize)))
  const rest = await Promise.all(Array.from({ length: pages - 1 }, (_, i) => page(i + 2)))
  return {
    items: [first, ...rest].flatMap((p) => p.items).map((n) => ({ ...n, source })),
    total: first.total,
    unread: first.unread,
    more: pages * first.pageSize < first.total,
  }
}

/**
 * Both sources' notices as one list, newest first. A source with more to load has shown
 * everything down to its oldest loaded notice, so the other's older ones wait until it has
 * loaded further: the list never gets a notice inserted above ones already read.
 */
function merge(loaded: Loaded[]) {
  const limits = loaded.filter((l) => l.more && l.items.length).map((l) => when(l.items.at(-1)!))
  const limit = limits.length ? Math.max(...limits) : -Infinity
  return loaded
    .flatMap((l) => l.items)
    .filter((n) => when(n) >= limit)
    .sort(newestFirst)
}

const settingLabels = {
  comment_reply: 'notificationCenter.settings.comment_reply',
  chart_comment: 'notificationCenter.settings.chart_comment',
  chart_upvotes: 'notificationCenter.settings.chart_upvotes',
  dan_comment: 'notificationCenter.settings.dan_comment',
  dan_upvotes: 'notificationCenter.settings.dan_upvotes',
  comment_upvotes: 'notificationCenter.settings.comment_upvotes',
  comment_removed: 'notificationCenter.settings.comment_removed',
} as const

// What to be told about. The choice is one for the whole site: it is written to both sources,
// and a kind counts as on only while both have it on.
function NotificationSettings() {
  const { t } = useTranslation()
  const { csrfToken } = useSession()
  const { notify } = useNotification()
  const [items, setItems] = useState<NotificationSetting[] | null>(null)
  const [error, setError] = useState('')
  const together = (answers: { items: NotificationSetting[] }[]) =>
    answers[0].items.map((setting) => ({
      kind: setting.kind,
      enabled: answers.every(
        (answer) => answer.items.find((other) => other.kind === setting.kind)?.enabled ?? true,
      ),
    }))
  useEffect(() => {
    const controller = new AbortController()
    Promise.all(
      ids.map((source) =>
        api<{ items: NotificationSetting[] }>(addresses.settings(source), {
          signal: controller.signal,
        }),
      ),
    )
      .then((answers) => setItems(together(answers)))
      .catch((e) => {
        if (!controller.signal.aborted) setError(e.message)
      })
    return () => controller.abort()
  }, [])
  const toggle = async (kind: string, enabled: boolean) => {
    const previous = items
    setItems((current) => current?.map((s) => (s.kind === kind ? { ...s, enabled } : s)) ?? null)
    try {
      const answers = await Promise.all(
        ids.map((source) =>
          api<{ items: NotificationSetting[] }>(
            addresses.settings(source),
            jsonRequest('PUT', { [kind]: enabled }, csrfToken),
          ),
        ),
      )
      setItems(together(answers))
    } catch (e) {
      setItems(previous)
      notify((e as Error).message, 'error')
    }
  }
  return (
    <div>
      {error ? (
        <Notice>{error}</Notice>
      ) : !items ? (
        <Skeleton className="h-40 rounded-xl" />
      ) : (
        <ul className="space-y-3">
          {items.map((s) => (
            <li key={s.kind}>
              <label className="flex cursor-pointer items-center gap-3 text-sm">
                <Checkbox
                  checked={s.enabled}
                  onCheckedChange={(checked) => void toggle(s.kind, checked === true)}
                />
                {s.kind in settingLabels
                  ? t(settingLabels[s.kind as keyof typeof settingLabels])
                  : s.kind}
              </label>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// The count last seen, so the next area's bell starts from it and not from nothing.
let lastCount = 0
const countListeners = new Set<(count: number) => void>()
let countTimer = 0
let countUsers = 0

/** Asks both sources how many notices are unread; a source that does not answer counts none. */
function refreshNoticeCount() {
  void Promise.all(
    ids.map((source) =>
      api<{ count: number }>(addresses.unread(source))
        .then((r) => r.count)
        .catch(() => 0),
    ),
  ).then((counts) => {
    lastCount = counts.reduce((a, b) => a + b, 0)
    for (const listener of countListeners) listener(lastCount)
  })
}

/**
 * How many of the site's notices are unread, over both sources: read again every minute, when
 * the window comes back, and whenever read state changes anywhere on the page.
 */
export function useNoticeCount() {
  const { user } = useSession()
  const [count, setCount] = useState(lastCount)
  useEffect(() => {
    if (!user) {
      lastCount = 0
      setCount(0)
      return
    }
    countListeners.add(setCount)
    if (countUsers++ === 0) {
      refreshNoticeCount()
      countTimer = window.setInterval(refreshNoticeCount, 60000)
      window.addEventListener('focus', refreshNoticeCount)
      window.addEventListener(changedEvent, refreshNoticeCount)
    }
    return () => {
      countListeners.delete(setCount)
      if (--countUsers === 0) {
        window.clearInterval(countTimer)
        window.removeEventListener('focus', refreshNoticeCount)
        window.removeEventListener(changedEvent, refreshNoticeCount)
      }
    }
  }, [user])
  return user ? count : 0
}

/** The latest notice of either source; null when there is none, undefined when unknown. */
export async function latestNotice(): Promise<Sourced | null | undefined> {
  const answers = await Promise.all(
    ids.map((source) => loadSource(source, 'all', 1).catch(() => null)),
  )
  const loaded = answers.flatMap((answer) => answer ?? [])
  if (!loaded.length) return undefined
  return loaded.flatMap((l) => l.items).sort(newestFirst)[0] ?? null
}

/** A notice in a line, for the list of chats. */
export function NoticeLine({ n }: { n: AppNotification }) {
  const text = useNotificationText()
  return <>{text(n)}</>
}

/**
 * Keeps a list that grows at both ends where the reader left it: older items put above keep
 * the view still, and newer ones put below are followed while the reader is at the bottom.
 * Call the returned function on scroll.
 */
export function useChatScroll(
  ref: RefObject<HTMLElement | null>,
  first: string | number | undefined,
  count: number,
) {
  const fromBottom = useRef(0)
  const lastFirst = useRef(first)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    if (!count) {
      fromBottom.current = 0
      lastFirst.current = undefined
      return
    }
    const above = lastFirst.current !== undefined && first !== lastFirst.current
    lastFirst.current = first
    if (above || fromBottom.current < 48)
      el.scrollTop = el.scrollHeight - el.clientHeight - (above ? fromBottom.current : 0)
  }, [ref, first, count])
  return () => {
    const el = ref.current
    if (el) fromBottom.current = el.scrollHeight - el.clientHeight - el.scrollTop
  }
}

/**
 * The site's own notices, shown as a chat: oldest at the top, the newest at the bottom.
 * Opening it reads them all, as opening a chat does; those that were new are marked so while
 * it stays open. Each source keeps its own notices; this asks both and puts them together.
 */
export function SystemChat({ header }: { header: ReactNode }) {
  const { t } = useTranslation()
  const { csrfToken } = useSession()
  const { notify } = useNotification()
  const go = useGo()
  const count = useNoticeCount()
  // How many pages of each source are loaded.
  const [depth, setDepth] = useState(1)
  const [data, setData] = useState<Loaded[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [responding, setResponding] = useState<string | null>(null)
  const [settings, setSettings] = useState(false)
  // Notices that were unread when they were first shown here.
  const [fresh, setFresh] = useState<ReadonlySet<string>>(() => new Set())
  const scroller = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const controller = new AbortController()
    setError('')
    setBusy(true)
    // What one source gives is shown even when the other does not answer.
    Promise.allSettled(ids.map((source) => loadSource(source, 'all', depth, controller.signal)))
      .then((answers) => {
        if (controller.signal.aborted) return
        const failed = answers.find((answer) => answer.status === 'rejected')
        setError(failed ? (failed.reason as Error).message : '')
        const loaded = answers.flatMap((answer) =>
          answer.status === 'fulfilled' ? answer.value : [],
        )
        setData(loaded)
        const unread = loaded.flatMap((l) => l.items).filter((n) => !n.read)
        if (unread.length)
          setFresh((seen) => new Set([...seen, ...unread.map((n) => `${n.source}/${n.id}`)]))
        if (loaded.some((l) => l.unread))
          void markAllRead(csrfToken)
            .then(announceChange)
            .catch(() => {})
      })
      .finally(() => !controller.signal.aborted && setBusy(false))
    return () => controller.abort()
  }, [depth, attempt, csrfToken])
  // New notices arrive by the count: read them as they come.
  const seenCount = useRef(count)
  useEffect(() => {
    if (count > seenCount.current) setAttempt((n) => n + 1)
    seenCount.current = count
  }, [count])
  const items = data ? merge(data).reverse() : []
  const key = (n: Sourced) => `${n.source}/${n.id}`
  const onScroll = useChatScroll(scroller, items[0] && key(items[0]), items.length)
  const more = !!data?.some((l) => l.more)
  const run = async (request: Promise<unknown>) => {
    try {
      await request
      announceChange()
      setAttempt((n) => n + 1)
    } catch (e) {
      notify((e as Error).message, 'error')
    }
  }
  const respond = async (n: Sourced, accept: boolean) => {
    setResponding(n.id)
    try {
      await run(api(addresses.respond(n), jsonRequest('POST', { accept }, csrfToken)))
    } finally {
      setResponding(null)
    }
  }
  return (
    <>
      <div className="flex items-center gap-2 border-b px-3 py-2">
        {header}
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('notificationCenter.settingsTitle')}
          title={t('notificationCenter.settingsTitle')}
          onClick={() => setSettings(true)}
        >
          <GearSixIcon className="size-5" />
        </Button>
      </div>
      <div
        ref={scroller}
        onScroll={onScroll}
        className="min-h-0 flex-1 space-y-3 overflow-y-auto px-3 py-4 sm:px-5"
        data-testid="inbox"
      >
        {more && (
          <div className="flex justify-center">
            <Button
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() => setDepth((n) => n + 1)}
            >
              {t('notificationCenter.more')}
            </Button>
          </div>
        )}
        {error && (
          <div className="flex flex-col items-start gap-3">
            <Notice>{error}</Notice>
            <Button variant="outline" onClick={() => setAttempt((n) => n + 1)}>
              {t('messages.retry')}
            </Button>
          </div>
        )}
        {!data ? (
          !error && (
            <div className="space-y-3" role="status" aria-label={t('notificationCenter.loading')}>
              <Skeleton className="h-20 w-3/4 rounded-[20px]" />
              <Skeleton className="h-20 w-2/3 rounded-[20px]" />
            </div>
          )
        ) : items.length === 0 ? (
          !error && (
            <p className="py-12 text-center text-sm text-muted-foreground">
              {t('notificationCenter.empty')}
            </p>
          )
        ) : (
          <ul className="space-y-3">
            {items.map((n) => {
              const to = target(n)
              const isNew = fresh.has(key(n))
              return (
                <li key={key(n)} data-source={n.source} className="flex items-end gap-2">
                  <div
                    className={cn(
                      'group/notice relative flex max-w-[min(36rem,92%)] flex-wrap items-start gap-2 rounded-[20px] rounded-bl-md bg-muted p-3',
                      isNew && 'ring-2 ring-link/40',
                    )}
                  >
                    <button
                      type="button"
                      disabled={!to}
                      className="flex min-w-0 flex-1 items-start gap-3 rounded-xl text-left outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default"
                      onClick={() => to && go(n.source, to)}
                    >
                      <NotificationIcon n={n} />
                      <NotificationContent n={n} />
                    </button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      className="shrink-0 opacity-60 hover:opacity-100 focus-visible:opacity-100"
                      disabled={n.kind === 'chart_collaboration' && n.data.status === 'pending'}
                      aria-label={t('notificationCenter.dismiss')}
                      title={t('notificationCenter.dismiss')}
                      onClick={() =>
                        void run(api(addresses.one(n), jsonRequest('DELETE', {}, csrfToken)))
                      }
                    >
                      <XIcon />
                    </Button>
                    {n.kind === 'chart_collaboration' && (
                      <div
                        className="flex w-full flex-wrap items-center gap-2 pl-12 text-sm"
                        role="status"
                      >
                        {n.data.status === 'pending' ? (
                          <>
                            <p className="w-full text-muted-foreground">
                              {t('collaboration.permissions')}
                            </p>
                            <Button
                              size="sm"
                              disabled={responding !== null}
                              onClick={() => void respond(n, true)}
                            >
                              {t('collaboration.accept')}
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              disabled={responding !== null}
                              onClick={() => void respond(n, false)}
                            >
                              {t('collaboration.decline')}
                            </Button>
                          </>
                        ) : (
                          t(
                            n.data.status === 'accepted'
                              ? 'collaboration.accepted'
                              : n.data.status === 'revoked'
                                ? 'collaboration.revoked'
                                : 'collaboration.declined',
                          )
                        )}
                      </div>
                    )}
                    {isNew && (
                      <span className="absolute -top-1 -right-1 size-2.5 rounded-full bg-link">
                        <span className="sr-only">{t('notificationCenter.unread')}</span>
                      </span>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>
      <p className="border-t px-4 py-3 text-center text-xs text-muted-foreground">
        {t('chat.systemReadOnly')}
      </p>
      <Dialog open={settings} onOpenChange={setSettings}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('notificationCenter.settingsTitle')}</DialogTitle>
          </DialogHeader>
          <NotificationSettings />
        </DialogContent>
      </Dialog>
    </>
  )
}
