import { describe, expect, it } from 'vitest'
import { reviewedSources } from './tags'

describe('reviewedSources', () => {
  it('gives a tag manager both sources and a source manager theirs', () => {
    expect(reviewedSources(['TAG_MANAGER'])).toEqual(['fanmade', 'official'])
    expect(reviewedSources(['OFFICIAL_MANAGER'])).toEqual(['official'])
    expect(reviewedSources(['TITLE_MANAGER'])).toEqual([])
  })
})
