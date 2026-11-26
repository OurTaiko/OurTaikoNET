import { askSource, fallbackFileUrl, fetchFile, fileUrl } from './distribution'
import { parseOtanim, type Otanim } from './don3d/otanim'
import { verifiedResource, type ResourceFile } from './resource-download'

// The layered nameplate the game draws (the set OurTaikoPlay's Tools/nameplate-samples/lumen
// bakes, published to the bucket's nameplate/ by tools/nameplate/publish-lumen.sh, where the game
// downloads it from as well): every title is a band, an effect over it and an icon on its end,
// each an .otanim drawn on the whole 408x96 plate canvas, and the plate itself is a list of
// layouts saying what to draw in which order and where the text goes.

export type Slot = 'band' | 'effect' | 'icon'
export const slots: Slot[] = ['band', 'effect', 'icon']

/** How a title looks: items of the set, "<slot>/<name>"; a title has a band, the rest may be none. */
export type TitleLayers = { band: string; effect: string | null; icon: string | null }

/** One thing a layout draws, in order: a picture (role) or a text box (text_title, text_name). */
export type PlateRow = {
  role: string
  clip?: string
  x?: number
  y?: number
  width?: number
  size?: number
  rgba?: number[]
  stroke?: number
}

export type LumenCatalog = {
  states: Record<string, PlateRow[]>
  titles: Record<string, TitleLayers>
  aiTitles: Record<string, TitleLayers>
  // Effects that run outside the band; every other effect is cut to the band's shape.
  unclipped: string[]
}

export type Lumen = { catalog: LumenCatalog; files: Record<string, ResourceFile> }

const itemId = /^(band|effect|icon|composition)\/[a-z0-9][a-z0-9_.-]{0,63}$/
const filePath =
  /^lumen\/(band|effect|icon|composition)\/[a-z0-9][a-z0-9_.-]{0,63}-[a-f0-9]{12}\.otanim$/

function readTitles(value: unknown): Record<string, TitleLayers> {
  if (!value || typeof value !== 'object') throw new Error('NAMEPLATE_LAYERS_INVALID')
  const titles: Record<string, TitleLayers> = {}
  for (const [label, parts] of Object.entries(value as Record<string, Record<string, unknown>>)) {
    const band = parts?.band
    if (typeof band !== 'string' || !band.startsWith('band/'))
      throw new Error('NAMEPLATE_LAYERS_INVALID')
    const optional = (slot: Slot) => {
      const item = parts[slot]
      if (item === undefined || item === null) return null
      if (typeof item !== 'string' || !item.startsWith(slot + '/'))
        throw new Error('NAMEPLATE_LAYERS_INVALID')
      return item
    }
    titles[label] = { band, effect: optional('effect'), icon: optional('icon') }
  }
  return titles
}

/** Checks lumen.json and gives every file its addresses from the visitor's root. */
export async function readLumen(value: unknown): Promise<Lumen> {
  const data = value as { version?: number; catalog?: Record<string, unknown>; files?: unknown }
  if (data?.version !== 1 || !data.catalog || !data.files || typeof data.files !== 'object')
    throw new Error('NAMEPLATE_LAYERS_INVALID')
  const states = data.catalog.states
  if (!states || typeof states !== 'object') throw new Error('NAMEPLATE_LAYERS_INVALID')
  const catalog: LumenCatalog = {
    states: states as Record<string, PlateRow[]>,
    titles: readTitles(data.catalog.titles),
    aiTitles: readTitles(data.catalog.aiTitles),
    unclipped: Array.isArray(data.catalog.unclipped) ? (data.catalog.unclipped as string[]) : [],
  }
  const source = await askSource()
  const files: Record<string, ResourceFile> = {}
  for (const [id, file] of Object.entries(
    data.files as Record<string, { path: string; size: number; sha256: string }>,
  )) {
    if (
      !itemId.test(id) ||
      !filePath.test(file?.path) ||
      !/^[a-f0-9]{64}$/.test(file.sha256) ||
      !Number.isSafeInteger(file.size) ||
      file.size <= 0 ||
      file.size > 8 * 1024 * 1024
    )
      throw new Error('NAMEPLATE_LAYERS_INVALID')
    const path = 'nameplate/' + file.path
    files[id] = {
      url: fileUrl(path, source),
      fallbackUrl: fallbackFileUrl(path, source) || undefined,
      size: file.size,
      sha256: file.sha256,
    }
  }
  return { catalog, files }
}

// One read of lumen.json per page load; a failed read is tried again next time it is asked for.
let lumen: Promise<Lumen> | undefined
export function loadLumen(): Promise<Lumen> {
  lumen ??= fetchFile('nameplate/lumen.json', { credentials: 'omit' })
    .then((response) => {
      if (!response.ok) throw new Error('NAMEPLATE_LAYERS_UNAVAILABLE')
      return response.json()
    })
    .then(readLumen)
    .catch((error: unknown) => {
      lumen = undefined
      throw error
    })
  return lumen
}

/** An item of the set and its picture, ready to draw. */
export type LayerItem = { file: Otanim; picture: ImageBitmap }

const items = new Map<string, Promise<LayerItem>>()

/** The item `id` ("band/lumen-title-gold"), read and checked once per page load. */
export function loadItem(set: Lumen, id: string): Promise<LayerItem> {
  let item = items.get(id)
  if (!item) {
    const file = set.files[id]
    item = file
      ? verifiedResource(file).then(async (bytes) => {
          const otanim = await parseOtanim(new Uint8Array(bytes))
          const picture = await createImageBitmap(
            new Blob([otanim.texture as BlobPart], { type: 'image/png' }),
            {
              premultiplyAlpha: 'none',
              colorSpaceConversion: 'none',
            },
          )
          return { file: otanim, picture }
        })
      : Promise.reject(new Error('NAMEPLATE_LAYER_MISSING'))
    item.catch(() => items.delete(id))
    items.set(id, item)
  }
  return item
}

