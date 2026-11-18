import { ApiError, createApi, jsonRequest } from '@sitekit/api/request'
import { i18n, t } from '../i18n'

export { ApiError, jsonRequest }

// The server's messages are Chinese; known codes are shown in the page's language.
export const api = createApi((data, status) => {
  const key = `errors.${data.code ?? ''}`
  if (data.code && i18n.exists(key)) return i18n.t(key as 'errors.unexpected', { status })
  return data.message || t('errors.unexpected', { status })
})
