import { expect, test } from 'vitest'
import { chartText } from './chart-language'

const chart = {
  title: 'Tentai Kansoku',
  subtitle: '--BUMP OF CHICKEN',
  titleTranslations: {
    origin: 'Tentai Kansoku',
    en: 'Stargazing',
    ja: '天体観測',
    zh_tw: '天體觀測',
  },
  subtitleTranslations: { origin: '--BUMP OF CHICKEN', ja: ' ' },
}

test("a chart's name is in the visitor's language, else their system's, else the original", () => {
  expect(chartText(chart, 'ja', ['en-US']).title).toBe('天体観測')
  // No Korean name: the system's languages in their order, the first that has one.
  expect(chartText(chart, 'ko', ['ko-KR', 'en-US', 'ja']).title).toBe('Stargazing')
  // Neither has one: the original, not the English.
  expect(chartText(chart, 'zh-Hans', ['zh-CN', 'fr']).title).toBe('Tentai Kansoku')
  expect(chartText(chart, 'zh-Hans', ['zh-TW']).title).toBe('天體觀測')
  // No language given is Chinese, the site's own.
  expect(chartText(chart, null, []).title).toBe('Tentai Kansoku')
  expect(chartText(chart, null, ['ja']).title).toBe('天体観測')
})

test('an empty translation falls through, and the subtitle loses its TJA mark', () => {
  expect(chartText(chart, 'ja', []).subtitle).toBe('BUMP OF CHICKEN')
  expect(chartText({ title: 'A', subtitle: '' }, 'en', []).title).toBe('A')
})
