import { describe, expect, it } from 'vitest'
import { cardCoverAddress } from './card-cover'
import { canonicalSource, readSource } from '../distribution'

describe('cover addresses', () => {
  const cover = `https://distrib.ourtaiko.org/fanmade/${'c'.repeat(64)}.webp`
  const mirror = readSource({ mirror: true, baseUrl: 'https://static.sakura-bot.cn/ourtaiko/' })
  it('takes a bucket cover from the mirror for mainland visitors', () => {
    expect(cardCoverAddress(cover, mirror, false)).toBe(
      `https://static.sakura-bot.cn/ourtaiko/fanmade/${'c'.repeat(64)}.webp`,
    )
  })
  it('keeps the original address for other visitors and after the mirror failed', () => {
    expect(cardCoverAddress(cover, canonicalSource, false)).toBe(cover)
    expect(cardCoverAddress(cover, mirror, true)).toBe(cover)
  })
  it('waits for the visitor root before loading a bucket cover', () => {
    expect(cardCoverAddress(cover, null, false)).toBeUndefined()
  })
  it('uses the API cover route as it is', () => {
    const route = '/api/v1/sources/fanmade/charts/song/cover'
    expect(cardCoverAddress(route, null, false)).toBe(route)
    expect(cardCoverAddress(route, mirror, false)).toBe(route)
  })
})
