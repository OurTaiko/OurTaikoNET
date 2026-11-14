import { useCallback, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import {
  ArrowBendUpLeftIcon,
  ArrowFatUpIcon,
  BellIcon,
  BellSimpleIcon,
  ChatCircleIcon,
  CheckIcon,
  EnvelopeSimpleIcon,
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './components/ui/dropdown-menu'
import { SourceTag } from './components/source-tag'
import { UserAvatar } from './components/user-avatar'
import { RelativeTime, commentPath, danCommentPath } from './comments'
import { danTitle } from './dan-title'
import { cn } from './lib/utils'
import { Notice } from './notifications'
import { useNotification } from './notification-context'
import { useSession } from './session-context'
import { visit } from './navigation'
import { sources } from './source'
import type { SourceId } from './source'

// One inbox for the whole site. Each source keeps its own notices, so the bell and the inbox
// ask both and put the answers together; a notice remembers which source it came from, and
// everything done to it goes back to that source.
type Sourced = AppNotification & { source: SourceId }
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
/** The inbox's page; it belongs to no source. */
export const inboxPage = '/notifications'
const when = (n: AppNotification) => Date.parse(n.createdAt)
const newestFirst = (a: AppNotification, b: AppNotification) => when(b) - when(a)

// Any change to read state anywhere refreshes the bell's count.
const changedEvent = 'ourtaiko:notifications-changed'
const announceChange = () => window.dispatchEvent(new Event(changedEvent))

/** Goes to a page of a source, or of none (''). */
const useGo = () => (area: SourceId | '', path: string) => visit(`${area && `/${area}`}${path}`)

function useNotificationText() {
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
function target(n: AppNotification) {
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

function NotificationIcon({ n }: { n: AppNotification }) {
  if (n.actor)
    return (
      <span className="relative shrink-0">
        <UserAvatar
          nickname={n.actor.nickname || '?'}
          avatarUrl={n.actor.avatarUrl}
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

function NotificationContent({ n }: { n: Sourced }) {
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

function UnreadDot({ read }: { read: boolean }) {
  const { t } = useTranslation()
  return read ? (
    <span className="size-2 shrink-0" aria-hidden="true" />
  ) : (
    <span className="mt-1.5 size-2 shrink-0 rounded-full bg-link">
      <span className="sr-only">{t('notificationCenter.unread')}</span>
    </span>
  )
}

const markRead = (n: Sourced, csrf: string) =>
  n.read
    ? Promise.resolve()
    : api(addresses.one(n), jsonRequest('PATCH', { read: true }, csrf)).then(announceChange)

/** Marks everything read, in each source. */
const markAllRead = (csrf: string) =>
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

// The count last seen, so the bell of the next area shown starts from it and not from nothing.
let lastCount = 0

// The bell in the top bar: how many are unread, and a menu of the latest.
export function NotificationBell() {
  const { t } = useTranslation()
  const { user, csrfToken } = useSession()
  const location = useLocation()
  const go = useGo()
  const [count, setCountState] = useState(lastCount)
  const setCount = useCallback((next: number) => {
    lastCount = next
    setCountState(next)
  }, [])
  const [recent, setRecent] = useState<Sourced[] | null>(null)
  const refresh = useCallback(() => {
    if (!user) return
    // A source that does not answer counts for nothing; the other's count still shows.
    void Promise.all(
      ids.map((source) =>
        api<{ count: number }>(addresses.unread(source))
          .then((r) => r.count)
          .catch(() => 0),
      ),
    ).then((counts) => setCount(counts.reduce((a, b) => a + b, 0)))
  }, [user, setCount])
  useEffect(() => {
    if (!user) {
      setCount(0)
      return
    }
    refresh()
    const timer = window.setInterval(refresh, 60000)
    window.addEventListener('focus', refresh)
    window.addEventListener(changedEvent, refresh)
    return () => {
      window.clearInterval(timer)
      window.removeEventListener('focus', refresh)
      window.removeEventListener(changedEvent, refresh)
    }
  }, [user, refresh, setCount, location.pathname])
  if (!user) return null
  const open = (n: Sourced) => {
    void markRead(n, csrfToken).catch(() => {})
    const to = target(n)
    if (to) go(n.source, to)
    else go('', inboxPage)
  }
  return (
    <DropdownMenu
      onOpenChange={(isOpen) => {
        if (!isOpen) return
        setRecent(null)
        void Promise.all(ids.map((source) => loadSource(source, 'all', 1).catch(() => null))).then(
          (answers) => {
            const loaded = answers.flatMap((answer) => answer ?? [])
            setRecent(
              loaded
                .flatMap((l) => l.items)
                .sort(newestFirst)
                .slice(0, 6),
            )
            if (loaded.length === ids.length) setCount(loaded.reduce((a, l) => a + l.unread, 0))
          },
        )
      }}
    >
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            className="relative"
            data-testid="notification-bell"
            aria-label={
              count ? t('notificationCenter.bellUnread', { count }) : t('notificationCenter.title')
            }
          />
        }
      >
        <BellIcon className="size-4" aria-hidden="true" />
        {count > 0 && (
          <span className="absolute -top-0.5 -right-0.5 min-w-4 rounded-full bg-primary px-1 text-center text-[10px] leading-4 font-semibold text-primary-foreground tabular-nums">
            {count > 99 ? '99+' : count}
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[min(22rem,calc(100vw-2rem))] p-1">
        <div className="flex items-center justify-between px-2 py-1.5">
          <span className="text-sm font-semibold">{t('notificationCenter.title')}</span>
          {count > 0 && (
            <span className="text-xs text-muted-foreground">
              {t('notificationCenter.unreadCount', { count })}
            </span>
          )}
        </div>
        <DropdownMenuSeparator />
        {recent === null ? (
          <div className="space-y-2 p-2" role="status" aria-label={t('notificationCenter.loading')}>
            <Skeleton className="h-12 rounded-xl" />
            <Skeleton className="h-12 rounded-xl" />
          </div>
        ) : recent.length === 0 ? (
          <p className="px-2 py-6 text-center text-sm text-muted-foreground">
            {t('notificationCenter.empty')}
          </p>
        ) : (
          recent.map((n) => (
            <DropdownMenuItem
              key={`${n.source}/${n.id}`}
              className="items-start gap-3 rounded-xl p-2"
              onClick={() => open(n)}
            >
              <NotificationIcon n={n} />
              <NotificationContent n={n} />
              <UnreadDot read={n.read} />
            </DropdownMenuItem>
          ))
        )}
        <DropdownMenuSeparator />
        {count > 0 && (
          <DropdownMenuItem
            onClick={() => {
              void markAllRead(csrfToken)
                .then(announceChange)
                .catch(() => {})
            }}
          >
            <CheckIcon />
            {t('notificationCenter.markAllRead')}
          </DropdownMenuItem>
        )}
        <DropdownMenuItem onClick={() => go('', inboxPage)}>
          <BellSimpleIcon />
          {t('notificationCenter.viewAll')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
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
    <section
      aria-labelledby="notification-settings-heading"
      className="space-y-4 rounded-[20px] bg-card p-6 shadow-elev-1"
    >
      <div className="space-y-1">
        <h2 id="notification-settings-heading" className="text-lg font-extrabold">
          {t('notificationCenter.settingsTitle')}
        </h2>
      </div>
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
    </section>
  )
}

export function NotificationsPage() {
  const { t } = useTranslation()
  const { user, csrfToken, loading } = useSession()
  const { notify } = useNotification()
  const go = useGo()
  const [filter, setFilter] = useState<'all' | 'unread'>('all')
  // How many pages of each source are loaded.
  const [depth, setDepth] = useState(1)
  const [data, setData] = useState<Loaded[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [responding, setResponding] = useState<string | null>(null)
  useEffect(() => {
    if (!user) return
    const controller = new AbortController()
    setError('')
    setBusy(true)
    // What one source gives is shown even when the other does not answer.
    Promise.allSettled(ids.map((source) => loadSource(source, filter, depth, controller.signal)))
      .then((answers) => {
        if (controller.signal.aborted) return
        const failed = answers.find((answer) => answer.status === 'rejected')
        setError(failed ? (failed.reason as Error).message : '')
        setData(answers.flatMap((answer) => (answer.status === 'fulfilled' ? answer.value : [])))
      })
      .finally(() => !controller.signal.aborted && setBusy(false))
    return () => controller.abort()
  }, [user, filter, depth, attempt])
  // Refresh when another view (such as the bell menu) changes read state.
  useEffect(() => {
    const reload = () => setAttempt((n) => n + 1)
    window.addEventListener(changedEvent, reload)
    return () => window.removeEventListener(changedEvent, reload)
  }, [])
  if (loading) return <p className="text-sm text-muted-foreground">{t('messages.connecting')}</p>
  if (!user)
    return (
      <Notice kind="info" title={t('notificationCenter.title')}>
        <a
          href={`${SITE_API_ROOT}/auth/sso/login?returnTo=${encodeURIComponent(inboxPage)}`}
          className="text-link hover:underline"
        >
          {t('notificationCenter.signIn')}
        </a>
      </Notice>
    )
  const run = async (request: Promise<unknown>) => {
    try {
      await request
      announceChange()
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
  const items = data ? merge(data) : []
  const unread = data?.reduce((total, l) => total + l.unread, 0) ?? 0
  const more = !!data?.some((l) => l.more)
  return (
    <section className="mx-auto grid max-w-5xl items-start gap-8 lg:grid-cols-[minmax(0,1fr)_18rem]">
      <div className="min-w-0 space-y-6">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div className="space-y-1">
            <h1 className="text-4xl font-extrabold tracking-tight sm:text-5xl">
              <span className="mark">{t('notificationCenter.title')}</span>
            </h1>
            {data && (
              <p className="text-sm text-muted-foreground">
                {t('notificationCenter.unreadCount', { count: unread })}
              </p>
            )}
          </div>
          <Button
            variant="outline"
            disabled={!unread}
            onClick={() => void run(markAllRead(csrfToken))}
          >
            <CheckIcon />
            {t('notificationCenter.markAllRead')}
          </Button>
        </div>
        <div
          role="tablist"
          aria-label={t('notificationCenter.filter')}
          className="inline-flex rounded-full bg-muted p-1"
        >
          {(['all', 'unread'] as const).map((value) => (
            <button
              key={value}
              type="button"
              role="tab"
              aria-selected={filter === value}
              className={cn(
                'rounded-full px-4 py-1.5 text-sm transition-all duration-200 active:scale-[0.96] motion-reduce:transition-none',
                filter === value
                  ? 'bg-card font-bold text-secondary-foreground shadow-elev-1'
                  : 'text-muted-foreground hover:text-foreground',
              )}
              onClick={() => {
                setFilter(value)
                setDepth(1)
                setData(null)
              }}
            >
              {value === 'all' ? t('notificationCenter.all') : t('notificationCenter.unread')}
            </button>
          ))}
        </div>
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
              <Skeleton className="h-20 rounded-[20px]" />
              <Skeleton className="h-20 rounded-[20px]" />
            </div>
          )
        ) : items.length === 0 ? (
          !error && (
            <p className="rounded-[20px] bg-muted py-12 text-center text-sm text-muted-foreground">
              {filter === 'unread'
                ? t('notificationCenter.noUnread')
                : t('notificationCenter.empty')}
            </p>
          )
        ) : (
          <ul
            className="divide-y overflow-hidden rounded-[20px] bg-card shadow-elev-1"
            data-testid="inbox"
          >
            {items.map((n) => {
              const to = target(n)
              return (
                <li
                  key={`${n.source}/${n.id}`}
                  data-source={n.source}
                  className={cn('flex flex-wrap items-start gap-3 p-4', !n.read && 'bg-link/5')}
                >
                  <UnreadDot read={n.read} />
                  <button
                    type="button"
                    disabled={!to}
                    className="flex min-w-0 flex-1 items-start gap-3 rounded-xl text-left outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default"
                    onClick={() => {
                      if (!to) return
                      void markRead(n, csrfToken).catch(() => {})
                      go(n.source, to)
                    }}
                  >
                    <NotificationIcon n={n} />
                    <NotificationContent n={n} />
                  </button>
                  {n.kind === 'chart_collaboration' && (
                    <div
                      className="order-last flex w-full flex-wrap items-center gap-2 pl-5 text-sm"
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
                  <div className="flex shrink-0 gap-1">
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={
                        n.read
                          ? t('notificationCenter.markUnread')
                          : t('notificationCenter.markRead')
                      }
                      title={
                        n.read
                          ? t('notificationCenter.markUnread')
                          : t('notificationCenter.markRead')
                      }
                      onClick={() =>
                        void run(
                          api(addresses.one(n), jsonRequest('PATCH', { read: !n.read }, csrfToken)),
                        )
                      }
                    >
                      {n.read ? <EnvelopeSimpleIcon /> : <CheckIcon />}
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      disabled={n.kind === 'chart_collaboration' && n.data.status === 'pending'}
                      aria-label={t('notificationCenter.dismiss')}
                      title={t('notificationCenter.dismiss')}
                      onClick={() =>
                        void run(api(addresses.one(n), jsonRequest('DELETE', {}, csrfToken)))
                      }
                    >
                      <XIcon />
                    </Button>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
        {more && (
          <div className="flex justify-center">
            <Button variant="outline" disabled={busy} onClick={() => setDepth((n) => n + 1)}>
              {t('notificationCenter.more')}
            </Button>
          </div>
        )}
      </div>
      <aside className="space-y-4">
        <NotificationSettings />
      </aside>
    </section>
  )
}
