// Where this visitor's public files come from. The API names every public file by its path
// under the distribution root ("fanmade/<sha256>.webp", "3dchara/cos/0.glb", ...); the server
// says once per page load which root this visitor reads (GET /api/v2/distribution/source): the
// mirror for mainland visitors, the canonical root otherwise, this server's own in development.
// This file is the one place that joins a path to a root. Nothing is kept in the browser.
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

// A root is an https address (or one on this site, as in development) without credentials,
// query or fragment, ending in a slash; anything else is not used.
function readRoot(value: unknown): string | null {
  if (typeof value !== 'string') return null
  try {
    const here = typeof location === 'undefined' ? undefined : location.origin
    const url = new URL(value)
    const trusted = url.protocol === 'https:' || (here !== undefined && url.origin === here)
    if (
      !trusted ||
      url.username ||
      url.password ||
      url.search ||
      url.hash ||
      !url.pathname.endsWith('/')
    )
      return null
    return url.href
  } catch {
    return null
  }
}

/** The server's answer as the client uses it; the canonical root for anything untrusted. */
export function readSource(answer: unknown): DistributionSource {
  if (!answer || typeof answer !== 'object') return canonicalSource
  const fields = answer as Partial<DistributionSource>
  const baseUrl = readRoot(fields.baseUrl)
  if (!baseUrl) return canonicalSource
  return {
    mirror: fields.mirror === true,
    baseUrl,
    canonicalBaseUrl: readRoot(fields.canonicalBaseUrl) ?? canonicalBaseUrl,
  }
}

/** The two roots a file can be read from, as fileUrl and its fallbacks need them. */
export type FileRoot = Pick<DistributionSource, 'baseUrl' | 'canonicalBaseUrl'>

let known: DistributionSource | null = null
const listeners = new Set<(source: DistributionSource) => void>()

// A mirror answer is kept in the browser, so later visits use it at once; refreshSource asks
// again (after moving between networks, say). The canonical root is not kept: it is asked for
// on every page load, so a visitor the server could not place yet is placed once it can. A
// browser that keeps nothing asks once per page load.
const storageKey = 'ourtaiko.distributionSource'

function stored(): DistributionSource | null {
  try {
    const text = localStorage.getItem(storageKey)
    if (!text) return null
    const answer = readSource(JSON.parse(text))
    // A stored answer this page cannot trust (readSource's fallback), or a canonical one kept
    // by an earlier release, is asked again.
    return answer.mirror ? answer : null
  } catch {
    return null
  }
}

function store(answer: DistributionSource) {
  try {
    if (!answer.mirror) {
      localStorage.removeItem(storageKey)
      return
    }
    localStorage.setItem(storageKey, JSON.stringify(answer))
  } catch {
    // Kept for this page load only.
  }
}

// Asked once per page load unless the browser kept the answer: every picture, download and the
// footer share it.
let asked: Promise<DistributionSource> | null = null
export function askSource(): Promise<DistributionSource> {
  if (asked) return asked
  const kept = stored()
  if (kept) {
    known = kept
    asked = Promise.resolve(kept)
    return asked
  }
  asked = fetch(`${SITE_API_ROOT}/distribution/source`, {
    headers: { Accept: 'application/json' },
    credentials: 'omit',
    cache: 'no-store',
  })
    .then((response) => (response.ok ? response.json() : null))
    .then((answer) => {
      const source = readSource(answer)
      // Only an answer the server gave is kept; a failed request is asked again next time.
      if (answer) store(source)
      return source
    })
    .catch(() => canonicalSource)
    .then((answer) => (known = answer))
  return asked
}

/** Forgets the kept answer and asks the server again; every page part shows the new root. */
export async function refreshSource(): Promise<DistributionSource> {
  try {
    localStorage.removeItem(storageKey)
  } catch {
    // Nothing was kept.
  }
  asked = null
  known = null
  const answer = await askSource()
  listeners.forEach((listener) => listener(answer))
  return answer
}

