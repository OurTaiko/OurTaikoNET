import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { ThemeContext } from './theme-context'
import type { Theme } from './theme-context'

/** storageKey names the browser's saved choice, one per site. */
export function ThemeProvider({
  storageKey,
  children,
}: {
  storageKey: string
  children: ReactNode
}) {
  const [theme, setTheme] = useState<Theme>(() => {
    try {
      const saved = localStorage.getItem(storageKey)
      return saved === 'light' || saved === 'dark' ? saved : 'system'
    } catch {
      return 'system'
    }
  })
  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const apply = () => {
      const dark = theme === 'dark' || (theme === 'system' && media.matches)
      document.documentElement.classList.toggle('dark', dark)
      document.documentElement.classList.toggle('scheme-dark', dark)
      document.documentElement.classList.toggle('scheme-light', !dark)
      const color = getComputedStyle(document.documentElement)
        .getPropertyValue('--background')
        .trim()
      document.querySelector('meta[name="theme-color"]')?.setAttribute('content', color)
    }
    apply()
    media.addEventListener('change', apply)
    try {
      localStorage.setItem(storageKey, theme)
    } catch {
      /* Storage may be unavailable. */
    }
    return () => media.removeEventListener('change', apply)
  }, [theme, storageKey])
  useEffect(() => {
    const sync = (event: StorageEvent) => {
      if (event.key !== storageKey) return
      setTheme(event.newValue === 'light' || event.newValue === 'dark' ? event.newValue : 'system')
    }
    window.addEventListener('storage', sync)
    return () => window.removeEventListener('storage', sync)
  }, [storageKey])
  return <ThemeContext.Provider value={{ theme, setTheme }}>{children}</ThemeContext.Provider>
}
