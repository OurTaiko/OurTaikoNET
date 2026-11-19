import type { PlayerTotals, PublicProfile } from '@players/api/types'

/** What the server says of a player. A backend rolled back behind the website has no totals. */
export type ProfileAnswer = Omit<PublicProfile, 'user' | 'totalsAvailable'> & {
  user: Partial<PlayerTotals> & Pick<PublicProfile['user'], 'id' | 'nickname' | 'avatarPath'>
  totalsAvailable?: boolean
}

const noTotals: PlayerTotals = {
  firstLoginAt: null,
  lastActiveAt: null,
  chartCount: 0,
  scoreCount: 0,
  commentCount: 0,
  karma: 0,
  playlistCount: 0,
}

/** The profile the pages read: its numbers are always there, and say when they mean nothing. */
export const withTotals = (answer: ProfileAnswer): PublicProfile => ({
  ...answer,
  user: { ...noTotals, ...answer.user },
  totalsAvailable: answer.totalsAvailable ?? false,
})
