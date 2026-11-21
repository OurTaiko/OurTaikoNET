import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import {
  ArrowFatUpIcon,
  ChatCircleIcon,
  GameControllerIcon,
  GitBranchIcon,
  PencilSimpleLineIcon,
  UsersIcon,
} from '@phosphor-icons/react'
import type { ChartBase, Difficulty } from '../api/types'
import { chartText } from '../chart-language'
import { i18n } from '../i18n'
import { coverSource } from '../cover'
import { CoverCategories } from '../categories'
import { supportsChart } from '../courses'
import { Badge } from './ui/badge'
import { LevelChips } from './difficulty-badges'
import { useSite } from '../site'
import { compactNumber } from './vote-control'
import { SongCover, coverChip } from './song-cover'
import { ScrollingText } from './scrolling-text'

export { formatDuration } from './song-cover'

/**
 * A chart in a list: a tile of one height whatever the chart, so a grid of them stays even. The
 * cover is on top with what varies in number over it (tempo and categories, length, the marks
 * of a 1P/2P pair and of branching); below it every part has one line: the title, which slides
 * sideways when it is too long, the subtitle, the courses with their star levels, and two rows
 * the site fills, each item an <li>, from the badges below and its own. `coverNote` takes the
 * place of the length.
 */
export function ChartCard({
  chart,
  facts,
  footer,
  coverNote,
}: {
  chart: ChartBase & { hasFiles?: boolean }
  facts?: ReactNode
  footer?: ReactNode
  coverNote?: ReactNode
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
        corner={coverNote}
        marks={<ChartMarks difficulties={chart.difficulties} />}
      >
        <CoverCategories ids={chart.categoryIds} />
      </SongCover>
      <div className="flex flex-col flex-1 gap-2 p-3 min-w-0">
        <div className="space-y-0.5 min-w-0">
          <h3 className="font-bold text-sm leading-snug" title={text.title}>
            <ScrollingText>{text.title}</ScrollingText>
          </h3>
          <p className="h-4 text-muted-foreground text-xs truncate" title={text.subtitle}>
            {text.subtitle}
          </p>
        </div>
        {/* A song without files has no courses yet: their line is kept. */}
        <div className="h-[1.375rem] min-w-0">
          <LevelChips difficulties={chart.difficulties} oneRow />
        </div>
        <ul className={row} aria-label={t('messages.chartInformation')}>
          {facts}
        </ul>
        <ul className={row}>{footer}</ul>
      </div>
    </Link>
  )
}

// One line of badges, kept when it is empty; what does not fit is cut off.
const row = 'flex items-center gap-1.5 h-5 min-w-0 overflow-hidden'

/** Over the cover: whether any block is one of a 1P/2P pair, and whether any block branches. */
function ChartMarks({ difficulties }: { difficulties: Difficulty[] }) {
  const { t } = useTranslation()
  const marks = [
    difficulties.some((d) => d.side !== 'single') && {
      key: 'double',
      label: t('messages.hasDouble'),
      icon: <UsersIcon className="size-3" aria-hidden="true" />,
    },
    difficulties.some((d) => d.branching) && {
      key: 'branching',
      label: t('messages.hasBranching'),
      icon: <GitBranchIcon className="size-3" aria-hidden="true" />,
    },
  ].filter((mark) => !!mark)
  if (marks.length === 0) return null
  return (
    <ul className="flex gap-1">
      {marks.map((mark) => (
        <li key={mark.key} className={coverChip} title={mark.label}>
          {mark.icon}
          <span className="sr-only">{mark.label}</span>
        </li>
      ))}
    </ul>
  )
}

/** The chart's maker: its MAKER, else the uploader, after the pen the chart's page marks makers with. */
export function MakerBadge({ name }: { name: string }) {
  const { t } = useTranslation()
  return (
    <li className="min-w-0 max-w-full">
      <Badge
        variant="outline"
        className="min-w-0 max-w-full"
        title={t('messages.chartCreatorWithName', { name })}
      >
        <PencilSimpleLineIcon className="shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="sr-only">{t('messages.chartCreator')}</span>
        <span className="truncate">{name}</span>
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

/** How many times the chart was played: the scores of all its blocks. */
export function PlayCountBadge({ count }: { count: number }) {
  const { t, i18n } = useTranslation()
  return (
    <li>
      <Badge
        variant="outline"
        className="tabular-nums"
        title={t('interactions.playCount', { count })}
      >
        <GameControllerIcon className="size-3" aria-hidden="true" />
        <span className="sr-only">{t('interactions.playCount', { count })}</span>
        <span aria-hidden="true">{compactNumber(count, i18n.resolvedLanguage ?? '')}</span>
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
