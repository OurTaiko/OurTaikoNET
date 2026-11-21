import { useTranslation } from 'react-i18next'
import { cn } from '../lib/utils'
import type { Difficulty } from '../api/types'
import {
  blockName,
  isSupportedCourse,
  slotKey,
  groupCourses,
  groupLevel,
  groupName,
  type BaseCourse,
} from '../courses'
import easyEmblem from '../assets/difficulty/easy.png'
import normalEmblem from '../assets/difficulty/normal.png'
import hardEmblem from '../assets/difficulty/hard.png'
import oniEmblem from '../assets/difficulty/oni.png'
import uraEmblem from '../assets/difficulty/ura.png'

// Two forms of a course. Where there is room (a carousel slide, a list row, a heading) it is a
// small box with the game's own emblem, the one drawn when choosing a difficulty, above its
// level. Where a row lists up to five courses next to a title it is a small square of the
// course's colour, as an 18px emblem loses its shape and takes half the title's width; the
// colours follow the emblems, so the two forms read alike.

// Fixed course colours, never tinted by the accent; white text passes AA on every one.
const difficultyColors: Record<BaseCourse, string> = {
  Easy: 'bg-[#c2410c]',
  Normal: 'bg-[#2f7d32]',
  Hard: 'bg-[#1565c0]',
  Oni: 'bg-[#c2185b]',
  Edit: 'bg-[#6a3fc0]',
}

const emblems: Record<BaseCourse, string> = {
  Easy: easyEmblem,
  Normal: normalEmblem,
  Hard: hardEmblem,
  Oni: oniEmblem,
  Edit: uraEmblem,
}

const boxSizes = {
  // A list row or a table cell.
  sm: {
    box: 'min-w-10 gap-0.5 rounded-[10px] px-1 pt-1 pb-1',
    emblem: 'h-[1.3125rem] w-[1.875rem]',
    level: 'text-xs',
  },
  // A carousel slide or a heading.
  md: {
    box: 'min-w-[3.25rem] gap-1 rounded-[12px] px-1.5 pt-1.5 pb-1',
    emblem: 'h-7 w-10',
    level: 'text-sm',
  },
}

/**
 * A course as a small box: the game's emblem above the level ("8", or "8/9" for a pair). The
 * course is read out by name; pass `name={null}` where the text beside it already names it.
 * Nothing for a course the site does not know.
 */
export function LevelBox({
  course,
  level,
  name,
  size = 'sm',
  className,
}: {
  course: string
  level: number | string
  /** The course as it is read out: its own name when left out, nothing when null. */
  name?: string | null
  size?: keyof typeof boxSizes
  className?: string
}) {
  if (!isSupportedCourse(course)) return null
  const label = name === undefined ? blockName(course) : name
  const sizes = boxSizes[size]
  return (
    <span
      title={label ?? undefined}
      className={cn(
        'inline-flex shrink-0 flex-col items-center bg-muted text-foreground ring-1 ring-border ring-inset',
        sizes.box,
        className,
      )}
    >
      <img
        src={emblems[course]}
        alt=""
        aria-hidden="true"
        draggable={false}
        className={cn(
          'shrink-0 object-contain select-none',
          sizes.emblem,
          // The emblems' black outline disappears on a dark box; a faint light edge keeps the shape.
          'dark:drop-shadow-[0_0_1px_rgb(255_255_255/0.75)]',
        )}
      />
      <span
        className={cn('leading-none font-extrabold whitespace-nowrap tabular-nums', sizes.level)}
      >
        {label && <span className="sr-only">{label} </span>}
        {level}
      </span>
    </span>
  )
}

/** A chart's courses as level boxes: LevelChips where there is room for them. */
export function LevelBoxes({ difficulties }: { difficulties: Difficulty[] }) {
  const { t } = useTranslation()
  return (
    <ul aria-label={t('messages.chartDifficulty')} className="flex flex-wrap gap-2">
      {groupCourses(difficulties).map((group) => (
        <li key={group.base}>
          <LevelBox
            course={group.base}
            level={groupLevel(group)}
            name={groupName(group)}
            size="md"
          />
        </li>
      ))}
    </ul>
  )
}

/**
 * A chart's courses as small squares with their star levels: the form a song tile shows.
 * With `oneRow` they never wrap, and narrow instead where five do not fit.
 */
export function LevelChips({
  difficulties,
  oneRow = false,
}: {
  difficulties: Difficulty[]
  oneRow?: boolean
}) {
  const { t } = useTranslation()
  return (
    <ul
      aria-label={t('messages.chartDifficulty')}
      className={cn('flex gap-1', oneRow ? 'min-w-0' : 'flex-wrap')}
    >
      {groupCourses(difficulties).map((group) => (
        <li
          key={group.base}
          title={groupName(group)}
          className={cn(
            'inline-flex justify-center items-center rounded-[7px] h-[1.375rem] font-bold text-white text-xs tabular-nums',
            oneRow ? 'w-[1.625rem] min-w-0 shrink' : 'px-1.5 min-w-[1.625rem]',
            difficultyColors[group.base],
          )}
        >
          <span className="sr-only">{groupName(group)} </span>
          {groupLevel(group)}
        </li>
      ))}
    </ul>
  )
}

/** A block's level box with its name beside it ("Oni", "Oni 1P"), as a heading or a list shows it. */
export function CourseLabel({
  course,
  level,
  name,
  className,
}: {
  course: string
  level: number | string
  name: string
  className?: string
}) {
  return (
    <span className={cn('inline-flex items-center gap-2 text-sm font-bold', className)}>
      <LevelBox course={course} level={level} name={null} />
      <span>{name}</span>
    </span>
  )
}

/** Every block of a chart, each as its level box and name: the upload form's preview. */
export function DifficultyBadges({ difficulties }: { difficulties: Difficulty[] }) {
  const { t } = useTranslation()
  const blocks = difficulties
    .filter((d) => isSupportedCourse(d.course))
    .filter((d, index, all) => all.findIndex((other) => slotKey(other) === slotKey(d)) === index)
  return (
    <ul aria-label={t('messages.chartDifficulty')} className="flex flex-wrap gap-x-4 gap-y-2">
      {blocks.map((d) => (
        <li key={slotKey(d)}>
          <CourseLabel course={d.course} level={d.level} name={blockName(d)} />
        </li>
      ))}
    </ul>
  )
}
