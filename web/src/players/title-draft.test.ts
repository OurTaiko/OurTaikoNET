import { describe, expect, it } from 'vitest'
import type { Title } from '@players/api/types'
import { draftBody, draftOf, draftTitle, emptyDraft } from './title-draft'

const title: Title = {
  id: 'ten-charts',
  name: '十谱达人',
  nameTranslations: { en: 'Ten-Chart Master', ko: '' },
  description: '说明',
  style: 'gold',
  layers: {
    band: 'band/lumen-title-gold',
    effect: 'effect/lumen-title-gold',
    icon: null,
    effectOverflow: true,
    ids: { band: 3, effect: 45, icon: null },
  },
  visibility: 'public',
  obtain: '上传 10 张谱面',
  ownerId: 'a'.repeat(32),
}

describe('a title as a form keeps it', () => {
  it('changes nothing of a title it was made from', () => {
    expect(draftBody(draftOf(title))).toEqual({
      name: '十谱达人',
      // A language without a name is left out, as the server keeps it.
      nameTranslations: { en: 'Ten-Chart Master' },
      description: '说明',
      // Its parts by number, and whether the effect overflows: the rest is the server's.
      layers: { ids: { band: 3, effect: 45, icon: null }, effectOverflow: true },
    })
  })

  it('trims the names and leaves out the languages left empty', () => {
    expect(
      draftBody({ ...emptyDraft, name: '  新称号 ', en: ' New ', ja: '   ', ko: '' }),
    ).toMatchObject({ name: '新称号', nameTranslations: { en: 'New' } })
  })

  it('draws as the title it will be, private and unnamed until it is saved', () => {
    expect(draftTitle(emptyDraft)).toMatchObject({
      id: '',
      name: '—',
      visibility: 'private',
      ownerId: null,
      layers: { band: 'band/lumen-title-wood', effect: null, icon: null, effectOverflow: false },
    })
    // A change to a title keeps what the form does not write.
    expect(draftTitle({ ...draftOf(title), name: '百谱达人' }, title)).toMatchObject({
      id: 'ten-charts',
      name: '百谱达人',
      visibility: 'public',
      obtain: '上传 10 张谱面',
      ownerId: title.ownerId,
    })
  })
})
