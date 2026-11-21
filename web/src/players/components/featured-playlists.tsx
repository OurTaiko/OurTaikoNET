import { useRef, type CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { HeartIcon, MusicNotesIcon } from '@phosphor-icons/react'
import type { ChartBase, Playlist } from '@sitekit/api/types'
import { chartText } from '@sitekit/chart-language'
import { LevelChips } from '@sitekit/components/difficulty-badges'
import { Badge } from '@sitekit/components/ui/badge'
import { buttonVariants } from '@sitekit/components/ui/button'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { UserAvatar } from '@sitekit/components/user-avatar'
import { supportsChart } from '@sitekit/courses'
import { cn } from '@sitekit/lib/utils'
import type { SourceId } from '@sitekit/source'
import {
  PlaylistCover,
  isSource,
  playlistKey,
  playlistPage,
  usePlaylistsNamed,
} from '@players/featured'
import { playerQueries } from '@players/queries'
import { CarouselBar, useCarousel, useDrift } from './carousel'
import { SourceTag, songPage } from './song-tile'

type Recommended = { source: SourceId; playlist: Playlist; note: string }

// How many of a playlist's songs a narrow slide shows at once. A wide slide has a height of its
// own and gives the list what is left of it, which is no more than this.
const rowsShown = 4

/**
 * Every song of a playlist, in its order, in a few rows: a list longer than that scrolls down
 * by itself while `moving` and begins again at its end (useDrift, as the community's activity
 * does), and the visitor can scroll it themselves. `songs` is absent while they are read.
 */
function Songs({
  source,
  songs,
  rows,
  moving,
}: {
  source: SourceId
  songs: ChartBase[] | undefined
  rows: number
  moving: boolean
}) {
  const { t, i18n } = useTranslation()
  const box = useRef<HTMLDivElement>(null)
  const copy = useRef<HTMLOListElement>(null)
  const loops = !!songs && songs.length > rowsShown
  useDrift(box, copy, loops && moving)
  const row = 'flex items-center gap-3 border-muted border-t h-[2.125rem]'
  const list = (again: boolean) =>
    songs?.map((chart, index) => (
      <li key={chart.id} className={row}>
        <span className="w-6 text-muted-foreground tabular-nums">{index + 1}</span>
        <a
          href={songPage(source, chart.id)}
          tabIndex={again ? -1 : undefined}
          className="flex-1 rounded-sm outline-none focus-visible:ring-3 focus-visible:ring-ring min-w-0 font-semibold hover:underline truncate underline-offset-4"
        >
          {chartText(chart, i18n.resolvedLanguage).title}
        </a>
        <span className="shrink-0">
          <LevelChips difficulties={chart.difficulties} />
        </span>
      </li>
    ))
  return (
    <div
      ref={box}
      style={{ '--rows': rows } as CSSProperties}
      // relative: the box clips what it scrolls. The level chips' screen-reader words are
      // absolute, and an unpositioned box leaves them to the page, which they stretch.
      className={cn(
        'relative @xl:flex-1 -mx-1 mt-1 px-1 h-[calc(var(--rows)*2.125rem)] @xl:h-auto @xl:min-h-0 overflow-y-auto overscroll-contain text-sm [scrollbar-width:none]',
        loops && 'activity-fade',
      )}
    >
      <ol aria-label={t('home.playlistSongs')}>
        {list(false) ??
          Array.from({ length: rows }, (_, index) => (
            <li key={index} className={row}>
              <Skeleton className="w-2/3 h-3.5" />
            </li>
          ))}
      </ol>
      {loops && (
        // The same songs again, only to scroll into: a pointer can use them, a keyboard and a
        // screen reader have the first.
        <ol ref={copy} aria-hidden="true">
          {list(true)}
        </ol>
      )}
    </div>
  )
}

/** The cover on a stack of two sheets: a playlist is several songs. */
function CoverStack({ playlist, large = false }: { playlist: Playlist; large?: boolean }) {
  const corner = large ? 'rounded-[20px]' : 'rounded-[16px]'
  return (
    <span className={cn('block relative shrink-0', large ? 'w-49 h-42' : 'w-29 h-24')}>
      <span
        className={cn(
          'absolute bg-primary/15',
          corner,
          large ? 'top-5 left-7 w-42 h-32' : 'top-3 left-5 w-24 h-18',
        )}
      />
      <span
        className={cn(
          'absolute bg-primary/30',
          corner,
          large ? 'top-2.5 left-3.5 w-42 h-37' : 'top-1.5 left-2.5 w-24 h-21',
        )}
      />
      <PlaylistCover
        playlist={playlist}
        className={cn('top-0 left-0 absolute shadow-elev-1', corner, large ? 'size-42' : 'size-24')}
      />
    </span>
  )
}

function Slide({
  item,
  shown,
  moving,
}: {
  item: Recommended
  shown: boolean
  /** Whether what moves by itself may: not while paused or while the visitor is at it. */
  moving: boolean
}) {
  const { t } = useTranslation()
  const { source, playlist, note } = item
  const page = playlistPage(source, playlist.id)
  // Its songs are read once it is shown, and kept: the playlist's own page reads the same.
  const detail = useQuery({ ...playerQueries.playlist(source, playlist.id), enabled: shown })
  const songs = detail.data?.charts?.filter(supportsChart)
  const rows = Math.min(songs?.length ?? playlist.chartCount, rowsShown)
  const owner = playlist.owner?.nickname || t('profile.unknownPlayer')
  return (
    <article
      data-accent={source}
      aria-hidden={!shown}
      inert={!shown}
      className={cn(
        'flex flex-col @xl:flex-row col-start-1 row-start-1 @xl:h-80 min-w-0 transition-[opacity,visibility] motion-reduce:transition-none duration-500 ease-emphasized',
        !shown && 'invisible opacity-0',
      )}
    >
      {/* The name below is the link a keyboard and a screen reader get; the covers are for the pointer. */}
      <a
        href={page}
        tabIndex={-1}
        aria-hidden="true"
        className="hidden @xl:flex justify-center items-center bg-secondary w-62 shrink-0"
      >
        <CoverStack playlist={playlist} large />
      </a>
      <div className="flex flex-col flex-1 gap-2 p-4 @xl:p-6 min-w-0">
        <div className="flex gap-3.5 min-w-0">
          <a href={page} tabIndex={-1} aria-hidden="true" className="@xl:hidden shrink-0">
            <CoverStack playlist={playlist} />
          </a>
          <div className="flex flex-col flex-1 gap-2 min-w-0">
            <p className="flex flex-wrap items-center gap-1.5">
              <SourceTag source={source} />
              <Badge variant="outline" className="tabular-nums">
                <MusicNotesIcon aria-hidden="true" />
                {t('playlists.songCount', { count: playlist.chartCount })}
              </Badge>
              <Badge variant="outline" className="tabular-nums">
                <HeartIcon aria-hidden="true" />
                {t('playlists.favoriteCount', { count: playlist.favoriteCount })}
              </Badge>
            </p>
            <h3 className="font-extrabold text-xl @xl:text-[1.75rem] line-clamp-2 @xl:line-clamp-1 leading-tight">
              <a
                href={page}
                className="rounded-sm outline-none focus-visible:ring-3 focus-visible:ring-ring hover:underline underline-offset-4"
              >
                {playlist.name}
              </a>
            </h3>
          </div>
        </div>
        {note && (
          <p className="text-muted-foreground text-sm line-clamp-2 @xl:line-clamp-1 break-words">
            {note}
          </p>
        )}
        {rows > 0 && (songs || detail.isPending) && (
          <Songs source={source} songs={songs} rows={rows} moving={shown && moving} />
        )}
        <div className="flex justify-between items-center gap-3 mt-auto pt-1">
          {playlist.owner ? (
            <span className="flex items-center gap-2 min-w-0">
              <UserAvatar
                nickname={owner}
                avatarPath={playlist.owner.avatarPath}
                className="size-7"
              />
              <span className="font-semibold text-sm truncate">{owner}</span>
            </span>
          ) : (
            <span />
          )}
          <a className={cn(buttonVariants(), 'shrink-0')} href={page}>
            {t('home.openPlaylist')}
          </a>
        </div>
      </div>
    </article>
  )
}

/** One of the playlists in the list beside the slides: pressing it shows that one. */
function Row({ item, shown, onShow }: { item: Recommended; shown: boolean; onShow: () => void }) {
  const { t } = useTranslation()
  const { source, playlist } = item
  const owner = playlist.owner?.nickname
  const count = t('playlists.songCount', { count: playlist.chartCount })
  return (
    <button
      type="button"
      data-accent={source}
      aria-current={shown}
      onClick={onShow}
      className={cn(
        'flex items-center gap-3 px-3 py-2 rounded-[18px] outline-none focus-visible:ring-3 focus-visible:ring-ring/40 w-full min-w-0 text-left transition-colors',
        shown ? 'bg-muted' : 'hover:bg-accent',
      )}
    >
      <PlaylistCover playlist={playlist} className="rounded-[12px] size-10" />
      <span className="flex-1 min-w-0">
        <span className={cn('block font-bold text-sm truncate', shown && 'text-primary')}>
          {playlist.name}
        </span>
        <span className="flex items-center gap-2 text-muted-foreground text-xs">
          <SourceTag source={source} />
          <span className="truncate">{owner ? `${count} · ${owner}` : count}</span>
        </span>
      </span>
    </button>
  )
}

/**
 * The playlists the site's administrator recommends, of either source: one at a time (./carousel)
 * with the administrator's line and its songs scrolling by, and beside them, where there is room, all of
 * them to choose from. Nothing is shown while none is recommended or the list cannot be read:
 * the home page is whole without it.
 */
export function FeaturedPlaylists() {
  const { t } = useTranslation()
  const chosen = useQuery(playerQueries.featured())
  const items = chosen.data?.items ?? []
  const named = usePlaylistsNamed(items)
  // In the administrator's order, without any its source does not show (any more).
  const shown = items.flatMap((item): Recommended[] => {
    const playlist = named.found.get(playlistKey(item.source, item.playlistId))
    return playlist && isSource(item.source)
      ? [{ source: item.source, playlist, note: item.note }]
      : []
  })
  const count = shown.length
  const carousel = useCarousel(count)
  if (!items.length || (!count && !named.loading)) return null
  return (
    <section aria-labelledby="featured-heading" data-testid="home-featured" className="space-y-4">
      <h2 id="featured-heading" className="font-extrabold text-xl tracking-tight">
        {t('home.featured')}
      </h2>
      {count === 0 ? (
        <Skeleton className="rounded-[28px] w-full h-[14rem] sm:h-[23.25rem]" />
      ) : (
        <div
          className={cn(
            'gap-4 grid grid-cols-1',
            count > 1 && 'xl:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]',
          )}
          {...carousel.hold}
        >
          <div
            role="group"
            aria-roledescription="carousel"
            aria-labelledby="featured-heading"
            className="@container flex flex-col bg-card shadow-elev-1 rounded-[28px] min-w-0 overflow-hidden"
          >
            <div
              className="flex-1 grid touch-pan-y"
              aria-live={carousel.moving ? 'off' : 'polite'}
              {...carousel.swipe}
            >
              {shown.map((item, index) => (
                <Slide
                  key={playlistKey(item.source, item.playlist.id)}
                  item={item}
                  shown={index === carousel.current}
                  moving={!carousel.still}
                />
              ))}
            </div>
            {count > 1 && (
              <CarouselBar
                carousel={carousel}
                notes={shown.map((item, index) => ({
                  key: playlistKey(item.source, item.playlist.id),
                  source: item.source,
                  label: t('home.playlistOf', {
                    n: index + 1,
                    total: count,
                    title: item.playlist.name,
                  }),
                }))}
                labels={{
                  previous: t('home.previousPlaylist'),
                  next: t('home.nextPlaylist'),
                  pause: t('home.pausePlaylists'),
                  resume: t('home.resumePlaylists'),
                }}
              />
            )}
          </div>
          {count > 1 && (
            <ol className="hidden xl:flex flex-col gap-0.5 bg-card shadow-elev-1 p-2 rounded-[28px] min-w-0">
              {shown.map((item, index) => (
                <li key={playlistKey(item.source, item.playlist.id)}>
                  <Row
                    item={item}
                    shown={index === carousel.current}
                    onShow={() => carousel.go(index)}
                  />
                </li>
              ))}
            </ol>
          )}
        </div>
      )}
    </section>
  )
}
