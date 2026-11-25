import { afterEach, expect, it, vi } from 'vitest'
import {
  itemName,
  layersOf,
  legacyLayers,
  partsOf,
  plateSpecs,
  plateState,
  rankPicture,
  readLumen,
  titleSpecs,
  type LumenCatalog,
} from './nameplate-layers'

const canonicalRoot = 'https://distrib.ourtaiko.org/'
const mirrorRoot = 'https://mirror.example/ourtaiko/'
const sha256 = 'a'.repeat(64)

const catalog: LumenCatalog = {
  states: {
    '1p_shougou_dani': [
      { role: 'base', clip: '1p_shougou_dani/3' },
      { role: 'title_instance' },
      { role: 'base', clip: '1p_shougou_dani/8' },
      { role: 'plate_player_no' },
      { role: 'text_title', x: 226.5, y: 34.5, width: 215, size: 22 },
      { role: 'dani' },
      { role: 'text_name', x: 261, y: 67.5, width: 190, size: 24, stroke: 3 },
      { role: 'blank_instance' },
      { role: 'rankup_instance' },
    ],
    newentry_shougou: [
      { role: 'base', clip: 'newentry_shougou/3' },
      { role: 'title_instance' },
      { role: 'text_title' },
    ],
  },
  titles: {
    wood: { band: 'band/lumen-title-wood', effect: null, icon: null },
    gold: { band: 'band/lumen-title-gold', effect: 'effect/lumen-title-gold', icon: null },
    ai_3: { band: 'band/lumen-title-ai_3', effect: 'effect/lumen-title-ai_3', icon: null },
    toho_y22_1: {
      band: 'band/lumen-title-toho_y22',
      effect: null,
      icon: 'icon/lumen-title-toho_y22_qr',
    },
    toho_y22_2: {
      band: 'band/lumen-title-toho_y22',
      effect: null,
      icon: 'icon/lumen-title-toho_y22_2',
    },
  },
  aiTitles: { wood: { band: 'band/lumen-ai-title-wood', effect: null, icon: null } },
  unclipped: ['effect/lumen-title-gold'],
}

afterEach(() => vi.unstubAllGlobals())

it('reads the set and gives every file its address from the visitor root and the canonical one', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      Response.json({ mirror: true, baseUrl: mirrorRoot, canonicalBaseUrl: canonicalRoot }),
    ),
  )
  const set = await readLumen({
    version: 1,
    catalog,
    files: {
      'band/lumen-title-wood': {
        path: 'lumen/band/lumen-title-wood-0123456789ab.otanim',
        size: 10,
        sha256,
      },
    },
  })
  expect(set.catalog.titles.gold.effect).toBe('effect/lumen-title-gold')
  expect(set.files['band/lumen-title-wood']).toEqual({
    url: mirrorRoot + 'nameplate/lumen/band/lumen-title-wood-0123456789ab.otanim',
    fallbackUrl: canonicalRoot + 'nameplate/lumen/band/lumen-title-wood-0123456789ab.otanim',
    size: 10,
    sha256,
  })
})

it('refuses a set whose files or titles are not what the game carries', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      Response.json({ mirror: false, baseUrl: canonicalRoot, canonicalBaseUrl: canonicalRoot }),
    ),
  )
  const file = { path: 'lumen/band/x-0123456789ab.otanim', size: 10, sha256 }
  for (const bad of [
    { version: 2, catalog, files: {} },
    { version: 1, catalog, files: { 'band/x': { ...file, path: '../x.otanim' } } },
    { version: 1, catalog, files: { 'hat/x': file } },
    { version: 1, catalog, files: { 'band/x': { ...file, size: 0 } } },
    { version: 1, catalog: { ...catalog, titles: { x: { band: 'icon/x' } } }, files: {} },
    {
      version: 1,
      catalog: { ...catalog, titles: { x: { band: 'band/x', icon: 'band/y' } } },
      files: {},
    },
  ])
    await expect(readLumen(bad)).rejects.toThrow('NAMEPLATE_LAYERS_INVALID')
})

