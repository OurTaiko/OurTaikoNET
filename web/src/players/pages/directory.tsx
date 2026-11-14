import { useRememberedSource } from '@sitekit/shell/site'
import { UsersPage } from '@sitekit/users'

/** The players, by what they did in one source's songs: the source the menu's switch is at. */
export function DirectoryPage() {
  return <UsersPage source={useRememberedSource()} />
}
