import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CaretLeftIcon, CaretRightIcon, PauseIcon, PlayIcon } from '@phosphor-icons/react'
import type { ChartBase } from '@sitekit/api/types'
import { chartText } from '@sitekit/chart-language'
import { RelativeTime } from '@sitekit/comments'
import { LevelChips } from '@sitekit/components/difficulty-badges'
import { SongCover } from '@sitekit/components/song-cover'
import { Button, buttonVariants } from '@sitekit/components/ui/button'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { UserAvatar } from '@sitekit/components/user-avatar'
import { cn } from '@sitekit/lib/utils'
import type { SourceId } from '@sitekit/source'
import { Notice } from '@players/notice'
import { SourceTag, coverOf, songPage } from './song-tile'

export type Work = { source: SourceId; chart: ChartBase }

/** Whether the visitor asked their system for less motion: nothing then moves until they start it. */
export const prefersStill = () =>
  typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

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
          image={cover ? coverOf(source, chart) : undefined}
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
        <LevelChips difficulties={chart.difficulties} />
      </div>
      <div className="flex flex-col gap-3 col-span-2 @xl:col-span-1 @xl:col-start-2 @xl:px-6 @xl:pt-3 @xl:pb-6 min-w-0 min-h-0">
        {/* Where the card is narrow the slides differ in height by this much, so it is left out. */}
        {chart.description && (
          <p className="hidden @xl:block text-muted-foreground text-sm line-clamp-2 break-words">
            {chart.description}
          </p>
        )}
        <div className="flex justify-between items-center gap-3 mt-auto">
          <span className="flex items-center gap-2 min-w-0">
            <UserAvatar
              nickname={chart.uploader}
              avatarUrl={chart.uploaderAvatarUrl}
              className="size-7"
            />
            <span className="font-semibold text-sm truncate">{chart.uploader}</span>
          </span>
          <a className={cn(buttonVariants(), 'shrink-0')} href={page}>
            {t('home.openChart')}
          </a>
        </div>
      </div>
    </article>
  )
}

/**
 * The newest charts of both sources, one at a time, moving on by itself. The row of notes
 * under it is the way around: a note for each chart, red (don) for an official one and
 * teal (ka) for a fanmade one, and the one shown stretches into a roll that fills until
 * the next takes over.
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
  const [index, setIndex] = useState(0)
  const [paused, setPaused] = useState(prefersStill)
  // The pointer is over it, or the keyboard is in it: it waits.
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  // Covers are fetched for the chart shown and its neighbours, and kept once fetched.
  const [fetched, setFetched] = useState<Set<number>>(() => new Set())
  const swipe = useRef<number | null>(null)
  const count = works.length
  const current = count ? index % count : 0
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
  const go = (to: number) => setIndex((to + count) % count)
  const moving = !paused && !hovered && !focused && count > 1
  return (
    <section
      aria-roledescription="carousel"
      aria-labelledby="recent-works-heading"
      data-testid="home-recent"
      className="@container flex flex-col bg-card shadow-elev-1 rounded-[28px] min-w-0 overflow-hidden"
      onPointerEnter={(event) => event.pointerType === 'mouse' && setHovered(true)}
      onPointerLeave={() => setHovered(false)}
      onFocus={() => setFocused(true)}
      onBlur={(event) => !event.currentTarget.contains(event.relatedTarget) && setFocused(false)}
    >
      <h2 id="recent-works-heading" className="sr-only">
        {t('home.recentCharts')}
      </h2>
      <div
        className="flex-1 grid touch-pan-y"
        aria-live={moving ? 'off' : 'polite'}
        onTouchStart={(event) => (swipe.current = event.touches[0].clientX)}
        onTouchEnd={(event) => {
          const moved = event.changedTouches[0].clientX - (swipe.current ?? 0)
          swipe.current = null
          if (Math.abs(moved) > 48) go(current + (moved < 0 ? 1 : -1))
        }}
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
      <div className="flex items-center gap-1 px-3 py-2 border-t">
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('home.previousChart')}
          onClick={() => go(current - 1)}
        >
          <CaretLeftIcon weight="bold" />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('home.nextChart')}
          onClick={() => go(current + 1)}
        >
          <CaretRightIcon weight="bold" />
        </Button>
        <ol className="flex flex-1 justify-center items-center min-w-0 note-lane">
          {works.map((work, i) => (
            <li key={`${work.source}/${work.chart.id}`}>
              <button
                type="button"
                data-accent={work.source}
                aria-current={i === current}
                aria-label={t('home.chartOf', {
                  n: i + 1,
                  total: count,
                  title: chartText(work.chart, i18n.resolvedLanguage).title,
                })}
                className="group place-items-center grid px-[3px] sm:px-1 rounded-full outline-none h-8"
                onClick={() => go(i)}
              >
                <span className="note">
                  {i === current && (
                    <span
                      className="note-roll"
                      style={{ animationPlayState: moving ? 'running' : 'paused' }}
                      onAnimationEnd={() => go(current + 1)}
                    />
                  )}
                </span>
              </button>
            </li>
          ))}
        </ol>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t(paused ? 'home.resumeCharts' : 'home.pauseCharts')}
          onClick={() => setPaused(!paused)}
        >
          {paused ? <PlayIcon weight="fill" /> : <PauseIcon weight="fill" />}
        </Button>
      </div>
    </section>
  )
}
