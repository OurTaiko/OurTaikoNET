import { createHash } from 'node:crypto'
import { afterEach, expect, it, vi } from 'vitest'
import { loadNameplateArt, nameplateArtSources } from './nameplate-art-source'
const bytes = 'PNG bytes'
const sha256 = createHash('sha256').update(bytes).digest('hex')
const baseUrl = 'https://distrib.ourtaiko.org/nameplate/'
const mirrorBaseUrl = 'https://mirror.example/ourtaiko/nameplate/'
const files = {
  shadow: { path: 'shadow.png', size: bytes.length, sha256 },
  outline: { path: 'outline.png', size: bytes.length, sha256 },
}
afterEach(() => vi.unstubAllGlobals())
it('uses only the server-approved mirror paths and retains canonical fallbacks', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      Response.json({ version: 1, files, baseUrl, mirrorBaseUrl, mirrorPaths: ['shadow.png'] }),
    ),
  )
  const sources = await nameplateArtSources()
  expect(sources.shadow.url).toBe(mirrorBaseUrl + 'shadow.png?v=' + sha256.slice(0, 12))
  expect(sources.shadow.fallbackUrl).toBe(baseUrl + 'shadow.png?v=' + sha256.slice(0, 12))
  expect(sources.outline.url).toContain(baseUrl)
  expect(sources.outline.fallbackUrl).toBeUndefined()
})
it('supports an older NET server without the nameplate API', async () => {
  vi.resetModules()
  const { nameplateArtSources } = await import('./nameplate-art-source')
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(new Response('', { status: 404 }))
    .mockResolvedValueOnce(new Response('', { status: 404 }))
    .mockResolvedValueOnce(Response.json({ version: 1, files }))
  vi.stubGlobal('fetch', fetch)
  expect((await nameplateArtSources()).shadow.url).toContain(baseUrl)
  expect(fetch.mock.calls[1][0]).toBe('/api/v1/distribution/source')
  expect(fetch.mock.calls[2][0]).toBe(baseUrl + 'catalog.json')
})
it('reads the catalog from the mirror without the nameplate API', async () => {
  vi.resetModules()
  const { nameplateArtSources } = await import('./nameplate-art-source')
  const mirrorRoot = 'https://mirror.example/ourtaiko/'
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(new Response('', { status: 503 }))
    .mockResolvedValueOnce(Response.json({ mirror: true, baseUrl: mirrorRoot }))
    .mockResolvedValueOnce(Response.json({ version: 1, files }))
  vi.stubGlobal('fetch', fetch)
  const sources = await nameplateArtSources()
  expect(fetch.mock.calls[2][0]).toBe(mirrorBaseUrl + 'catalog.json')
  expect(sources.shadow.url).toBe(mirrorBaseUrl + 'shadow.png?v=' + sha256.slice(0, 12))
  expect(sources.shadow.fallbackUrl).toBe(baseUrl + 'shadow.png?v=' + sha256.slice(0, 12))
})
it('verifies all art before exposing blob URLs', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) =>
      url === '/api/v1/nameplate-art/catalog'
        ? Response.json({ version: 1, files, baseUrl, mirrorBaseUrl, mirrorPaths: ['shadow.png'] })
        : new Response(url.startsWith(mirrorBaseUrl) ? 'bad' : bytes),
    ),
  )
  const art = await loadNameplateArt()
  expect(Object.keys(art)).toEqual(['shadow', 'outline'])
  for (const value of Object.values(art)) {
    expect(value).toMatch(/^blob:/)
    URL.revokeObjectURL(value)
  }
})
it('rejects catalog paths that escape the resource collection', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      Response.json({ version: 1, files: { shadow: { ...files.shadow, path: '../token.png' } } }),
    ),
  )
  await expect(nameplateArtSources()).rejects.toThrow('NAMEPLATE_CATALOG_INVALID')
})
