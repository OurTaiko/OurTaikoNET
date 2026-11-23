// The play backdrops (scene presets) a playlist can name: the keys of "presets" in the game's
// stage catalog, <distribution root>/stage/catalog.json. The game picks the backdrop of a song
// opened from a playlist by that name unless the chart names its own (SCENEPRESET). The catalog
// is the game's file, written by hand, so it may carry // and /* */ comments.
import { queryOptions } from '@tanstack/react-query'
import { fetchFile } from './distribution'

/** The file under the distribution root that lists the presets. */
export const stageCatalogPath = 'stage/catalog.json'

// Presets the game keeps for its own modes (段位道場, practice, tutorial): no playlist names them.
const modeOnly = new Set(['DAN', 'PRACTICE', 'TUTORIAL'])

// A preset's name as the server accepts it (SCENE_PRESET_INVALID otherwise).
const presetForm = /^[A-Za-z0-9_]{1,32}$/

/**
 * `text` without its // and /* *\/ comments, as JSON.parse takes it. Text inside strings is
 * kept as it is, so a URL's "//" stays.
 */
export function stripJsonComments(text: string): string {
  let out = ''
  let i = 0
  while (i < text.length) {
    const char = text[i]
    if (char === '"') {
      // A string, through its closing quote; an escaped quote does not close it.
      let end = i + 1
      while (end < text.length && text[end] !== '"') end += text[end] === '\\' ? 2 : 1
      out += text.slice(i, end + 1)
      i = end + 1
    } else if (char === '/' && text[i + 1] === '/') {
      const end = text.indexOf('\n', i)
      i = end === -1 ? text.length : end
    } else if (char === '/' && text[i + 1] === '*') {
      const end = text.indexOf('*/', i + 2)
      i = end === -1 ? text.length : end + 2
      // What the comment stood between stays apart.
      out += ' '
    } else {
      out += char
      i++
    }
  }
  return out
}

/**
 * The presets a playlist can choose from the catalog's text, in the file's order: every key of
 * "presets" but the mode-only ones and names the server would refuse. Throws when the text is
 * not a catalog.
 */
export function readScenePresets(text: string): string[] {
  const catalog = JSON.parse(stripJsonComments(text)) as { presets?: unknown } | null
  const presets = catalog?.presets
  if (!presets || typeof presets !== 'object' || Array.isArray(presets))
    throw new Error('STAGE_CATALOG_INVALID')
  return Object.keys(presets).filter((name) => !modeOnly.has(name) && presetForm.test(name))
}

/**
 * The choices for a playlist whose preset is `current`: none (""), then `presets`, and `current`
 * when it is not among them (the catalog changed or could not be read), so that saving keeps it.
 */
export function scenePresetChoices(presets: readonly string[], current: string): string[] {
  const choices = ['', ...presets]
  if (current && !presets.includes(current)) choices.push(current)
  return choices
}

/** The catalog's presets, read once per page load (the file changes with game releases). */
export const scenePresetQuery = () =>
  queryOptions({
    queryKey: ['stage', 'scenePresets'] as const,
    queryFn: async ({ signal }) => {
      const response = await fetchFile(stageCatalogPath, { credentials: 'omit', signal })
      if (!response.ok) throw new Error('STAGE_CATALOG_UNAVAILABLE')
      return readScenePresets(await response.text())
    },
    staleTime: Infinity,
    gcTime: Infinity,
  })
