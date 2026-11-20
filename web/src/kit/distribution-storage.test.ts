import { afterEach, expect, it, vi } from 'vitest'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const mirrorAnswer = {
  mirror: true,
  baseUrl: 'https://static.sakura-bot.cn/ourtaiko/',
  canonicalBaseUrl: 'https://distrib.ourtaiko.org/',
}
const canonicalAnswer = {
  mirror: false,
  baseUrl: 'https://distrib.ourtaiko.org/',
  canonicalBaseUrl: 'https://distrib.ourtaiko.org/',
}

// A browser's storage, with what it starts with.
function storage(start: Record<string, string> = {}) {
  const kept = new Map(Object.entries(start))
  const fake = {
    getItem: (key: string) => kept.get(key) ?? null,
    setItem: (key: string, value: string) => void kept.set(key, value),
    removeItem: (key: string) => void kept.delete(key),
  }
  vi.stubGlobal('localStorage', fake)
  return kept
}

function answering(...answers: unknown[]) {
  const fetch = vi.fn(async () => Response.json(answers.shift()))
  vi.stubGlobal('fetch', fetch)
  return fetch
}

const key = 'ourtaiko.distributionSource'
const day = 24 * 60 * 60 * 1000

// What a page kept, read back.
const keptIn = (kept: Map<string, string>) => JSON.parse(kept.get(key)!)

it('asks once, keeps the answer, and a later page uses it without asking', async () => {
  const kept = storage()
  const fetch = answering(mirrorAnswer)
  vi.resetModules()
  let { askSource } = await import('./distribution')
  expect(await askSource()).toEqual(mirrorAnswer)
  expect(await askSource()).toEqual(mirrorAnswer)
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(keptIn(kept).source).toEqual(mirrorAnswer)
  vi.resetModules()
  ;({ askSource } = await import('./distribution'))
  expect(await askSource()).toEqual(mirrorAnswer)
  expect(fetch).toHaveBeenCalledTimes(1)
})

it('keeps the canonical answer as well', async () => {
  storage()
  const fetch = answering(canonicalAnswer)
  vi.resetModules()
  let { askSource } = await import('./distribution')
  expect(await askSource()).toEqual(canonicalAnswer)
  vi.resetModules()
  ;({ askSource } = await import('./distribution'))
  expect(await askSource()).toEqual(canonicalAnswer)
  expect(fetch).toHaveBeenCalledTimes(1)
})

it('asks again once the kept answer is a day old, so a wrong answer mends itself', async () => {
  // A mainland visitor the server could not place yet was told the canonical root.
  vi.useFakeTimers({ toFake: ['Date'] })
  const kept = storage()
  const fetch = answering(canonicalAnswer, mirrorAnswer)
  vi.resetModules()
  let { askSource } = await import('./distribution')
  expect(await askSource()).toEqual(canonicalAnswer)
  vi.setSystemTime(Date.now() + day - 1)
  vi.resetModules()
  ;({ askSource } = await import('./distribution'))
  expect(await askSource()).toEqual(canonicalAnswer)
  expect(fetch).toHaveBeenCalledTimes(1)
  vi.setSystemTime(Date.now() + 1)
  vi.resetModules()
  ;({ askSource } = await import('./distribution'))
  expect(await askSource()).toEqual(mirrorAnswer)
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(keptIn(kept).source).toEqual(mirrorAnswer)
})

it('asks again for an answer an earlier release kept without an expiry', async () => {
  for (const old of [mirrorAnswer, canonicalAnswer]) {
    storage({ [key]: JSON.stringify(old) })
    const fetch = answering(mirrorAnswer)
    vi.resetModules()
    const { askSource } = await import('./distribution')
    expect(await askSource()).toEqual(mirrorAnswer)
    expect(fetch).toHaveBeenCalledTimes(1)
  }
})

it('asks again on refresh and keeps the new answer', async () => {
  const kept = storage()
  const fetch = answering(mirrorAnswer, canonicalAnswer)
  vi.resetModules()
  const { askSource, refreshSource } = await import('./distribution')
  expect(await askSource()).toEqual(mirrorAnswer)
  expect(await refreshSource()).toEqual(canonicalAnswer)
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(keptIn(kept).source).toEqual(canonicalAnswer)
  expect(await askSource()).toEqual(canonicalAnswer)
})

it('asks again for a kept answer it cannot trust, and keeps nothing from a failed request', async () => {
  const kept = storage({
    [key]: JSON.stringify({
      source: { mirror: true, baseUrl: 'http://evil.example/' },
      until: Date.now() + day,
    }),
  })
  const fetch = vi.fn(async () => new Response('', { status: 503 }))
  vi.stubGlobal('fetch', fetch)
  vi.resetModules()
  const { askSource, canonicalSource } = await import('./distribution')
  expect(await askSource()).toBe(canonicalSource)
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(kept.get(key)).toContain('evil.example')
})

it('works in a browser that keeps nothing', async () => {
  vi.stubGlobal('localStorage', {
    getItem: () => {
      throw new Error('blocked')
    },
    setItem: () => {
      throw new Error('blocked')
    },
    removeItem: () => {
      throw new Error('blocked')
    },
  })
  answering(mirrorAnswer, canonicalAnswer)
  vi.resetModules()
  const { askSource, refreshSource } = await import('./distribution')
  expect(await askSource()).toEqual(mirrorAnswer)
  expect(await refreshSource()).toEqual(canonicalAnswer)
})
