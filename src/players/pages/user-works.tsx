import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { HeartIcon, MusicNotesIcon, PlaylistIcon } from '@phosphor-icons/react'
import { api } from '@sitekit/api/client'
import type { ChartBase, Comment, Playlist, PublicUser, UserSpace } from '@sitekit/api/types'
import { CommentBody, RelativeTime, commentPermalink, commentSubjectPath } from '@sitekit/comments'
import { supportsChart } from '@sitekit/courses'
import { danTitle } from '@sitekit/dan-title'
import { Button } from '@sitekit/components/ui/button'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import type { SourceId } from '@sitekit/source'
import { formatLocale } from '@players/i18n'
import { Notice } from '@players/notice'
import { SongTile, SourceTag, sourceApi, sources } from '@players/components/song-tile'

type Page<T> = { items: T[]; total: number; page: number; pageSize: number }
export type SourceProfiles = Partial<Record<SourceId, PublicUser | null>>

/**
 * What each source knows of a player: counts of charts, plays, comments and when they were
 * there. Absent while loading; null when that source does not know them or did not answer.
 */
export function useSourceProfiles(id: string): SourceProfiles {
  const [profiles, setProfiles] = useState<{ id: string; of: SourceProfiles }>({ id, of: {} })
  useEffect(() => {
    const controller = new AbortController()
    for (const source of sources) {
      const put = (user: PublicUser | null) =>
        !controller.signal.aborted &&
        setProfiles((current) => ({
          id,
          of: { ...(current.id === id ? current.of : {}), [source]: user },
        }))
      api<UserSpace>(`${sourceApi(source)}/users/${encodeURIComponent(id)}`, {
        signal: controller.signal,
      })
        .then((space) => put(space.user))
        .catch(() => put(null))
    }
    return () => controller.abort()
  }, [id])
  return profiles.id === id ? profiles.of : {}
}

/** The numbers under a player's name: plays in each source, then what they made in both. */
export function SourceStats({ profiles }: { profiles: SourceProfiles }) {
  const { t, i18n } = useTranslation()
  const locale = formatLocale(i18n.resolvedLanguage ?? 'zh-Hans')
  const loading = sources.some((source) => profiles[source] === undefined)
  const sum = (field: 'chartCount' | 'commentCount' | 'karma') =>
    sources.reduce((total, source) => total + (profiles[source]?.[field] ?? 0), 0)
  const tiles: { key: string; source?: SourceId; value: number; label: string }[] = [
    ...sources.map((source) => ({
      key: `plays-${source}`,
      source,
      value: profiles[source]?.scoreCount ?? 0,
      label: t('profile.stats.plays'),
    })),
    { key: 'charts', value: sum('chartCount'), label: t('profile.stats.charts') },
    { key: 'comments', value: sum('commentCount'), label: t('profile.stats.comments') },
    { key: 'karma', value: sum('karma'), label: t('interactions.karma') },
  ]
  return (
    <dl className="gap-3 grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5" data-testid="player-stats">
      {tiles.map((tile) => (
        <div
          key={tile.key}
          className="flex flex-col gap-1 bg-card shadow-elev-1 p-4 rounded-[20px] min-w-0"
        >
          <div className="h-[1.375rem]">
            {tile.source ? (
              <SourceTag source={tile.source} />
            ) : (
              <span className="text-muted-foreground text-xs">{t('profile.stats.site')}</span>
            )}
          </div>
          <dd className="font-extrabold text-3xl tabular-nums tracking-tight">
            {loading ? (
              <Skeleton className="rounded-md w-12 h-9" />
            ) : (
              tile.value.toLocaleString(locale)
            )}
          </dd>
          <dt className="text-muted-foreground text-sm">{tile.label}</dt>
        </div>
      ))}
    </dl>
  )
}

