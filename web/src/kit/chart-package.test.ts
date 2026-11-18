import { createHash } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ChartBase } from './api/types'
import { buildChartPackage, entryName, packageFilename } from './chart-package'

const hash = (data: string) => createHash('sha256').update(data).digest('hex')
const audio = 'OggS audio bytes'
const tja = `TITLE:Song\nWAVE:${hash(audio)}.ogg\n`
// The package names the chart as the site does; the stored TJA keeps its headers.
const packagedTja = `TITLE:Renamed\nSUBTITLE:Artist\nTITLEJA:改名\nWAVE:${hash(audio)}.ogg\n`
const cover = 'RIFF cover'
// Files are read from the visitor's root: the canonical one here (the source request fails).
const cdn = 'https://distrib.ourtaiko.org/fanmade'
const chart = {
  id: 'song',
  tjaName: 'song.tja',
  difficulties: [],
  audioHash: hash(audio),
  coverHash: hash(cover),
  wave: `${hash(audio)}.ogg`,
  createdAt: '2026-10-12T00:00:00Z',
  updateAt: '2026-10-12T00:00:00Z',
  title: 'Renamed',
  subtitle: 'Artist',
  titleTranslations: { ja: '改名' },
  subtitleTranslations: {},
} as unknown as ChartBase

/** Names and contents of the stored (uncompressed) entries, in archive order. */
async function entries(archive: Blob) {
  const bytes = new Uint8Array(await archive.arrayBuffer())
  const view = new DataView(bytes.buffer)
  const text = new TextDecoder()
  const found: Record<string, string> = {}
  for (let at = 0; view.getUint32(at, true) === 0x04034b50;) {
    const flags = view.getUint16(at + 6, true)
    const nameLength = view.getUint16(at + 26, true)
    const start = at + 30 + nameLength + view.getUint16(at + 28, true)
    const name = text.decode(bytes.subarray(at + 30, at + 30 + nameLength))
    // client-zip streams entries: sizes follow the data in a 16-byte descriptor.
    expect(flags & 8).toBe(8)
    let end = start
    while (view.getUint32(end, true) !== 0x08074b50) end++
    found[name] = text.decode(bytes.subarray(start, end))
    at = end + 16
  }
  return found
}

function serve(files: Record<string, string>, manifest?: unknown) {
  const requested: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string, init?: RequestInit) => {
      requested.push(`${init?.credentials ?? 'default'} ${input}`)
      if (input === '/api/v2/sources/fanmade/charts/song/resources')
        return manifest ? Response.json(manifest) : new Response('{}', { status: 404 })
      return input in files ? new Response(files[input]) : new Response('', { status: 403 })
    }),
  )
  return requested
}

const resource = (data: string, extension: string) => ({
  path: `fanmade/${hash(data)}.${extension}`,
  sha256: hash(data),
  size: data.length,
  contentType: 'application/octet-stream',
})
const manifest = (resources: Record<string, unknown>) => ({ chartId: 'song', resources })
const stored = {
  [`${cdn}/${hash(tja)}.tja`]: tja,
  [`${cdn}/${hash(audio)}.ogg`]: audio,
  [`${cdn}/${hash(cover)}.webp`]: cover,
  [`${cdn}/preview.ogg`]: 'preview',
}

afterEach(() => vi.unstubAllGlobals())

