// What the site's frame (./shell) knows of the area being shown. The frame is drawn once, above
// the three areas (src/root.tsx), so that it stays put when a link crosses from one area to
// another; each area's layout tells it where the page is and who is signed in there, and puts
// its notification bell into the frame's top bar.
import { useLayoutEffect, useSyncExternalStore } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { areaOf, type SourceId } from '../source'

export type FrameUser = { id: string; nickname: string; avatarPath?: string }

export type Told = {
  /** The area shown: a source, or '' for the pages of no source. */
  area: SourceId | ''
  /** The address inside the area, as its router has it. */
  path: string
  /** Who is signed in; while the area's session loads, the one before. */
  user: FrameUser | null
  loading: boolean
  /** Whether the menu offers the area's own chart manager. */
  uploadsHere: boolean
}

function first(): Told {
  const area = areaOf(location.pathname) ?? ''
  return {
    area,
    path: area ? location.pathname.slice(area.length + 1) || '/' : location.pathname,
    user: null,
    loading: true,
    uploadsHere: false,
  }
}

let told: Told = first()
let signOutHere: () => void = () => {}
let bellSlot: HTMLElement | null = null
const watchers = new Set<() => void>()
const tell = () => watchers.forEach((watcher) => watcher())
const watch = (watcher: () => void) => {
  watchers.add(watcher)
  return () => void watchers.delete(watcher)
}

/** In the frame: what the area told. */
export const useTold = () => useSyncExternalStore(watch, () => told)
/** In the frame: signs out through the area's session. */
export const signOut = () => signOutHere()
/** In the frame: where an area's bell goes. */
export function setBellSlot(slot: HTMLElement | null) {
  if (slot === bellSlot) return
  bellSlot = slot
  tell()
}

/** In an area's layout: tells the frame about the page shown. */
export function useTellFrame({
  area,
  path,
  user,
  loading,
  uploadsHere,
  signOut,
}: Omit<Told, 'user'> & { user: FrameUser | null; signOut: () => void }) {
  useLayoutEffect(() => {
    signOutHere = signOut
  })
  const id = user?.id
  const nickname = user?.nickname
  const avatarPath = user?.avatarPath
  useLayoutEffect(() => {
    const who: FrameUser | null = loading
      ? told.user
      : id && nickname !== undefined
        ? { id, nickname, avatarPath }
        : null
    if (
      told.area === area &&
      told.path === path &&
      told.loading === loading &&
      told.uploadsHere === uploadsHere &&
      told.user?.id === who?.id &&
      told.user?.nickname === who?.nickname &&
      told.user?.avatarPath === who?.avatarPath
    )
      return
    told = { area, path, user: who, loading, uploadsHere }
    tell()
  }, [area, path, id, nickname, avatarPath, loading, uploadsHere])
}

/** In an area's layout: shows its notification bell in the frame's top bar. */
export function FrameBell({ children }: { children: ReactNode }) {
  const slot = useSyncExternalStore(watch, () => bellSlot)
  return slot ? createPortal(children, slot) : null
}
