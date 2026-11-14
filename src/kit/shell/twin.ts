// Where the source switch goes. Both sources have the same pages, so changing source on one
// of them means "this page, of the other source's songs".

/** The pages every source has, as the address reads after the source. */
const shared = [
  '/',
  '/playlists',
  '/dans',
  '/dans/new',
  '/me/playlists',
  '/me/profile',
  '/login',
  '/register',
]
/** The chart manager and its publish form: only for those who may upload to the source. */
const uploading = ['/me/charts', '/me/charts/publish']

/**
 * The page of the other source that matches `pathname` (an address after the source, as the
 * source's router has it). The same page where both sources have it. One song, playlist or
 * dan course belongs to its own source and has no counterpart, so those give the list of
 * their kind. mayUpload: whether the player may upload to the other source.
 */
export function twinPath(pathname: string, mayUpload: boolean): string {
  const path = pathname.replace(/\/+$/, '') || '/'
  if (shared.includes(path)) return path
  if (uploading.includes(path) || path === '/upload') return mayUpload ? path : '/'
  if (path.startsWith('/playlists/')) return '/playlists'
  if (path.startsWith('/dans/')) return '/dans'
  return '/'
}
