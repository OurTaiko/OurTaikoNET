import { describe, expect, it } from 'vitest'
import { avatarSource } from './avatar-source'

describe('public avatar distribution', () => {
  const original = `https://distrib.ourtaiko.org/sso/avatars/${'a'.repeat(32)}/${'b'.repeat(32)}.webp`
  it('asks the shared regional endpoint without changing the original path', () => {
    const selected = new URL(avatarSource(original), 'https://net.ourtaiko.org')
    expect(selected.pathname).toBe('/api/v1/distribution/avatar')
    expect(selected.searchParams.get('url')).toBe(original)
    expect(selected.searchParams.get('redirect')).toBe('1')
  })
  it.each([
    `${original}?signature=private`,
    'https://other.example/a.webp',
    `${original}#fragment`,
  ])('preserves signed and external URLs: %s', (url) => expect(avatarSource(url)).toBe(url))
})
