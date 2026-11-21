import { afterEach, expect, it, vi } from 'vitest'

afterEach(() => vi.unstubAllGlobals())

const mirror = {
  mirror: true,
  baseUrl: 'https://static.sakura-bot.cn/ourtaiko/',
  canonicalBaseUrl: 'https://distrib.ourtaiko.org/',
}

// A page: its storage, its root element's style, its fonts, and a server that answers the
// distribution source and each font file through `files` (a status, or a promise of one).
function page(saved: Record<string, string>, files: Record<string, () => Promise<number>>) {
  const kept = new Map(Object.entries(saved))
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => kept.get(key) ?? null,
    setItem: (key: string, value: string) => void kept.set(key, value),
    removeItem: (key: string) => void kept.delete(key),
  })
  const style = new Map<string, string>()
  const added: string[] = []
  vi.stubGlobal('document', {
    documentElement: {
      style: {
        setProperty: (name: string, value: string) => void style.set(name, value),
        removeProperty: (name: string) => void style.delete(name),
      },
    },
    fonts: { add: (face: { family: string }) => void added.push(face.family) },
  })
  vi.stubGlobal(
    'FontFace',
    class {
      constructor(
        public family: string,
        public data: ArrayBuffer,
      ) {}
      load = async () => this
    },
  )
  const asked: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      asked.push(url)
      if (url.endsWith('/distribution/source')) return Response.json(mirror)
      const status = await (files[url] ?? (async () => 404))()
      return new Response(status === 200 ? new ArrayBuffer(4) : null, { status })
    }),
  )
  return { kept, style, added, asked }
}

const misansOnMirror = 'https://static.sakura-bot.cn/ourtaiko/fonts/misans/4.003/MiSans-VF.ttf'
const misansCanonical = 'https://distrib.ourtaiko.org/fonts/misans/4.003/MiSans-VF.ttf'

it('shows the default without fetching anything', async () => {
  const { style, asked } = page({}, {})
  vi.resetModules()
  const font = await import('./font')
  await font.applyFont()
  expect(style.has('--font-chosen')).toBe(false)
  expect(asked).toEqual([])
})

it('keeps the bundled font while the chosen one arrives from the mirror, then takes it', async () => {
  let arrive!: (status: number) => void
  const { kept, style, added } = page(
    {},
    { [misansOnMirror]: () => new Promise((resolve) => (arrive = resolve)) },
  )
  vi.resetModules()
  const font = await import('./font')
  const choosing = font.chooseFont('misans')
  expect(kept.get('ourtaiko-net-font')).toBe('misans')
  await vi.waitFor(() => expect(arrive).toBeDefined())
  expect(style.has('--font-chosen')).toBe(false)
  arrive(200)
  await choosing
  expect(added).toEqual(['OurTaiko MiSans'])
  expect(style.get('--font-chosen')).toBe("'OurTaiko MiSans'")
})

it('falls back to the canonical root when the mirror fails', async () => {
  const { style, asked } = page(
    { 'ourtaiko-net-font': 'misans' },
    { [misansOnMirror]: async () => 502, [misansCanonical]: async () => 200 },
  )
  vi.resetModules()
  const font = await import('./font')
  await font.applyFont()
  expect(asked.slice(-2)).toEqual([misansOnMirror, misansCanonical])
  expect(style.get('--font-chosen')).toBe("'OurTaiko MiSans'")
})

it('stays in the shown font when the chosen one cannot be fetched, and tries again', async () => {
  const { style } = page({ 'ourtaiko-net-font': 'misans' }, {})
  vi.resetModules()
  const font = await import('./font')
  await font.applyFont()
  expect(style.has('--font-chosen')).toBe(false)
  expect(font.fontState()).toEqual({
    chosen: 'misans',
    shown: 'default',
    loading: false,
    failed: true,
  })
  const asked = vi.mocked(fetch).mock.calls.length
  await font.chooseFont('misans')
  expect(vi.mocked(fetch).mock.calls.length).toBeGreaterThan(asked)
  expect(font.fontState().failed).toBe(true)
})

it('shows only the latest choice when an earlier one arrives later', async () => {
  let arrive!: (status: number) => void
  const { style } = page(
    {},
    {
      [misansOnMirror]: () => new Promise((resolve) => (arrive = resolve)),
      'https://static.sakura-bot.cn/ourtaiko/fonts/noto-sans-sc/a85815a/NotoSansSC-VF.ttf':
        async () => 200,
    },
  )
  vi.resetModules()
  const font = await import('./font')
  const first = font.chooseFont('misans')
  await vi.waitFor(() => expect(arrive).toBeDefined())
  await font.chooseFont('noto-sans-sc')
  expect(style.get('--font-chosen')).toBe("'OurTaiko Noto Sans SC'")
  arrive(200)
  await first
  expect(style.get('--font-chosen')).toBe("'OurTaiko Noto Sans SC'")
})

it('reads an unknown saved font as the default', async () => {
  page({ 'ourtaiko-net-font': 'comic-sans' }, {})
  vi.resetModules()
  const font = await import('./font')
  expect(font.chosenFont()).toBe('default')
})
