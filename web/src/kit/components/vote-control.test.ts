import { describe, expect, it } from 'vitest'
import { tallyAfterVote } from './vote-control'

describe('tallyAfterVote', () => {
  const tally = { upvotes: 5, downvotes: 2 }
  it('counts an upvote and a 何意味 separately', () => {
    expect(tallyAfterVote(tally, 0, 1)).toEqual({ upvotes: 6, downvotes: 2 })
    expect(tallyAfterVote(tally, 0, -1)).toEqual({ upvotes: 5, downvotes: 3 })
  })
  it('moves a changed vote from one count to the other', () => {
    expect(tallyAfterVote(tally, 1, -1)).toEqual({ upvotes: 4, downvotes: 3 })
    expect(tallyAfterVote(tally, -1, 1)).toEqual({ upvotes: 6, downvotes: 1 })
  })
  it('takes a withdrawn vote out of its own count', () => {
    expect(tallyAfterVote(tally, 1, 0)).toEqual({ upvotes: 4, downvotes: 2 })
    expect(tallyAfterVote(tally, -1, 0)).toEqual({ upvotes: 5, downvotes: 1 })
  })
})