/** When the player first came and was last seen, over both sources; nothing when neither says. */
export function Seen({ profiles }: { profiles: SourceProfiles }) {
  const { t, i18n } = useTranslation()
  const times = (field: 'firstLoginAt' | 'lastActiveAt') =>
    sources.flatMap((source) => profiles[source]?.[field] ?? []).sort()
  const first = times('firstLoginAt')[0]
  const last = times('lastActiveAt').at(-1)
  if (!first || !last) return null
  const date = new Intl.DateTimeFormat(formatLocale(i18n.resolvedLanguage ?? 'zh-Hans'), {
    dateStyle: 'medium',
  })
  return (
    <p className="text-muted-foreground text-sm">
      {t('profile.since', {
        first: date.format(new Date(first)),
        last: date.format(new Date(last)),
      })}
    </p>
  )
}

/**
 * One source's part of a list of a player's things: its first page, and more on request.
 * Renders nothing when the source has none.
 */
function SourceWorks<T extends { id: string }>({
  source,
  address,
  layout,
  onTotal,
  children,
}: {
  source: SourceId
  /** The list's address without its page. */
  address: string
  layout: string
  onTotal: (total: number) => void
  children: (item: T) => ReactNode
}) {
  const { t, i18n } = useTranslation()
  const [list, setList] = useState<Page<T> | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const report = useRef(onTotal)
  report.current = onTotal
  const load = (page: number, signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    api<Page<T>>(`${address}${address.includes('?') ? '&' : '?'}page=${page}`, { signal })
      .then((result) => {
        if (signal?.aborted) return
        setList((current) => ({
          items: page === 1 ? result.items : [...(current?.items ?? []), ...result.items],
          total: result.total,
          page,
          pageSize: result.pageSize,
        }))
        report.current(result.total)
      })
      .catch((e: Error) => {
        if (signal?.aborted) return
        setError(e.message)
        if (page === 1) report.current(0)
      })
      .finally(() => !signal?.aborted && setLoading(false))
  }
  const first = useRef(load)
  first.current = load
  useEffect(() => {
    const controller = new AbortController()
    setList(null)
    first.current(1, controller.signal)
    return () => controller.abort()
  }, [address])
  if (!list && !loading && !error) return null
  if (list && list.total === 0) return null
  const more = !!list && list.page * list.pageSize < list.total
  return (
    <section className="space-y-3" data-testid={`works-${source}`}>
      <h3 className="flex items-center gap-2 font-bold text-sm">
        <SourceTag source={source} />
        {list && (
          <span className="text-muted-foreground tabular-nums">
            {list.total.toLocaleString(formatLocale(i18n.resolvedLanguage ?? 'zh-Hans'))}
          </span>
        )}
      </h3>
      {list ? (
        <div className={layout}>{list.items.map(children)}</div>
      ) : (
        !error && <Skeleton className="rounded-[20px] w-full h-32" />
      )}
      {error && <Notice>{error}</Notice>}
      {more && (
        <div className="flex justify-center pt-1">
          <Button variant="outline" disabled={loading} onClick={() => load(list.page + 1)}>
            {t('profile.more')}
          </Button>
        </div>
      )}
    </section>
  )
}

/** A list of a player's things in both sources, one part for each source that has any. */
function Works<T extends { id: string }>({
  address,
  layout,
  empty,
  children,
}: {
  address: (source: SourceId) => string
  layout: string
  empty: string
  children: (source: SourceId, item: T) => ReactNode
}) {
  const [totals, setTotals] = useState<Partial<Record<SourceId, number>>>({})
  return (
    <div className="space-y-8">
      {sources.map((source) => (
        <SourceWorks<T>
          key={source}
          source={source}
          address={address(source)}
          layout={layout}
          onTotal={(total) =>
            setTotals((current) =>
              current[source] === total ? current : { ...current, [source]: total },
            )
          }
        >
          {(item) => children(source, item)}
        </SourceWorks>
      ))}
      {sources.every((source) => totals[source] === 0) && (
        <p className="py-10 text-muted-foreground text-sm text-center">{empty}</p>
      )}
    </div>
  )
}

