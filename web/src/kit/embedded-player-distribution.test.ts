import { afterEach, describe, expect, it, vi } from 'vitest'
import { canonicalPlayerUrl, resolvePlayerUrl } from './embedded-player-source'

afterEach(() => vi.unstubAllGlobals())
const options = () => ({
  manifestUrl: '/api/v1/distribution/player-build',
  fallbackManifestUrl: 'https://distrib.ourtaiko.org/player-build.json',
  parentOrigin: 'https://net.ourtaiko.org',
  signal: new AbortController().signal,
})

describe('regional web player', () => {
  it('retains the OSS prefix and exact build version', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          Response.json({
            path: '/player/0123456789abcdef/index.html',
            baseUrl: 'https://static.sakura-bot.cn/ourtaiko/',
          }),
        ),
    )
    const selected = await resolvePlayerUrl(options())
    const url = new URL(selected)
    expect(url.pathname).toBe('/ourtaiko/player/0123456789abcdef/index.html')
    expect(url.searchParams.get('parentOrigin')).toBe('https://net.ourtaiko.org')
    expect(canonicalPlayerUrl(selected)).toBe(
      'https://distrib.ourtaiko.org/player/0123456789abcdef/index.html?parentOrigin=https%3A%2F%2Fnet.ourtaiko.org',
    )
  })
  it('uses the canonical manifest when the regional API is unavailable', async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(new Response('', { status: 503 }))
      .mockResolvedValueOnce(Response.json({ path: '/player/0123456789abcdef/index.html' }))
    vi.stubGlobal('fetch', fetcher)
    expect(new URL(await resolvePlayerUrl(options())).origin).toBe('https://distrib.ourtaiko.org')
    expect(fetcher).toHaveBeenCalledTimes(2)
  })
  it('does not start a fallback after visitor cancellation', async () => {
    const controller = new AbortController()
    controller.abort()
    const fetcher = vi.fn().mockRejectedValue(new DOMException('Aborted', 'AbortError'))
    vi.stubGlobal('fetch', fetcher)
    await expect(resolvePlayerUrl({ ...options(), signal: controller.signal })).rejects.toThrow(
      'Aborted',
    )
    expect(fetcher).toHaveBeenCalledTimes(1)
  })
  it.each([
    'http://mirror.example/',
    'https://user:secret@mirror.example/',
    'https://mirror.example/?signature=x',
  ])('rejects invalid regional roots: %s', async (baseUrl) => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(Response.json({ path: '/player/0123456789abcdef/index.html', baseUrl })),
    )
    await expect(resolvePlayerUrl(options())).rejects.toThrow('PLAYER_URL_INVALID')
  })
})
