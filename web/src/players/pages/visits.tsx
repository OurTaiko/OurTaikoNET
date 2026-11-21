import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
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
import { playerQueries } from '@players/queries'
import { useSession } from '@players/session'
import { browserZone } from '@sitekit/shell/visits'

// The periods offered, in days; today is the last day of each.
const periods = [7, 30, 90, 365]

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

type Bar = { key: string; label: string; mark: string; value: number }

// One number a day (or an hour) as bars on one scale from zero. A whole column answers the
// pointer, not only its bar, and says its label and number above the plot; the table below
// the charts has every day for those who do not point.
function Bars({ bars, format }: { bars: Bar[]; format: (n: number) => string }) {
  const [at, setAt] = useState<number | null>(null)
  const top = scaleTop(Math.max(0, ...bars.map((bar) => bar.value)))
  const shown = at !== null && at < bars.length ? bars[at] : null
  const marks = [bars[0], bars[Math.floor((bars.length - 1) / 2)], bars[bars.length - 1]]
  return (
    <div>
      <p className="h-5 text-sm tabular-nums" aria-hidden="true">
        {shown && (
          <>
            <span className="text-muted-foreground">{shown.label}</span>{' '}
            <span className="font-semibold">{format(shown.value)}</span>
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
                bars.length > 120 ? 'gap-0' : bars.length > 45 ? 'gap-px' : 'gap-0.5',
              )}
            >
              {bars.map((bar, i) => {
                const value = bar.value
                return (
                  <div
                    key={bar.key}
                    className="flex flex-1 justify-center items-end h-full"
                    onPointerEnter={() => setAt(i)}
                    onPointerDown={() => setAt(i)}
                  >
                    <div
                      className={cn(
                        'bg-primary rounded-t-[4px] w-full max-w-7 transition-opacity',
                        bars.length > 120 && 'rounded-t-none',
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
            {marks.map((bar, i) => (
              <span key={i}>{bar?.mark}</span>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

// How much the site is visited: today, the chosen period day by day and hour by hour, the
// pages seen most and the countries the views came from, all in the reader's time zone. Only the site's administrator sees it;
// everyone sees the totals at the foot of every page.
export function VisitsPage() {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const allowed = !!session.user?.siteAdmin
  const [period, setPeriod] = useState(30)
  const [zone] = useState(browserZone)
  const visits = useQuery({ ...playerQueries.visits(period, zone), enabled: allowed })
  const answer = visits.data
  // What was read before stays on show when reading it again fails.
  const error = answer ? '' : (visits.error?.message ?? '')

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
  const countries = report?.countries ?? []
  const mostFrom = Math.max(1, ...countries.map((country) => country.views))
  // A country by its name in the reader's language; one the browser cannot name by its letters.
  const regions = new Intl.DisplayNames(i18n.language, { type: 'region', fallback: 'code' })
  const countryName = (code: string) => {
    if (!code) return t('visits.unknownCountry')
    try {
      return regions.of(code) ?? code
    } catch {
      return code
    }
  }
  const dayBars = (pick: (day: (typeof days)[number]) => number): Bar[] =>
    days.map((day) => ({ key: day.day, label: day.day, mark: day.day.slice(5), value: pick(day) }))
  const hourBars: Bar[] = (report?.hours ?? []).map((value, h) => ({
    key: String(h),
    label: t('visits.hour', { h }),
    mark: String(h),
    value,
  }))

  return (
    <div className="space-y-6" data-testid="visits-panel">
      <div className="flex flex-wrap justify-between items-end gap-4">
        <div className="space-y-1">
          <h1 className="font-semibold text-2xl tracking-tight">{t('visits.heading')}</h1>
          <p className="max-w-2xl text-muted-foreground text-sm">{t('visits.about')}</p>
          {report && (
            <p className="text-muted-foreground text-xs" data-testid="visits-zone">
              {t('visits.zone', { zone: report.zone })}
            </p>
          )}
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
                <Bars bars={dayBars((day) => day.views)} format={format} />
              </CardContent>
            </Card>
            <Card className="rounded-3xl">
              <CardHeader>
                <CardTitle>{t('visits.visitorsHeading')}</CardTitle>
              </CardHeader>
              <CardContent>
                <Bars bars={dayBars((day) => day.visitors)} format={format} />
              </CardContent>
            </Card>
          </div>

          {hourBars.length > 0 && (
            <Card className="rounded-3xl">
              <CardHeader>
                <CardTitle>{t('visits.hoursHeading')}</CardTitle>
              </CardHeader>
              <CardContent>
                <Bars bars={hourBars} format={format} />
              </CardContent>
            </Card>
          )}

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
                          {page.page}
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

          <Card className="rounded-3xl">
            <CardHeader>
              <CardTitle>{t('visits.countriesHeading')}</CardTitle>
            </CardHeader>
            <CardContent>
              {countries.length === 0 ? (
                <p className="text-muted-foreground text-sm">{t('visits.noPages')}</p>
              ) : (
                <Table data-testid="visits-countries">
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('visits.country')}</TableHead>
                      <TableHead className="text-right">{t('visits.views')}</TableHead>
                      <TableHead className="text-right">{t('visits.addresses')}</TableHead>
                      <TableHead className="w-1/3 text-right">{t('visits.share')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {countries.map((country) => (
                      <TableRow key={country.country}>
                        <TableCell className={cn(!country.country && 'text-muted-foreground')}>
                          {countryName(country.country)}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {format(country.views)}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {format(country.addresses)}
                        </TableCell>
                        <TableCell>
                          <div className="flex justify-end items-center gap-2">
                            <div className="flex-1 max-sm:hidden">
                              <div
                                className="bg-primary rounded-full h-1.5"
                                style={{ width: `${(country.views / mostFrom) * 100}%` }}
                              />
                            </div>
                            <span className="w-14 text-muted-foreground text-xs text-right tabular-nums">
                              {percent.format(views ? country.views / views : 0)}
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
