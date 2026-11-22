import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@sitekit/components/ui/button'
import { cn } from '@sitekit/lib/utils'
import { scaleTop } from '@players/visit-stats'

/** One column of BarChart: a day or an hour, its number, and the part of it drawn solid. */
export type Bar = {
  key: string
  /** What the column is, in full: said above the plot while it is pointed at. */
  label: string
  /** The same in short, under the plot's ends and middle. */
  mark: string
  value: number
  /** A number within `value` (a day's visitors within its views). */
  part?: number
}

function Swatch({ className }: { className: string }) {
  return (
    <span className={cn('inline-block rounded-[3px] size-2.5', className)} aria-hidden="true" />
  )
}

/**
 * One number a day (or an hour) as bars on one scale from zero, and within each bar, solid, the
 * part of it that `names.part` names. A whole column answers the pointer, not only its bar, and
 * says its label and numbers above the plot; `idle` stands there while nothing is pointed at.
 * Those who do not point have the page's table of the days.
 */
export function BarChart({
  bars,
  format,
  names,
  idle,
}: {
  bars: Bar[]
  format: (n: number) => string
  /** What the bar and its solid part count, when the bars have parts. */
  names?: { value: string; part: string }
  idle?: ReactNode
}) {
  const [at, setAt] = useState<number | null>(null)
  const top = scaleTop(Math.max(0, ...bars.map((bar) => bar.value)))
  const shown = at !== null && at < bars.length ? bars[at] : null
  const marks = [bars[0], bars[Math.floor((bars.length - 1) / 2)], bars[bars.length - 1]]
  const tint = names ? 'bg-primary/30' : 'bg-primary'
  return (
    <div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 min-h-5 text-sm tabular-nums">
        {shown ? (
          <>
            <span className="text-muted-foreground">{shown.label}</span>
            {names ? (
              <>
                <span className="flex items-center gap-1.5">
                  <Swatch className="bg-primary/30" />
                  {names.value} <span className="font-semibold">{format(shown.value)}</span>
                </span>
                <span className="flex items-center gap-1.5">
                  <Swatch className="bg-primary" />
                  {names.part} <span className="font-semibold">{format(shown.part ?? 0)}</span>
                </span>
              </>
            ) : (
              <span className="font-semibold">{format(shown.value)}</span>
            )}
          </>
        ) : (
          <>
            {names && (
              <>
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <Swatch className="bg-primary/30" />
                  {names.value}
                </span>
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <Swatch className="bg-primary" />
                  {names.part}
                </span>
              </>
            )}
            {idle}
          </>
        )}
      </div>
      <div className="flex gap-2 mt-2" aria-hidden="true">
        <div className="flex flex-col justify-between w-10 h-48 text-muted-foreground text-xs text-right tabular-nums leading-none">
          <span>{format(top)}</span>
          <span>{format(top / 2)}</span>
          <span>0</span>
        </div>
        <div className="flex-1 min-w-0">
          <div className="relative h-48" onPointerLeave={() => setAt(null)}>
            {[0, 50, 100].map((line) => (
              <div
                key={line}
                className="right-0 left-0 absolute border-border/70 border-t"
                style={{ top: `${line}%` }}
              />
            ))}
            <div
              className={cn(
                'absolute inset-0 flex items-end',
                bars.length > 120 ? 'gap-0' : bars.length > 45 ? 'gap-px' : 'gap-0.5',
              )}
            >
              {bars.map((bar, i) => (
                <div
                  key={bar.key}
                  className={cn(
                    'flex flex-1 justify-center items-end rounded-t-md h-full',
                    at === i && 'bg-muted',
                  )}
                  onPointerEnter={() => setAt(i)}
                  onPointerDown={() => setAt(i)}
                >
                  <div
                    className={cn(
                      'relative flex items-end rounded-t-[4px] w-full max-w-7 overflow-hidden',
                      tint,
                      bars.length > 120 && 'rounded-t-none',
                    )}
                    style={{
                      height: `${(bar.value / top) * 100}%`,
                      minHeight: bar.value > 0 ? 2 : 0,
                    }}
                  >
                    {names && bar.value > 0 && (
                      <div
                        className="bg-primary w-full"
                        style={{ height: `${((bar.part ?? 0) / bar.value) * 100}%` }}
                      />
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
          <div className="flex justify-between mt-1.5 text-muted-foreground text-xs tabular-nums">
            {marks.map((bar, i) => (
              <span key={i}>{bar?.mark}</span>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

/** One row of RankList: something the period's views are counted by, and its views. */
export interface Rank {
  key: string
  label: string
  /** Where the row's name leads (a page of the site), opened beside this one. */
  href?: string
  /** The row of the views it is not known of: shown dimmed. */
  dim?: boolean
  views: number
  /** The different addresses its views came from, where that is counted. */
  addresses?: number
}

/**
 * The period's views by page, country or site address, most first: each row's views, its share
 * of all the period's views, and behind the row a bar as long as its views are to the most.
 * The first `limit` rows show; a button below shows the rest.
 */
export function RankList({
  testId,
  name,
  rows,
  total,
  empty,
  limit = 10,
  mono = false,
}: {
  testId: string
  /** What the rows are: the heading of their names. */
  name: string
  rows: Rank[]
  /** All the period's views, of which each row has a share. */
  total: number
  /** What to say when there are no rows. */
  empty: string
  limit?: number
  /** Whether the names are addresses, set in the fixed-width face. */
  mono?: boolean
}) {
  const { t, i18n } = useTranslation()
  const [all, setAll] = useState(false)
  const number = new Intl.NumberFormat(i18n.language)
  const percent = new Intl.NumberFormat(i18n.language, {
    style: 'percent',
    maximumFractionDigits: 1,
  })
  if (rows.length === 0) return <p className="text-muted-foreground text-sm">{empty}</p>
  const most = Math.max(1, ...rows.map((row) => row.views))
  const counted = rows.some((row) => row.addresses !== undefined)
  const columns = counted
    ? 'grid-cols-[minmax(0,1fr)_3.5rem_4rem_3.5rem]'
    : 'grid-cols-[minmax(0,1fr)_4rem_3.5rem]'
  return (
    <div>
      <div
        className={cn('gap-2 grid px-3 pb-1.5 text-muted-foreground text-xs', columns)}
        aria-hidden="true"
      >
        <span>{name}</span>
        {counted && <span className="text-right">{t('visits.addresses')}</span>}
        <span className="text-right">{t('visits.views')}</span>
        <span className="text-right">{t('visits.share')}</span>
      </div>
      <ol className="space-y-1" data-testid={testId}>
        {(all ? rows : rows.slice(0, limit)).map((row) => (
          <li key={row.key} className="relative rounded-lg overflow-hidden">
            <div
              className="left-0 absolute inset-y-0 bg-secondary rounded-lg"
              style={{ width: `${(row.views / most) * 100}%` }}
              aria-hidden="true"
            />
            <div
              className={cn('relative items-center gap-2 grid px-3 py-1.5 tabular-nums', columns)}
            >
              <span
                className={cn(
                  'truncate',
                  mono && 'font-mono text-xs',
                  row.dim && 'text-muted-foreground',
                )}
                title={row.label}
              >
                {row.href ? (
                  <a
                    href={row.href}
                    target="_blank"
                    rel="noreferrer"
                    className="rounded-sm focus-visible:outline-2 focus-visible:outline-ring hover:underline underline-offset-2"
                  >
                    {row.label}
                  </a>
                ) : (
                  row.label
                )}
              </span>
              {counted && (
                <span className="text-muted-foreground text-right">
                  <span className="sr-only">{t('visits.addresses')} </span>
                  {number.format(row.addresses ?? 0)}
                </span>
              )}
              <span className="font-semibold text-right">
                <span className="sr-only">{t('visits.views')} </span>
                {number.format(row.views)}
              </span>
              <span className="text-muted-foreground text-xs text-right">
                {percent.format(total ? row.views / total : 0)}
              </span>
            </div>
          </li>
        ))}
      </ol>
      {rows.length > limit && (
        <Button
          variant="ghost"
          size="sm"
          className="mt-2"
          aria-expanded={all}
          onClick={() => setAll(!all)}
        >
          {all ? t('visits.showLess') : t('visits.showAll', { n: rows.length })}
        </Button>
      )}
    </div>
  )
}
