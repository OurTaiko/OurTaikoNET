import { describe, expect, it } from 'vitest'
import { commitAddress, revisionOf } from './revision'

const hash = '9af7b0f0123456789abcdef0123456789abcdef0'

describe('the build revision', () => {
  it('is a full commit hash, shown by its first seven digits', () => {
    expect(revisionOf(hash)).toEqual({ full: hash, short: '9af7b0f' })
    expect(revisionOf(` ${hash.toUpperCase()}\n`)).toEqual({ full: hash, short: '9af7b0f' })
  })

  it('is none when the build was not told one', () => {
    for (const raw of [undefined, '', 'main', '9af7b0f', `${hash}0`, hash.replace('9', 'g')]) {
      expect(revisionOf(raw)).toBeNull()
    }
  })

  it('links to the commit in the repository', () => {
    expect(commitAddress('https://github.com/OurTaiko/OurTaikoNET', revisionOf(hash)!)).toBe(
      `https://github.com/OurTaiko/OurTaikoNET/commit/${hash}`,
    )
  })
})
