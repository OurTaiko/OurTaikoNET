import { afterEach, describe, expect, it, vi } from 'vitest'
import { canonicalPlayerUrl, resolvePlayerUrl } from './embedded-player-source'

afterEach(() => vi.unstubAllGlobals())
const options = () => ({
  manifestUrl: 'https://distrib.ourtaiko.org/player-build.json',
  parentOrigin: 'https://net.ourtaiko.org',
  signal: new AbortController().signal,
})
const manifest = () => Response.json({ path: '/player/0123456789abcdef/index.html' })

describe('regional web player', () => {
  it('puts the canonical build on the mirror root, keeping the OSS prefix', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(manifest()))
    const selected = await resolvePlayerUrl({
      ...options(),
      baseUrl: 'https://static.sakura-bot.cn/ourtaiko/',
    })
    const url = new URL(selected)
    expect(url.origin).toBe('https://static.sakura-bot.cn')
    expect(url.pathname).toBe('/ourtaiko/player/0123456789abcdef/index.html')
    expect(url.searchParams.get('parentOrigin')).toBe('https://net.ourtaiko.org')
    expect(canonicalPlayerUrl(selected)).toBe(
      'https://distrib.ourtaiko.org/player/0123456789abcdef/index.html?parentOrigin=https%3A%2F%2Fnet.ourtaiko.org',
    )
  })
  it('stays on the manifest root without a mirror', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(manifest()))
    const selected = await resolvePlayerUrl(options())
    expect(new URL(selected).origin).toBe('https://distrib.ourtaiko.org')
    expect(canonicalPlayerUrl(selected)).toBe(selected)
  })
  it('reports an unavailable manifest', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 503 })))
    await expect(resolvePlayerUrl(options())).rejects.toThrow('PLAYER_MANIFEST_UNAVAILABLE')
  })
  it.each([
    'http://mirror.example/',
    'https://user:secret@mirror.example/',
    'https://mirror.example/?signature=x',
    'https://mirror.example/no-slash',
  ])('rejects invalid roots: %s', async (baseUrl) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(manifest()))
    await expect(resolvePlayerUrl({ ...options(), baseUrl })).rejects.toThrow('PLAYER_URL_INVALID')
  })
})
