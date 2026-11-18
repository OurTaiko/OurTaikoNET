import { afterEach, expect, it, vi } from 'vitest'

afterEach(() => vi.unstubAllGlobals())

const mirror = 'https://mirror.example/ourtaiko/'
const canonical = 'https://distrib.ourtaiko.org/'
const tja = 'fanmade/' + 'a'.repeat(64) + '.tja'

function serve(answers: Record<string, () => Response>) {
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input)
    const answer = answers[url]
    if (!answer) throw new Error('unexpected ' + url)
    return answer()
  })
  vi.stubGlobal('fetch', fetch)
  return fetch
}

it("reads a chart's file from the visitor's root, then the canonical one", async () => {
  vi.resetModules()
  const { fetchChartFile } = await import('./resource-download')
  const fetch = serve({
    '/api/v2/distribution/source': () =>
      Response.json({ mirror: true, baseUrl: mirror, canonicalBaseUrl: canonical }),
    '/api/v2/sources/fanmade/charts/song/resources': () =>
      Response.json({ chartId: 'song', resources: { tja: { path: tja, sha256: 'a', size: 4 } } }),
    [mirror + tja]: () => new Response('', { status: 404 }),
    [canonical + tja]: () => new Response('TJA!'),
  })
  const response = await fetchChartFile('song', 'tja')
  expect(await response.text()).toBe('TJA!')
  expect(fetch.mock.calls.map(([url]) => String(url))).toEqual([
    '/api/v2/sources/fanmade/charts/song/resources',
    '/api/v2/distribution/source',
    mirror + tja,
    canonical + tja,
  ])
})

it('answers 404 for a file the chart does not have', async () => {
  vi.resetModules()
  const { fetchChartFile } = await import('./resource-download')
  serve({
    '/api/v2/sources/fanmade/charts/song/resources': () =>
      Response.json({ chartId: 'song', resources: {} }),
  })
  expect((await fetchChartFile('song', 'movie')).status).toBe(404)
})
