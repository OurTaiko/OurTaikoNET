import { expect, it } from 'vitest'
import { readScenePresets, scenePresetChoices, stripJsonComments } from './scene-presets'

const catalog = `{
  // The game's stage catalog, as written by hand.
  "version": 1,
  /* Mode-only presets come first,
     then the ones a playlist may name. */
  "presets": {
    "DAN": { "header": "dojo" },
    "TOHO": { "header": "toho", "url": "https://example.org//x" }, // trailing note
    "PRACTICE": {},
    "ANIMAL": { "soloOnly": true },
    "TUTORIAL": {},
    "not valid!": {},
    "${'A'.repeat(33)}": {},
    "KIDS_2": { "note": "a \\" quote // and /* not a comment */" }
  },
  "sets": { "DEFAULT": {} }
}`

it('strips comments but keeps strings that look like them', () => {
  const parsed = JSON.parse(stripJsonComments(catalog)) as {
    presets: Record<string, { url?: string; note?: string }>
  }
  expect(parsed.presets.TOHO.url).toBe('https://example.org//x')
  expect(parsed.presets.KIDS_2.note).toBe('a " quote // and /* not a comment */')
})

it('lists the presets a playlist can name, in file order', () => {
  expect(readScenePresets(catalog)).toEqual(['TOHO', 'ANIMAL', 'KIDS_2'])
})

it('refuses text that is not a catalog', () => {
  expect(() => readScenePresets('{"version":1}')).toThrow()
  expect(() => readScenePresets('{"presets":[]}')).toThrow()
  expect(() => readScenePresets('not json')).toThrow()
})

it('offers none first and keeps a current preset the catalog lacks', () => {
  expect(scenePresetChoices(['TOHO', 'ANIMAL'], '')).toEqual(['', 'TOHO', 'ANIMAL'])
  expect(scenePresetChoices(['TOHO', 'ANIMAL'], 'ANIMAL')).toEqual(['', 'TOHO', 'ANIMAL'])
  expect(scenePresetChoices(['TOHO'], 'OLD')).toEqual(['', 'TOHO', 'OLD'])
  expect(scenePresetChoices([], 'OLD')).toEqual(['', 'OLD'])
})
