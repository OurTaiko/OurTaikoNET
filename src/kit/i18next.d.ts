import 'i18next'
import type shared from './locales/zh-Hans.json'
import type fanmade from '@fanmade/locales/zh-Hans.json'
import type official from '@official/locales/zh-Hans.json'
import type players from '@players/locales/zh-Hans.json'

declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation'
    // Every area's keys together: the catalogue the song sources share with each source's
    // keys added (see i18n.ts), and the players' pages' own. There is one declaration for the
    // whole project, so a page only has its own area's keys at run time; a key of another
    // area is a mistake the types do not catch.
    resources: { translation: typeof shared & typeof fanmade & typeof official & typeof players }
    strictKeyChecks: true
  }
}
