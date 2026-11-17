import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchDonFile } from './assets'
import { donImage, donSource } from './catalog'

const bytes = new TextEncoder().encode('glb bytes')
const digest = async (data: Uint8Array) =>
  Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', data as BufferSource)), (b) =>
    b.toString(16).padStart(2, '0'),
  ).join('')

const base = 'https://distrib.ourtaiko.org/3dchara/'
const mirror = {
  mirrorBaseUrl: 'https://static.sakura-bot.cn/3dchara/',
  mirrorPaths: ['head/3.glb'],
}

async function source(mirrored: boolean) {
  const file = { path: 'head/3.glb', size: bytes.length, sha256: await digest(bytes) }
  return donSource(base, file, mirrored ? mirror : undefined)
}

// fetch answering by address: bytes, an error status, or a thrown error
function serve(answers: Record<string, Uint8Array | number | Error>) {
  const fetch = vi.fn(async (url: string) => {
    const answer = answers[url.split('?')[0]]
    if (answer instanceof Error) throw answer
    if (typeof answer === 'number') return new Response(null, { status: answer })
    return new Response(answer as BodyInit)
  })
  vi.stubGlobal('fetch', fetch)
  return fetch
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('fetchDonFile', () => {
  it('fetches a canonical file once and does not hash it', async () => {
    const fetch = serve({ [base + 'head/3.glb']: bytes })
    const hash = vi.spyOn(crypto.subtle, 'digest')
    const s = await source(false)
    hash.mockClear()
    expect(s.mirror).toBeNull()
    expect(new Uint8Array(await (await fetchDonFile(s)).arrayBuffer())).toEqual(bytes)
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(hash).not.toHaveBeenCalled()
  })

  it('takes the mirror copy when its SHA-256 is the catalog one', async () => {
    const fetch = serve({ [mirror.mirrorBaseUrl + 'head/3.glb']: bytes })
    const s = await source(true)
    expect(new Uint8Array(await (await fetchDonFile(s)).arrayBuffer())).toEqual(bytes)
    expect(fetch.mock.calls.map((c) => c[0])).toEqual([s.mirror])
  })

  it.each([
    ['different bytes', new TextEncoder().encode('stale bytes')],
    ['an error status', 404],
    ['a network error', new TypeError('failed to fetch')],
  ])('falls back to the canonical file on %s', async (_, answer) => {
    const fetch = serve({
      [mirror.mirrorBaseUrl + 'head/3.glb']: answer,
      [base + 'head/3.glb']: bytes,
    })
    const s = await source(true)
    expect(new Uint8Array(await (await fetchDonFile(s)).arrayBuffer())).toEqual(bytes)
    expect(fetch.mock.calls.map((c) => c[0])).toEqual([s.mirror, s.url])
  })
})

describe('donImage', () => {
  it('goes back to the canonical picture once when the mirror fails', async () => {
    const s = await source(true)
    const props = donImage(s)
    expect(props.src).toBe(s.mirror)
    const img = { src: props.src, getAttribute: () => img.src } as unknown as HTMLImageElement
    props.onError?.({ currentTarget: img })
    expect(img.src).toBe(s.url)
    props.onError?.({ currentTarget: img })
    expect(img.src).toBe(s.url)
    expect(donImage(await source(false))).toEqual({ src: s.url })
  })
})
