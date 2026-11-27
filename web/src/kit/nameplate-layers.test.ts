import { afterEach, expect, it, vi } from 'vitest'
import {
  itemName,
  layersOf,
  legacyLayers,
  plateSpecs,
  plateState,
  rankPicture,
  readSet,
  titleSpecs,
  type PlateLayout,
} from './nameplate-layers'

const canonicalRoot = 'https://distrib.ourtaiko.org/'
const mirrorRoot = 'https://mirror.example/ourtaiko/'
const sha256 = 'a'.repeat(64)

const cut = { effectOverflow: false }
const looks = {
  wood: { band: 'band/lumen-title-wood', effect: null, icon: null, ...cut },
  gold: {
    band: 'band/lumen-title-gold',
    effect: 'effect/lumen-title-gold',
    icon: null,
    effectOverflow: true,
  },
  ai_3: { band: 'band/lumen-title-ai_3', effect: 'effect/lumen-title-ai_3', icon: null, ...cut },
  toho_y22_1: {
    band: 'band/lumen-title-toho_y22',
    effect: null,
    icon: 'icon/lumen-title-toho_y22_qr',
    ...cut,
  },
  toho_y22_2: {
    band: 'band/lumen-title-toho_y22',
    effect: null,
    icon: 'icon/lumen-title-toho_y22_2',
    ...cut,
  },
}

const layout: PlateLayout = {
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
}

afterEach(() => vi.unstubAllGlobals())

it('reads the set and gives every file its address from the visitor root and the canonical one', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      Response.json({ mirror: true, baseUrl: mirrorRoot, canonicalBaseUrl: canonicalRoot }),
    ),
  )
  const set = await readSet({ version: 1, layout, files: { 'band/lumen-title-wood': sha256 } })
  expect(Object.keys(set.layout.states)).toContain('1p_shougou_dani')
  // A picture is named by its hash, and checked against it.
  expect(set.files['band/lumen-title-wood']).toEqual({
    url: mirrorRoot + `nameplate/items/${sha256}.otanim`,
    fallbackUrl: canonicalRoot + `nameplate/items/${sha256}.otanim`,
    sha256,
  })
})

it('refuses a set whose files or layouts are not what the game carries', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      Response.json({ mirror: false, baseUrl: canonicalRoot, canonicalBaseUrl: canonicalRoot }),
    ),
  )
  for (const bad of [
    { version: 2, layout, files: {} },
    { version: 1, layout, files: { 'band/x': '../x' } },
    { version: 1, layout, files: { 'hat/x': sha256 } },
    { version: 1, layout, files: { 'band/x': { sha256 } } },
    // Without the plate's layouts there is nothing to draw a plate by.
    { version: 1, layout: {}, files: {} },
  ])
    await expect(readSet(bad)).rejects.toThrow('NAMEPLATE_LAYERS_INVALID')
})

it('dresses a title from before the layers as its band did', () => {
  expect(legacyLayers('gold')).toEqual({
    band: 'band/lumen-title-gold',
    effect: 'effect/lumen-title-gold',
    icon: null,
    // The glint ran over the whole plate.
    effectOverflow: true,
  })
  expect(legacyLayers('rainbow')).toEqual({
    band: 'band/lumen-title-rainbow',
    effect: null,
    icon: null,
    effectOverflow: false,
  })
  expect(legacyLayers('teal')).toEqual(looks.wood)
  const own = looks.toho_y22_2
  expect(layersOf({ style: 'wood', layers: own })).toBe(own)
  expect(layersOf({ style: 'purple' }).effect).toBe('effect/lumen-title-purple')
})

it('names a part without its slot and the set’s prefix', () => {
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

it('cuts an effect to the band’s shape unless its title lets it run over the plate', () => {
  expect(titleSpecs(looks.ai_3)).toEqual({
    band: [{ id: 'band/lumen-title-ai_3' }],
    effect: [
      { id: 'band/lumen-title-wood', mask: true },
      { id: 'effect/lumen-title-ai_3', cut: true },
    ],
    icon: null,
  })
  expect(titleSpecs(looks.gold).effect).toEqual([{ id: 'effect/lumen-title-gold' }])
  // The same effect either way: it is the title's to say.
  expect(titleSpecs({ ...looks.ai_3, effectOverflow: true }).effect).toEqual([
    { id: 'effect/lumen-title-ai_3' },
  ])
  expect(titleSpecs({ ...looks.gold, effectOverflow: false }).effect).toEqual([
    { id: 'band/lumen-title-wood', mask: true },
    { id: 'effect/lumen-title-gold', cut: true },
  ])
  expect(titleSpecs(looks.wood).effect).toEqual([])
})

it('draws a plate in its layout’s order, the icon over the outline and under the badge', () => {
  const specs = plateSpecs(
    layout.states['1p_shougou_dani'],
    looks.toho_y22_1,
    '1p',
    rankPicture(10, false, 1),
  )
  // The title's name goes between the two: on the band, under the rest.
  expect(specs).toEqual({
    under: [
      { id: 'composition/plates', clip: '1p_shougou_dani/3' },
      { id: 'band/lumen-title-toho_y22' },
    ],
    over: [
      { id: 'composition/plates', clip: '1p_shougou_dani/8' },
      { id: 'icon/lumen-title-toho_y22_qr' },
      { id: 'composition/badges', clip: '1p' },
      { id: 'composition/dan-full', clip: '1kyuu_red' },
      { id: 'composition/blank', clip: 'off' },
    ],
  })
  // A layout without a badge puts the icon right after the band.
  expect(plateSpecs(layout.states.newentry_shougou, looks.toho_y22_2, '1p', null)).toEqual({
    under: [
      { id: 'composition/plates', clip: 'newentry_shougou/3' },
      { id: 'band/lumen-title-toho_y22' },
    ],
    over: [{ id: 'icon/lumen-title-toho_y22_2' }],
  })
})

it('puts the title’s effect over its name and the rest of the plate over the effect', () => {
  const { under, over } = plateSpecs(layout.states['1p_shougou_dani'], looks.ai_3, '1p', null)
  expect(under.at(-1)).toEqual({ id: 'band/lumen-title-ai_3' })
  expect(over.slice(0, 3)).toEqual([
    { id: 'band/lumen-title-wood', mask: true },
    { id: 'effect/lumen-title-ai_3', cut: true },
    { id: 'composition/plates', clip: '1p_shougou_dani/8' },
  ])
  // Without a title there is no name to put anything over.
  const plain = plateSpecs(layout.states.newentry_shougou, null, '1p', null)
  expect(plain).toEqual({
    under: [{ id: 'composition/plates', clip: 'newentry_shougou/3' }],
    over: [],
  })
})
