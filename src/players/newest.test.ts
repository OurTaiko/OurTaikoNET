import { expect, test } from 'vitest'
import { newestOf } from './newest'

const at = (item: { at: string }) => item.at
const names = (items: { name: string }[]) => items.map((item) => item.name).join(',')

test('the newest of all lists together, whichever list they are in', () => {
  const fanmade = [
    { name: 'f3', at: '2026-11-14T07:14:10.432155Z' },
    { name: 'f2', at: '2026-11-14T05:09:45.04331Z' },
    { name: 'f1', at: '2026-11-14T05:09:40.568211Z' },
  ]
  const official = [
    { name: 'o2', at: '2026-11-14T05:09:45.1Z' },
    { name: 'o1', at: '2026-11-12T02:46:22.573617Z' },
  ]
  expect(names(newestOf([fanmade, official], 4, at))).toBe('f3,o2,f2,f1')
  expect(names(newestOf([official, fanmade], 10, at))).toBe('f3,o2,f2,f1,o1')
  expect(newestOf([[], []], 10, at)).toEqual([])
})

test('charts made at the same moment keep the order they came in', () => {
  const same = '2026-11-14T00:00:00Z'
  expect(
    names(
      newestOf(
        [
          [
            { name: 'a', at: same },
            { name: 'b', at: same },
          ],
          [{ name: 'c', at: same }],
        ],
        10,
        at,
      ),
    ),
  ).toBe('a,b,c')
})
