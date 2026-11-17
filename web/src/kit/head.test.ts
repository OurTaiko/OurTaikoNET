import { describe, expect, it } from 'vitest'
import { DESCRIPTION_LIMIT, headFor, summarize } from './head'

const site = { name: 'OurTaikoNET · 自制', description: 'The site.', language: 'ja' }
const address = new URL('https://net.ourtaiko.org/fanmade/charts/abc?tab=chart#comments')

describe('page head', () => {
  it('puts the site name after the page title', () => {
    expect(headFor({ title: ' 千本桜 ' }, site, address).title).toBe('千本桜 | OurTaikoNET · 自制')
    expect(headFor({}, site, address).title).toBe('OurTaikoNET · 自制')
  })

  it('falls back to the site description and icon', () => {
    const head = headFor({ description: '  ' }, site, address)
    expect(head.description).toBe('The site.')
    expect(head.image).toBe('https://net.ourtaiko.org/icons/icon-512.png')
    expect(head.card).toBe('summary')
  })

  it('makes the picture absolute and asks for a large card', () => {
    const head = headFor({ image: '/api/v1/charts/abc/cover' }, site, address)
    expect(head.image).toBe('https://net.ourtaiko.org/api/v1/charts/abc/cover')
    expect(head.card).toBe('summary_large_image')
    expect(headFor({ image: '/avatar.webp', type: 'profile' }, site, address).card).toBe('summary')
  })

  it('names the page without what the visitor chose on it', () => {
    expect(headFor({}, site, address).canonical).toBe('https://net.ourtaiko.org/fanmade/charts/abc')
  })

  it('keeps private pages out of the index', () => {
    expect(headFor({ noindex: true }, site, address).robots).toBe('noindex, nofollow')
    expect(headFor({}, site, address).robots).toBe('index, follow')
  })

  it('names the locale the Open Graph way', () => {
    expect(headFor({}, site, address).locale).toBe('ja_JP')
    expect(headFor({}, { ...site, language: 'zh-Hans' }, address).locale).toBe('zh_CN')
  })
})

describe('summarize', () => {
  it('flattens whitespace', () => {
    expect(summarize('a\n\n b\tc ')).toBe('a b c')
  })

  it('cuts long text at a word', () => {
    const text = 'word '.repeat(60)
    const cut = summarize(text)
    expect(cut.length).toBeLessThanOrEqual(DESCRIPTION_LIMIT)
    expect(cut.endsWith('word…')).toBe(true)
  })

  it('cuts text without spaces where it must', () => {
    const cut = summarize('谱'.repeat(300))
    expect(cut).toHaveLength(DESCRIPTION_LIMIT)
    expect(cut.endsWith('…')).toBe(true)
  })
})
