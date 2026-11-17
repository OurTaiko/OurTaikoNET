import { describe, expect, it } from 'vitest'
import { avatarSource } from './avatar-source'
import { canonicalSource, readSource } from './distribution'

describe('public avatar distribution', () => {
  const original = `https://distrib.ourtaiko.org/sso/avatars/${'a'.repeat(32)}/${'b'.repeat(32)}.webp`
  const mirror = readSource({ mirror: true, baseUrl: 'https://static.sakura-bot.cn/ourtaiko/' })
  it('builds the mirror address directly, keeping the OSS prefix', () => {
    expect(avatarSource(original, mirror)).toBe(
      `https://static.sakura-bot.cn/ourtaiko/sso/avatars/${'a'.repeat(32)}/${'b'.repeat(32)}.webp`,
    )
  })
  it('keeps the canonical address for other visitors', () => {
    expect(avatarSource(original, canonicalSource)).toBe(original)
  })
  it.each([
    `${original}?signature=private`,
    'https://other.example/a.webp',
    `${original}#fragment`,
  ])('preserves signed and external URLs: %s', (url) => expect(avatarSource(url, mirror)).toBe(url))
  it.each([
    { mirror: true, baseUrl: 'http://mirror.example/' },
    { mirror: true, baseUrl: 'https://user:secret@mirror.example/' },
    { mirror: true, baseUrl: 'https://mirror.example/?x=1' },
    { mirror: true, baseUrl: 'https://mirror.example/no-slash' },
    { mirror: false, baseUrl: 'https://mirror.example/' },
    null,
  ])('treats an untrusted answer as canonical: %o', (answer) =>
    expect(readSource(answer)).toEqual(canonicalSource),
  )
})
