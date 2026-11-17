// Which root this visitor's public files (distrib.ourtaiko.org) come from. The server decides
// by country once per page load (GET /api/v1/distribution/source); pages then build public
// addresses themselves instead of asking for each file. Nothing is kept in the browser.
import { useEffect, useState } from 'react'
import { SITE_API_ROOT } from './api/config'

export const canonicalBaseUrl = 'https://distrib.ourtaiko.org/'

export type DistributionSource = { mirror: boolean; baseUrl: string; canonicalBaseUrl: string }

/** The canonical root, used when the answer is missing or not one we can trust. */
export const canonicalSource: DistributionSource = {
  mirror: false,
  baseUrl: canonicalBaseUrl,
  canonicalBaseUrl,
}

/** Accepts only an https root without credentials, query or fragment, ending in a slash. */
export function readSource(answer: unknown): DistributionSource {
  if (!answer || typeof answer !== 'object') return canonicalSource
  const { mirror, baseUrl } = answer as Partial<DistributionSource>
  if (mirror !== true || typeof baseUrl !== 'string') return canonicalSource
  try {
    const url = new URL(baseUrl)
    if (
      url.protocol !== 'https:' ||
      url.username ||
      url.password ||
      url.search ||
      url.hash ||
      !url.pathname.endsWith('/')
    )
      return canonicalSource
    return { mirror: true, baseUrl: url.href, canonicalBaseUrl }
  } catch {
    return canonicalSource
  }
}

// Asked once per page load: every avatar and the footer share the answer.
let asked: Promise<DistributionSource> | null = null
export function askSource(): Promise<DistributionSource> {
  asked ??= fetch(`${SITE_API_ROOT}/distribution/source`, {
    headers: { Accept: 'application/json' },
    credentials: 'omit',
    cache: 'no-store',
  })
    .then((response) => (response.ok ? response.json() : null))
    .then(readSource)
    .catch(() => canonicalSource)
  return asked
}

let known: DistributionSource | null = null

/** This visitor's source; null only until the one request of this page load is answered. */
export function useDistributionSource(): DistributionSource | null {
  const [source, setSource] = useState<DistributionSource | null>(known)
  useEffect(() => {
    if (known) return
    let gone = false
    void askSource().then((answer) => {
      known = answer
      if (!gone) setSource(answer)
    })
    return () => {
      gone = true
    }
  }, [])
  return source
}

/** Moves a canonical public address onto the visitor's root; anything else is kept. */
export function onSource(original: string, source: DistributionSource): string {
  if (!source.mirror || !original.startsWith(canonicalBaseUrl)) return original
  return source.baseUrl + original.slice(canonicalBaseUrl.length)
}
