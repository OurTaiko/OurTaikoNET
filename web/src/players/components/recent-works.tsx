import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChartBase } from '@sitekit/api/types'
import { chartText } from '@sitekit/chart-language'
import { chartOwners } from '@sitekit/chart-owners'
import { RelativeTime } from '@sitekit/comments'
import { LevelChips, LevelBoxes } from '@sitekit/components/difficulty-badges'
import { SongCover } from '@sitekit/components/song-cover'
import { buttonVariants } from '@sitekit/components/ui/button'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { UserAvatar } from '@sitekit/components/user-avatar'
import { cn } from '@sitekit/lib/utils'
import type { SourceId } from '@sitekit/source'
import { Notice } from '@players/notice'
import { CarouselBar, useCarousel } from './carousel'
import { SourceTag, coverOf, songPage } from './song-tile'

export type Work = { source: SourceId; chart: ChartBase }

function Slide({ work, shown, cover }: { work: Work; shown: boolean; cover: boolean }) {
  const { t, i18n } = useTranslation()
  const { source, chart } = work
  const text = chartText(chart, i18n.resolvedLanguage)
  const page = songPage(source, chart.id)
  return (
    <article
      data-accent={source}
      aria-hidden={!shown}
      inert={!shown}
      className={cn(
        'col-start-1 row-start-1 grid grid-cols-[6.5rem_minmax(0,1fr)] @xl:grid-cols-[20rem_minmax(0,1fr)] gap-x-4 gap-y-3 @xl:gap-0 p-4 @xl:p-0 @xl:h-80 min-w-0 transition-[opacity,visibility] motion-reduce:transition-none duration-500 ease-emphasized',
        !shown && 'invisible opacity-0',
      )}
    >
      {/* The title below is the link a keyboard and a screen reader get; this one is for the pointer. */}
      <a href={page} tabIndex={-1} aria-hidden="true" className="group @xl:row-span-2 self-start">
        <SongCover
          image={cover ? coverOf(chart) : undefined}
          categoryIds={chart.categoryIds}
          className="rounded-[16px] @xl:rounded-none"
        />
      </a>
      <div className="flex flex-col gap-2 @xl:px-6 @xl:pt-6 min-w-0">
        <p className="flex items-center gap-2 text-muted-foreground text-xs">
          <SourceTag source={source} />
          <RelativeTime value={chart.createdAt} />
        </p>
        <h3 className="font-extrabold text-xl @xl:text-[1.75rem] line-clamp-2 leading-tight">
          <a
            href={page}
            className="rounded-sm outline-none focus-visible:ring-3 focus-visible:ring-ring hover:underline underline-offset-4"
          >
            {text.title}
          </a>
        </h3>
        {text.subtitle && <p className="text-muted-foreground text-sm truncate">{text.subtitle}</p>}
        {/* A narrow slide has no room for the level boxes beside its cover, so it keeps the squares. */}
        <div className="@xl:hidden">
          <LevelChips difficulties={chart.difficulties} />
        </div>
        <div className="hidden @xl:block mt-1">
          <LevelBoxes difficulties={chart.difficulties} />
        </div>
      </div>
      <div className="flex flex-col gap-3 col-span-2 @xl:col-span-1 @xl:col-start-2 @xl:px-6 @xl:pt-3 @xl:pb-6 min-w-0 min-h-0">
        {/* Where the card is narrow the slides differ in height by this much, so it is left out. */}
        {chart.description && (
          <p className="hidden @xl:block text-muted-foreground text-sm line-clamp-2 break-words">
            {chart.description}
          </p>
        )}
        <div className="flex justify-between items-center gap-3 mt-auto">
          {chartOwners(chart).length > 0 ? (
            <span className="flex items-center gap-2 min-w-0">
              <UserAvatar
                nickname={chart.author}
                avatarPath={chart.authorAvatarPath}
                className="size-7"
              />
              <span className="font-semibold text-sm truncate">{chart.author}</span>
            </span>
          ) : (
            <span />
          )}
          <a className={cn(buttonVariants(), 'shrink-0')} href={page}>
            {t('home.openChart')}
          </a>
        </div>
      </div>
    </article>
  )
}

/**
 * The newest charts of both sources, one at a time (./carousel): a note for each chart under
 * them is the way around.
 */
export function RecentWorks({
  works,
  loading,
  failed,
}: {
  works: Work[]
  loading: boolean
  /** Neither source answered. */
  failed: boolean
}) {
  const { t, i18n } = useTranslation()
  const count = works.length
  const carousel = useCarousel(count)
  const { current } = carousel
  // Covers are fetched for the chart shown and its neighbours, and kept once fetched.
  const [fetched, setFetched] = useState<Set<number>>(() => new Set())
  useEffect(() => {
    if (!count) return
    setFetched((seen) => {
      const near = [current, (current + 1) % count, (current + count - 1) % count]
      return near.every((i) => seen.has(i)) ? seen : new Set([...seen, ...near])
    })
  }, [current, count])

  if (loading) return <Skeleton className="rounded-[28px] w-full h-[17rem] sm:h-[23.5rem]" />
  if (!count)
    return (
      <section className="bg-card shadow-elev-1 p-6 rounded-[28px]">
        <Notice>{t(failed ? 'home.loadFailed' : 'home.noCharts')}</Notice>
      </section>
    )
  return (
    <section
      aria-roledescription="carousel"
      aria-labelledby="recent-works-heading"
      data-testid="home-recent"
      className="@container flex flex-col bg-card shadow-elev-1 rounded-[28px] min-w-0 overflow-hidden"
      {...carousel.hold}
    >
      <h2 id="recent-works-heading" className="sr-only">
        {t('home.recentCharts')}
      </h2>
      <div
        className="flex-1 grid touch-pan-y"
        aria-live={carousel.moving ? 'off' : 'polite'}
        {...carousel.swipe}
      >
        {works.map((work, i) => (
          <Slide
            key={`${work.source}/${work.chart.id}`}
            work={work}
            shown={i === current}
            cover={fetched.has(i)}
          />
        ))}
      </div>
      <CarouselBar
        carousel={carousel}
        notes={works.map((work, i) => ({
          key: `${work.source}/${work.chart.id}`,
          source: work.source,
          label: t('home.chartOf', {
            n: i + 1,
            total: count,
            title: chartText(work.chart, i18n.resolvedLanguage).title,
          }),
        }))}
        labels={{
          previous: t('home.previousChart'),
          next: t('home.nextChart'),
          pause: t('home.pauseCharts'),
          resume: t('home.resumeCharts'),
        }}
      />
    </section>
  )
}
