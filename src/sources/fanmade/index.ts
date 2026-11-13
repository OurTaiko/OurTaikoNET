// Everything of the fanmade source the app needs: what it gives the shared pages, its page for
// replacing a chart's files, and its own text.
import type { Language } from '@sitekit/i18n'
import en from './locales/en.json'
import ja from './locales/ja.json'
import ko from './locales/ko.json'
import zh from './locales/zh-Hans.json'

export { site } from './site'
export { UpdatePage } from './upload'
export const catalogues: Record<Language, object> = { 'zh-Hans': zh, en, ja, ko }
