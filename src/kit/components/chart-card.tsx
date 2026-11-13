import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ArrowFatUpIcon, ChatCircleIcon, GitBranchIcon, UsersIcon } from '@phosphor-icons/react'
import type { ChartBase, Difficulty } from '../api/types'
import { chartText } from '../chart-language'
import { i18n } from '../i18n'
import { coverSource } from '../cover'
import { CategoryLabels } from '../categories'
import { supportsChart } from '../courses'
import { Badge } from './ui/badge'
import { LevelChips } from './difficulty-badges'
import { useSite } from '../site'
import { compactNumber } from './vote-control'
import { SongCover } from './song-cover'

export { formatDuration } from './song-cover'

/**
 * A chart in a list: a tile with the cover on top (tempo and length over it), then title and
 * subtitle, the courses with their star levels, the categories after the site's facts, and a
 * bottom row. The site fills the two rows, each item an <li>, from the badges below and its own.
 */
export function ChartCard({
  chart,
  facts,
  footer,
}: {
  chart: ChartBase & { hasFiles?: boolean }
  facts?: ReactNode
  footer?: ReactNode
}) {
  const { t } = useTranslation()
  if (!supportsChart(chart)) return null
  const text = chartText(chart, i18n.resolvedLanguage)
  return (
    <Link
      className="group flex flex-col bg-card shadow-elev-1 hover:shadow-elev-2 rounded-[20px] outline-none focus-visible:ring-ring focus-visible:ring-3 focus-visible:ring-offset-2 min-w-0 overflow-hidden hover:-translate-y-0.5 motion-reduce:transform-none transition-[translate,box-shadow] motion-reduce:transition-none duration-200 ease-emphasized"
      data-testid="chart-card"
      to={`/charts/${chart.id}`}
    >
      <SongCover
        image={coverSource(chart)}
        categoryIds={chart.categoryIds}
        bpm={chart.bpm}
        duration={chart.hasFiles === false ? undefined : chart.duration}
      />
      <div className="flex flex-col flex-1 gap-2 p-3 min-w-0">
        <div className="space-y-0.5 min-w-0">
          <h3 className="font-bold text-sm line-clamp-2 leading-snug" title={text.title}>
            {text.title}
          </h3>
          <p className="min-h-4 text-muted-foreground text-xs truncate" title={text.subtitle}>
            {text.subtitle}
          </p>
        </div>
        <LevelChips difficulties={chart.difficulties} />
        {/* Two rows of facts at most; a third row is cut off whole. */}
        <div className="max-h-[3.25rem] overflow-hidden">
          <CategoryLabels ids={chart.categoryIds}>{facts}</CategoryLabels>
        </div>
        <ul
          className="flex flex-wrap items-center gap-1.5 mt-auto"
          aria-label={t('messages.chartInformation')}
        >
          {footer}
        </ul>
      </div>
    </Link>
  )
}

/** The chart's maker: its MAKER, else the uploader. */
export function MakerBadge({ name }: { name: string }) {
  const { t } = useTranslation()
  return (
    <li className="min-w-0 max-w-full">
      <Badge
        variant="outline"
        className="gap-0 p-0 min-w-0 max-w-full"
        title={t('messages.chartCreatorWithName', { name })}
      >
        <span className="flex items-center bg-muted px-2 border-r h-full text-muted-foreground shrink-0">
          {t('messages.chartCreator')}
        </span>
        <span className="px-2 truncate">{name}</span>
      </Badge>
    </li>
  )
}

/** Shown when any block is one of a 1P/2P pair. */
export function DoubleBadge({ difficulties }: { difficulties: Difficulty[] }) {
  const { t } = useTranslation()
  if (!difficulties.some((d) => d.side !== 'single')) return null
  return (
    <li>
      <Badge variant="outline" title={t('messages.hasDouble')}>
        <UsersIcon className="size-3" aria-hidden="true" />
        <span className="sr-only">{t('messages.hasDouble')}</span>
      </Badge>
    </li>
  )
}

/** Shown when any block branches. */
export function BranchingBadge({ difficulties }: { difficulties: Difficulty[] }) {
  const { t } = useTranslation()
  if (!difficulties.some((d) => d.branching)) return null
  return (
    <li>
      <Badge variant="outline" title={t('messages.hasBranching')}>
        <GitBranchIcon className="size-3" aria-hidden="true" />
        <span className="sr-only">{t('messages.hasBranching')}</span>
      </Badge>
    </li>
  )
}

export function ScoreBadge({ score }: { score: number }) {
  const { t, i18n } = useTranslation()
  return (
    <li>
      <Badge
        variant="outline"
        className="tabular-nums"
        title={t('interactions.scoreExact', { score })}
      >
        <ArrowFatUpIcon className="size-3" aria-hidden="true" />
        <span className="sr-only">{t('interactions.score')}</span>
        {compactNumber(score, i18n.resolvedLanguage ?? '')}
      </Badge>
    </li>
  )
}

export function CommentCountBadge({ count }: { count: number }) {
  const { t, i18n } = useTranslation()
  return (
    <li>
      <Badge
        variant="outline"
        className="tabular-nums"
        title={t('interactions.commentCount', { count })}
      >
        <ChatCircleIcon className="size-3" aria-hidden="true" />
        <span className="sr-only">{t('interactions.commentCount', { count })}</span>
        <span aria-hidden="true">{compactNumber(count, i18n.resolvedLanguage ?? '')}</span>
      </Badge>
    </li>
  )
}

// The site's own card, rows filled (Site.chartCard), for the shared pages that list charts.
export function SiteChartCard({ chart }: { chart: ChartBase }) {
  return useSite().chartCard(chart)
}
