import { createHash } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { verifiedResource } from './resource-download'

const bytes = 'verified audio'
const hash = createHash('sha256').update(bytes).digest('hex')
const mirror = 'https://mirror.example/song.ogg'
const canonical = 'https://distrib.ourtaiko.org/song.ogg?signature=a%2Fb'
afterEach(() => vi.unstubAllGlobals())

describe('regional resource downloads', () => {
  it.each(['status', 'hash', 'size', 'network'])(
    'falls back once after mirror %s failure and verifies canonical bytes',
    async (failure) => {
      const fetch = vi.fn(async (url: string) => {
        if (url === canonical) return new Response(bytes)
        if (failure === 'network') throw new TypeError('offline')
        return new Response(failure === 'hash' ? 'x'.repeat(bytes.length) : 'x', {
          status: failure === 'status' ? 503 : 200,
        })
      })
      vi.stubGlobal('fetch', fetch)
      const result = await verifiedResource({
        url: mirror,
        fallbackUrl: canonical,
        size: bytes.length,
        sha256: hash,
      })
      expect(new TextDecoder().decode(result)).toBe(bytes)
      expect(fetch.mock.calls.map(([url]) => url)).toEqual([mirror, canonical])
      for (const call of vi.mocked(globalThis.fetch).mock.calls)
        expect(call[1]?.credentials).toBe('omit')
    },
  )
  it('gives up on a stalled mirror but not on a slow one that keeps sending', async () => {
    vi.useFakeTimers()
    try {
      const fetch = vi.fn(async (url: string, init?: RequestInit) => {
        if (url === canonical) {
          // Slow but steady: one byte every 20 s, longer in total than any stall limit.
          let sent = 0
          return new Response(
            new ReadableStream({
              async pull(controller) {
                await new Promise((resolve) => setTimeout(resolve, 20_000))
                controller.enqueue(new TextEncoder().encode(bytes[sent++]))
                if (sent === bytes.length) controller.close()
              },
            }),
          )
        }
        return new Response(
          new ReadableStream({
            start(controller) {
              init?.signal?.addEventListener('abort', () => controller.error(init.signal!.reason))
            },
          }),
        )
      })
      vi.stubGlobal('fetch', fetch)
      const result = verifiedResource({
        url: mirror,
        fallbackUrl: canonical,
        size: bytes.length,
        sha256: hash,
      })
      await vi.advanceTimersByTimeAsync(30_000 + 20_000 * bytes.length)
      expect(new TextDecoder().decode(await result)).toBe(bytes)
      expect(fetch.mock.calls.map(([url]) => url)).toEqual([mirror, canonical])
    } finally {
      vi.useRealTimers()
    }
  })
  it('does not accept corrupt bytes from either origin', async () => {
    const fetch = vi.fn(async () => new Response('x'.repeat(bytes.length)))
    vi.stubGlobal('fetch', fetch)
    await expect(
      verifiedResource({ url: mirror, fallbackUrl: canonical, sha256: hash }),
    ).rejects.toThrow('RESOURCE_INTEGRITY_FAILED')
    expect(fetch).toHaveBeenCalledTimes(2)
  })
  it('does not try another origin after user cancellation', async () => {
    const cancel = new AbortController()
    const fetch = vi.fn(async () => {
      cancel.abort()
      throw new DOMException('aborted', 'AbortError')
    })
    vi.stubGlobal('fetch', fetch)
    await expect(
      verifiedResource({ url: mirror, fallbackUrl: canonical }, cancel.signal),
    ).rejects.toThrow('aborted')
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('takes a path from the visitor root and falls back to the canonical one', async () => {
    vi.resetModules()
    const { pathResource } = await import('./resource-download')
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        Response.json({
          mirror: true,
          baseUrl: 'https://mirror.example/ourtaiko/',
          canonicalBaseUrl: 'https://distrib.ourtaiko.org/',
        }),
      ),
    )
    expect(await pathResource('database/song.ogg', { size: 3 })).toEqual({
      url: 'https://mirror.example/ourtaiko/database/song.ogg',
      fallbackUrl: 'https://distrib.ourtaiko.org/database/song.ogg',
      size: 3,
    })
  })
  it('has no fallback on the canonical root', async () => {
    vi.resetModules()
    const { pathResource } = await import('./resource-download')
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('', { status: 503 })),
    )
    expect(await pathResource('database/song.ogg')).toEqual({
      url: 'https://distrib.ourtaiko.org/database/song.ogg',
      fallbackUrl: undefined,
    })
  })
})
