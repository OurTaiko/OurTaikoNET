import { useEffect, useRef, useState, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { CrownSimpleIcon, PauseIcon, PlayIcon, UploadSimpleIcon } from '@phosphor-icons/react'
import { chartText } from '@sitekit/chart-language'
import { SYSTEM_OWNER_ID } from '@sitekit/chart-owners'
import { RelativeTime } from '@sitekit/comments'
import { LevelChips } from '@sitekit/components/difficulty-badges'
import { Button } from '@sitekit/components/ui/button'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { UserAvatar } from '@sitekit/components/user-avatar'
import { cn } from '@sitekit/lib/utils'
import type { Activity } from '@players/api/types'
import { Notice } from '@players/notice'
import { playerQueries } from '@players/queries'
import { prefersStill } from './recent-works'
import { SourceTag, songPage } from './song-tile'

// The crown's colours, as on a player's scores: cleared, full combo, all good.
const crownColors = ['text-zinc-400', 'text-amber-500', 'text-fuchsia-500']
const did = ['home.cleared', 'home.fullCombo', 'home.allGood'] as const

/**
 * Scrolls box down by itself while `moving`, a line every second or so. The list is in it
 * twice (`loop` is the second copy), so that reaching the copy and jumping back by its
 * distance shows no seam. The visitor's own scrolling always wins, and holds it a moment.
 */
function useDrift(
  box: RefObject<HTMLDivElement | null>,
  loop: RefObject<HTMLUListElement | null>,
  moving: boolean,
) {
  useEffect(() => {
    const element = box.current
    if (!element) return
    let touched = 0
    const touch = () => (touched = performance.now())
    // The seam, for the visitor's scrolling as for ours.
    const wrap = () => {
      const length = loop.current?.offsetTop
      if (length && element.scrollTop >= length) element.scrollTop -= length
    }
    element.addEventListener('wheel', touch, { passive: true })
    element.addEventListener('touchstart', touch, { passive: true })
    element.addEventListener('touchmove', touch, { passive: true })
    element.addEventListener('scroll', wrap, { passive: true })
    let frame = 0
    if (moving) {
      let last = performance.now()
      let at = element.scrollTop
      const step = (now: number) => {
        // A tab that was in the background comes back with one long frame.
        const passed = Math.min(now - last, 100)
        last = now
        if (now - touched > 2000) {
          // Browsers round scrollTop; ours is kept apart unless the visitor moved it.
          if (Math.abs(element.scrollTop - at) > 2) at = element.scrollTop
          at += (28 * passed) / 1000
          const length = loop.current?.offsetTop
          if (length && at >= length) at -= length
          element.scrollTop = at
        }
        frame = requestAnimationFrame(step)
      }
      frame = requestAnimationFrame(step)
    }
    return () => {
      cancelAnimationFrame(frame)
      element.removeEventListener('wheel', touch)
      element.removeEventListener('touchstart', touch)
      element.removeEventListener('touchmove', touch)
      element.removeEventListener('scroll', wrap)
    }
  }, [box, loop, moving])
}

/** One row. `copy` is the second showing of it, which the keyboard passes over. */
function Row({ item, copy = false }: { item: Activity; copy?: boolean }) {
  const { t, i18n } = useTranslation()
  const source = item.source === 'official' ? 'official' : 'fanmade'
  const system = item.user.id === SYSTEM_OWNER_ID
  const name = system
    ? t('home.officialUploader')
    : item.user.nickname || t('profile.unknownPlayer')
  const title = chartText({ ...item.chart, subtitle: '' }, i18n.resolvedLanguage).title
  const play = item.kind === 'score' ? item.play : undefined
  const crown = play ? Math.min(Math.max(play.clearStatus, 1), 3) - 1 : 0
  const who = (
    <>
      <UserAvatar nickname={name} avatarPath={item.user.avatarPath} className="size-10" />
      {/* What happened, on the corner of who did it: a crown for a play, an arrow for an upload. */}
      <span
        data-accent={source}
        className="-right-1 -bottom-1 absolute place-items-center grid bg-card rounded-full size-5"
      >
        {play ? (
          <CrownSimpleIcon weight="fill" className={cn('size-3.5', crownColors[crown])} />
        ) : (
          <UploadSimpleIcon weight="bold" className="size-3 text-primary" />
        )}
      </span>
    </>
  )
  return (
    <li className="flex items-center gap-3 hover:bg-muted px-3 py-2.5 rounded-[16px] transition-colors">
      {system ? (
        <span className="relative shrink-0">{who}</span>
      ) : (
        <Link
          to={`/players/${item.user.id}`}
          aria-label={name}
          tabIndex={copy ? -1 : undefined}
          className="relative rounded-full outline-none focus-visible:ring-3 focus-visible:ring-ring shrink-0"
        >
          {who}
        </Link>
      )}
      <a
        href={songPage(source, item.chart.id)}
        tabIndex={copy ? -1 : undefined}
        className="flex-1 space-y-0.5 rounded-md outline-none focus-visible:ring-3 focus-visible:ring-ring min-w-0"
      >
        <span className="flex justify-between items-baseline gap-3 text-muted-foreground text-xs">
          <span className="truncate">
            <span className="font-semibold text-foreground">{name}</span>{' '}
            {t(play ? did[crown] : 'home.uploaded')}
          </span>
          <span className="shrink-0">
            <RelativeTime value={item.at} />
          </span>
        </span>
        <span className="flex items-center gap-2">
          <span className="flex-1 font-bold text-sm truncate">{title}</span>
          {play ? (
            <>
              <span className="text-muted-foreground text-xs tabular-nums">
                {play.score.toLocaleString(i18n.resolvedLanguage)}
              </span>
              <LevelChips
                difficulties={[
                  { course: play.course, side: play.side, level: play.level, maker: '' },
                ]}
              />
            </>
          ) : (
            <SourceTag source={source} />
          )}
        </span>
      </a>
    </li>
  )
}

/** What players have just done on either source: plays they cleared and charts they uploaded. */
export function CommunityActivity() {
  const { t } = useTranslation()
  const activity = useQuery(playerQueries.activity())
  const items = activity.data?.items ?? (activity.isError ? 'failed' : null)
  const [paused, setPaused] = useState(prefersStill)
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const copy = useRef<HTMLUListElement>(null)
  const rows = Array.isArray(items) ? items : []
  // A handful fits without moving.
  const loops = rows.length > 6
  useDrift(box, copy, loops && !paused && !hovered && !focused)
  const key = (item: Activity) => `${item.kind}/${item.source}/${item.play?.id ?? item.chart.id}`
  return (
    <section
      aria-labelledby="activity-heading"
      data-testid="home-activity"
      className="flex flex-col bg-card shadow-elev-1 rounded-[28px] min-w-0 h-[26rem] xl:h-auto xl:min-h-[23.5rem]"
    >
      <div className="flex justify-between items-center gap-3 pt-4 pr-3 pb-1 pl-6">
        <h2 id="activity-heading" className="font-extrabold text-xl tracking-tight">
          {t('home.activity')}
        </h2>
        {loops && (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t(paused ? 'home.resumeActivity' : 'home.pauseActivity')}
            onClick={() => setPaused(!paused)}
          >
            {paused ? <PlayIcon weight="fill" /> : <PauseIcon weight="fill" />}
          </Button>
        )}
      </div>
      <div className="relative flex-1 min-h-0">
        <div
          ref={box}
          className={cn(
            'absolute inset-0 px-3 pt-2 pb-3 overflow-y-auto [scrollbar-width:none]',
            loops && 'activity-fade',
          )}
          onPointerEnter={(event) => event.pointerType === 'mouse' && setHovered(true)}
          onPointerLeave={() => setHovered(false)}
          onFocus={() => setFocused(true)}
          onBlur={(event) =>
            !event.currentTarget.contains(event.relatedTarget) && setFocused(false)
          }
        >
          {items === null && (
            <div className="space-y-2 px-3">
              {[0, 1, 2, 3, 4].map((row) => (
                <Skeleton key={row} className="rounded-[16px] h-14" />
              ))}
            </div>
          )}
          {items === 'failed' && <Notice>{t('home.activityFailed')}</Notice>}
          {Array.isArray(items) && !rows.length && (
            <p className="px-3 py-2 text-muted-foreground text-sm">{t('home.noActivity')}</p>
          )}
          <ul>
            {rows.map((item) => (
              <Row key={key(item)} item={item} />
            ))}
          </ul>
          {loops && (
            // The same rows again, only to scroll into: a pointer can use them, a keyboard and a
            // screen reader have the first.
            <ul ref={copy} aria-hidden="true">
              {rows.map((item) => (
                <Row key={key(item)} item={item} copy />
              ))}
            </ul>
          )}
        </div>
      </div>
    </section>
  )
}
