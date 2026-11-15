import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react'
import { cn } from '../lib/utils'

// The game's nameplate art, unchanged, from the public bucket (tools/nameplate uploads OurTaikoPlay's
// Assets/OurTaiko/Art/global/nameplate to https://distrib.ourtaiko.org/nameplate/). Every piece is
// placed as the game's NameplateView places it on the 408x96 plate canvas (Nijiiro nameplate.lua).
export const NAMEPLATE_ART_BASE = 'https://distrib.ourtaiko.org/nameplate/'

type ArtFile = { path: string; size: number; sha256: string }
type ArtKey =
  | 'shadow'
  | 'outline'
  | 'bands'
  | 'rainbow'
  | 'danBackground'
  | 'danRingSilver'
  | 'danRingGold'
  | 'danRingRainbow'
  | 'danText'
  | 'danTextGold'
  | 'badge1p'
  | 'badge2p'
type Art = Record<ArtKey, string>

// One catalog read per page load; files are asked for by hash, so a replaced file is never stale.
let art: Promise<Art | null> | undefined
function loadArt(): Promise<Art | null> {
  art ??= fetch(`${NAMEPLATE_ART_BASE}catalog.json`)
    .then((r) => (r.ok ? r.json() : null))
    .then((catalog: { files?: Record<ArtKey, ArtFile> } | null) => {
      if (!catalog?.files) return null
      const urls = {} as Art
      for (const [key, file] of Object.entries(catalog.files) as [ArtKey, ArtFile][])
        urls[key] = `${NAMEPLATE_ART_BASE}${file.path}?v=${file.sha256.slice(0, 12)}`
      return urls
    })
    .catch(() => {
      art = undefined // try again next time something needs it
      return null
    })
  return art
}

let loaded: Art | null = null
export function useNameplateArt(): Art | null {
  const [value, setValue] = useState(loaded)
  useEffect(() => {
    if (value) return
    let current = true
    void loadArt().then((a) => {
      if (a) loaded = a
      if (current) setValue(a)
    })
    return () => {
      current = false
    }
  }, [value])
  return value
}

export type TitleStyle = 'wood' | 'teal' | 'wood2' | 'purple' | 'gold' | 'rainbow'
// frame_top's rows 0..4 (PlayerInfo.titleBackground); rainbow is frame_top_rainbow.
const bandRows: Record<Exclude<TitleStyle, 'rainbow'>, number> = {
  wood: 0,
  teal: 1,
  wood2: 2,
  purple: 3,
  gold: 4,
}

// A piece of a sprite sheet stretched over its element: the sheet is sheetW x sheetH, the piece
// at (x, y) is w x h.
function sprite(
  url: string | undefined,
  sheetW: number,
  sheetH: number,
  x: number,
  y: number,
  w: number,
  h: number,
): CSSProperties {
  if (!url) return {}
  const at = (offset: number, sheet: number, box: number) =>
    sheet === box ? '0%' : `${(offset / (sheet - box)) * 100}%`
  return {
    backgroundImage: `url("${url}")`,
    backgroundSize: `${(sheetW / w) * 100}% ${(sheetH / h) * 100}%`,
    backgroundPosition: `${at(x, sheetW, w)} ${at(y, sheetH, h)}`,
    backgroundRepeat: 'no-repeat',
  }
}

