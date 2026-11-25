import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { keepPreviousData, queryOptions, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowUUpLeftIcon,
  PaperPlaneTiltIcon,
  SealCheckIcon,
  TagIcon,
  TrashIcon,
} from '@phosphor-icons/react'
import { api, jsonRequest } from './api/client'
import { SITE_API_ROOT } from './api/config'
import { Button } from './components/ui/button'
import { Skeleton } from './components/ui/skeleton'
import { TitleBandArt } from './components/nameplate-art'
import { RelativeTime } from './comments'
import { normalizeLanguage } from './i18n'
import { cn } from './lib/utils'
import { layersOf, type TitleLayers } from './nameplate-layers'
import { visit } from './navigation'
import { Notice } from './notifications'
import { useChatScroll } from './notification-center'
import { queryClient } from './query/client'
import { useSession } from './session'

// 称号中心: what a player is told about titles, shown as a chat among their chats like the
// site's notices (./notification-center), but kept by the site itself and about titles alone: a
// request for review that waits for the title managers, a request of one's own approved or
// returned with the manager's reply, a title of one's own that a manager removed.

/** The id of 称号中心 among the chats. */
export const titleCenter = 'titles'

/** A title as a notice shows it: as it was when the notice was written. */
type NoticeTitle = {
  id: string
  name: string
  nameTranslations?: Record<string, string>
  style?: string
  layers?: TitleLayers
}
type NoticeReview = { id: string; kind: 'publish' | 'change'; request: string; reply: string }
export type TitleNotice = {
  id: number
  kind: string
  data: { title?: NoticeTitle; review?: NoticeReview; userId?: string; userName?: string }
  read: boolean
  createdAt: string
}
type Page = { items: TitleNotice[]; total: number; unread: number; page: number; pageSize: number }

const mine = `${SITE_API_ROOT}/me/title-notices`
const addresses = {
  list: (page: number) => `${mine}?page=${page}`,
  unread: `${mine}/unread`,
  read: `${mine}/read`,
}
const keys = {
  all: ['title-notices'] as const,
  unread: ['title-notices', 'unread'] as const,
  latest: (count: number) => ['title-notices', 'latest', count] as const,
  list: (depth: number) => ['title-notices', 'list', depth] as const,
}
const announceChange = () => void queryClient.invalidateQueries({ queryKey: keys.unread })
const markAllRead = (csrf: string) => api(addresses.read, jsonRequest('POST', {}, csrf))

/**
 * How many notices of 称号中心 are unread: read again every minute while the page is shown, and
 * whenever they are read.
 */
export function useTitleNoticeCount() {
  const { user } = useSession()
  const { data } = useQuery({
    queryKey: keys.unread,
    queryFn: ({ signal }) => api<{ count: number }>(addresses.unread, { signal }),
    enabled: !!user,
    refetchInterval: 60_000,
  })
  return user ? (data?.count ?? 0) : 0
}

/** The latest notice; null when there is none. Read again whenever the unread count changes. */
export const latestTitleNoticeQuery = (count: number) =>
  queryOptions({
    queryKey: keys.latest(count),
    queryFn: async ({ signal }) =>
      (await api<Page>(addresses.list(1), { signal })).items[0] ?? null,
    staleTime: 0,
    retry: false,
    placeholderData: keepPreviousData,
  })

// Opening the chat reads the notices, so each opening asks again and nothing is kept for the
// next. How many pages is part of the question.
const listQuery = (depth: number) =>
  queryOptions({
    queryKey: keys.list(depth),
    queryFn: async ({ signal }) => {
      const pages = await Promise.all(
        Array.from({ length: depth }, (_, n) => api<Page>(addresses.list(n + 1), { signal })),
      )
      const last = pages[pages.length - 1]
      return {
        items: pages.flatMap((page) => page.items),
        unread: pages[0].unread,
        more: last.page * last.pageSize < last.total,
      }
    },
    staleTime: 0,
    gcTime: 0,
    placeholderData: keepPreviousData,
  })

const kinds = ['review_requested', 'review_approved', 'review_returned', 'title_removed'] as const
type Kind = (typeof kinds)[number]
const known = (kind: string): kind is Kind => (kinds as readonly string[]).includes(kind)

/** What a notice says, in a line. A kind this page does not know yet says so. */
function useNoticeText() {
  const { t, i18n } = useTranslation()
  return (n: TitleNotice) => {
    if (!known(n.kind)) return t('titleCenter.unknown')
    const title = n.data.title
    const name =
      title?.nameTranslations?.[normalizeLanguage(i18n.resolvedLanguage)] || title?.name || ''
    const kind = n.data.review?.kind ?? 'publish'
    switch (n.kind) {
      case 'review_requested':
        return t(`titleCenter.requested.${kind}`, {
          user: n.data.userName || t('messages.unknownMember'),
          title: name,
        })
      case 'review_approved':
        return t(`titleCenter.approved.${kind}`, { title: name })
      case 'review_returned':
        return t(`titleCenter.returned.${kind}`, { title: name })
      case 'title_removed':
        return t('titleCenter.removed', { title: name })
    }
  }
}

