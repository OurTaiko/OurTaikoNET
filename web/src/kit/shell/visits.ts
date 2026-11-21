// How much the site is visited. The frame (./shell) tells the server each page it shows, and
// the footer shows the totals. Nothing is kept in the browser: no cookie, no id. The server
// keeps each view with its time, the address it came from and that address's country, and
// tells one day's visitors apart by salted hashes (the backend's internal/visits); its days
// are those of the time zone it is asked in.
import { useEffect, useState } from 'react'

const address = '/api/v2/visits'

export type VisitCount = { views: number; visitors: number }
export type VisitSummary = { total: VisitCount; today: VisitCount; since: string }

/** The browser's time zone (Asia/Shanghai), in which the server is asked; empty: UTC. */
export function browserZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone ?? ''
  } catch {
    return ''
  }
}

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
  const zone = browserZone()
  asked ??= fetch(zone ? `${address}?tz=${encodeURIComponent(zone)}` : address, {
    headers: { Accept: 'application/json' },
    credentials: 'omit',
  })
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
