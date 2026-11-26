import { createContext, useContext } from 'react'
import type { ReactNode, RefObject } from 'react'
export type NoticeKind = 'error' | 'success' | 'info'
export type Notification = {
  message: ReactNode
  kind: NoticeKind
  title: string
  /** Where focus goes when the notice closes: the control the action came from. */
  finalFocus?: RefObject<HTMLElement | null>
}
/** A question put before an action, in the site's own dialog rather than the browser's. */
export type Confirmation = {
  message: ReactNode
  /** The button that goes ahead, named for the action ("Delete", "Fix songs"). */
  confirmLabel: string
  /** The dialog's title; a general one when left out. */
  title?: string
  /** For an action that removes or loses something: the button is drawn as destructive. */
  destructive?: boolean
}
export const NotificationContext = createContext<{
  publish: (id: string, notice: Notification) => void
  dismiss: (id: string) => void
  notify: (
    message: ReactNode,
    kind?: NoticeKind,
    title?: string,
    finalFocus?: RefObject<HTMLElement | null>,
  ) => void
  /** Resolves true when the person goes ahead, false when they cancel or close the dialog. */
  confirm: (request: Confirmation) => Promise<boolean>
} | null>(null)
export function useNotification() {
  const context = useContext(NotificationContext)
  if (!context) throw new Error('NotificationProvider missing')
  return context
}
