import type { Title } from '@players/api/types'
import { plainLook, type TitleLayers } from '@sitekit/nameplate-layers'

// A title as a form keeps it while it is written: its words (a name, the name in the other
// languages, a description) and its look.

export const languages = ['en', 'ja', 'ko'] as const

/** The longest text that goes with a review, as the server counts it. */
export const noteLimit = 500

export type TitleDraft = {
  name: string
  description: string
  layers: TitleLayers
  en: string
  ja: string
  ko: string
}

export const emptyDraft: TitleDraft = {
  name: '',
  description: '',
  layers: plainLook,
  en: '',
  ja: '',
  ko: '',
}

/** The draft that changes nothing of a title. */
export function draftOf(
  title: Pick<Title, 'name' | 'nameTranslations' | 'description' | 'layers'>,
): TitleDraft {
  return {
    name: title.name,
    description: title.description,
    layers: title.layers,
    en: title.nameTranslations.en ?? '',
    ja: title.nameTranslations.ja ?? '',
    ko: title.nameTranslations.ko ?? '',
  }
}

/** A draft as the server takes a title. */
export function draftBody(draft: TitleDraft) {
  return {
    name: draft.name.trim(),
    nameTranslations: Object.fromEntries(
      languages.filter((l) => draft[l].trim()).map((l) => [l, draft[l].trim()]),
    ),
    description: draft.description,
    // The parts are picked by number; the rest of a look is the server's to say.
    layers: { ids: draft.layers.ids, effectOverflow: draft.layers.effectOverflow },
  }
}

/** A draft as a title to draw, before it is saved. */
export function draftTitle(draft: TitleDraft, from?: Title): Title {
  return {
    id: from?.id ?? '',
    style: from?.style ?? 'wood',
    visibility: from?.visibility ?? 'private',
    obtain: from?.obtain ?? '',
    ownerId: from?.ownerId ?? null,
    ...draftBody(draft),
    layers: draft.layers,
    name: draft.name.trim() || '—',
  }
}
