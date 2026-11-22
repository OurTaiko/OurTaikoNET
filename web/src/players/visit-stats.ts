// What the visits page works out from a report (./pages/visits) before drawing it.
import type { VisitDay } from './api/types'

/** A round number at or above `most` whose half is round too: the top of a chart's scale. */
export function scaleTop(most: number) {
  if (most <= 4) return 4
  const power = 10 ** Math.floor(Math.log10(most))
  const step = (power === 1 ? [10] : [1, 2, 4, 5, 10]).find((s) => s * power >= most) ?? 10
  return step * power
}

/**
 * A period's days as a whole: its views, its visitors (each day's, summed), the views of an
 * average day, and the day seen most (the earliest of equals; null when nothing was seen).
 */
export function periodTotals(days: VisitDay[]) {
  let views = 0
  let visitors = 0
  let peak: VisitDay | null = null
  for (const day of days) {
    views += day.views
    visitors += day.visitors
    if (day.views > (peak?.views ?? 0)) peak = day
  }
  return { views, visitors, average: days.length ? Math.round(views / days.length) : 0, peak }
}

/** The hour of the day seen most (the earliest of equals), or null when none was seen at all. */
export function peakHour(hours: number[]): number | null {
  let peak: number | null = null
  hours.forEach((views, hour) => {
    if (views > (peak === null ? 0 : hours[peak])) peak = hour
  })
  return peak
}

/** The pages whose address has `text` in it, whatever its case; all of them for no text. */
export function pagesWith<T extends { page: string }>(pages: T[], text: string): T[] {
  const wanted = text.trim().toLowerCase()
  return wanted ? pages.filter((page) => page.page.toLowerCase().includes(wanted)) : pages
}