/** This visitor's source; null only until it is known (kept, or the one request answered). */
export function useDistributionSource(): DistributionSource | null {
  const [source, setSource] = useState<DistributionSource | null>(known ?? stored())
  useEffect(() => {
    let gone = false
    const listener = (answer: DistributionSource) => {
      if (!gone) setSource(answer)
    }
    listeners.add(listener)
    void askSource().then(listener)
    return () => {
      gone = true
      listeners.delete(listener)
    }
  }, [])
  return source
}

// A path as the API gives it: relative to the root, never starting with a slash.
const clean = (path: string) => path.replace(/^\/+/, '')

/** The address of the file at `path` on the visitor's root; empty when there is no file. */
export function fileUrl(
  path: string | null | undefined,
  source: Pick<DistributionSource, 'baseUrl'>,
): string {
  return path ? source.baseUrl + clean(path) : ''
}

/** The address of the file at `path` on the canonical root; empty when there is no file. */
export function canonicalFileUrl(
  path: string | null | undefined,
  source: Pick<DistributionSource, 'canonicalBaseUrl'> = canonicalSource,
): string {
  return path ? source.canonicalBaseUrl + clean(path) : ''
}

/**
 * Where to try a file again once loading it from the visitor's root failed: the canonical
 * root, or empty when that is where it already came from (there is nothing else to try).
 */
export function fallbackFileUrl(path: string | null | undefined, source: FileRoot): string {
  const canonical = canonicalFileUrl(path, source)
  return canonical && canonical !== fileUrl(path, source) ? canonical : ''
}

/** fileUrl and fallbackFileUrl, once the visitor's root is known (askSource). */
export async function fileUrls(path: string): Promise<{ url: string; fallbackUrl: string }> {
  const source = await askSource()
  return { url: fileUrl(path, source), fallbackUrl: fallbackFileUrl(path, source) }
}

/**
 * Fetches `url`, and `fallbackUrl` (when there is one) once that fails: a network error or an
 * error status. The last answer is returned as it is, so the caller still sees a failed status;
 * a cancelled request is not tried again.
 */
export async function fetchWithFallback(
  url: string,
  fallbackUrl: string | undefined,
  init?: RequestInit,
): Promise<Response> {
  if (!fallbackUrl || fallbackUrl === url) return fetch(url, init)
  try {
    const response = await fetch(url, init)
    if (response.ok || init?.signal?.aborted) return response
  } catch (error) {
    if (init?.signal?.aborted) throw error
  }
  return fetch(fallbackUrl, init)
}

/** Fetches the file at `path` from the visitor's root, then the canonical one (fetchWithFallback). */
export async function fetchFile(path: string, init?: RequestInit): Promise<Response> {
  const { url, fallbackUrl } = await fileUrls(path)
  return fetchWithFallback(url, fallbackUrl, init)
}

/**
 * The address a picture at `path` is shown from after `failures` of its addresses failed to
 * load: nothing while the visitor's root is unknown or there is no file, then the visitor's
 * root, then the canonical root, then nothing.
 */
export function pictureUrl(
  path: string | null | undefined,
  source: FileRoot | null,
  failures: number,
): string {
  if (!path || !source) return ''
  if (failures === 0) return fileUrl(path, source)
  return failures === 1 ? fallbackFileUrl(path, source) : ''
}

/** pictureUrl for a component: call `onError` when the shown address fails to load. */
export function useFileUrl(path: string | null | undefined) {
  const source = useDistributionSource()
  const [failed, setFailed] = useState({ path: '', count: 0 })
  const count = path && failed.path === path ? failed.count : 0
  const shown = pictureUrl(path, source, count)
  return {
    source: shown || undefined,
    onError: () => {
      if (path && shown) setFailed({ path, count: count + 1 })
    },
  }
}
