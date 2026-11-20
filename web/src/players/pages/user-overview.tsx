import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueries, useQuery } from '@tanstack/react-query'
import { RelativeTime } from '@sitekit/comments'
import { LevelBox } from '@sitekit/components/difficulty-badges'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { cn } from '@sitekit/lib/utils'
import type { SourceId } from '@sitekit/source'
import type { Nameplate as Plate } from '@players/api/types'
import { Crown } from '@players/components/crown'
import { TitleBand } from '@players/components/nameplate'
import { SongTile, SourceTag, sources } from '@players/components/song-tile'
import { chartSources } from '@players/pages/user-works'
import { danName, verdictKey } from '@players/dan'
import { DanEmblem } from '@sitekit/components/nameplate-art'
import { formatLocale } from '@players/i18n'
import { Notice } from '@players/notice'
import { playerQueries } from '@players/queries'

export type Section = 'scores' | 'charts' | 'titles' | 'dans'
const isSource = (id: string): id is SourceId => sources.some((source) => source === id)

/** A part of the overview: a heading, a way to the tab that has all of it, and a few of it. */
function Panel({
  title,
  more,
  onMore,
  className,
  children,
}: {
  title: string
  more: string
  onMore: () => void
  className?: string
  children: ReactNode
}) {
  return (
    <section
      className={cn('space-y-4 bg-card shadow-elev-1 p-6 rounded-[28px] min-w-0', className)}
    >
      <div className="flex flex-wrap justify-between items-center gap-x-4 gap-y-1">
        <h2 className="font-extrabold text-xl tracking-tight">{title}</h2>
        <button
          type="button"
          onClick={onMore}
          className="inline-flex items-center gap-1 rounded-full outline-none focus-visible:ring-3 focus-visible:ring-ring font-semibold text-primary text-sm hover:underline"
        >
          {more}
        </button>
      </div>
      {children}
    </section>
  )
}

