// The rules that give a title by themselves (the backend's internal/titlerules), in words.
import type { TFunction } from 'i18next'
import { chartText } from '@sitekit/chart-language'
import type { Course, TitleRule } from '@players/api/types'

export const ruleKinds = [
  'chart_clear',
  'category_clear',
  'charts_uploaded',
  'site_login',
  'game_login',
] as const
export const ruleSources = ['fanmade', 'official'] as const
export const ruleCourses: Course[] = ['Easy', 'Normal', 'Hard', 'Oni', 'Edit']
// Each source's categories, as its backend names them (songs.Fanmade, songs.Official).
export const ruleCategories: Record<(typeof ruleSources)[number], string[]> = {
  fanmade: ['game', 'virtual-singer', 'pop', 'classic', 'variety', 'anime'],
  official: ['namco', 'pop', 'game', 'anime', 'vocaloid', 'variety', 'kids', 'classics'],
}

/** A pasted chart page address or a bare ID: the chart's ID. */
export const chartIdOf = (text: string) =>
  /[0-9a-f]{32}/.exec(text.trim())?.[0] ?? text.trim().split(/[/?#]/).filter(Boolean).pop() ?? ''

/** A category's name; an ID these pages have no word for stays as it is. */
export const categoryName = (t: TFunction, source: string, category: string): string =>
  t(`rules.categories.${source}.${category}` as 'rules.categories.fanmade.game', {
    defaultValue: category,
  })

export type RuleClear = 1 | 2 | 3
const clearOf = (clear: number | null): RuleClear => (clear === 2 || clear === 3 ? clear : 1)

/** What a player has to do to meet the rule. */
export function ruleText(rule: TitleRule, t: TFunction, language: string): string {
  if (rule.kind === 'site_login' || rule.kind === 'game_login') return t(`rules.text.${rule.kind}`)
  const source = rule.source ? t(`shell.${rule.source}`) : ''
  if (rule.kind === 'charts_uploaded')
    return t('rules.text.charts_uploaded', { source, count: rule.count ?? 0 })
  const words = {
    source,
    clear: t(`rules.clears.${clearOf(rule.clear)}`),
    course: rule.course ? t('rules.atLeast', { course: t(`courses.${rule.course}`) }) : '',
  }
  if (rule.kind === 'chart_clear') {
    const chart = rule.chart
    const title = chart ? chartText(chart, language).title || chart.id : (rule.chartId ?? '')
    return (
      t('rules.text.chart_clear', { ...words, title }) + (chart?.gone ? t('rules.chartGone') : '')
    )
  }
  const category = rule.category && rule.source ? categoryName(t, rule.source, rule.category) : ''
  return rule.count === null
    ? t('rules.text.category_all', { ...words, category })
    : t('rules.text.category_count', { ...words, category, count: rule.count })
}
