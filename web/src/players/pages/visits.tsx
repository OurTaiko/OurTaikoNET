import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ArrowsClockwiseIcon } from '@phosphor-icons/react'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Input } from '@sitekit/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@sitekit/components/ui/table'
import { cn } from '@sitekit/lib/utils'
import { BarChart, RankList } from '@players/components/visit-charts'
import type { Bar } from '@players/components/visit-charts'
import { Notice } from '@players/notice'
import { playerQueries } from '@players/queries'
import { useSession } from '@players/session'
import { pagesWith, peakHour, periodTotals } from '@players/visit-stats'
import { browserZone } from '@sitekit/shell/visits'

// The periods offered, in days; today is the last day of each. The address names any but the
// first shown (?days=90), so a period can be come back to.
const periods = [7, 30, 90, 365]
const usual = 30

/** One of the three counts at the top: its views large, and what else is known of it below. */
function Count({
  title,
  views,
  lines,
}: {
  title: string
  views: string
  lines: { label: string; value?: string }[]
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-1 px-6 max-md:py-4 md:first:pl-0 md:last:pr-0 max-md:first:pt-0 max-md:last:pb-0 max-md:px-0 min-w-0">
      <h2 className="font-bold text-sm">{title}</h2>
      <p className="font-extrabold text-4xl tabular-nums tracking-tight">
        {views}
        <span className="ml-1.5 font-normal text-muted-foreground text-sm tracking-normal">
          {t('visits.views')}
        </span>
      </p>
      {lines.map((line) => (
        <p key={line.label} className="text-muted-foreground text-sm tabular-nums">
          {line.label}
          {line.value !== undefined && (
            <span className="ml-1.5 font-semibold text-foreground">{line.value}</span>
          )}
        </p>
      ))}
    </div>
  )
}

