import { i18n, t } from '../i18n'
import { noticeRewards } from '@sitekit/rewards/store'

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status = 0,
    public readonly code = '',
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

// The server's messages are Chinese; known codes are shown in the page's language.
function errorMessage(data: { code?: string; message?: string }, status: number): string {
  const key = `errors.${data.code ?? ''}`
  if (data.code && i18n.exists(key)) return i18n.t(key as 'errors.unexpected', { status })
  return data.message || t('errors.unexpected', { status })
}

export async function api<T>(url: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(url, { credentials: 'include', ...init })
  // Any answer may say the player was given something (../rewards).
  noticeRewards(response)
  const data = await response.json().catch((e: unknown) => {
    // Given up while the answer was arriving: the caller gets the abort, not an empty answer.
    if (init.signal?.aborted) throw e
    return {}
  })
  if (!response.ok)
    throw new ApiError(errorMessage(data, response.status), response.status, data.code ?? '')
  return data as T
}

export function jsonRequest(method: string, body: unknown, csrf = ''): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
    body: JSON.stringify(body),
  }
}
