import { describe, expect, it } from 'vitest'
import { itemName, sameLook } from './user-don'

describe('the Don tab', () => {
  const look = { costume: true, cos: 3, head: 0, body: 0, puchi: 0, colors: null }

  it('compares looks, colours in either case', () => {
    expect(sameLook(look, { ...look })).toBe(true)
    expect(sameLook(look, { ...look, cos: 4 })).toBe(false)
    expect(
      sameLook(
        { ...look, colors: ['#aabbcc', '#000000', '#FFFFFF'] },
        { ...look, colors: ['#AABBCC', '#000000', '#ffffff'] },
      ),
    ).toBe(true)
    expect(sameLook(look, { ...look, colors: ['#68BFC0', '#F94728', '#F9F0E1'] })).toBe(false)
    // makeup: a look from before it existed has none
    expect(sameLook(look, { ...look, paint: 0 })).toBe(true)
    expect(sameLook(look, { ...look, paint: 2 })).toBe(false)
  })

  it('names an item in the reader’s language, else the cabinet’s Japanese, else its number', () => {
    const item = {
      id: 30,
      name: { ja: '鏡もち', en: 'Kagami mochi' },
      file: { path: 'cos/30.glb', size: 1, sha256: 'x' },
    }
    expect(itemName(item, 'en', 30)).toBe('Kagami mochi')
    expect(itemName(item, 'zh-Hans', 30)).toBe('鏡もち')
    expect(itemName({ ...item, name: {} }, 'ja', 30)).toBe('#30')
    expect(itemName(undefined, 'ja', 7)).toBe('#7')
  })
})
