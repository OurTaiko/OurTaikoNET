// The players' pages speak through the site's one set of words (@sitekit/i18n), with their own
// added to it.
import { addSourceCatalogues, i18n, type Language } from '@sitekit/i18n'
import zh from './locales/zh-Hans.json'
import en from './locales/en.json'
import ja from './locales/ja.json'
import ko from './locales/ko.json'

export {
  i18n,
  languageNames,
  normalizeLanguage,
  formatLocale,
  browserLanguage,
} from '@sitekit/i18n'
export type { Language } from '@sitekit/i18n'

const catalogues: Record<Language, object> = { 'zh-Hans': zh, en, ja, ko }
addSourceCatalogues(catalogues)
// Utilities translate at call time; React components use useTranslation instead.
export const t = i18n.t.bind(i18n)
