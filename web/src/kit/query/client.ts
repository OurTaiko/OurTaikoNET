// What the server has told this page: one cache for the whole site, above the three areas
// (src/root.tsx), so that it stays when a link crosses from one area to another. Pages read it
// through the queries of the part of the site they show; what they change, they invalidate.
import { QueryClient, focusManager } from '@tanstack/react-query'
import { ApiError } from '../api/request'

// A request that the server refused will be refused again; one that got no answer may not be.
export function shouldRetry(failures: number, error: unknown) {
  const refused = error instanceof ApiError && error.status >= 400 && error.status < 500
  return !refused && failures < 2
}

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Shown at once when a page is opened again, and asked again once this old.
      staleTime: 30_000,
      gcTime: 10 * 60_000,
      retry: shouldRetry,
    },
  },
})

// Coming back to a window that stayed visible (beside another) counts as coming back too.
focusManager.setEventListener((changed) => {
  if (typeof window === 'undefined') return
  const come = () => changed()
  window.addEventListener('focus', come)
  document.addEventListener('visibilitychange', come)
  return () => {
    window.removeEventListener('focus', come)
    document.removeEventListener('visibilitychange', come)
  }
})
