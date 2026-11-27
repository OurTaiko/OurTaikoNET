import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react'
import { cn } from '../lib/utils'

import { loadNameplateArt } from '../nameplate-art-source'
import {
  itemIconLeft,
  loadItem,
  loadSet,
  plateSpecs,
  plateState,
  rankPicture,
  textLift,
  titleRoom,
  titleSpecs,
  type LayerSpec,
  type NameplateSet,
  type PlateRow,
  type TitleLayers,
} from '../nameplate-layers'
import { usePlateCanvas, type DrawLayer, type PlateFrame } from '../nameplate-renderer'

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
  art ??= loadNameplateArt()
    .then((value) => value as Art)
    .catch(() => {
      art = undefined
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

// draw_in_box: text centred in its box and, when wider than the box, squeezed horizontally to its
// width (names); `even` shrinks it evenly instead, keeping its shape (titles, as the game draws them).
// `room` is how wide the text may be, as a fraction of the box's width, still centred.
function Squeezed({
  children,
  style,
  className,
  even = false,
  room = 1,
}: {
  children: ReactNode
  style: CSSProperties
  className?: string
  even?: boolean
  room?: number
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
      setScale(width > 0 ? Math.min(1, (outer.clientWidth * room) / width) : 1)
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(outer)
    return () => observer.disconnect()
  }, [children, room])
  return (
    <span
      ref={box}
      className={cn('absolute flex items-center justify-center', className)}
      style={style}
    >
      <span
        ref={text}
        className="inline-block whitespace-nowrap"
        style={{
          transform: scale < 1 ? (even ? `scale(${scale})` : `scaleX(${scale})`) : undefined,
        }}
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

// What the layered plate is drawn from (lib nameplate-layers): the catalog once, and each item's
// file and picture once, whoever asks.
let setLoaded: NameplateSet | null = null
/** The layered nameplate set (GET /api/v2/nameplate/set), null until it is read. */
export function useNameplateSet(): NameplateSet | null {
  const [value, setValue] = useState(setLoaded)
  useEffect(() => {
    if (value) return
    let current = true
    loadSet()
      .then((set) => {
        setLoaded = set
        if (current) setValue(set)
      })
      .catch(() => {})
    return () => {
      current = false
    }
  }, [value])
  return value
}

/**
 * The pictures of each part of `parts`, read together, so that the canvases drawing them start
 * at once; null until they are. One that cannot be read is left out.
 */
function useLayers(set: NameplateSet | null, parts: LayerSpec[][]): DrawLayer[][] | null {
  const key = JSON.stringify(parts)
  const [layers, setLayers] = useState<{ key: string; layers: DrawLayer[][] } | null>(null)
  useEffect(() => {
    if (!set) return
    let current = true
    const wanted = JSON.parse(key) as LayerSpec[][]
    const read = (part: LayerSpec[]) =>
      Promise.all(
        part.map((spec) =>
          loadItem(set, spec.id).then(
            (item): DrawLayer | null => {
              const clip =
                item.file.clips.find((c) => c.name === (spec.clip ?? 'idle')) ??
                (spec.clip ? undefined : item.file.clips[0])
              return clip ? { ...item, clip, mask: spec.mask, cut: spec.cut } : null
            },
            (): DrawLayer | null => null,
          ),
        ),
      ).then((got) => got.filter((l): l is DrawLayer => l !== null))
    void Promise.all(wanted.map(read)).then((got) => {
      if (current) setLayers({ key, layers: got })
    })
    return () => {
      current = false
    }
  }, [set, key])
  return layers?.key === key ? layers.layers : null
}

/** Where the icon `id` really starts on the plate (iconLeft); null without one, or until it is read. */
function useIconLeft(set: NameplateSet | null, id: string | null): number | null {
  const [left, setLeft] = useState<{ id: string; left: number } | null>(null)
  useEffect(() => {
    if (!set || !id) return
    let current = true
    loadItem(set, id).then(
      (item) => {
        if (current) setLeft({ id, left: itemIconLeft(item) })
      },
      () => {},
    )
    return () => {
      current = false
    }
  }, [set, id])
  return id && left?.id === id ? left.left : null
}

/** A canvas drawing `layers` over `frame` of the plate, placed by `style`. */
function PlateLayers({
  layers,
  frame,
  style,
}: {
  layers: DrawLayer[] | null
  frame: PlateFrame
  style: CSSProperties
}) {
  const canvas = useRef<HTMLCanvasElement>(null)
  usePlateCanvas(canvas, layers, frame)
  return <canvas ref={canvas} aria-hidden className="pointer-events-none absolute" style={style} />
}

// Room around the plate for what stands out of it (an icon, a burst of sparkles), in plate pixels.
const OVER = 24

// The band in the 408x96 plate: what the plain wood band covers of the canvas (its picture's own
// extent, which every band shares), so that a title on its band alone is exactly the band's size.
const BAND = { x: 48, y: 16, w: 312, h: 29 }
// The title on its band, as every layout of the plate has it (text_title): 22 canvas pixels,
// centred at y 34.5 less the text's lift, which is a little under the band's middle. A plate
// puts it beside the badge; a band alone has none, and centres it between its round ends.
const TITLE = { size: 22, y: 34.5 - textLift(22), width: 270 }
// The canvases of a title on its band alone: the band and the room around it.
const bandStyle: CSSProperties = {
  left: `${(-OVER / BAND.w) * 100}%`,
  top: `${(-OVER / BAND.h) * 100}%`,
  width: `${((BAND.w + 2 * OVER) / BAND.w) * 100}%`,
  height: `${((BAND.h + 2 * OVER) / BAND.h) * 100}%`,
}

/**
 * A title on its band alone: the strip a nameplate wears, its band, effect and icon as the game
 * draws them (the icon and a burst of sparkles may stand out of it), and the title's name where
 * the plate puts it on the band. The element is the band: size it by its height.
 */
export function TitleBandArt({
  layers,
  name,
  className,
}: {
  layers: TitleLayers
  name: string
  className?: string
}) {
  const set = useNameplateSet()
  // The band under the title's name, its effect and icon over it.
  const specs = useMemo(() => {
    if (!set) return []
    const title = titleSpecs(layers)
    return [title.band, title.icon ? [...title.effect, title.icon] : title.effect]
  }, [set, layers])
  const drawn = useLayers(set, specs)
  // A band alone centres the title between its round ends, and keeps it clear of the icon too.
  const iconLeft = useIconLeft(set, layers.icon)
  const room = titleRoom(BAND.x + BAND.w / 2, TITLE.width, iconLeft) / TITLE.width
  const frame: PlateFrame = [
    BAND.x - OVER,
    BAND.y - OVER,
    BAND.x + BAND.w + OVER,
    BAND.y + BAND.h + OVER,
  ]
  return (
    <span
      className={cn(
        'inline-flex aspect-[312/29] h-5 max-w-full items-center align-middle',
        className,
      )}
      title={name}
    >
      {/* Where there is no room for the height asked for, the band is as wide as there is room
          and keeps its shape, in the middle of the height. */}
      <span className="@container relative block aspect-[312/29] w-full">
        <PlateLayers layers={drawn?.[0] ?? null} frame={frame} style={bandStyle} />
        <Squeezed
          even
          room={room}
          className="font-bold text-black leading-none"
          style={{
            left: `${((BAND.w - TITLE.width) / 2 / BAND.w) * 100}%`,
            width: `${(TITLE.width / BAND.w) * 100}%`,
            top: `${((TITLE.y - TITLE.size - BAND.y) / BAND.h) * 100}%`,
            height: `${((2 * TITLE.size) / BAND.h) * 100}%`,
            fontSize: `${(TITLE.size / BAND.w) * 100}cqw`,
            letterSpacing: `${(2 / BAND.w) * 100}cqw`,
          }}
        >
          {name}
        </Squeezed>
        <PlateLayers layers={drawn?.[1] ?? null} frame={frame} style={bandStyle} />
      </span>
    </span>
  )
}

// The part of the 408x96 canvas the plate covers (x 42..371, y 10..91).
const VIEW = { x: 42, y: 10, w: 329, h: 81 }
const place = (x: number, y: number, w: number, h: number): CSSProperties => ({
  left: `${((x - VIEW.x) / VIEW.w) * 100}%`,
  top: `${((y - VIEW.y) / VIEW.h) * 100}%`,
  width: `${(w / VIEW.w) * 100}%`,
  height: `${(h / VIEW.h) * 100}%`,
})
// The canvases of a whole plate: the plate and the room above and below it.
const PLATE_FRAME: PlateFrame = [0, -OVER, 408, 96 + OVER]
const plateStyle = place(0, -OVER, 408, 96 + 2 * OVER)
// Font sizes in canvas pixels, scaled with the plate (its width is VIEW.w canvas pixels).
const px = (n: number) => `${(n / VIEW.w) * 100}cqw`

export type NameplateArtProps = {
  name: string
  title: { name: string; layers: TitleLayers } | null
  dan: { index: number; gold: boolean; combo: number; label: string } | null
  /** 1 or 2: the 1P or 2P badge. */
  side?: 1 | 2
  className?: string
}

/**
 * The nameplate as the game draws it (PlateComposition): the layout a player's plate takes (with
 * or without a title and a 段位), its pictures in the layout's order with the title's band,
 * effect and icon among them, and the title and name in the layout's text boxes. Without title or
 * 段位 it is the coin plate. Scales with its width.
 */
export function NameplateArt({ name, title, dan, side = 1, className }: NameplateArtProps) {
  const set = useNameplateSet()
  const state = plateState(!!title, !!dan, side)
  const rows = set?.layout.states[state] ?? null
  const look = title?.layers ?? null
  const specs = useMemo(() => {
    if (!set || !rows) return []
    const rank = dan ? rankPicture(dan.index, dan.gold, dan.combo) : null
    const { under, over } = plateSpecs(rows, look, side === 2 ? '2p' : '1p', rank)
    return [under, over]
  }, [set, rows, look, side, dan])
  const drawn = useLayers(set, specs)
  // Before the catalog is read the text still goes where the plate puts it.
  const text = (role: 'text_title' | 'text_name') =>
    rows?.find((row) => row.role === role) ??
    (role === 'text_title'
      ? { role, x: 226.5, y: 34.5, width: 215, size: 22, stroke: 0 }
      : !title && !dan
        ? { role, x: 226, y: 53, width: 190, size: 30, stroke: 3 }
        : { role, x: dan ? 261 : 226, y: 67.5, width: 190, size: 24, stroke: 3 })
  const box = (row: PlateRow): CSSProperties => {
    const size = row.size ?? 24
    const y = (row.y ?? 0) - textLift(size)
    const width = row.width ?? 190
    return {
      ...place((row.x ?? 0) - width / 2, y - size, width, size * 2),
      fontSize: px(size),
      letterSpacing: px(2),
    }
  }
  const titleRow = text('text_title')
  const nameRow = text('text_name')
  const iconLeft = useIconLeft(set, look?.icon ?? null)
  const titleWidth = titleRow.width ?? 190
  const titleFits = titleRoom(titleRow.x ?? 0, titleWidth, iconLeft) / titleWidth
  return (
    <div
      className={cn('@container relative aspect-[329/81] w-full max-w-md select-none', className)}
      data-testid="nameplate"
    >
      {/* The title's name is printed on its band, under the title's effect and what the plate
          draws after it (the outline, icon, badge and 段位). */}
      <PlateLayers layers={drawn?.[0] ?? null} frame={PLATE_FRAME} style={plateStyle} />
      {dan && (
        <span
          role="img"
          aria-label={dan.label}
          title={dan.label}
          className="absolute"
          style={place(106, 44, 72, 40)}
        />
      )}
      {title && (
        <Squeezed
          even
          room={titleFits}
          className="font-bold text-black leading-none"
          style={box(titleRow)}
        >
          {title.name}
        </Squeezed>
      )}
      <PlateLayers layers={drawn?.[1] ?? null} frame={PLATE_FRAME} style={plateStyle} />
      <Squeezed
        className="font-bold text-white leading-none [paint-order:stroke_fill]"
        style={{ ...box(nameRow), WebkitTextStroke: `${px(2 * (nameRow.stroke ?? 3))} #000` }}
      >
        {name}
      </Squeezed>
    </div>
  )
}
