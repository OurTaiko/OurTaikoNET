import { useEffect } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { i18n, normalizeLanguage, formatLocale, browserLanguage } from './i18n'
import { useSession } from './session-context'
import { SiteHead } from './head'

export function LanguageProvider({ children }: { children: ReactNode }) {
  const { user } = useSession()
  const language = user ? normalizeLanguage(user.preferredLanguage) : null
  useEffect(() => {
    const updateLanguage = () => {
      void i18n.changeLanguage(language ?? browserLanguage())
    }
    updateLanguage()
    if (language !== null) return
    window.addEventListener('languagechange', updateLanguage)
    return () => window.removeEventListener('languagechange', updateLanguage)
  }, [language])
  return children
}

/** The page's language, and the site's name and description in it (see head.tsx). */
export function DocumentLanguage() {
  const { i18n: instance } = useTranslation()
  useEffect(() => {
    document.documentElement.lang = formatLocale(instance.resolvedLanguage)
  }, [instance.resolvedLanguage])
  return <SiteHead />
}
