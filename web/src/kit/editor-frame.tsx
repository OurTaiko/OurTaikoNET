// The chart editor, OurTaikoEdit's web version, inside the site's frame: an iframe of the bare
// editor the server gives at /edit/app/ (internal/edit). It is of this site's origin, so what it
// keeps (the person's projects, in IndexedDB) is this site's, and it can be talked to.
//
// With `embed` the page hands it a chart and stores what it saves (chart-editor.tsx). The
// messages, each { source, type, … } (OurTaikoEdit's src/OurTaikoEdit.Browser/wwwroot/main.js):
//   from the editor  ready; opened | open-failed { message }; save { request, tja };
//                    dirty { dirty }; leave
//   to the editor    open { chart }; save-result { request, saved, message?, line? }
import { useEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from './lib/utils'

export const EDITOR_ADDRESS = '/edit/app/'

export type FromEditor =
  | { type: 'ready' }
  | { type: 'opened' }
  | { type: 'open-failed'; message: string }
  | { type: 'save'; request: number; tja: string }
  | { type: 'dirty'; dirty: boolean }
  | { type: 'leave' }

export type ToEditor =
  | {
      type: 'open'
      chart: {
        id: string
        name: string
        tjaName: string
        tja: string
        audioName: string
        audio: ArrayBuffer | null
      }
    }
  | { type: 'save-result'; request: number; saved: boolean; message?: string; line?: number }

/** Sends a message to the editor (an ArrayBuffer in it is moved, not copied). */
export function tellEditor(frame: HTMLIFrameElement | null, message: ToEditor) {
  const transfer = message.type === 'open' && message.chart.audio ? [message.chart.audio] : []
  frame?.contentWindow?.postMessage(
    { source: 'ourtaiko-net', ...message },
    location.origin,
    transfer,
  )
}

export function EditorFrame({
  embed = false,
  onMessage,
  frameRef,
  className,
}: {
  embed?: boolean
  onMessage?: (message: FromEditor) => void
  frameRef?: RefObject<HTMLIFrameElement | null>
  className?: string
}) {
  const { t, i18n } = useTranslation()
  // The editor's words follow the site's language as it is when the editor opens (its own
  // setting, when chosen, wins); changing it later would load the editor anew.
  const [address] = useState(() => {
    const query = new URLSearchParams({ lang: i18n.resolvedLanguage ?? i18n.language })
    if (embed) query.set('embed', '1')
    return `${EDITOR_ADDRESS}?${query}`
  })
  const own = useRef<HTMLIFrameElement>(null)
  const frame = frameRef ?? own
  const listener = useRef(onMessage)
  useEffect(() => {
    listener.current = onMessage
  })
  useEffect(() => {
    const hear = (event: MessageEvent) => {
      // Only this frame's editor, of this site.
      if (event.source !== frame.current?.contentWindow || event.origin !== location.origin) return
      const data = event.data as { source?: string } | null
      if (data?.source !== 'ourtaikoedit') return
      listener.current?.(data as unknown as FromEditor)
    }
    window.addEventListener('message', hear)
    return () => window.removeEventListener('message', hear)
  }, [frame])
  return (
    <iframe
      ref={frame}
      src={address}
      title={t('editor.frameTitle')}
      // The editor takes the keyboard (its shortcuts) and the clipboard (copying notes).
      allow="clipboard-read; clipboard-write; fullscreen"
      className={cn(
        'block h-[calc(100svh-6.5rem)] min-h-[32rem] w-full rounded-[28px] border bg-card',
        className,
      )}
    />
  )
}

/** /edit: the editor with the person's own projects, kept in this browser. */
export function EditorPage() {
  return <EditorFrame />
}
