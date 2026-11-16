// How much the site is visited. The frame (./shell) tells the server each page it shows, and
// the footer shows the totals. Nothing is kept in the browser: no cookie, no id. The server
// counts pages a day and tells one day's visitors apart without keeping who they were (the
// backend's internal/visits).
import { useEffect, useState } from 'react'

const address = '/api/v1/visits'

export type VisitCount = { views: number; visitors: number }
export type VisitSummary = { total: VisitCount; today: VisitCount; since: string }

/**
 * Counts the page at `path` (an address of the site, without its query) once it has stayed
 * for a moment: a page that only passes on to another (an old address, a redirect) is not one.
 */
export function useCountVisit(path: string) {
  useEffect(() => {
    const timer = window.setTimeout(() => {
      // A page drawn ahead of time, or by a program that says so, has no reader.
      if (navigator.webdriver) return
      fetch(address, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path }),
        credentials: 'omit',
        keepalive: true,
      }).catch(() => {})
    }, 600)
    return () => window.clearTimeout(timer)
  }, [path])
}

// Asked once per page load: every footer shows the same answer.
let asked: Promise<VisitSummary | null> | null = null
function ask() {
  asked ??= fetch(address, { headers: { Accept: 'application/json' }, credentials: 'omit' })
    .then((response) => (response.ok ? (response.json() as Promise<VisitSummary>) : null))
    .catch(() => null)
  return asked
}

/** The site's totals; null until they are known, and when they cannot be read. */
export function useVisitSummary(): VisitSummary | null {
  const [summary, setSummary] = useState<VisitSummary | null>(null)
  useEffect(() => {
    let gone = false
    void ask().then((answer) => {
      if (!gone && answer?.total && answer.today) setSummary(answer)
    })
    return () => {
      gone = true
    }
  }, [])
  return summary
}
