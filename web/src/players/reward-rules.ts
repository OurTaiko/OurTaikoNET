// The rules that give an item by themselves (the backend's internal/rewards/rules), in words.
import type { TFunction } from 'i18next'
import { chartText } from '@sitekit/chart-language'
import { danTitle } from '@sitekit/dan-title'
import type { Course, RewardRule } from '@players/api/types'

export const ruleKinds = [
  'chart_clear',
  'category_clear',
  'dan_clear',
  'charts_uploaded',
  'site_login',
  'game_login',
] as const
export const ruleSources = ['fanmade', 'official'] as const
export const ruleCourses: Course[] = ['Easy', 'Normal', 'Hard', 'Oni', 'Edit']

/** The verdicts a dan_clear rule may take: 1–3 red, 4–6 gold. */
export const danVerdicts = ['1', '2', '3', '4', '5', '6'] as const

/** A pasted chart or 段位 page address or a bare ID: the chart's or the course's ID. */
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
export function ruleText(rule: RewardRule, t: TFunction, language: string): string {
  if (rule.kind === 'site_login' || rule.kind === 'game_login') return t(`rules.text.${rule.kind}`)
  // Of the fanmade source, the only one players upload to; it names none.
  if (rule.kind === 'charts_uploaded')
    return t('rules.text.charts_uploaded', { count: rule.count ?? 0 })
  const source = rule.source ? t(`shell.${rule.source}`) : ''
  if (rule.kind === 'dan_clear') {
    const dan = rule.dan
    const title = dan ? danTitle(dan.title, language) || dan.code : (rule.danId ?? '')
    const verdict = danVerdicts.find((v) => Number(v) === rule.clear) ?? '1'
    return (
      t('rules.text.dan_clear', { source, title, verdict: t(`rules.verdicts.${verdict}`) }) +
      (!dan || dan.gone ? t('rules.danGone') : dan.fixed ? '' : t('rules.danNotFixed'))
    )
  }
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