// draw_in_box: text centred in its box and squeezed horizontally, never shrunk, to the box's width.
function Squeezed({
  children,
  style,
  className,
}: {
  children: ReactNode
  style: CSSProperties
  className?: string
}) {
  const box = useRef<HTMLSpanElement>(null)
  const text = useRef<HTMLSpanElement>(null)
  const [scale, setScale] = useState(1)
  useLayoutEffect(() => {
    const outer = box.current
    const inner = text.current
    if (!outer || !inner) return
    const measure = () => {
      const width = inner.offsetWidth
      setScale(width > 0 ? Math.min(1, outer.clientWidth / width) : 1)
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(outer)
    return () => observer.disconnect()
  }, [children])
  return (
    <span
      ref={box}
      className={cn('absolute flex items-center justify-center', className)}
      style={style}
    >
      <span
        ref={text}
        className="inline-block whitespace-nowrap"
        style={{ transform: scale < 1 ? `scaleX(${scale})` : undefined }}
      >
        {children}
      </span>
    </span>
  )
}

/**
 * The 段位 emblem as the game draws it: dan_ring_{silver,gold,rainbow} by the pass's combo, and the
 * rank's name from dan_text (red pass) or dan_text_gold (gold pass). 72x40; size it by its height.
 */
export function DanEmblem({
  index,
  gold,
  combo,
  className,
  label,
}: {
  index: number
  gold: boolean
  combo: number
  className?: string
  label: string
}) {
  const a = useNameplateArt()
  const ring = a
    ? ([a.danRingSilver, a.danRingGold, a.danRingRainbow][combo] ?? a.danRingSilver)
    : undefined
  const sheet = a ? (gold ? a.danTextGold : a.danText) : undefined
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className={cn('relative inline-block aspect-[72/40] h-10 shrink-0', className)}
      style={ring ? { backgroundImage: `url("${ring}")`, backgroundSize: '100% 100%' } : undefined}
    >
      <span
        className="absolute inset-0"
        style={sprite(sheet, 360, 200, (index % 5) * 72, Math.floor(index / 5) * 40, 72, 40)}
      />
    </span>
  )
}

// The band's opaque part in each 408x56 row (measured: x 46..362, y 14..48).
const BAND = { x: 46, y: 14, w: 316, h: 34 }

/** A title on its band alone, cut from the game's band art: the strip a nameplate wears. */
export function TitleBandArt({
  style,
  name,
  className,
}: {
  style: TitleStyle
  name: string
  className?: string
}) {
  const a = useNameplateArt()
  const rainbow = style === 'rainbow'
  const row = rainbow ? 0 : (bandRows[style] ?? 0)
  return (
    <span
      className={cn(
        '@container relative inline-block aspect-[316/34] h-7 max-w-full align-middle',
        className,
      )}
      style={sprite(
        rainbow ? a?.rainbow : a?.bands,
        408,
        rainbow ? 336 : 280,
        BAND.x,
        row * 56 + BAND.y,
        BAND.w,
        BAND.h,
      )}
      title={name}
    >
      {/* The game's title: 22 canvas pixels on a band 34 high (316 wide), black. */}
      <Squeezed
        className="font-bold text-black leading-none"
        style={{ inset: '0 8%', fontSize: `${(22 / BAND.w) * 100}cqw` }}
      >
        {name}
      </Squeezed>
    </span>
  )
}

// The part of the 408x96 canvas the plate's art covers (shadow / outline: x 42..371, y 10..91).
const VIEW = { x: 42, y: 10, w: 329, h: 81 }
const place = (x: number, y: number, w: number, h: number): CSSProperties => ({
  left: `${((x - VIEW.x) / VIEW.w) * 100}%`,
  top: `${((y - VIEW.y) / VIEW.h) * 100}%`,
  width: `${(w / VIEW.w) * 100}%`,
  height: `${(h / VIEW.h) * 100}%`,
})
// Font sizes in canvas pixels, scaled with the plate (its width is VIEW.w canvas pixels).
const px = (n: number) => `${(n / VIEW.w) * 100}cqw`
// A text box centred at (cx, cy), w wide.
const box = (cx: number, cy: number, w: number, fontSize: number): CSSProperties => ({
  ...place(cx - w / 2, cy - fontSize, w, fontSize * 2),
  fontSize: px(fontSize),
  letterSpacing: px(2),
})

export type NameplateArtProps = {
  name: string
  title: { name: string; style: TitleStyle } | null
  dan: { index: number; gold: boolean; combo: number; label: string } | null
  /** 1 or 2: the 1P or 2P badge. */
  side?: 1 | 2
  className?: string
}