function RecentPlays({ id, onMore }: { id: string; onMore: () => void }) {
  const { t, i18n } = useTranslation()
  const [source, setSource] = useState<'all' | SourceId>('all')
  const recent = useQuery(
    playerQueries.scores(id, { view: 'all', sort: 'recent', source, page: 1 }),
  )
  const plays = recent.data
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  return (
    <Panel
      title={t('profile.overview.recent')}
      more={t('profile.overview.allScores')}
      onMore={onMore}
    >
      <div role="group" aria-label={t('shell.source')} className="flex flex-wrap gap-2">
        {(['all', ...sources] as const).map((item) => (
          <button
            key={item}
            type="button"
            aria-pressed={item === source}
            data-accent={item === 'all' ? undefined : item}
            onClick={() => setSource(item)}
            className={cn(
              'inline-flex items-center px-3.5 border rounded-full outline-none focus-visible:ring-3 focus-visible:ring-ring h-8 font-medium text-sm transition-colors',
              item === source
                ? 'border-transparent bg-secondary font-semibold text-secondary-foreground'
                : 'border-input text-muted-foreground hover:bg-muted',
            )}
          >
            {item === 'all' ? t('profile.overview.both') : t(`shell.${item}`)}
          </button>
        ))}
      </div>
      {plays === undefined ? (
        recent.error ? (
          <Notice>{recent.error.message}</Notice>
        ) : (
          <Skeleton className="rounded-[14px] w-full h-48" />
        )
      ) : plays.items.length === 0 ? (
        <p className="py-8 text-muted-foreground text-sm text-center">{t('scores.empty')}</p>
      ) : (
        <ul className="divide-y" data-testid="recent-plays">
          {plays.items.slice(0, 6).map((row) => {
            const title = row.titleTranslations?.[language] || row.title
            return (
              <li key={row.source + row.id} className="flex items-center gap-3 py-2.5 min-w-0">
                {isSource(row.source) && <SourceTag source={row.source} />}
                <div className="flex-1 space-y-0.5 min-w-0">
                  {row.chartUrl ? (
                    <a
                      href={row.chartUrl}
                      className="block font-semibold text-sm hover:underline truncate"
                    >
                      {title}
                    </a>
                  ) : (
                    <span className="block font-semibold text-sm truncate">{title}</span>
                  )}
                  <p className="text-muted-foreground text-xs truncate">
                    {t(`courses.${row.course}`)}
                    {row.side !== 'single' && ` · ${row.side.toUpperCase()}`}
                    {' · '}
                    <RelativeTime value={row.submittedAt} />
                  </p>
                </div>
                <LevelBox course={row.course} level={row.level} name={t(`courses.${row.course}`)} />
                <Crown status={row.ClearStatus} />
                <span className="sm:w-[5.5rem] font-mono font-semibold text-sm text-right tabular-nums shrink-0">
                  {row.score.toLocaleString(formatLocale(language))}
                </span>
                <span className="hidden sm:inline w-28 font-mono text-muted-foreground text-xs text-right tabular-nums whitespace-nowrap shrink-0">
                  {row.good} / {row.ok} / {row.bad}
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </Panel>
  )
}

function Titles({ id, plate, onMore }: { id: string; plate: Plate; onMore: () => void }) {
  const { t } = useTranslation()
  const answer = useQuery(playerQueries.heldTitles(id))
  const held = answer.data
  return (
    <Panel title={t('profile.tabs.titles')} more={t('profile.overview.allTitles')} onMore={onMore}>
      {held === undefined ? (
        answer.error ? (
          <Notice>{answer.error.message}</Notice>
        ) : (
          <Skeleton className="rounded-[14px] w-full h-16" />
        )
      ) : held.titles.length === 0 ? (
        <p className="py-4 text-muted-foreground text-sm text-center">{t('titles.none')}</p>
      ) : (
        <>
          <ul className="flex flex-wrap gap-2.5" data-testid="overview-titles">
            {held.titles.slice(0, 8).map(({ title }) => (
              <li key={title.id} className="max-w-full">
                <TitleBand
                  title={title}
                  className={cn(
                    plate.title?.id === title.id &&
                      'ring-2 ring-primary ring-offset-2 ring-offset-card',
                  )}
                />
              </li>
            ))}
          </ul>
        </>
      )}
    </Panel>
  )
}

function DanRanks({ id, onMore }: { id: string; onMore: () => void }) {
  const { t, i18n } = useTranslation()
  const answer = useQuery(playerQueries.dans(id))
  const dans = answer.data
  const date = new Intl.DateTimeFormat(formatLocale(i18n.resolvedLanguage ?? 'zh-Hans'), {
    dateStyle: 'medium',
  })
  const highest = dans && [...dans.badges].sort((a, b) => b.index - a.index)[0]
  return (
    <Panel title={t('profile.overview.dans')} more={t('profile.overview.allDans')} onMore={onMore}>
      {dans === undefined ? (
        answer.error ? (
          <Notice>{answer.error.message}</Notice>
        ) : (
          <Skeleton className="rounded-[14px] w-full h-20" />
        )
      ) : !highest && dans.history.length === 0 ? (
        <p className="py-4 text-muted-foreground text-sm text-center">{t('dans.noHistory')}</p>
      ) : (
        <div className="space-y-3">
          {highest && (
            <div className="flex items-center gap-3 bg-muted p-3 rounded-[14px]">
              <DanEmblem
                index={highest.index}
                gold={highest.gold}
                combo={highest.combo}
                label={danName(highest.index)}
              />
              <div className="min-w-0">
                <p className="text-muted-foreground text-xs">{t('profile.overview.highest')}</p>
                <p className="font-semibold text-sm truncate">
                  {t(verdictKey(highest.verdict))} · {date.format(new Date(highest.achievedAt))}
                </p>
              </div>
            </div>
          )}
          <ul className="divide-y">
            {dans.history.slice(0, 3).map((attempt, index) => (
              <li key={index} className="flex justify-between items-baseline gap-3 py-2 min-w-0">
                <span className="font-medium text-sm truncate">
                  {attempt.courseTitle || (attempt.dan !== null ? danName(attempt.dan) : '—')}
                </span>
                <span className="text-muted-foreground text-xs whitespace-nowrap shrink-0">
                  {t(verdictKey(attempt.verdict))} · {date.format(new Date(attempt.createdAt))}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Panel>
  )
}

/** The newest charts the player uploaded (to Fanmade); nothing when they made none. */
function Charts({ id, onMore }: { id: string; onMore: () => void }) {
  const { t } = useTranslation()
  const lists = useQueries({
    queries: chartSources.map((source) => playerQueries.ownerCharts(source, id)),
  })
  if (lists.some((list) => list.isPending)) return null
  // A source that does not answer leaves the other's charts showing.
  const charts = lists
    .flatMap((list) => list.data ?? [])
    .sort((a, b) => Date.parse(b.chart.createdAt) - Date.parse(a.chart.createdAt))
    .slice(0, 6)
  if (!charts.length) return null
  return (
    <Panel
      title={t('profile.overview.charts')}
      more={t('profile.overview.allCharts')}
      onMore={onMore}
      className="lg:col-span-2"
    >
      <div className="song-grid" data-testid="overview-charts">
        {charts.map(({ source, chart }) => (
          <SongTile key={`${source}/${chart.id}`} source={source} chart={chart} />
        ))}
      </div>
    </Panel>
  )
}

/** A little of everything about a player on one screen; each part leads to its own tab. */
export function OverviewTab({
  id,
  plate,
  onOpen,
}: {
  id: string
  plate: Plate
  onOpen: (section: Section) => void
}) {
  return (
    <div className="items-start gap-4 grid grid-cols-1 lg:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]">
      <RecentPlays id={id} onMore={() => onOpen('scores')} />
      <div className="space-y-4 min-w-0">
        <Titles id={id} plate={plate} onMore={() => onOpen('titles')} />
        <DanRanks id={id} onMore={() => onOpen('dans')} />
      </div>
      <Charts id={id} onMore={() => onOpen('charts')} />
    </div>
  )
}
