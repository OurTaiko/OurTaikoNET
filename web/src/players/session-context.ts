// The site's session, with the player as the players' pages know them: their user groups too.
import { useSession as useSiteSession } from '@sitekit/session-context'
import type { User } from './api/types'

export const useSession = () => useSiteSession<User>()
