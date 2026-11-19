import { expect, test } from 'vitest'
import { withTotals, type ProfileAnswer } from './profile-totals'

const answer: ProfileAnswer = {
  user: { id: 'a', nickname: 'Don', avatarPath: '' },
  nameplate: {
    userId: 'a',
    nickname: 'Don',
    avatarPath: '',
    title: null,
    dan: null,
    showDan: true,
  },
  sources: [],
  profilesAvailable: true,
}

test('a backend without totals leaves numbers that are marked as missing', () => {
  const profile = withTotals(answer)
  expect(profile.totalsAvailable).toBe(false)
  expect(profile.user.scoreCount).toBe(0)
  expect(profile.user.firstLoginAt).toBeNull()
})

test('the totals the server gives are kept', () => {
  const profile = withTotals({
    ...answer,
    user: { ...answer.user, scoreCount: 7 },
    totalsAvailable: true,
  })
  expect(profile.totalsAvailable).toBe(true)
  expect(profile.user.scoreCount).toBe(7)
})