it('dresses a title from before the layers as its band did', () => {
  expect(legacyLayers('gold')).toEqual({
    band: 'band/lumen-title-gold',
    effect: 'effect/lumen-title-gold',
    icon: null,
  })
  expect(legacyLayers('rainbow')).toEqual({
    band: 'band/lumen-title-rainbow',
    effect: null,
    icon: null,
  })
  expect(legacyLayers('teal')).toEqual({ band: 'band/lumen-title-wood', effect: null, icon: null })
  const own = {
    band: 'band/lumen-title-toho_y22',
    effect: null,
    icon: 'icon/lumen-title-toho_y22_2',
  }
  expect(layersOf({ style: 'wood', layers: own })).toBe(own)
  expect(layersOf({ style: 'purple' }).effect).toBe('effect/lumen-title-purple')
})

it('offers each part once, with the titles it comes from, and not the AI ones', () => {
  expect(partsOf(catalog, 'band').map((p) => p.id)).toEqual([
    'band/lumen-title-wood',
    'band/lumen-title-gold',
    'band/lumen-title-ai_3',
    'band/lumen-title-toho_y22',
  ])
  expect(
    partsOf(catalog, 'band').find((p) => p.id === 'band/lumen-title-toho_y22')?.titles,
  ).toEqual(['toho_y22_1', 'toho_y22_2'])
  expect(partsOf(catalog, 'icon')).toHaveLength(2)
  expect(itemName('band/lumen-title-toho_y22')).toBe('toho_y22')
  expect(itemName('band/lumen-ai-title-wood')).toBe('wood')
})

it('picks the layout and the 段位 picture as the game does', () => {
  expect(plateState(false, false)).toBe('1p_coin')
  expect(plateState(true, false, 2)).toBe('2p_shougou')
  expect(plateState(true, true)).toBe('1p_shougou_dani')
  expect(plateState(false, true)).toBe('1p_dani')
  expect(rankPicture(5, false, 0)).toBeNull()
  expect(rankPicture(6, false, 0)).toEqual({ id: 'composition/dan-clear', clip: '5kyuu_red' })
  expect(rankPicture(24, true, 2)).toEqual({ id: 'composition/dan-donfull', clip: '14dan_gold' })
})

it('cuts an effect that stays inside the band to its shape, and lets a burst run outside it', () => {
  expect(titleSpecs(catalog, catalog.titles.ai_3).band).toEqual([
    { id: 'band/lumen-title-ai_3' },
    { id: 'band/lumen-title-wood', mask: true },
    { id: 'effect/lumen-title-ai_3', cut: true },
  ])
  expect(titleSpecs(catalog, catalog.titles.gold).band).toEqual([
    { id: 'band/lumen-title-gold' },
    { id: 'effect/lumen-title-gold' },
  ])
})

it('draws a plate in its layout’s order, the icon over the outline and under the badge', () => {
  const specs = plateSpecs(
    catalog,
    catalog.states['1p_shougou_dani'],
    catalog.titles.toho_y22_1,
    '1p',
    rankPicture(10, false, 1),
  )
  expect(specs).toEqual([
    { id: 'composition/plates', clip: '1p_shougou_dani/3' },
    { id: 'band/lumen-title-toho_y22' },
    { id: 'composition/plates', clip: '1p_shougou_dani/8' },
    { id: 'icon/lumen-title-toho_y22_qr' },
    { id: 'composition/badges', clip: '1p' },
    { id: 'composition/dan-full', clip: '1kyuu_red' },
    { id: 'composition/blank', clip: 'off' },
  ])
  // A layout without a badge puts the icon right after the band.
  expect(
    plateSpecs(catalog, catalog.states.newentry_shougou, catalog.titles.toho_y22_2, '1p', null),
  ).toEqual([
    { id: 'composition/plates', clip: 'newentry_shougou/3' },
    { id: 'band/lumen-title-toho_y22' },
    { id: 'icon/lumen-title-toho_y22_2' },
  ])
})
