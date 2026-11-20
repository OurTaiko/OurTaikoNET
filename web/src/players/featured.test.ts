import { describe, expect, it } from 'vitest'
import { playlistInAddress } from './featured'

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