describe('chart package', () => {
  it('names the archive after the displayed title', () => {
    expect(packageFilename('千本桜 / Senbonzakura: "A"?')).toBe('千本桜 Senbonzakura A.zip')
    expect(packageFilename(' ..さくら. ')).toBe('さくら.zip')
    expect(packageFilename('<>|')).toBe('chart.zip')
  })

  it('keeps stored names', () => {
    expect(entryName(`fanmade/${'a'.repeat(64)}.OGG`, 'a'.repeat(64))).toBe(`${'a'.repeat(64)}.ogg`)
    expect(() => entryName('fanmade/audio', 'a'.repeat(64))).toThrow('PACKAGE_RESOURCE_INVALID')
  })

  it('packs the TJA, audio and cover from the resource manifest', async () => {
    const requested = serve(
      stored,
      manifest({
        cover: resource(cover, 'webp'),
        preview: { ...resource('preview', 'ogg'), path: 'fanmade/preview.ogg' },
        audio: resource(audio, 'ogg'),
        tja: resource(tja, 'tja'),
      }),
    )
    const archive = await buildChartPackage(chart, new AbortController().signal)
    expect(await entries(archive)).toEqual({
      [`${hash(tja)}.tja`]: packagedTja,
      [`${hash(audio)}.ogg`]: audio,
      [`${hash(cover)}.webp`]: cover,
    })
    // The preview clip is not part of the package; file hosts get no cookies.
    expect(requested.filter((line) => !line.includes(' /api/')).sort()).toEqual(
      [`${hash(tja)}.tja`, `${hash(audio)}.ogg`, `${hash(cover)}.webp`]
        .map((name) => `omit ${cdn}/${name}`)
        .sort(),
    )
  })

  it('packs the background movie between the audio and the cover', async () => {
    const movie = 'ftyp movie'
    serve(
      { ...stored, [`${cdn}/${hash(movie)}.mp4`]: movie },
      manifest({
        movie: resource(movie, 'mp4'),
        cover: resource(cover, 'webp'),
        audio: resource(audio, 'ogg'),
        tja: resource(tja, 'tja'),
      }),
    )
    const archive = await buildChartPackage(chart, new AbortController().signal)
    expect(Object.keys(await entries(archive))).toEqual([
      `${hash(tja)}.tja`,
      `${hash(audio)}.ogg`,
      `${hash(movie)}.mp4`,
      `${hash(cover)}.webp`,
    ])
  })

  it('packs the background image after the cover', async () => {
    const background = 'RIFF background'
    serve(
      { ...stored, [`${cdn}/${hash(background)}.webp`]: background },
      manifest({
        bgimage: resource(background, 'webp'),
        cover: resource(cover, 'webp'),
        audio: resource(audio, 'ogg'),
        tja: resource(tja, 'tja'),
      }),
    )
    const archive = await buildChartPackage(chart, new AbortController().signal)
    expect(Object.keys(await entries(archive))).toEqual([
      `${hash(tja)}.tja`,
      `${hash(audio)}.ogg`,
      `${hash(cover)}.webp`,
      `${hash(background)}.webp`,
    ])
  })

  it('omits the cover when the chart has none', async () => {
    serve(stored, manifest({ audio: resource(audio, 'ogg'), tja: resource(tja, 'tja') }))
    const archive = await buildChartPackage(chart, new AbortController().signal)
    expect(Object.keys(await entries(archive))).toEqual([`${hash(tja)}.tja`, `${hash(audio)}.ogg`])
  })

  it('reads the files from the mirror for a mainland visitor, else from the canonical root', async () => {
    vi.resetModules()
    const { buildChartPackage } = await import('./chart-package')
    const mirror = 'https://mirror.example/ourtaiko/'
    const requested: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string) => {
        requested.push(input)
        if (input === '/api/v2/distribution/source')
          return Response.json({
            mirror: true,
            baseUrl: mirror,
            canonicalBaseUrl: 'https://distrib.ourtaiko.org/',
          })
        if (input === '/api/v2/sources/fanmade/charts/song/resources')
          return Response.json(
            manifest({ audio: resource(audio, 'ogg'), tja: resource(tja, 'tja') }),
          )
        // The mirror has the TJA but not (yet) the audio.
        if (input === `${mirror}fanmade/${hash(tja)}.tja`) return new Response(tja)
        return input in stored ? new Response(stored[input]) : new Response('', { status: 404 })
      }),
    )
    const archive = await buildChartPackage(chart, new AbortController().signal)
    expect(Object.keys(await entries(archive))).toEqual([`${hash(tja)}.tja`, `${hash(audio)}.ogg`])
    expect(requested).toContain(`${mirror}fanmade/${hash(audio)}.ogg`)
    expect(requested).toContain(`${cdn}/${hash(audio)}.ogg`)
    expect(requested).not.toContain(`${cdn}/${hash(tja)}.tja`)
  })

  it('rejects changed, corrupt or missing files', async () => {
    const signal = new AbortController().signal
    // The page is older than the files when the audio it names is gone.
    serve(stored, manifest({ audio: resource('newer audio', 'ogg'), tja: resource(tja, 'tja') }))
    await expect(buildChartPackage(chart, signal)).rejects.toThrow('PACKAGE_CHART_CHANGED')
    serve(
      { ...stored, [`${cdn}/${hash(audio)}.ogg`]: 'OggS AUDIO bytes' },
      manifest({ audio: resource(audio, 'ogg'), tja: resource(tja, 'tja') }),
    )
    await expect(buildChartPackage(chart, signal)).rejects.toThrow('PACKAGE_INTEGRITY_FAILED')
    serve({}, manifest({ audio: resource(audio, 'ogg'), tja: resource(tja, 'tja') }))
    await expect(buildChartPackage(chart, signal)).rejects.toThrow('PACKAGE_DOWNLOAD_FAILED')
  })
})
