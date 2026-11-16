// The signed-in player's own 3D Don, standing in a corner of every page (the look they chose on
// their page, from the public bucket). A click makes it jump to the middle of the screen: their
// nameplate below it, and above it three ways on: their playlists, their page, their charts. A
// click outside, Escape or the Don again sends it back. Loaded lazily by the frame, so three.js
// comes only with a signed-in player; it draws nothing until the catalog and look are read.
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { CSSProperties, MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { FileTextIcon, PlaylistIcon, UserIcon } from '@phosphor-icons/react'
import { api } from '../api/client'
import { SITE_API_ROOT } from '../api/config'
import { danNames, usePlate } from '../components/profile-plate'
import { NameplateArt } from '../components/nameplate-art'
import { DonPreview, type DonLook, type DonMotionName } from '../don3d'
import { readDonCatalog, type ServedDonCatalog } from '../don3d/catalog-request'
import { cn } from '../lib/utils'
import { visit } from '../navigation'
import type { SourceId } from '../source'

// The Don is drawn once, at its size in the middle, and scaled down in the corner, so moving it
// is a transform and never a new canvas size.
const BIG = 260 // its height in the middle
const SMALL = 120 // and in the corner
// Its frame, in the game's layout units: the feet just above the bottom edge, and room on both
// sides and above for the widest and tallest costumes and the puchichara (the game's DonPreview
// keeps 240 a side; a narrower frame cut costumes off at its edges).
const frame = { width: 520, height: 440, pivotX: 0.5, pivotY: 0.03 }
const WIDE = frame.width / frame.height
const JUMP_MS = 650
const BUTTONS = 48 // the row of buttons above the Don, and its gap
const PLATE_GAP = 4

type Look = { catalog: ServedDonCatalog; look: DonLook }
// The same for every area's frame (the frame is drawn anew when the area changes).
const looks = new Map<string, Promise<Look>>()
function readLook(userId: string): Promise<Look> {
  let found = looks.get(userId)
  if (!found) {
    found = Promise.all([
      readDonCatalog(),
      api<{ look: DonLook }>(`${SITE_API_ROOT}/users/${encodeURIComponent(userId)}/don`),
    ]).then(([catalog, answer]) => ({ catalog, look: answer.look }))
    found.catch(() => looks.delete(userId))
    looks.set(userId, found)
  }
  return found
}

type Viewport = { width: number; height: number; wide: boolean }
function useViewport(): Viewport {
  const read = () => ({
    width: window.innerWidth,
    height: window.innerHeight,
    wide: window.matchMedia('(min-width: 1024px)').matches,
  })
  const [viewport, setViewport] = useState(read)
  useEffect(() => {
    const update = () => setViewport(read())
    window.addEventListener('resize', update)
    return () => window.removeEventListener('resize', update)
  }, [])
  return viewport
}

export default function DonBuddy({
  user,
  source,
}: {
  user: { id: string; nickname: string }
  source: SourceId
}) {
  const { t, i18n } = useTranslation()
  const [don, setDon] = useState<Look | null>(null)
  const [open, setOpen] = useState(false)
  const [motion, setMotion] = useState<DonMotionName>('SelectLoop')
  const plate = usePlate(user.id)
  const viewport = useViewport()
  const buddy = useRef<HTMLButtonElement>(null)
  const middle = useRef<HTMLAnchorElement>(null)
  const still =
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches

  useEffect(() => {
    let current = true
    setDon(null)
    readLook(user.id).then(
      (found) => current && setDon(found),
      () => undefined, // an extra on every page: without it the page is as it was
    )
    return () => {
      current = false
    }
  }, [user.id])

  // A jump there and back, then a happy loop in the middle or the idle one in the corner.
  const moved = useRef(false)
  useEffect(() => {
    const rest: DonMotionName = open ? 'EntryLoop' : 'SelectLoop'
    if (!moved.current) {
      moved.current = open
      setMotion(rest)
      return
    }
    setMotion('GeneralJump')
    const settle = window.setTimeout(() => setMotion(rest), JUMP_MS)
    return () => window.clearTimeout(settle)
  }, [open])

  const close = useCallback(() => setOpen(false), [])
  useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, close])
  // Focus follows: to the player's page when it opens, back to the Don when it closes.
  const wasOpen = useRef(false)
  useLayoutEffect(() => {
    if (open) middle.current?.focus({ preventScroll: true })
    else if (wasOpen.current) buddy.current?.focus({ preventScroll: true })
    wasOpen.current = open
  }, [open])

  if (!don) return null

  // Where the Don stands: in the corner (above the phone's bottom bar), or in the middle with the
  // buttons over it and the plate under it, the three centred together.
  const { width, height, wide } = viewport
  const plateWidth = Math.min(360, width - 32)
  const plateHeight = (plateWidth * 81) / 329
  const column = BUTTONS + BIG + PLATE_GAP + plateHeight
  const top = Math.max(16, (height - column) / 2)
  const corner = {
    x: width - SMALL * WIDE - (wide ? 24 : 16),
    y: height - SMALL - (wide ? 24 : 92),
  }
  const centre = { x: (width - BIG * WIDE) / 2, y: top + BUTTONS }
  const at = open ? centre : corner
  const scale = open ? 1 : SMALL / BIG
  const move = (curve: string) => (still ? 'none' : `transform ${JUMP_MS}ms cubic-bezier(${curve})`)
  // The sideways move and the growth are even; the vertical one is a hop: up to the middle it
  // rises past and lands, down to the corner it springs up first and drops.
  const xStyle: CSSProperties = {
    transform: `translateX(${at.x}px)`,
    transition: move('0.4, 0, 0.2, 1'),
  }
  const yStyle: CSSProperties = {
    transform: `translateY(${at.y}px)`,
    transition: move(open ? '0.3, 0, 0.4, 1.5' : '0.4, -0.7, 0.6, 1'),
  }
  const sizeStyle: CSSProperties = {
    transform: `scale(${scale})`,
    transformOrigin: '0 0',
    transition: move('0.4, 0, 0.2, 1'),
  }

  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  const title = plate?.title
    ? {
        name: plate.title.nameTranslations?.[language] || plate.title.name,
        style: plate.title.style,
      }
    : null
  const dan = plate?.dan
    ? {
        index: plate.dan.index,
        gold: plate.dan.gold,
        combo: plate.dan.combo ?? 0,
        label: danNames[plate.dan.index] ?? '',
      }
    : null
  const go = (event: MouseEvent<HTMLAnchorElement>, href: string) => {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return
    event.preventDefault()
    setOpen(false)
    visit(href)
  }
  const ways = [
    { key: 'playlists', href: `/${source}/me/playlists`, icon: PlaylistIcon },
    { key: 'page', href: `/players/${encodeURIComponent(user.id)}`, icon: UserIcon },
    { key: 'charts', href: '/fanmade/me/charts', icon: FileTextIcon },
  ] as const

  return (
    <>
      {open && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label={t('shell.buddy.title', { name: user.nickname })}
          className="z-50 fixed inset-0"
        >
          <button
            type="button"
            tabIndex={-1}
            aria-label={t('shell.buddy.close')}
            className="absolute inset-0 bg-foreground/40 backdrop-blur-[2px] animate-in fade-in duration-200 cursor-default"
            onClick={close}
          />
          <nav
            aria-label={t('shell.buddy.ways')}
            className="absolute inset-x-0 flex justify-center gap-2 px-4 animate-in fade-in slide-in-from-bottom-2 duration-300 motion-reduce:animate-none"
            style={{ top }}
          >
            {ways.map((way) => (
              <a
                key={way.key}
                ref={way.key === 'page' ? middle : undefined}
                href={way.href}
                onClick={(event) => go(event, way.href)}
                className={cn(
                  'inline-flex items-center gap-1.5 shadow-elev-2 px-4 rounded-full h-10 font-bold text-sm whitespace-nowrap outline-none focus-visible:ring-3 focus-visible:ring-ring transition-colors',
                  way.key === 'page'
                    ? 'bg-primary text-primary-foreground hover:bg-primary/90'
                    : 'bg-card text-foreground hover:bg-muted',
                )}
              >
                <way.icon className="size-4 shrink-0" aria-hidden="true" />
                {t(`shell.buddy.${way.key}`)}
              </a>
            ))}
          </nav>
          <div
            className="absolute animate-in fade-in slide-in-from-top-2 duration-300 motion-reduce:animate-none"
            style={{
              top: top + BUTTONS + BIG + PLATE_GAP,
              left: (width - plateWidth) / 2,
              width: plateWidth,
            }}
          >
            <NameplateArt name={user.nickname} title={title} dan={dan} />
          </div>
        </div>
      )}
      {/* The Don itself: one canvas that moves between the corner and the middle. */}
      <div
        className={cn('fixed top-0 left-0 pointer-events-none', open ? 'z-50' : 'z-30')}
        style={xStyle}
      >
        <div style={yStyle}>
          <div style={sizeStyle}>
            <button
              ref={buddy}
              type="button"
              aria-label={t(open ? 'shell.buddy.close' : 'shell.buddy.open')}
              aria-expanded={open}
              aria-haspopup="dialog"
              onClick={() => setOpen((o) => !o)}
              className="block rounded-[32px] outline-none focus-visible:ring-3 focus-visible:ring-ring pointer-events-auto cursor-pointer"
              style={{ width: BIG * WIDE, height: BIG }}
            >
              <DonPreview
                catalog={don.catalog}
                baseUrl={don.catalog.baseUrl}
                look={don.look}
                motion={motion}
                frame={frame}
                className="size-full pointer-events-none"
              />
            </button>
          </div>
        </div>
      </div>
    </>
  )
}
