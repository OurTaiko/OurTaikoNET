import { describe, expect, it } from 'vitest'
import type { TFunction } from 'i18next'
import type { RewardRule } from '@players/api/types'
import zh from './locales/zh-Hans.json'
import en from './locales/en.json'
import ja from './locales/ja.json'
import ko from './locales/ko.json'
import { chartIdOf, ruleKinds, ruleText } from './reward-rules'

// A translator over the players' own words (and the two source names of the shared ones).
const translator = (words: object) =>
  ((key: string, options: Record<string, unknown> = {}) => {
    const all = { ...words, shell: { fanmade: '自制', official: '官方' } }
    const found = key.split('.').reduce<unknown>((at, part) => (at as never)?.[part], all)
    const text = typeof found === 'string' ? found : String(options.defaultValue ?? key)
    return text.replace(/{{(\w+)}}/g, (_, name) => String(options[name]))
  }) as unknown as TFunction

const rule = (fields: Partial<RewardRule>): RewardRule => ({
  id: 1,
  item: 'title',
  titleId: 't',
  donPart: null,
  donItem: null,
  kind: 'site_login',
  source: null,
  chartId: null,
  chart: null,
  danId: null,
  dan: null,
  category: null,
  course: null,
  clear: null,
  count: null,
  ...fields,
})

describe('reward rules in words', () => {
  const t = translator(zh)
  it('says what each kind asks for', () => {
    expect(ruleText(rule({}), t, 'zh-Hans')).toBe('登录网站')
    expect(ruleText(rule({ kind: 'game_login' }), t, 'zh-Hans')).toBe('登录游戏')
    expect(
      ruleText(
        rule({
          kind: 'chart_clear',
          source: 'official',
          chartId: 'c',
          chart: {
            id: 'c',
            title: 'さいたま2000',
            titleTranslations: { zh: '埼玉2000' },
            gone: false,
          },
          course: 'Oni',
          clear: 2,
        }),
        t,
        'zh-Hans',
      ),
    ).toBe('全连官方曲源的《埼玉2000》（魔王及以上）')
    expect(
      ruleText(
        rule({ kind: 'category_clear', source: 'official', category: 'namco', clear: 1 }),
        t,
        'zh-Hans',
      ),
    ).toBe('通关官方曲源「南梦宫原创」分类的全部歌曲')
    expect(
      ruleText(
        rule({ kind: 'category_clear', source: 'fanmade', category: 'anime', clear: 3, count: 10 }),
        t,
        'zh-Hans',
      ),
    ).toBe('全良自制曲源「动画」分类的 10 首歌曲')
    expect(ruleText(rule({ kind: 'charts_uploaded', count: 5 }), t, 'zh-Hans')).toBe(
      '参与制作 5 张谱面',
    )
  })
  it('names a 段位 course and the verdict it takes, and says when it gives nothing', () => {
    const dan = (fixed: boolean, gone: boolean) =>
      rule({
        kind: 'dan_clear',
        source: 'fanmade',
        danId: 'd',
        dan: { id: 'd', title: { ja: '十段', zh: '十段位' }, code: 'ABC123', fixed, gone },
        clear: 5,
      })
    expect(ruleText(dan(true, false), t, 'zh-Hans')).toBe(
      '在自制曲源的段位《十段位》取得金合格・全连以上',
    )
    expect(ruleText(dan(false, false), t, 'zh-Hans')).toBe(
      '在自制曲源的段位《十段位》取得金合格・全连以上（段位已取消固定，暂不发放）',
    )
    expect(ruleText(dan(true, true), t, 'zh-Hans')).toBe(
      '在自制曲源的段位《十段位》取得金合格・全连以上（段位已删除）',
    )
  })
  it('marks a chart that is gone, and keeps a category it has no word for', () => {
    const gone = rule({
      kind: 'chart_clear',
      source: 'fanmade',
      chartId: 'c',
      chart: { id: 'c', title: 'Song', titleTranslations: {}, gone: true },
      clear: 1,
    })
    expect(ruleText(gone, t, 'zh-Hans')).toBe('通关自制曲源的《Song》（歌曲已下架）')
    expect(
      ruleText(
        rule({ kind: 'category_clear', source: 'fanmade', category: 'new', clear: 1 }),
        t,
        'zh-Hans',
      ),
    ).toContain('「new」')
  })
  it('has every word in every language', () => {
    for (const words of [zh, en, ja, ko]) {
      for (const kind of ruleKinds) expect(words.rules.kinds[kind]).toBeTruthy()
      for (const verdict of ['1', '2', '3', '4', '5', '6'] as const)
        expect(words.rules.verdicts[verdict]).toBeTruthy()
      // The categories are the backend's; every language names the same ones.
      for (const source of ['fanmade', 'official'] as const)
        expect(Object.keys(words.rules.categories[source]).sort()).toEqual(
          Object.keys(zh.rules.categories[source]).sort(),
        )
    }
  })
  it('takes a chart ID out of a pasted address', () => {
    const id = '0123456789abcdef0123456789abcdef'
    expect(chartIdOf(` https://net.ourtaiko.org/fanmade/charts/${id}?tab=scores `)).toBe(id)
    expect(chartIdOf(id)).toBe(id)
    expect(chartIdOf('/official/charts/short-id')).toBe('short-id')
  })
})
