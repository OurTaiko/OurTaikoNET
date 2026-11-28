import { describe, expect, it } from 'vitest'
import type { ChartTag, Tag } from './api/types'
import { pickerRows, tagName, tagNamed, tagsMatching } from './tags'

const touhou: Tag = {
  id: 'touhou',
  names: { en: 'TOUHOU PROJECT', 'zh-Hans': '东方 PROJECT', ja: '東方Project' },
  status: 'official',
  chartCount: 107,
}
const bms: Tag = { id: 'bms', names: { en: 'BMS' }, status: 'official', chartCount: 10 }
const denpa: Tag = {
  id: '0123456789abcdef0123456789abcdef',
  names: { en: 'Denpa' },
  status: 'pending',
  chartCount: 40,
}
const catalog = [touhou, bms, denpa]

describe('tagName', () => {
  it('is the name in the visitor language, else English, else the id', () => {
    expect(tagName(touhou, 'ja')).toBe('東方Project')
    expect(tagName(touhou, 'zh-CN')).toBe('东方 PROJECT')
    expect(tagName(touhou, 'ko')).toBe('TOUHOU PROJECT')
    expect(tagName(undefined, 'en', 'gone')).toBe('gone')
  })
})

describe('tagNamed', () => {
  it('finds a tag by its id or a name of any language, whatever the case', () => {
    expect(tagNamed(catalog, ' 东方 project ')).toBe(touhou)
    expect(tagNamed(catalog, 'TOUHOU')).toBe(touhou)
    expect(tagNamed(catalog, 'denpa')).toBe(denpa)
  })
  it('finds none for part of a name, or for nothing', () => {
    expect(tagNamed(catalog, 'Den')).toBeUndefined()
    expect(tagNamed(catalog, '  ')).toBeUndefined()
  })
})

describe('tagsMatching', () => {
  it('keeps the tags with the text in a name', () => {
    expect(tagsMatching(catalog, 'proj')).toEqual([touhou])
    expect(tagsMatching(catalog, '东方')).toEqual([touhou])
    expect(tagsMatching(catalog, '')).toEqual(catalog)
    expect(tagsMatching(catalog, 'zzz')).toEqual([])
  })
})

describe('pickerRows', () => {
  it('puts the tags of the chart first, then official ones, the most used first', () => {
    const onChart: ChartTag[] = [{ ...denpa, count: 2, mine: true }]
    const rows = pickerRows(catalog, onChart)
    expect(rows.map((row) => row.id)).toEqual([denpa.id, 'touhou', 'bms'])
    expect(rows[0]).toMatchObject({ count: 2, mine: true, chartCount: 40 })
    expect(rows[1]).toMatchObject({ count: 0, mine: false, chartCount: 107 })
  })
  it('orders pending tags after official ones however used', () => {
    expect(pickerRows(catalog, []).map((row) => row.id)).toEqual(['touhou', 'bms', denpa.id])
  })
})
