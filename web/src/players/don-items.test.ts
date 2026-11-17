import { afterEach, describe, expect, it, vi } from 'vitest'
import { donItemsOf, readDonItems } from './don-items'

// What the dressing room asks: a locked part the player does not own is barred; owned, free, none
// and taken-off are not; the rules that give a part are found by part and number.
describe('Don items', () => {
  afterEach(() => vi.unstubAllGlobals())

  const serve = (answers: Record<string, unknown>, fail = false) =>
    vi.stubGlobal('fetch', async (url: string) =>
      fail
        ? new Response('{}', { status: 500 })
        : new Response(JSON.stringify(answers[url] ?? {}), { status: 200 }),
    )

  it('bars locked parts the player does not own, and finds their rules', async () => {
    const rule = { id: 3, donPart: 'paint', donItem: 2, titleId: null, kind: 'site_login' }
    serve({
      '/api/v1/don/items': {
        locked: [
          { part: 'cos', id: 30 },
          { part: 'paint', id: 2 },
        ],
        rules: [rule],
      },
      '/api/v1/me/don/items': { owned: [{ part: 'cos', id: 30, acquiredAt: '' }] },
    })
    const items = donItemsOf(await readDonItems(true))
    expect(items.barred('cos', 30)).toBe(false)
    expect(items.barred('paint', 2)).toBe(true)
    expect(items.barred('cos', 31)).toBe(false)
    expect(items.barred('cos', 0)).toBe(false)
    expect(items.rulesFor('paint', 2).map((r) => r.id)).toEqual([3])
    expect(items.rulesFor('cos', 30)).toEqual([])
  })

  it("someone else's page asks nothing of the session", async () => {
    const asked: string[] = []
    vi.stubGlobal('fetch', async (url: string) => {
      asked.push(url)
      return new Response(JSON.stringify({ locked: [], rules: [] }), { status: 200 })
    })
    await readDonItems(false)
    expect(asked).toEqual(['/api/v1/don/items'])
  })

  it('bars nothing when it cannot tell (the server still says no)', async () => {
    serve({}, true)
    expect(donItemsOf(await readDonItems(true)).barred('paint', 2)).toBe(false)
  })
})
