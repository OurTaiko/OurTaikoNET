import 'i18next'
import type shared from './locales/zh-Hans.json'
import type fanmade from '@fanmade/locales/zh-Hans.json'
import type official from '@official/locales/zh-Hans.json'

declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation'
    // The shared catalogue with each source's keys added (see i18n.ts). A page only has its
    // own source's keys at run time; a key of the other source is a mistake the types do not
    // catch.
    resources: { translation: typeof shared & typeof fanmade & typeof official }
    strictKeyChecks: true
  }
}
