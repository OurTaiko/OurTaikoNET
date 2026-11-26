import { describe, expect, it } from 'vitest'
import { tagName } from './tags'

const touhou = {
  id: 'touhou',
  names: { 'zh-Hans': '东方 PROJECT', en: 'TOUHOU PROJECT', ja: '東方Project' },
}

describe('tagName', () => {
  it('names a tag in the visitor’s language', () => {
    expect(tagName(touhou, 'zh-CN')).toBe('东方 PROJECT')
    expect(tagName(touhou, 'ja')).toBe('東方Project')
  })
  it('falls back to English, then to the id', () => {
    expect(tagName(touhou, 'ko')).toBe('TOUHOU PROJECT')
    expect(tagName(undefined, 'en', 'bms')).toBe('bms')
  })
})
