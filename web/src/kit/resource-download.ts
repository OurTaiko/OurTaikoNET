import { endpoints } from './api/endpoints'
import { fileUrls } from './distribution'

/** One of a chart's stored files as its manifest lists it: a path under the distribution root. */
export type ChartResource = { path: string; sha256: string; size: number; contentType: string }
export type ChartResources = Partial<
  Record<'tja' | 'audio' | 'cover' | 'preview' | 'bgimage' | 'movie', ChartResource>
>

/**
 * A chart's stored files (GET .../charts/{id}/resources), or null when the API has no manifest
 * for it (404). Reading them from their paths lets a mainland visitor take them from the mirror;
 * the API's own file routes always send the canonical root.
 */
export async function readChartResources(
  chartId: string,
  signal?: AbortSignal,
): Promise<ChartResources | null> {
  const response = await fetch(endpoints.resources(chartId), { signal })
  if (response.status === 404) return null
  if (!response.ok) throw new Error('RESOURCES_UNAVAILABLE')
  const { resources } = (await response.json()) as { resources?: ChartResources }
  return resources ?? {}
}

/** A file to download: its address, and the address to try once that fails (if any). */
export type ResourceFile = {
  url: string
  fallbackUrl?: string
  size?: number
  sha256?: string
}

/** The file at `path` under the distribution root, as verifiedResource downloads it. */
export async function pathResource(
  path: string,
  check: { size?: number; sha256?: string } = {},
): Promise<ResourceFile> {
  const { url, fallbackUrl } = await fileUrls(path)
  return { url, fallbackUrl: fallbackUrl || undefined, ...check }
}

/** How long an attempt may go without receiving a byte before the next address is tried. */
const stallMs = 30_000

function resourceUrl(value: string): string {
  const url = new URL(value, globalThis.location?.origin ?? 'http://localhost')
  if (url.username || url.password || url.hash || !['https:', 'http:'].includes(url.protocol))
    throw new Error('RESOURCE_URL_INVALID')
  if (url.protocol !== 'https:' && !['localhost', '127.0.0.1', '[::1]'].includes(url.hostname))
    throw new Error('RESOURCE_URL_INVALID')
  return value
}

export async function verifiedResource(
  file: ResourceFile,
  signal?: AbortSignal,
): Promise<ArrayBuffer> {
  const urls = [...new Set([file.url, file.fallbackUrl].filter((v): v is string => !!v))]
  urls.forEach(resourceUrl)
  let error: unknown
  for (const url of urls) {
    // An attempt fails when no bytes arrive for a while, not after a fixed total time:
    // a large package file over a slow line keeps going as long as it makes progress.
    const stalled = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    const progress = () => {
      clearTimeout(timer)
      timer = setTimeout(() => stalled.abort(new Error('RESOURCE_DOWNLOAD_STALLED')), stallMs)
    }
    try {
      progress()
      const attempt = signal ? AbortSignal.any([signal, stalled.signal]) : stalled.signal
      const response = await fetch(url, { signal: attempt, credentials: 'omit' })
      if (!response.ok) throw new Error('RESOURCE_DOWNLOAD_FAILED')
      const limit = file.size ?? 256 * 1024 * 1024
      if (!Number.isSafeInteger(limit) || limit <= 0 || limit > 256 * 1024 * 1024 || !response.body)
        throw new Error('RESOURCE_INTEGRITY_FAILED')
      const reader = response.body.getReader()
      const chunks: Uint8Array[] = []
      let size = 0
      try {
        for (;;) {
          const { done, value } = await reader.read()
          if (done) break
          progress()
          size += value.byteLength
          if (size > limit) throw new Error('RESOURCE_INTEGRITY_FAILED')
          chunks.push(value)
        }
      } finally {
        await reader.cancel()
      }
      if (file.size !== undefined && size !== file.size)
        throw new Error('RESOURCE_INTEGRITY_FAILED')
      const joined = new Uint8Array(size)
      let at = 0
      for (const chunk of chunks) {
        joined.set(chunk, at)
        at += chunk.byteLength
      }
      const bytes = joined.buffer
      if (file.sha256) {
        const hash = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))
        if (Array.from(hash, (b) => b.toString(16).padStart(2, '0')).join('') !== file.sha256)
          throw new Error('RESOURCE_INTEGRITY_FAILED')
      }
      return bytes
    } catch (reason) {
      if (signal?.aborted) throw reason
      error = reason
    } finally {
      clearTimeout(timer)
    }
  }
  throw error ?? new Error('RESOURCE_DOWNLOAD_FAILED')
}
