import { describe, expect, it } from 'vitest'
import contract from '../../../contracts/chart-text.json'
import { chartText } from './chart-language'

// The same cases are the server's (internal/site/chart_text_test.go): a page's link card names
// the song as the page does.
describe("a chart's name in the visitor's language, else their system's, else the original", () => {
  const charts: Record<string, Parameters<typeof chartText>[0]> = contract.charts
  for (const c of contract.cases)
    it(c.name, () => {
      expect(chartText(charts[c.chart], c.language, c.system)).toEqual({
        title: c.title,
        subtitle: c.subtitle,
      })
    })
})

it('a chart without a subtitle has none', () => {
  expect(chartText({ title: 'A' }, 'en', [])).toEqual({ title: 'A', subtitle: '' })
})
