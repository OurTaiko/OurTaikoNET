import { normalizeLanguage } from './i18n'

type Translations = Partial<Record<'origin' | 'en' | 'zh' | 'ja' | 'ko' | 'zh_tw', string>>

type LocalizedChart = {
  title: string
  /** Absent where only the title is shown. */
  subtitle?: string
  /** As the TJA has them: `origin` is TITLE/SUBTITLE itself, the other keys locales. */
  titleTranslations?: Translations
  subtitleTranslations?: Translations
}

/** The dictionary's keys for a language tag, best first; none for a language it has no key for. */
function keysOf(language: string): (keyof Translations)[] {
  const code = language.trim().toLowerCase().replaceAll('_', '-')
  const base = code.split('-')[0]
  if (base === 'zh') return /-(tw|hk|mo|hant)\b/.test(code) ? ['zh_tw', 'zh'] : ['zh']
  return base === 'en' || base === 'ja' || base === 'ko' ? [base] : []
}

/** The languages the visitor's system asks for, in its order. */
const systemLanguages = (): readonly string[] =>
  typeof navigator === 'undefined'
    ? []
    : navigator.languages?.length
      ? navigator.languages
      : [navigator.language]

/**
 * The API returns all translations. Language selection and fallback belong here: the name in
 * the visitor's language (the one the site speaks to them: their account's, or their
 * browser's while signed out), else in a language their system asks for, else the original.
 */
export function chartText(
  chart: LocalizedChart,
  language?: string | null,
  system: readonly string[] = systemLanguages(),
) {
  // No language given is the site's own, Chinese, as an account without a preference has it.
  const keys = [normalizeLanguage(language), ...system].flatMap(keysOf)
  const select = (field: 'title' | 'subtitle') => {
    const translations = field === 'title' ? chart.titleTranslations : chart.subtitleTranslations
    return (
      [...keys.map((key) => translations?.[key]), translations?.origin, chart[field]].find(
        (value) => value?.trim(),
      ) ?? ''
    )
  }
  return { title: select('title'), subtitle: select('subtitle').replace(/^(--|\+\+)/, '') }
}
