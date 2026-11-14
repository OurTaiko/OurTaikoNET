import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import zh from './locales/zh-Hans.json'
import en from './locales/en.json'
import ja from './locales/ja.json'
import ko from './locales/ko.json'

export const languageNames = {
  'zh-Hans': '简体中文',
  en: 'English',
  ja: '日本語',
  ko: '한국어',
} as const
export type Language = keyof typeof languageNames
export function normalizeLanguage(value?: string | null): Language {
  const code = value?.trim().toLowerCase().replaceAll('_', '-') ?? ''
  if (code === 'zh' || code === 'zh-cn' || code === 'zh-sg' || code === 'zh-hans') return 'zh-Hans'
  const base = code.split('-')[0]
  return base === 'en' || base === 'ja' || base === 'ko' ? base : 'zh-Hans'
}
export function formatLocale(language?: string): string {
  return normalizeLanguage(language) === 'zh-Hans' ? 'zh-CN' : normalizeLanguage(language)
}

export function browserLanguage(): Language {
  const languages =
    typeof navigator === 'undefined'
      ? []
      : navigator.languages?.length
        ? navigator.languages
        : [navigator.language]
  for (const language of languages) {
    const base = language.trim().toLowerCase().replaceAll('_', '-').split('-')[0]
    if (base === 'zh') return 'zh-Hans'
    if (base === 'en' || base === 'ja' || base === 'ko') return base
  }
  return 'en'
}

// Start with the browser language; LanguageProvider applies the SSO preference
// once the session is available. No separate preference is stored locally.
// The words every area shares, kept apart from the store: an area's own are merged into the
// store, and the next area starts from these again.
const shared: Record<Language, object> = { 'zh-Hans': zh, en, ja, ko }
export const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  resources: {
    'zh-Hans': { translation: structuredClone(zh) },
    en: { translation: structuredClone(en) },
    ja: { translation: structuredClone(ja) },
    ko: { translation: structuredClone(ko) },
  },
  lng: browserLanguage(),
  fallbackLng: 'zh-Hans',
  supportedLngs: Object.keys(languageNames),
  load: 'currentOnly',
  initAsync: false,
  interpolation: { escapeValue: false }, // React escapes rendered values.
  returnNull: false,
  returnEmptyString: false,
})
/** Adds a source's own keys and overrides (its src/sources/<source>/locales); its text wins
 * where both have a key. */
export function addSourceCatalogues(catalogues: Record<Language, object>) {
  for (const [language, catalogue] of Object.entries(catalogues))
    i18n.addResourceBundle(language, 'translation', catalogue, true, true)
}
/**
 * Makes the words those of one area: the shared ones with the area's own over them, and
 * nothing left of the area shown before. What is on screen is drawn again by whoever calls.
 */
export function setAreaCatalogues(catalogues: Record<Language, object>) {
  for (const language of Object.keys(shared) as Language[]) {
    i18n.removeResourceBundle(language, 'translation')
    i18n.addResourceBundle(language, 'translation', structuredClone(shared[language]))
    i18n.addResourceBundle(
      language,
      'translation',
      structuredClone(catalogues[language]),
      true,
      true,
    )
  }
}
// Utilities translate at call time; React components use useTranslation instead.
export const t = i18n.t.bind(i18n)
