// How the site asks its API, whichever pages ask: each part of the site words errors its own way.
import { noticeRewards } from '../rewards/store'

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly retryAfter = 0,
    /** The answer's HTTP status; 0 when there was none. */
    public readonly status = 0,
    /** The API's code for the error; empty when it gave none. */
    public readonly code = '',
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

type ErrorBody = { code?: string; message?: string }

/**
 * A function that asks the API. `describe` words an error for the reader; `unreadable` stands in
 * for an answer that is not JSON.
 */
export function createApi(
  describe: (data: ErrorBody, status: number) => string,
  unreadable: (status: number) => ErrorBody = () => ({}),
) {
  return async function api<T>(url: string, init: RequestInit = {}): Promise<T> {
    const response = await fetch(url, { credentials: 'include', ...init })
    // Any answer may say the player was given something (../rewards).
    noticeRewards(response)
    const data = await response.json().catch((e: unknown) => {
      // A request given up while its answer was still arriving has no answer: the caller gets
      // the abort, not a made-up one (which, on a 200, would pass for the data asked for).
      if (init.signal?.aborted) throw e
      return unreadable(response.status)
    })
    if (!response.ok)
      throw new ApiError(
        describe(data, response.status),
        Number(response.headers.get('Retry-After')) || 0,
        response.status,
        data.code ?? '',
      )
    return data as T
  }
}

export function jsonRequest(method: string, body: unknown, csrf = ''): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
    body: JSON.stringify(body),
  }
}
