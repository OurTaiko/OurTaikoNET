export type ResourceFile = {
  url: string
  fallbackUrl?: string
  size?: number
  sha256?: string
}

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
  const urls = [...new Set([file.url, file.fallbackUrl ?? originResource(file.url)].filter((v): v is string => !!v))]
  urls.forEach(resourceUrl)
  let error: unknown
  for (const url of urls) {
    try {
      const timeout = AbortSignal.timeout(urls.length > 1 && url === urls[0] ? 30_000 : 120_000)
      const attempt = signal ? AbortSignal.any([signal, timeout]) : timeout
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
    }
  }
  throw error ?? new Error('RESOURCE_DOWNLOAD_FAILED')
}

// API file routes accept origin=1 to bypass regional routing after a client-side
// CDN error. Never add routing parameters to external or signed object URLs.
export function originResource(value: string): string | null {
  const origin = globalThis.location?.origin ?? 'http://localhost'
  const url = new URL(value, origin)
  if (
    url.origin !== origin ||
    !/^\/api\/v1\/sources\/(fanmade|official)\/(charts|playlists)\/[^/]+\/(tja|audio|preview|cover|bgimage|movie)$/.test(
      url.pathname,
    ) ||
    url.searchParams.get('origin') === '1'
  )
    return null
  url.searchParams.set('origin', '1')
  return value.startsWith('/') ? url.pathname + url.search : url.toString()
}

export function retryResourceOrigin(element: {
  src: string
  getAttribute(name: string): string | null
}): boolean {
  const fallback = originResource(element.getAttribute('src') ?? element.src)
  if (!fallback) return false
  element.src = fallback
  return true
}

export async function fetchResource(url: string, options?: RequestInit): Promise<Response> {
  try {
    const response = await fetch(url, options)
    if (response.ok || !originResource(url)) return response
  } catch (error) {
    if (options?.signal?.aborted || !originResource(url)) throw error
  }
  return fetch(originResource(url)!, options)
}
