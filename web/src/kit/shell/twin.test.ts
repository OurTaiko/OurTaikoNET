import { describe, expect, it } from 'vitest'
import { twinPath } from './twin'

describe('the page the source switch goes to', () => {
  it.each(['/', '/playlists', '/dans', '/dans/new', '/me/playlists', '/me/profile', '/login'])(
    'keeps %s, which both sources have',
    (path) => {
      expect(twinPath(path, false)).toBe(path)
    },
  )
  it('reads an address with a trailing slash as the same page', () => {
    expect(twinPath('/playlists/', false)).toBe('/playlists')
  })
  it('keeps the chart manager for a player who may upload there', () => {
    expect(twinPath('/me/charts', true)).toBe('/me/charts')
    expect(twinPath('/me/charts/publish', true)).toBe('/me/charts/publish')
    expect(twinPath('/upload', true)).toBe('/upload')
  })
  it('goes to the songs when the player may not upload there', () => {
    expect(twinPath('/me/charts', false)).toBe('/')
    expect(twinPath('/me/charts/publish', false)).toBe('/')
  })
  it.each([
    ['/playlists/0123456789abcdef0123456789abcdef', '/playlists'],
    ['/dans/0123456789abcdef0123456789abcdef', '/dans'],
    ['/dans/0123456789abcdef0123456789abcdef/edit', '/dans'],
    ['/charts/0123456789abcdef0123456789abcdef', '/'],
    ['/charts/0123456789abcdef0123456789abcdef/update', '/'],
    ['/charts/0123456789abcdef0123456789abcdef/comments/abc', '/'],
  ])("goes from %s, which is one source's own, to %s", (from, to) => {
    expect(twinPath(from, true)).toBe(to)
  })
  it('goes to the songs from an address it does not know', () => {
    expect(twinPath('/somewhere/else', true)).toBe('/')
  })
})
