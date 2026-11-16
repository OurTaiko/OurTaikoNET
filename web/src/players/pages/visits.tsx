import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate } from 'react-router-dom'
import { api } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { VisitDay, VisitReport } from '@players/api/types'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@sitekit/components/ui/table'
import { cn } from '@sitekit/lib/utils'
import { Notice } from '@players/notice'
import { useSession } from '@players/session-context'

// The periods offered, in days; today is the last day of each.
const periods = [7, 30, 90, 365]
// What the server files the views of pages beyond a day's limit under.
const otherPages = '(other)'

/** A round number at or above `most` whose half is round too: the top of a chart's scale. */
function scaleTop(most: number) {
  if (most <= 4) return 4
  const power = 10 ** Math.floor(Math.log10(most))
  const step = (power === 1 ? [10] : [1, 2, 4, 5, 10]).find((s) => s * power >= most) ?? 10
  return step * power
}

function Figure({ label, value, note }: { label: string; value: string; note?: string }) {
  return (
    <div className="min-w-0">
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className="font-extrabold text-2xl tabular-nums tracking-tight">{value}</p>
      {note && <p className="text-muted-foreground text-xs">{note}</p>}
    </div>
  )
}

// One number a day as bars on one scale from zero. A whole column answers the pointer, not
// only its bar, and says its day and number above the plot; the table below the charts has
// every day for those who do not point.
function DayBars({
  days,
  pick,
  format,
}: {
  days: VisitDay[]
  pick: (day: VisitDay) => number
  format: (n: number) => string
}) {
  const [at, setAt] = useState<number | null>(null)
  const top = scaleTop(Math.max(0, ...days.map(pick)))
  const shown = at !== null && at < days.length ? days[at] : null
  const marks = [days[0], days[Math.floor((days.length - 1) / 2)], days[days.length - 1]]
  return (
    <div>
      <p className="h-5 text-sm tabular-nums" aria-hidden="true">
        {shown && (
          <>
            <span className="text-muted-foreground">{shown.day}</span>{' '}
            <span className="font-semibold">{format(pick(shown))}</span>
          </>
        )}
      </p>
      <div className="flex gap-2 mt-1">
        <div className="flex flex-col justify-between w-10 h-44 text-muted-foreground text-xs text-right tabular-nums leading-none">
          <span>{format(top)}</span>
          <span>{format(top / 2)}</span>
          <span>0</span>
        </div>
        <div className="flex-1 min-w-0">
          <div className="relative h-44" onPointerLeave={() => setAt(null)}>
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
                days.length > 120 ? 'gap-0' : days.length > 45 ? 'gap-px' : 'gap-0.5',
              )}
            >
              {days.map((day, i) => {
                const value = pick(day)
                return (
                  <div
                    key={day.day}
                    className="flex flex-1 justify-center items-end h-full"
                    onPointerEnter={() => setAt(i)}
                    onPointerDown={() => setAt(i)}
                  >
                    <div
                      className={cn(
                        'bg-primary rounded-t-[4px] w-full max-w-7 transition-opacity',
                        days.length > 120 && 'rounded-t-none',
                        at !== null && at !== i && 'opacity-40',
                      )}
                      style={{ height: `${(value / top) * 100}%`, minHeight: value > 0 ? 2 : 0 }}
                    />
                  </div>
                )
              })}
            </div>
          </div>
          <div className="flex justify-between mt-1.5 text-muted-foreground text-xs tabular-nums">
            {marks.map((day, i) => (
              <span key={i}>{day?.day.slice(5)}</span>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

// How much the site is visited: today, the chosen period day by day, and the pages seen most.
// Only the site's administrator sees it; everyone sees the totals at the foot of every page.
export function VisitsPage() {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const allowed = !!session.user?.siteAdmin
  const [period, setPeriod] = useState(30)
  const [answer, setAnswer] = useState<{ period: number; report: VisitReport } | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!allowed) return
    let dropped = false
    api<VisitReport>(endpoints.visitReport(period))
      .then((report) => {
        if (dropped) return
        setAnswer({ period, report })
        setError('')
      })
      .catch((e: Error) => {
        if (!dropped) setError(e.message)
      })
    return () => {
      dropped = true
    }
  }, [allowed, period])

  if (session.loading) return null
  if (!allowed) return <Navigate to="/" replace />

  const number = new Intl.NumberFormat(i18n.language)
  const format = (n: number) => number.format(n)
  const percent = new Intl.NumberFormat(i18n.language, {
    style: 'percent',
    maximumFractionDigits: 1,
  })
  // The report of the period before stays up, dimmed, while the next one is read.
  const report = answer?.report
  const stale = answer?.period !== period
  const days = report?.days ?? []
  const views = days.reduce((sum, day) => sum + day.views, 0)
  const visitors = days.reduce((sum, day) => sum + day.visitors, 0)
  const pages = report?.pages ?? []
  const mostSeen = Math.max(1, ...pages.map((page) => page.views))

  return (
    <div className="space-y-6" data-testid="visits-panel">
      <div className="flex flex-wrap justify-between items-end gap-4">
        <div className="space-y-1">
          <h1 className="font-semibold text-2xl tracking-tight">{t('visits.heading')}</h1>
          <p className="max-w-2xl text-muted-foreground text-sm">{t('visits.about')}</p>
        </div>
        <div className="flex gap-1.5" role="group" aria-label={t('visits.range')}>
          {periods.map((n) => (
            <Button
              key={n}
              size="sm"
              variant={n === period ? 'default' : 'outline'}
              aria-pressed={n === period}
              onClick={() => setPeriod(n)}
            >
              {t('visits.days', { n })}
            </Button>
          ))}
        </div>
      </div>
      {error && <Notice>{error}</Notice>}

      {report && (
        <div className={cn('space-y-6 transition-opacity', stale && 'opacity-60')}>
          <div className="gap-4 grid grid-cols-1 md:grid-cols-3">
            {[
              {
                title: t('visits.today'),
                count: report.today,
                visitorsLabel: t('visits.visitors'),
                note: '',
              },
              {
                title: t('visits.period', { n: answer.period }),
                count: { views, visitors },
                visitorsLabel: t('visits.visitorDays'),
                note: '',
              },
              {
                title: t('visits.total'),
                count: report.total,
                visitorsLabel: t('visits.visitorDays'),
                note: report.since ? t('visits.since', { day: report.since }) : '',
              },
            ].map((tile) => (
              <Card key={tile.title} className="rounded-3xl">
                <CardHeader>
                  <CardTitle className="flex flex-wrap justify-between items-baseline gap-2">
                    {tile.title}
                    {tile.note && (
                      <span className="font-normal text-muted-foreground text-xs">{tile.note}</span>
                    )}
                  </CardTitle>
                </CardHeader>
                <CardContent className="gap-4 grid grid-cols-2">
                  <Figure label={t('visits.views')} value={format(tile.count.views)} />
                  <Figure label={tile.visitorsLabel} value={format(tile.count.visitors)} />
                </CardContent>
              </Card>
            ))}
          </div>

          <div className="gap-4 grid grid-cols-1 xl:grid-cols-2">
            <Card className="rounded-3xl">
              <CardHeader>
                <CardTitle>{t('visits.viewsHeading')}</CardTitle>
              </CardHeader>
              <CardContent>
                <DayBars days={days} pick={(day) => day.views} format={format} />
              </CardContent>
            </Card>
            <Card className="rounded-3xl">
              <CardHeader>
                <CardTitle>{t('visits.visitorsHeading')}</CardTitle>
              </CardHeader>
              <CardContent>
                <DayBars days={days} pick={(day) => day.visitors} format={format} />
              </CardContent>
            </Card>
          </div>

          <Card className="rounded-3xl">
            <CardHeader>
              <CardTitle>{t('visits.pagesHeading')}</CardTitle>
            </CardHeader>
            <CardContent>
              {pages.length === 0 ? (
                <p className="text-muted-foreground text-sm">{t('visits.noPages')}</p>
              ) : (
                <Table data-testid="visits-pages">
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('visits.page')}</TableHead>
                      <TableHead className="text-right">{t('visits.views')}</TableHead>
                      <TableHead className="w-1/3 text-right">{t('visits.share')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {pages.map((page) => (
                      <TableRow key={page.page}>
                        <TableCell className="font-mono text-xs break-all whitespace-normal">
                          {page.page === otherPages ? t('visits.other') : page.page}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {format(page.views)}
                        </TableCell>
                        <TableCell>
                          <div className="flex justify-end items-center gap-2">
                            <div className="flex-1 max-sm:hidden">
                              <div
                                className="bg-primary rounded-full h-1.5"
                                style={{ width: `${(page.views / mostSeen) * 100}%` }}
                              />
                            </div>
                            <span className="w-14 text-muted-foreground text-xs text-right tabular-nums">
                              {percent.format(views ? page.views / views : 0)}
                            </span>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>

          <details className="bg-card shadow-elev-1 px-6 py-4 rounded-3xl">
            <summary className="font-semibold text-sm cursor-pointer">{t('visits.table')}</summary>
            <div className="mt-3 max-h-96 overflow-y-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('visits.day')}</TableHead>
                    <TableHead className="text-right">{t('visits.views')}</TableHead>
                    <TableHead className="text-right">{t('visits.visitors')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {[...days].reverse().map((day) => (
                    <TableRow key={day.day}>
                      <TableCell className="tabular-nums">{day.day}</TableCell>
                      <TableCell className="text-right tabular-nums">{format(day.views)}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {format(day.visitors)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </details>
        </div>
      )}
    </div>
  )
}
