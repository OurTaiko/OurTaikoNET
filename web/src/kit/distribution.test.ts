import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  canonicalFileUrl,
  canonicalSource,
  fallbackFileUrl,
  fetchWithFallback,
  fileUrl,
  pictureUrl,
  readSource,
} from './distribution'

const avatar = `sso/avatars/${'a'.repeat(32)}/${'b'.repeat(32)}.webp`
const mirror = readSource({
  mirror: true,
  baseUrl: 'https://static.sakura-bot.cn/ourtaiko/',
  canonicalBaseUrl: 'https://distrib.ourtaiko.org/',
})

afterEach(() => vi.unstubAllGlobals())

describe('distribution source', () => {
  it('reads the server answer', () => {
    expect(mirror).toEqual({
      mirror: true,
      baseUrl: 'https://static.sakura-bot.cn/ourtaiko/',
      canonicalBaseUrl: 'https://distrib.ourtaiko.org/',
    })
    expect(
      readSource({
        mirror: false,
        baseUrl: 'https://distrib.ourtaiko.org/',
        canonicalBaseUrl: 'https://distrib.ourtaiko.org/',
      }),
    ).toEqual(canonicalSource)
  })
  it.each([
    { mirror: true, baseUrl: 'http://mirror.example/' },
    { mirror: true, baseUrl: 'https://user:secret@mirror.example/' },
    { mirror: true, baseUrl: 'https://mirror.example/?x=1' },
    { mirror: true, baseUrl: 'https://mirror.example/no-slash' },
    { mirror: true, baseUrl: 'javascript:alert(1)//' },
    { mirror: true },
    null,
  ])('treats an untrusted answer as canonical: %o', (answer) =>
    expect(readSource(answer)).toEqual(canonicalSource),
  )
  it('keeps the canonical root when the answer names an untrusted one', () => {
    expect(
      readSource({
        mirror: true,
        baseUrl: 'https://mirror.example/',
        canonicalBaseUrl: 'http://other.example/',
      }).canonicalBaseUrl,
    ).toBe('https://distrib.ourtaiko.org/')
  })
})

describe('file addresses', () => {
  it('joins a path to the visitor root, keeping the mirror prefix', () => {
    expect(fileUrl(avatar, mirror)).toBe(`https://static.sakura-bot.cn/ourtaiko/${avatar}`)
    expect(fileUrl(avatar, canonicalSource)).toBe(`https://distrib.ourtaiko.org/${avatar}`)
    expect(canonicalFileUrl(avatar)).toBe(`https://distrib.ourtaiko.org/${avatar}`)
  })
  it('has no address for an empty or missing path', () => {
    for (const path of ['', undefined, null]) {
      expect(fileUrl(path, mirror)).toBe('')
      expect(canonicalFileUrl(path)).toBe('')
      expect(fallbackFileUrl(path, mirror)).toBe('')
    }
  })
  it('falls back to the canonical root only when it is another', () => {
    expect(fallbackFileUrl(avatar, mirror)).toBe(`https://distrib.ourtaiko.org/${avatar}`)
    expect(fallbackFileUrl(avatar, canonicalSource)).toBe('')
  })
  it('shows a picture from the visitor root, then the canonical root, then none', () => {
    const cover = `fanmade/${'c'.repeat(64)}.webp`
    expect(pictureUrl(cover, null, 0)).toBe('')
    expect(pictureUrl(cover, mirror, 0)).toBe(`https://static.sakura-bot.cn/ourtaiko/${cover}`)
    expect(pictureUrl(cover, mirror, 1)).toBe(`https://distrib.ourtaiko.org/${cover}`)
    expect(pictureUrl(cover, mirror, 2)).toBe('')
    expect(pictureUrl(cover, canonicalSource, 0)).toBe(`https://distrib.ourtaiko.org/${cover}`)
    expect(pictureUrl(cover, canonicalSource, 1)).toBe('')
    expect(pictureUrl('', mirror, 0)).toBe('')
  })
})

describe('fetchWithFallback', () => {
  it.each(['status', 'network'])('tries the fallback once after a %s failure', async (failure) => {
    const fetch = vi.fn(async (url: string) => {
      if (url === 'https://b.example/f') return new Response('ok')
      if (failure === 'network') throw new TypeError('offline')
      return new Response('', { status: 404 })
    })
    vi.stubGlobal('fetch', fetch)
    const response = await fetchWithFallback('https://a.example/f', 'https://b.example/f')
    expect(await response.text()).toBe('ok')
    expect(fetch.mock.calls.map(([url]) => url)).toEqual([
      'https://a.example/f',
      'https://b.example/f',
    ])
  })
  it('answers the first address alone without a fallback', async () => {
    const fetch = vi.fn(async () => new Response('', { status: 404 }))
    vi.stubGlobal('fetch', fetch)
    expect((await fetchWithFallback('https://a.example/f', undefined)).status).toBe(404)
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('does not try again after cancellation', async () => {
    const cancel = new AbortController()
    const fetch = vi.fn(async () => {
      cancel.abort()
      throw new DOMException('aborted', 'AbortError')
    })
    vi.stubGlobal('fetch', fetch)
    await expect(
      fetchWithFallback('https://a.example/f', 'https://b.example/f', { signal: cancel.signal }),
    ).rejects.toThrow('aborted')
    expect(fetch).toHaveBeenCalledTimes(1)
  })
})
