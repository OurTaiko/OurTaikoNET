import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { useNotification } from './notification-context'

/**
 * The address without the mark the server adds to the page it sends a failed sign-in to
 * (internal/signin's ErrorPage: ?error=sso), or null when it carries no such mark.
 */
export function clearSignInFailure(params: URLSearchParams): URLSearchParams | null {
  if (params.get('error') !== 'sso') return null
  const cleared = new URLSearchParams(params)
  cleared.delete('error')
  return cleared
}

/**
 * A sign-in that failed comes back as ?error=sso. Say so once, in a dialog, and take it off the
 * address at once: left there, it would come back with every reload and be carried along in
 * returnTo to wherever the next sign-in leads. Each area's Layout calls this.
 */
export function useSignInFailure() {
  const { t } = useTranslation()
  const { notify } = useNotification()
  const [params, setParams] = useSearchParams()
  // What the address's query becomes without the mark; null when there is none.
  const cleared = clearSignInFailure(params)?.toString() ?? null
  // StrictMode runs an effect twice on the same address; say it once.
  const told = useRef(false)
  useEffect(() => {
    if (cleared === null) {
      told.current = false
      return
    }
    if (told.current) return
    told.current = true
    notify(t('messages.signInWasIncompleteOrAuthorizationExpiredPleaseSignInAgain'), 'error')
    setParams(cleared, { replace: true, preventScrollReset: true })
  }, [cleared, notify, setParams, t])
}
