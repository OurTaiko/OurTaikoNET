import { describe, expect, it } from 'vitest'
import { pagesWith, peakHour, periodTotals, scaleTop } from './visit-stats'

describe('scaleTop', () => {
  it('is a round number at or above the most', () => {
    expect(scaleTop(0)).toBe(4)
    expect(scaleTop(4)).toBe(4)
    expect(scaleTop(7)).toBe(10)
    expect(scaleTop(34)).toBe(40)
    expect(scaleTop(400)).toBe(400)
    expect(scaleTop(1234)).toBe(2000)
  })
})

describe('periodTotals', () => {
  it('sums the days, averages the views and names the day seen most', () => {
    const days = [
      { day: '2026-11-18', views: 10, visitors: 4 },
      { day: '2026-11-19', views: 31, visitors: 9 },
      { day: '2026-11-20', views: 31, visitors: 12 },
      { day: '2026-11-21', views: 0, visitors: 0 },
    ]
    expect(periodTotals(days)).toEqual({ views: 72, visitors: 25, average: 18, peak: days[1] })
  })

  it('has no day seen most when nobody came', () => {
    expect(periodTotals([{ day: '2026-11-21', views: 0, visitors: 0 }]).peak).toBeNull()
    expect(periodTotals([])).toEqual({ views: 0, visitors: 0, average: 0, peak: null })
  })
})

describe('peakHour', () => {
  it('is the earliest of the hours seen most', () => {
    expect(peakHour([0, 3, 9, 2, 9])).toBe(2)
    expect(peakHour([0, 0, 0])).toBeNull()
    expect(peakHour([])).toBeNull()
  })
})

describe('pagesWith', () => {
  const pages = [{ page: '/' }, { page: '/fanmade/charts/12' }, { page: '/Official/songs' }]
  it('keeps the pages with the text in their address, whatever its case', () => {
    expect(pagesWith(pages, ' official ')).toEqual([pages[2]])
    expect(pagesWith(pages, 'charts')).toEqual([pages[1]])
    expect(pagesWith(pages, '  ')).toBe(pages)
  })
})