/**
 * The nameplate as the game draws it (NameplateView): plate, title band, band outline, 段位 chip
 * (its plate, ring, then rank), 1P/2P badge, title, name. Without title or 段位 it is the coin
 * plate: plate, badge and a larger name. Scales with its width.
 */
export function NameplateArt({ name, title, dan, side = 1, className }: NameplateArtProps) {
  const a = useNameplateArt()
  const coin = !title && !dan
  const rainbow = title?.style === 'rainbow'
  const row = title && !rainbow ? (bandRows[title.style as Exclude<TitleStyle, 'rainbow'>] ?? 0) : 0
  const nameBox = coin
    ? box(226, 46, 190, 30)
    : dan
      ? box(261, 64.5, 190, 24)
      : box(226, 64.5, 190, 24)
  const layer = 'absolute pointer-events-none'
  return (
    <div
      className={cn('@container relative aspect-[329/81] w-full max-w-md select-none', className)}
      data-testid="nameplate"
    >
      {a && (
        <>
          <span
            className={layer}
            style={{ ...place(0, 0, 408, 96), ...sprite(a.shadow, 408, 96, 0, 0, 408, 96) }}
          />
          {!coin && (
            <>
              {rainbow ? (
                <>
                  <span
                    className={cn(layer, 'nameplate-rainbow-under')}
                    style={{
                      ...place(0, 0, 408, 56),
                      ...sprite(a.rainbow, 408, 336, 0, 0, 408, 56),
                    }}
                  />
                  <span
                    className={cn(layer, 'nameplate-rainbow')}
                    style={{
                      ...place(0, 0, 408, 56),
                      ...sprite(a.rainbow, 408, 336, 0, 0, 408, 56),
                    }}
                  />
                </>
              ) : (
                <span
                  className={layer}
                  style={{
                    ...place(0, 0, 408, 56),
                    ...sprite(a.bands, 408, 280, 0, row * 56, 408, 56),
                  }}
                />
              )}
              <span
                className={layer}
                style={{ ...place(0, 0, 408, 96), ...sprite(a.outline, 408, 96, 0, 0, 408, 96) }}
              />
            </>
          )}
          {dan && !coin && (
            <>
              <span
                className={layer}
                style={{
                  ...place(44, 44, 160, 48),
                  ...sprite(a.danBackground, 160, 48, 0, 0, 160, 48),
                }}
              />
              <span
                className={layer}
                style={{
                  ...place(106, 44, 72, 40),
                  ...sprite(
                    [a.danRingSilver, a.danRingGold, a.danRingRainbow][dan.combo] ??
                      a.danRingSilver,
                    72,
                    40,
                    0,
                    0,
                    72,
                    40,
                  ),
                }}
              />
              <span
                role="img"
                aria-label={dan.label}
                title={dan.label}
                className="absolute"
                style={{
                  ...place(106, 44, 72, 40),
                  ...sprite(
                    dan.gold ? a.danTextGold : a.danText,
                    360,
                    200,
                    (dan.index % 5) * 72,
                    Math.floor(dan.index / 5) * 40,
                    72,
                    40,
                  ),
                }}
              />
            </>
          )}
          <span
            className={layer}
            style={{
              ...place(36, 4, 88, 88),
              ...sprite(side === 2 ? a.badge2p : a.badge1p, 88, 88, 0, 0, 88, 88),
            }}
          />
        </>
      )}
      {title && (
        <Squeezed className="font-bold text-black leading-none" style={box(226.5, 32, 215, 22)}>
          {title.name}
        </Squeezed>
      )}
      <Squeezed
        className="font-bold text-white leading-none [paint-order:stroke_fill]"
        style={{ ...nameBox, WebkitTextStroke: `${px(6)} #000` }}
      >
        {name}
      </Squeezed>
    </div>
  )
}