// How much the site is visited: today, the chosen period day by day and hour by hour, the
// pages seen most, the countries the views came from and the site's addresses they were seen
// at, all in the reader's time zone. Only the site's administrator sees it;
// everyone sees the totals at the foot of every page.
export function VisitsPage() {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const allowed = !!session.user?.siteAdmin
  const [params, setParams] = useSearchParams()
  const asked = Number(params.get('days'))
  const period = periods.includes(asked) ? asked : usual
  const setPeriod = (n: number) =>
    setParams(n === usual ? {} : { days: String(n) }, { replace: true })
  const [search, setSearch] = useState('')
  const [zone] = useState(browserZone)
  const visits = useQuery({ ...playerQueries.visits(period, zone), enabled: allowed })
  const answer = visits.data
  // What was read before stays on show when reading it again fails.
  const error = answer ? '' : (visits.error?.message ?? '')

  if (session.loading) return null
  if (!allowed) return <Navigate to="/" replace />

  const number = new Intl.NumberFormat(i18n.language)
  const format = (n: number) => number.format(n)
  // The report of the period before stays up, dimmed, while the next one is read.
  const report = answer?.report
  const stale = answer?.period !== period
  const days = report?.days ?? []
  const { views, visitors, average, peak } = periodTotals(days)
  const pages = report?.pages ?? []
  const found = pagesWith(pages, search)
  const busiest = peakHour(report?.hours ?? [])
  const countries = report?.countries ?? []
  const sites = report?.sites ?? []
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
  // A day in full, with its day of the week: 2026-11-21 is a date of the report's time zone,
  // so it is read and written as one of UTC to stay that date.
  const fullDay = new Intl.DateTimeFormat(i18n.language, {
    dateStyle: 'medium',
    timeZone: 'UTC',
  })
  const weekday = new Intl.DateTimeFormat(i18n.language, { weekday: 'short', timeZone: 'UTC' })
  const dayBars: Bar[] = days.map((day) => {
    const date = new Date(`${day.day}T00:00:00Z`)
    return {
      key: day.day,
      label: `${fullDay.format(date)} ${weekday.format(date)}`,
      mark: day.day.slice(5),
      value: day.views,
      part: day.visitors,
    }
  })
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
          {report && (
            <p className="text-muted-foreground text-xs" data-testid="visits-zone">
              {t('visits.zone', { zone: report.zone })}
            </p>
          )}
        </div>
        <div className="flex items-center gap-1.5">
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
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={t('visits.refresh')}
            title={t('visits.refresh')}
            disabled={visits.isFetching}
            onClick={() => void visits.refetch()}
          >
            <ArrowsClockwiseIcon
              className={cn(visits.isFetching && 'motion-safe:animate-spin')}
              aria-hidden="true"
            />
          </Button>
        </div>
      </div>
      {error && <Notice>{error}</Notice>}

      {report && (
        <div className={cn('space-y-4 transition-opacity', stale && 'opacity-60')}>
          <Card>
            <CardContent className="grid grid-cols-1 md:grid-cols-3 divide-border max-md:divide-y md:divide-x">
              <Count
                title={t('visits.today')}
                views={format(report.today.views)}
                lines={[{ label: t('visits.visitors'), value: format(report.today.visitors) }]}
              />
              <Count
                title={t('visits.period', { n: answer.period })}
                views={format(views)}
                lines={[
                  { label: t('visits.visitorDays'), value: format(visitors) },
                  {
                    label: [
                      t('visits.average', { n: format(average) }),
                      peak && t('visits.peak', { day: peak.day.slice(5), n: format(peak.views) }),
                    ]
                      .filter(Boolean)
                      .join(t('visits.listComma')),
                  },
                ]}
              />
              <Count
                title={t('visits.total')}
                views={format(report.total.views)}
                lines={[
                  { label: t('visits.visitorDays'), value: format(report.total.visitors) },
                  ...(report.since ? [{ label: t('visits.since', { day: report.since }) }] : []),
                ]}
              />
            </CardContent>
          </Card>

          <div className="gap-4 grid grid-cols-1 xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
            <Card>
              <CardHeader>
                <CardTitle>{t('visits.trendHeading')}</CardTitle>
              </CardHeader>
              <CardContent>
                <BarChart
                  bars={dayBars}
                  format={format}
                  names={{ value: t('visits.views'), part: t('visits.visitors') }}
                />
              </CardContent>
            </Card>
            {hourBars.length > 0 && (
              <Card>
                <CardHeader>
                  <CardTitle>{t('visits.hoursHeading')}</CardTitle>
                </CardHeader>
                <CardContent>
                  <BarChart
                    bars={hourBars}
                    format={format}
                    idle={
                      busiest !== null && (
                        <span className="text-muted-foreground">
                          {t('visits.peakHour', { hour: t('visits.hour', { h: busiest }) })}
                        </span>
                      )
                    }
                  />
                </CardContent>
              </Card>
            )}
          </div>

          <div className="items-start gap-4 grid grid-cols-1 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
            <Card>
              <CardHeader className="flex flex-wrap justify-between items-center gap-3">
                <CardTitle>{t('visits.pagesHeading')}</CardTitle>
                {pages.length > 0 && (
                  <Input
                    type="search"
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    placeholder={t('visits.filterPages')}
                    aria-label={t('visits.filterPages')}
                    className="w-48 h-8"
                  />
                )}
              </CardHeader>
              <CardContent>
                <RankList
                  // A list filtered anew starts folded again.
                  key={search}
                  testId="visits-pages"
                  name={t('visits.page')}
                  total={views}
                  empty={pages.length ? t('visits.noMatch') : t('visits.noPages')}
                  mono
                  rows={found.map((page) => ({
                    key: page.page,
                    label: page.page,
                    href: page.page,
                    views: page.views,
                  }))}
                />
              </CardContent>
            </Card>

            <div className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle>{t('visits.countriesHeading')}</CardTitle>
                </CardHeader>
                <CardContent>
                  <RankList
                    testId="visits-countries"
                    name={t('visits.country')}
                    total={views}
                    empty={t('visits.noPages')}
                    limit={8}
                    rows={countries.map((country) => ({
                      key: country.country,
                      label: countryName(country.country),
                      dim: !country.country,
                      views: country.views,
                      addresses: country.addresses,
                    }))}
                  />
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>{t('visits.sitesHeading')}</CardTitle>
                </CardHeader>
                <CardContent>
                  <RankList
                    testId="visits-sites"
                    name={t('visits.site')}
                    total={views}
                    empty={t('visits.noPages')}
                    limit={8}
                    rows={sites.map((site) => ({
                      key: site.site,
                      label: site.site || t('visits.unknownSite'),
                      dim: !site.site,
                      views: site.views,
                      addresses: site.addresses,
                    }))}
                  />
                </CardContent>
              </Card>
            </div>
          </div>

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

          <details className="px-6 text-muted-foreground text-sm">
            <summary className="cursor-pointer">{t('visits.aboutHeading')}</summary>
            <p className="mt-2 max-w-2xl">{t('visits.about')}</p>
          </details>
        </div>
      )}
    </div>
  )
}
