import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '../lib/utils'
import type { DonCatalog, DonLook, DonRoot } from './catalog'
import type { DonMotionName } from './chara'
import {
  createDonRenderer,
  type DonFrame,
  type DonRenderer,
  type DonRendererState,
} from './renderer'

export type DonPreviewProps = {
  catalog: DonCatalog
  // Where the catalog's files are read from (catalog.ts DonRoot); keep the same object while it
  // does not change, as a new one makes a new renderer.
  root: DonRoot
  look: DonLook
  motion?: DonMotionName
  mirrored?: boolean
  bpm?: number
  // Holds the motion at a point of its loop (0..1).
  frozenAt?: number | null
  frame?: Partial<DonFrame>
  // A one-shot motion plays over and over (the dressing room shows one state at a time).
  repeatMotion?: boolean
  className?: string
}

// A live 3D Don in a canvas (its own) that fills this element (give it a size). The Don stands near the
// bottom centre; a small label shows while its parts download.
export function DonPreview({
  catalog,
  root,
  look,
  motion = 'Normal',
  mirrored = false,
  bpm = 120,
  frozenAt = null,
  frame,
  repeatMotion = false,
  className,
}: DonPreviewProps) {
  const { t } = useTranslation()
  const boxRef = useRef<HTMLDivElement>(null)
  const [renderer, setRenderer] = useState<DonRenderer | null>(null)
  const [state, setState] = useState<DonRendererState>({ loading: true, error: null })
  const frameKey = JSON.stringify(frame ?? {})

  useEffect(() => {
    const box = boxRef.current
    if (!box) return
    // a canvas a renderer: disposing one loses its context, so a remount draws on a new one
    const canvas = document.createElement('canvas')
    canvas.className = 'absolute inset-0 size-full'
    box.prepend(canvas)
    let created: DonRenderer
    try {
      created = createDonRenderer(canvas, {
        catalog,
        root,
        frame: JSON.parse(frameKey) as Partial<DonFrame>,
        repeatMotion,
        onStateChange: setState,
      })
    } catch (error) {
      // no WebGL
      setState({ loading: false, error: error instanceof Error ? error : new Error(String(error)) })
      return () => canvas.remove()
    }
    // The layout size, not the drawn one: a scaled-down Don (the frame's, in its corner) keeps the
    // resolution it has when shown at full size.
    const resize = () =>
      created.resize(canvas.clientWidth, canvas.clientHeight, window.devicePixelRatio || 1)
    resize()
    const observer = new ResizeObserver(resize)
    observer.observe(canvas)
    // the pixel ratio changes when the window moves to another screen or the page is zoomed
    let media: MediaQueryList | null = null
    const watchRatio = () => {
      media?.removeEventListener('change', onRatio)
      media = window.matchMedia(`(resolution: ${window.devicePixelRatio || 1}dppx)`)
      media.addEventListener('change', onRatio)
    }
    const onRatio = () => {
      resize()
      watchRatio()
    }
    watchRatio()
    setRenderer(created)
    return () => {
      observer.disconnect()
      media?.removeEventListener('change', onRatio)
      setRenderer(null)
      created.dispose()
      canvas.remove()
    }
  }, [catalog, root, frameKey, repeatMotion])

  const lookKey = JSON.stringify([
    look.costume,
    look.cos,
    look.head,
    look.body,
    look.puchi,
    look.paint ?? 0,
    look.colors,
  ])
  useEffect(() => {
    if (!renderer) return
    const [costume, cos, head, body, puchi, paint, colors] = JSON.parse(lookKey) as [
      boolean,
      number,
      number,
      number,
      number,
      number,
      DonLook['colors'],
    ]
    void renderer.setLook({ costume, cos, head, body, puchi, paint, colors })
  }, [renderer, lookKey])

  useEffect(() => renderer?.setMotion(motion), [renderer, motion])
  useEffect(() => renderer?.setMirrored(mirrored), [renderer, mirrored])
  useEffect(() => renderer?.setBpm(bpm), [renderer, bpm])
  useEffect(() => renderer?.freeze(frozenAt), [renderer, frozenAt])

  return (
    <div ref={boxRef} className={cn('relative', className)}>
      {(state.loading || state.error) && (
        <div
          role="status"
          className="text-muted-foreground pointer-events-none absolute inset-x-0 bottom-2 text-center text-xs"
        >
          {state.error ? t('messages.failedToLoadDon') : t('messages.loadingDon')}
        </div>
      )}
    </div>
  )
}
