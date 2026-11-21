import { describe, expect, it } from 'vitest'
import { clearSignInFailure } from './sign-in-failure'

describe('clearSignInFailure', () => {
  it('takes the failed sign-in mark off and keeps everything else', () => {
    expect(clearSignInFailure(new URLSearchParams('error=sso'))?.toString()).toBe('')
    expect(
      clearSignInFailure(new URLSearchParams('returnTo=%2Fme&error=sso&page=2'))?.toString(),
    ).toBe('returnTo=%2Fme&page=2')
  })

  it('leaves addresses without it alone', () => {
    expect(clearSignInFailure(new URLSearchParams(''))).toBeNull()
    expect(clearSignInFailure(new URLSearchParams('page=2'))).toBeNull()
    expect(clearSignInFailure(new URLSearchParams('error=other'))).toBeNull()
  })

  it('does not change the address it was given', () => {
    const params = new URLSearchParams('error=sso&q=don')
    clearSignInFailure(params)
    expect(params.toString()).toBe('error=sso&q=don')
  })
})