export function ChartsTab({ id }: { id: string }) {
  const { t } = useTranslation()
  return (
    <Works<ChartBase>
      address={(source) => `${sourceApi(source)}/charts?owner=${encodeURIComponent(id)}`}
      layout="song-grid"
      empty={t('profile.noCharts')}
    >
      {(source, chart) =>
        supportsChart(chart) && <SongTile key={chart.id} source={source} chart={chart} />
      }
    </Works>
  )
}

export function PlaylistsTab({ id }: { id: string }) {
  const { t, i18n } = useTranslation()
  const locale = formatLocale(i18n.resolvedLanguage ?? 'zh-Hans')
  return (
    <Works<Playlist>
      address={(source) =>
        `${sourceApi(source)}/playlists?owner=${encodeURIComponent(id)}&q=&sort=updated`
      }
      layout="gap-4 grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3"
      empty={t('profile.noPlaylists')}
    >
      {(source, playlist) => (
        <a
          key={playlist.id}
          href={`/${source}/playlists/${encodeURIComponent(playlist.id)}`}
          className="flex flex-col gap-3 bg-card shadow-elev-1 hover:shadow-elev-2 p-5 rounded-[20px] outline-none focus-visible:ring-3 focus-visible:ring-ring min-w-0 hover:-translate-y-0.5 motion-reduce:transform-none transition-[translate,box-shadow] motion-reduce:transition-none duration-200 ease-emphasized"
        >
          <div className="flex items-start gap-3 min-w-0">
            <span
              data-accent={source}
              className="flex justify-center items-center bg-secondary rounded-[14px] size-11 text-secondary-foreground shrink-0"
            >
              <PlaylistIcon className="size-5" aria-hidden="true" />
            </span>
            <div className="space-y-0.5 min-w-0">
              <h4 className="font-bold line-clamp-2 leading-snug">{playlist.name}</h4>
              {playlist.description && (
                <p className="text-muted-foreground text-sm line-clamp-2">{playlist.description}</p>
              )}
            </div>
          </div>
          <p className="flex flex-wrap items-center gap-x-4 gap-y-1 mt-auto text-muted-foreground text-xs tabular-nums">
            <span className="inline-flex items-center gap-1">
              <MusicNotesIcon className="size-3.5" aria-hidden="true" />
              {t('home.songs', { n: playlist.chartCount.toLocaleString(locale) })}
            </span>
            <span className="inline-flex items-center gap-1">
              <HeartIcon className="size-3.5" aria-hidden="true" />
              {t('profile.favorites', { n: playlist.favoriteCount.toLocaleString(locale) })}
            </span>
          </p>
        </a>
      )}
    </Works>
  )
}

export function CommentsTab({ id }: { id: string }) {
  const { t, i18n } = useTranslation()
  return (
    <Works<Comment>
      address={(source) => `${sourceApi(source)}/users/${encodeURIComponent(id)}/comments?sort=new`}
      layout="space-y-3"
      empty={t('profile.noComments')}
    >
      {(source, comment) => (
        <article
          key={comment.id}
          className="space-y-2 bg-card shadow-elev-1 p-4 rounded-[20px] min-w-0"
        >
          <header className="flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground text-xs">
            <a
              href={`/${source}${commentSubjectPath(comment)}`}
              className="max-w-full font-medium text-foreground hover:underline truncate"
            >
              {comment.danId
                ? danTitle(comment.danTitle ?? {}, i18n.resolvedLanguage)
                : comment.chartTitle}
            </a>
            <span aria-hidden="true">·</span>
            <RelativeTime value={comment.createdAt} />
          </header>
          <CommentBody text={comment.body} />
          <footer className="flex flex-wrap items-center gap-3 text-muted-foreground text-xs">
            <span className="tabular-nums">
              {t('interactions.points', { count: comment.score })}
            </span>
            <a
              href={`/${source}${commentPermalink(comment)}`}
              className="text-link hover:underline"
            >
              {t('interactions.viewInContext')}
            </a>
          </footer>
        </article>
      )}
    </Works>
  )
}
