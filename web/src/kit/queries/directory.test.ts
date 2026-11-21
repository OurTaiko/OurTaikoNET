import { expect, it } from 'vitest'
import { setArea } from '../source'
import { directoryAddress, directoryQuery } from './directory'

const query = { q: '鼓手', sort: 'active', page: 2 } as const

it('asks the same list from every area', () => {
  for (const area of ['fanmade', 'official', ''] as const) {
    setArea(area)
    expect(directoryAddress(query)).toBe('/api/v2/users?q=%E9%BC%93%E6%89%8B&sort=active&page=2')
    expect(directoryQuery(query).queryKey).toEqual(['players', 'directory', query])
  }
})

it('says how many players a page has only when asked to', () => {
  setArea('')
  expect(directoryAddress({ ...query, pageSize: 12 })).toBe(
    '/api/v2/users?q=%E9%BC%93%E6%89%8B&sort=active&page=2&pageSize=12',
  )
})