/** A notice in a line, for the list of chats. */
export function TitleNoticeLine({ n }: { n: TitleNotice }) {
  return <>{useNoticeText()(n)}</>
}

const icons: Record<Kind, ReactNode> = {
  review_requested: <PaperPlaneTiltIcon weight="fill" />,
  review_approved: <SealCheckIcon weight="fill" />,
  review_returned: <ArrowUUpLeftIcon weight="bold" />,
  title_removed: <TrashIcon weight="fill" />,
}

/** The round mark of 称号中心, as an avatar among the chats. */
export function TitleCenterAvatar({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        'flex shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary [&_svg]:size-5',
        className,
      )}
    >
      <TagIcon weight="fill" />
    </span>
  )
}

/** Where a notice leads: the managers' queue, or the player's own titles. */
const target = (n: TitleNotice) =>
  n.kind === 'review_requested'
    ? '/manage/titles'
    : n.kind.startsWith('review_')
      ? '/me/titles'
      : ''

/**
 * 称号中心, shown as a chat: oldest at the top, the newest at the bottom. Opening it reads
 * them all, as opening a chat does; those that were new are marked so while it stays open.
 */
export function TitleCenter({ header }: { header: ReactNode }) {
  const { t, i18n } = useTranslation()
  const { csrfToken } = useSession()
  const text = useNoticeText()
  const count = useTitleNoticeCount()
  const [depth, setDepth] = useState(1)
  const { data, error: failure, isFetching: busy, refetch } = useQuery(listQuery(depth))
  const error = busy || data ? '' : (failure?.message ?? '')
  // Notices that were unread when they were first shown here.
  const [fresh, setFresh] = useState<ReadonlySet<number>>(() => new Set())
  const scroller = useRef<HTMLDivElement>(null)
  // What has come is on screen, and so has been read.
  useEffect(() => {
    if (!data) return
    const unread = data.items.filter((n) => !n.read)
    if (unread.length) setFresh((seen) => new Set([...seen, ...unread.map((n) => n.id)]))
    if (data.unread)
      void markAllRead(csrfToken)
        .then(announceChange)
        .catch(() => {})
  }, [data, csrfToken])
  // New notices arrive by the count: read them as they come.
  const seenCount = useRef(count)
  useEffect(() => {
    if (count > seenCount.current) void refetch()
    seenCount.current = count
  }, [count, refetch])
  const items = data ? [...data.items].reverse() : []
  const onScroll = useChatScroll(scroller, items[0]?.id, items.length)
  const language = normalizeLanguage(i18n.resolvedLanguage)
  return (
    <>
      <div className="flex items-center gap-2 border-b px-3 py-2">{header}</div>
      <div
        ref={scroller}
        onScroll={onScroll}
        className="min-h-0 flex-1 space-y-3 overflow-y-auto px-3 py-4 sm:px-5"
        data-testid="title-center"
      >
        {data?.more && (
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
            <Button variant="outline" onClick={() => void refetch()}>
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
          <p className="py-12 text-center text-sm text-muted-foreground">
            {t('titleCenter.empty')}
          </p>
        ) : (
          <ul className="space-y-3">
            {items.map((n) => {
              const isNew = fresh.has(n.id)
              const to = target(n)
              const title = n.data.title
              const review = n.data.review
              return (
                <li key={n.id} className="flex items-end gap-2">
                  <div
                    className={cn(
                      'relative flex max-w-[min(36rem,92%)] flex-col gap-2.5 rounded-[20px] rounded-bl-md bg-muted p-3',
                      isNew && 'ring-2 ring-link/40',
                    )}
                  >
                    <div className="flex items-start gap-3">
                      <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-card text-primary [&_svg]:size-4.5">
                        {known(n.kind) ? icons[n.kind] : <TagIcon />}
                      </span>
                      <div className="min-w-0 space-y-0.5">
                        <p className="text-sm">{text(n)}</p>
                        <p className="text-xs text-muted-foreground">
                          <RelativeTime value={n.createdAt} />
                        </p>
                      </div>
                    </div>
                    {title && (
                      <div className="flex justify-center rounded-xl bg-card/70 px-3 py-4">
                        <TitleBandArt
                          layers={layersOf(title)}
                          name={title.nameTranslations?.[language] || title.name}
                          className="h-6"
                        />
                      </div>
                    )}
                    {n.kind === 'review_requested' && review?.request && (
                      <p className="whitespace-pre-line rounded-xl bg-card/70 px-3 py-2 text-sm">
                        {review.request}
                      </p>
                    )}
                    {n.kind !== 'review_requested' && review?.reply && (
                      <p className="whitespace-pre-line rounded-xl bg-card/70 px-3 py-2 text-sm">
                        <span className="text-xs font-semibold text-muted-foreground">
                          {t('titleCenter.reply')}{' '}
                        </span>
                        {review.reply}
                      </p>
                    )}
                    {to && (
                      <Button
                        size="sm"
                        variant="outline"
                        className="self-start"
                        onClick={() => visit(to)}
                      >
                        {t(
                          n.kind === 'review_requested'
                            ? 'titleCenter.toReviews'
                            : 'titleCenter.toMine',
                        )}
                      </Button>
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
        {t('titleCenter.readOnly')}
      </p>
    </>
  )
}
