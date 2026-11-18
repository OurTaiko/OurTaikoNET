import { createHash } from 'node:crypto'
import { afterEach, expect, it, vi } from 'vitest'

const bytes = 'PNG bytes'
const sha256 = createHash('sha256').update(bytes).digest('hex')
const canonicalRoot = 'https://distrib.ourtaiko.org/'
const mirrorRoot = 'https://mirror.example/ourtaiko/'
// As the API gives them: from the distribution root.
const files = {
  shadow: { path: 'nameplate/shadow.png', size: bytes.length, sha256 },
  outline: { path: 'nameplate/outline.png', size: bytes.length, sha256 },
}
// As the bucket's own catalog.json gives them: from the nameplate folder.
const folderFiles = {
  shadow: { path: 'shadow.png', size: bytes.length, sha256 },
  outline: { path: 'outline.png', size: bytes.length, sha256 },
}
const mainland = { mirror: true, baseUrl: mirrorRoot, canonicalBaseUrl: canonicalRoot }
const elsewhere = { mirror: false, baseUrl: canonicalRoot, canonicalBaseUrl: canonicalRoot }

// The module keeps the source answer for the page load, so each test loads it anew.
async function load(answer: (url: string) => Response | Promise<Response>) {
  vi.resetModules()
  const fetch = vi.fn(async (url: string) => answer(url))
  vi.stubGlobal('fetch', fetch)
  return { fetch, ...(await import('./nameplate-art-source')) }
}

afterEach(() => vi.unstubAllGlobals())

it('reads every file from the visitor root and keeps the canonical one to fall back to', async () => {
  const { nameplateArtSources } = await load((url) =>
    url === '/api/v2/distribution/source'
      ? Response.json(mainland)
      : Response.json({ version: 1, files }),
  )
  const sources = await nameplateArtSources()
  expect(sources.shadow.url).toBe(mirrorRoot + 'nameplate/shadow.png')
  expect(sources.shadow.fallbackUrl).toBe(canonicalRoot + 'nameplate/shadow.png')
  expect(sources.outline.url).toBe(mirrorRoot + 'nameplate/outline.png')
})
it('has nothing to fall back to on the canonical root', async () => {
  const { nameplateArtSources } = await load((url) =>
    url === '/api/v2/distribution/source'
      ? Response.json(elsewhere)
      : Response.json({ version: 1, files }),
  )
  const sources = await nameplateArtSources()
  expect(sources.shadow.url).toBe(canonicalRoot + 'nameplate/shadow.png')
  expect(sources.shadow.fallbackUrl).toBeUndefined()
})
it('reads the bucket catalog from the visitor root without the nameplate API', async () => {
  const { fetch, nameplateArtSources } = await load((url) => {
    if (url === '/api/v2/nameplate-art/catalog') return new Response('', { status: 503 })
    if (url === '/api/v2/distribution/source') return Response.json(mainland)
    return Response.json({ version: 1, files: folderFiles })
  })
  const sources = await nameplateArtSources()
  expect(fetch.mock.calls.map(([url]) => url)).toEqual([
    '/api/v2/nameplate-art/catalog',
    '/api/v2/distribution/source',
    mirrorRoot + 'nameplate/catalog.json',
  ])
  expect(sources.shadow.url).toBe(mirrorRoot + 'nameplate/shadow.png')
  expect(sources.shadow.fallbackUrl).toBe(canonicalRoot + 'nameplate/shadow.png')
})
it('reads the bucket catalog from the canonical root when the mirror fails', async () => {
  const { fetch, nameplateArtSources } = await load((url) => {
    if (url === '/api/v2/nameplate-art/catalog') return new Response('', { status: 404 })
    if (url === '/api/v2/distribution/source') return Response.json(mainland)
    if (url.startsWith(mirrorRoot)) return new Response('', { status: 404 })
    return Response.json({ version: 1, files: folderFiles })
  })
  expect((await nameplateArtSources()).outline.url).toBe(mirrorRoot + 'nameplate/outline.png')
  expect(fetch.mock.calls.at(-1)?.[0]).toBe(canonicalRoot + 'nameplate/catalog.json')
})
it('verifies all art before exposing blob URLs', async () => {
  const { loadNameplateArt } = await load((url) => {
    if (url === '/api/v2/distribution/source') return Response.json(mainland)
    if (url === '/api/v2/nameplate-art/catalog') return Response.json({ version: 1, files })
    return new Response(url.startsWith(mirrorRoot) ? 'bad' : bytes)
  })
  const art = await loadNameplateArt()
  expect(Object.keys(art)).toEqual(['shadow', 'outline'])
  for (const value of Object.values(art)) {
    expect(value).toMatch(/^blob:/)
    URL.revokeObjectURL(value)
  }
})
it.each(['nameplate/../token.png', 'shadow.png', 'other/shadow.png'])(
  'rejects catalog paths outside the nameplate folder: %s',
  async (path) => {
    const { nameplateArtSources } = await load((url) =>
      url === '/api/v2/distribution/source'
        ? Response.json(elsewhere)
        : Response.json({ version: 1, files: { shadow: { ...files.shadow, path } } }),
    )
    await expect(nameplateArtSources()).rejects.toThrow('NAMEPLATE_CATALOG_INVALID')
  },
)
