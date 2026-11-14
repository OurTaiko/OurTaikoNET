import { expect, test } from 'vitest'
import { inTurns } from './in-turns'

const at = (item: string) => item.slice(1)

test('the lists take turns, the one with the newest item first', () => {
  expect(inTurns([['a3', 'a1'], ['b9', 'b8', 'b7'], []], 10, at)).toEqual([
    'b9',
    'a3',
    'b8',
    'a1',
    'b7',
  ])
})

test('no more than the limit', () => {
  expect(
    inTurns(
      [
        ['a3', 'a1'],
        ['b9', 'b8', 'b7'],
      ],
      3,
      at,
    ),
  ).toEqual(['b9', 'a3', 'b8'])
  expect(inTurns([], 3, at)).toEqual([])
})
