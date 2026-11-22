import { describe, expect, it } from 'vitest'
import { playlistInAddress, wallJackets } from './featured'

const id = '0123456789abcdef0123456789abcdef'

describe('playlistInAddress', () => {
  it('finds the playlist a pasted address is the page of', () => {
    expect(playlistInAddress(`https://net.ourtaiko.org/fanmade/playlists/${id}`)).toEqual({
      source: 'fanmade',
      id,
    })
    expect(playlistInAddress(`/official/playlists/${id}?from=home#songs`)).toEqual({
      source: 'official',
      id,
    })
    expect(playlistInAddress(`official/playlists/${id}`)).toEqual({ source: 'official', id })
  })

  it('takes anything else for a name to look for', () => {
    for (const text of [
      'Beginner picks',
      id,
      `/elsewhere/playlists/${id}`,
      '/fanmade/playlists/liked',
      `/fanmade/playlists/${id}0`,
      `/fanmade/charts/${id}`,
    ])
      expect(playlistInAddress(text), text).toBeNull()
  })
})

describe('wallJackets', () => {
  const songs = (...covers: (string | undefined)[]) =>
    covers.map((songCoverPath) => ({ songCoverPath }))

  it('takes each cover once, in the order of the songs, leaving out songs without one', () => {
    expect(wallJackets(songs('a', undefined, 'b', 'a', '', 'c'), 3)).toEqual(['a', 'b', 'c'])
    expect(wallJackets(songs('a', 'b', 'c'), 2)).toEqual(['a', 'b'])
  })

  it('begins again from the first where there are fewer covers than places', () => {
    expect(wallJackets(songs('a', 'b'), 5)).toEqual(['a', 'b', 'a', 'b', 'a'])
  })

  it('is empty where no song has a cover', () => {
    expect(wallJackets(songs(undefined, ''), 4)).toEqual([])
    expect(wallJackets([], 4)).toEqual([])
  })
})
