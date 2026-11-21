import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueries } from '@tanstack/react-query'
import { HeartIcon, MusicNotesIcon, PlaylistIcon } from '@phosphor-icons/react'
import type { ChartBase, Comment, Playlist } from '@sitekit/api/types'
import { CommentBody, RelativeTime, commentPermalink, commentSubjectPath } from '@sitekit/comments'
import { supportsChart } from '@sitekit/courses'
import { danTitle } from '@sitekit/dan-title'
import { Button } from '@sitekit/components/ui/button'
import { VoteCounts } from '@sitekit/components/vote-control'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import type { SourceId } from '@sitekit/source'
import { useChartTitle } from '@sitekit/chart-name'
import type { PlayerTotals } from '@players/api/types'
import { formatLocale } from '@players/i18n'
import { Notice } from '@players/notice'
import { SongTile, SourceTag, sourceApi, sources } from '@players/components/song-tile'
import { playerQueries } from '@players/queries'

// Players upload their own charts to Fanmade only; Official's charts are the official ones.
export const chartSources: SourceId[] = ['fanmade']

/** The numbers under a player's name, over all sources. */
export function PlayerStats({ totals }: { totals: PlayerTotals }) {
  const { t, i18n } = useTranslation()
  const locale = formatLocale(i18n.resolvedLanguage ?? 'zh-Hans')
  const tiles: { key: string; value: number; label: string }[] = [
    { key: 'plays', value: totals.scoreCount, label: t('profile.stats.plays') },
    { key: 'charts', value: totals.chartCount, label: t('profile.stats.charts') },
    { key: 'comments', value: totals.commentCount, label: t('profile.stats.comments') },
    { key: 'karma', value: totals.karma, label: t('interactions.karma') },
  ]
  return (
    <dl className="gap-3 grid grid-cols-2 lg:grid-cols-4" data-testid="player-stats">
      {tiles.map((tile) => (
        <div
          key={tile.key}
          className="flex flex-col gap-1 bg-card shadow-elev-1 p-4 rounded-[20px] min-w-0"
        >
          <dd className="font-extrabold text-3xl tabular-nums tracking-tight">
            {tile.value.toLocaleString(locale)}
          </dd>
          <dt className="text-muted-foreground text-sm">{tile.label}</dt>
        </div>
      ))}
    </dl>
  )
}

/** When the player first came and was last seen; nothing when the site does not know. */
export function Seen({ totals }: { totals: PlayerTotals }) {
  const { t, i18n } = useTranslation()
  const { firstLoginAt: first, lastActiveAt: last } = totals
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
  named,
  children,
}: {
  source: SourceId
  /** The list's address without its page. */
  address: string
  layout: string
  /** Head the part with its source and count: for a list that has more than one source. */
  named: boolean
  children: (item: T) => ReactNode
}) {
  const { t, i18n } = useTranslation()
  // How many pages the visitor has asked for; each is read, and kept, by itself.
  const [asked, setAsked] = useState(1)
  const pages = useQueries({
    queries: Array.from({ length: asked }, (_, index) =>
      playerQueries.works<T>(source, address, index + 1),
    ),
  })
  const read = pages.flatMap((page) => page.data ?? [])
  const last = read.at(-1)
  const list = last && {
    items: read.flatMap((page) => page.items),
    total: last.total,
    page: read.length,
    pageSize: last.pageSize,
  }
  const loading = pages.some((page) => !page.data && page.isFetching)
  const failed = pages.find((page) => !page.data && page.isError)
  const error = loading ? '' : (failed?.error?.message ?? '')
  if (list && list.total === 0) return null
  const more = !!list && list.page * list.pageSize < list.total
  return (
    <section className="space-y-3" data-testid={`works-${source}`}>
      {named && (
        <h3 className="flex items-center gap-2 font-bold text-sm">
          <SourceTag source={source} />
          {list && (
            <span className="text-muted-foreground tabular-nums">
              {list.total.toLocaleString(formatLocale(i18n.resolvedLanguage ?? 'zh-Hans'))}
            </span>
          )}
        </h3>
      )}
      {list ? (
        <div className={layout}>{list.items.map(children)}</div>
      ) : (
        !error && <Skeleton className="rounded-[20px] w-full h-32" />
      )}
      {error && <Notice>{error}</Notice>}
      {more && (
        <div className="flex justify-center pt-1">
          <Button
            variant="outline"
            disabled={loading}
            onClick={() => (failed ? void failed.refetch() : setAsked(asked + 1))}
          >
            {t('profile.more')}
          </Button>
        </div>
      )}
    </section>
  )
}

/** A list of a player's things in the sources (both by default), one part for each that has any. */
function Works<T extends { id: string }>({
  address,
  layout,
  empty,
  of = sources,
  children,
}: {
  address: (source: SourceId) => string
  of?: SourceId[]
  layout: string
  empty: string
  children: (source: SourceId, item: T) => ReactNode
}) {
  // The first page of each part says how many it has; one that could not be read counts as none.
  const firsts = useQueries({
    queries: of.map((source) => playerQueries.works<T>(source, address(source), 1)),
  })
  const none = firsts.every((first) => (first.data ? first.data.total === 0 : first.isError))
  return (
    <div className="space-y-8">
      {of.map((source) => (
        <SourceWorks<T>
          key={source}
          source={source}
          address={address(source)}
          layout={layout}
          named={of.length > 1}
        >
          {(item) => children(source, item)}
        </SourceWorks>
      ))}
      {none && <p className="py-10 text-muted-foreground text-sm text-center">{empty}</p>}
    </div>
  )
}

export function ChartsTab({ id }: { id: string }) {
  const { t } = useTranslation()
  return (
    <Works<ChartBase>
      address={(source) => `${sourceApi(source)}/charts?owner=${encodeURIComponent(id)}`}
      of={chartSources}
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

/** A comment's song, named in the visitor's language once its chart is read. */
function ChartTitle({ source, id }: { source: SourceId; id?: string }) {
  const title = useChartTitle(source, id)
  return title ?? <Skeleton className="inline-block h-3 w-24 align-middle" />
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
              {comment.danId ? (
                danTitle(comment.danTitle ?? {}, i18n.resolvedLanguage)
              ) : (
                <ChartTitle source={source} id={comment.chartId} />
              )}
            </a>
            <span aria-hidden="true">·</span>
            <RelativeTime value={comment.createdAt} />
          </header>
          <CommentBody text={comment.body} />
          <footer className="flex flex-wrap items-center gap-3 text-muted-foreground text-xs">
            <span className="inline-flex gap-2 tabular-nums">
              <VoteCounts upvotes={comment.upvotes} downvotes={comment.downvotes} />
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