/** A new title's look until one is picked: the plain wood band. */
export const plainLook: TitleLayers = { band: 'band/lumen-title-wood', effect: null, icon: null }

/** How a title saved before the layers looks (the database gave every such title these too). */
export function legacyLayers(style: string): TitleLayers {
  const named = style === 'gold' || style === 'purple' || style === 'rainbow' ? style : 'wood'
  return {
    band: `band/lumen-title-${named}`,
    effect: named === 'gold' || named === 'purple' ? `effect/lumen-title-${named}` : null,
    icon: null,
  }
}

/** A title's layers, or those of its style when it came from before them (an old reward). */
export function layersOf(title: { layers?: TitleLayers | null; style?: string }): TitleLayers {
  return title.layers?.band ? title.layers : legacyLayers(title.style ?? 'wood')
}

/**
 * The items a title may be made of in one slot: the parts of the catalog's titles (not the AI
 * ones, which are cut for the AI plate), each with the titles it comes from, in catalog order.
 */
export function partsOf(catalog: LumenCatalog, slot: Slot): { id: string; titles: string[] }[] {
  const found = new Map<string, string[]>()
  for (const [label, parts] of Object.entries(catalog.titles)) {
    const id = parts[slot]
    if (!id) continue
    const titles = found.get(id) ?? []
    titles.push(label)
    found.set(id, titles)
  }
  return [...found].map(([id, titles]) => ({ id, titles }))
}

/** An item's name without its slot and the set's prefix: "toho_y22" for band/lumen-title-toho_y22. */
export function itemName(id: string): string {
  return id.slice(id.indexOf('/') + 1).replace(/^lumen-(ai-)?title-/, '')
}

/** The layout a player's plate takes (the game's PlateComposition.StateFor). */
export function plateState(hasTitle: boolean, hasDan: boolean, side: 1 | 2 = 1): string {
  const prefix = side === 2 ? '2p' : '1p'
  if (!hasTitle && !hasDan) return prefix + '_coin'
  return prefix + (hasTitle ? '_shougou' : '') + (hasDan ? '_dani' : '')
}

/** The 段位 emblem's file and clip (the game's PlateComposition.RankAsset); null below 五級. */
export function rankPicture(
  index: number,
  gold: boolean,
  combo: number,
): { id: string; clip: string } | null {
  if (index < 6 || index > 24) return null
  const level = index - 6
  const rank = level < 5 ? `${5 - level}kyuu` : `${level - 4}dan`
  const ring = combo === 2 ? 'donfull' : combo === 1 ? 'full' : 'clear'
  return { id: `composition/dan-${ring}`, clip: `${rank}_${gold ? 'gold' : 'red'}` }
}

// TMP sets the game's glyphs lower than the original's text, and the game raises each row by
// this much; the website's text sits the same (NameplateLayout.TextLift).
export function textLift(size: number): number {
  return size === 22 ? 2.5 : size === 24 ? 3 : size === 30 ? 7 : 0
}

/** A picture of a plate before it is read: an item of the set and the clip of it to play. */
export type LayerSpec = { id: string; clip?: string; mask?: boolean; cut?: boolean }

// The shape an effect that stays inside the band is cut to: the plain band (the game's
// NameplateLayers stencil).
export const bandShape = 'band/lumen-title-wood'

/**
 * A title's band; its effect (with the band's shape it is cut to, if it stays inside the band),
 * which goes over the title's text printed on the band; and its icon, which the plate draws later.
 */
export function titleSpecs(
  catalog: LumenCatalog,
  look: TitleLayers,
): { band: LayerSpec[]; effect: LayerSpec[]; icon: LayerSpec | null } {
  const effect: LayerSpec[] = !look.effect
    ? []
    : catalog.unclipped.includes(look.effect)
      ? [{ id: look.effect }]
      : [
          { id: bandShape, mask: true },
          { id: look.effect, cut: true },
        ]
  return { band: [{ id: look.band }], effect, icon: look.icon ? { id: look.icon } : null }
}

/**
 * A plate's pictures back to front, as the game's PlateComposition draws a layout's rows, in two
 * parts with the title's text between them: `under`, up to the title's band, and `over`, the
 * title's effect (which sparkles over the text), then its icon over the outline (with the badge's
 * row, or right after the effect on a layout without one), the badge, the 段位 emblem. A plate
 * without a title has all its pictures under.
 */
export function plateSpecs(
  catalog: LumenCatalog,
  rows: PlateRow[],
  look: TitleLayers | null,
  badge: '1p' | '2p',
  rank: { id: string; clip: string } | null,
): { under: LayerSpec[]; over: LayerSpec[] } {
  const title = look ? titleSpecs(catalog, look) : null
  const hasBadge = rows.some((row) => row.role === 'plate_player_no')
  const under: LayerSpec[] = []
  const over: LayerSpec[] = []
  let specs = under
  for (const row of rows) {
    switch (row.role) {
      case 'base':
        if (row.clip) specs.push({ id: 'composition/plates', clip: row.clip })
        break
      case 'title_instance':
      case 'ai_title_instance':
        if (!title) break
        specs.push(...title.band)
        specs = over
        specs.push(...title.effect)
        if (!hasBadge && title.icon) specs.push(title.icon)
        break
      case 'plate_player_no':
        if (title?.icon) specs.push(title.icon)
        specs.push({ id: 'composition/badges', clip: badge })
        break
      case 'blank_instance':
        specs.push({ id: 'composition/blank', clip: 'off' })
        break
      case 'dani':
        if (rank) specs.push(rank)
        break
    }
  }
  return { under, over }
}
