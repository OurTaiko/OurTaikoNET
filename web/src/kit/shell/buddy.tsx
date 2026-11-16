// The signed-in player's Don lives above the three areas (src/root.tsx draws it beside the area
// shown), so it keeps moving when a link crosses from one area to another and the area's frame
// is drawn anew. Each frame tells it who is signed in once its session is known; while a new
// area's session is still loading, the Don stays as it was.
import { Suspense, lazy, useEffect, useSyncExternalStore } from 'react'
import type { SourceId } from '../source'

type Rider = { id: string; nickname: string; source: SourceId } | null
let rider: Rider = null
const watchers = new Set<() => void>()

/** From the frame: the player signed in there (null: nobody), and the source its links use. */
export function useTellBuddy(
  user: { id: string; nickname: string } | null,
  loading: boolean,
  source: SourceId,
) {
  const id = user?.id
  const nickname = user?.nickname
  useEffect(() => {
    if (loading) return
    const next: Rider = id && nickname !== undefined ? { id, nickname, source } : null
    if (
      next?.id === rider?.id &&
      next?.nickname === rider?.nickname &&
      next?.source === rider?.source
    )
      return
    rider = next
    watchers.forEach((watcher) => watcher())
  }, [id, nickname, loading, source])
}

// Pages whose bottom right corner is theirs (an open chat's send button) put the Don away while
// they are shown. It is only hidden, so it is as it was when it comes back.
let hiders = 0
const hideWatchers = new Set<() => void>()

/** On a page that needs the corner the Don stands in: hides it while `hide` holds. */
export function useHideBuddy(hide: boolean) {
  useEffect(() => {
    if (!hide) return
    hiders++
    hideWatchers.forEach((watcher) => watcher())
    return () => {
      hiders--
      hideWatchers.forEach((watcher) => watcher())
    }
  }, [hide])
}

const DonBuddy = lazy(() => import('./don-buddy'))

/** The Don, drawn once for the whole site; three.js comes only with a signed-in player. */
export function FrameBuddy() {
  const current = useSyncExternalStore(
    (watcher) => {
      watchers.add(watcher)
      return () => void watchers.delete(watcher)
    },
    () => rider,
  )
  const hidden = useSyncExternalStore(
    (watcher) => {
      hideWatchers.add(watcher)
      return () => void hideWatchers.delete(watcher)
    },
    () => hiders > 0,
  )
  if (!current) return null
  return (
    <div className={hidden ? 'hidden' : 'contents'}>
      <Suspense fallback={null}>
        <DonBuddy user={current} source={current.source} />
      </Suspense>
    </div>
  )
}
