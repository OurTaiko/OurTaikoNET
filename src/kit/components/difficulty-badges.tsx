import { useTranslation } from 'react-i18next'
import { StarIcon } from '@phosphor-icons/react'
import { Badge } from './ui/badge'
import { cn } from '../lib/utils'
import type { Difficulty } from '../api/types'
import {
  blockName,
  isSupportedCourse,
  slotKey,
  groupCourses,
  groupLevel,
  groupName,
} from '../courses'

// Fixed course colours, never tinted by the accent; white text passes AA on every one.
export const difficultyColors = {
  Easy: 'bg-[#c2410c]',
  Normal: 'bg-[#2f7d32]',
  Hard: 'bg-[#1565c0]',
  Oni: 'bg-[#c2185b]',
  Edit: 'bg-[#6a3fc0]',
}

const swatch =
  'inline-flex items-center gap-1.5 border-0 rounded-[7px] font-bold text-white text-xs whitespace-nowrap'

/** A chart's courses as small squares with their star levels: the form a song tile shows. */
export function LevelChips({ difficulties }: { difficulties: Difficulty[] }) {
  const { t } = useTranslation()
  return (
    <ul aria-label={t('messages.chartDifficulty')} className="flex flex-wrap gap-1">
      {groupCourses(difficulties).map((group) => (
        <li
          key={group.base}
          title={groupName(group)}
          className={cn(
            'inline-flex justify-center items-center px-1.5 rounded-[7px] min-w-[1.625rem] h-[1.375rem] font-bold text-white text-xs tabular-nums',
            difficultyColors[group.base as keyof typeof difficultyColors] ?? 'bg-muted',
          )}
        >
          <span className="sr-only">{groupName(group)} </span>
          {groupLevel(group)}
        </li>
      ))}
    </ul>
  )
}

export function DifficultyBadges({
  difficulties,
  detailed = false,
  bookmarks = false,
}: {
  difficulties: Difficulty[]
  detailed?: boolean
  bookmarks?: boolean
}) {
  const { t } = useTranslation()

  const supported = difficulties.filter((d) => isSupportedCourse(d.course))
  const items = detailed
    ? supported
    : supported.filter(
        (d, index, all) => all.findIndex((other) => slotKey(other) === slotKey(d)) === index,
      )
  // Bookmarks show the single, P1 and P2 blocks of one course as one chart, so
  // a song has at most five and they fit the fixed card height.
  if (bookmarks)
    return (
      <ul
        aria-label={t('messages.chartDifficulty')}
        className="flex flex-col self-start gap-1.5 w-max min-w-24 shrink-0 whitespace-nowrap"
      >
        {groupCourses(difficulties).map((group) => (
          <li
            key={group.base}
            className={cn(
              swatch,
              'justify-between px-2.5 h-7',
              difficultyColors[group.base as keyof typeof difficultyColors] ?? 'bg-muted',
            )}
          >
            <span>{groupName(group)}</span>
            <span
              className="inline-flex items-center gap-0.5 tabular-nums"
              aria-label={group.sides
                .map((side) => t('common.stars', { count: side.level }))
                .filter((label, index, all) => all.indexOf(label) === index)
                .join(' / ')}
            >
              <StarIcon weight="fill" className="size-3" aria-hidden="true" />
              {groupLevel(group)}
            </span>
          </li>
        ))}
      </ul>
    )
  return (
    <div className="flex flex-wrap gap-2">
      {items.map((d) => (
        <Badge
          className={cn(
            swatch,
            'h-6 px-2',
            difficultyColors[d.course as keyof typeof difficultyColors],
          )}
          key={slotKey(d)}
        >
          {blockName(d)}
          <span
            className="inline-flex items-center gap-1 tabular-nums"
            aria-label={t('common.stars', { count: d.level })}
          >
            <StarIcon weight="fill" className="size-3" aria-hidden="true" />
            {d.level}
          </span>
        </Badge>
      ))}
    </div>
  )
}
