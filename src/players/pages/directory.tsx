import { SourceTag } from '@sitekit/components/source-tag'
import { useRememberedSource } from '@sitekit/shell/site'
import { UsersPage } from '@sitekit/users'

/**
 * The players, by what they did in one source's songs: the source the menu's switch is at.
 * The tag under the heading says which.
 */
export function DirectoryPage() {
  const source = useRememberedSource()
  return (
    <UsersPage source={source}>
      <div className="pt-1">
        <SourceTag source={source} />
      </div>
    </UsersPage>
  )
}
