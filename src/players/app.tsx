// The pages about players, whichever source they play: their scores on both, titles, 段位
// badges and nameplate, and for the site's administrator the manage panel. From
// OurTaikoProfile's website.
import { StrictMode } from 'react'
import { BrowserRouter, Link, Route, Routes } from 'react-router-dom'
import { I18nextProvider, useTranslation } from 'react-i18next'
import { IconContext } from '@phosphor-icons/react'
import { PLAYERS_BASE } from '@sitekit/source'
import { i18n } from './i18n'
import { LanguageProvider, DocumentLanguage } from './language'
import { SessionProvider } from './session'
import { ThemeProvider } from './theme'
import { Layout } from '@players/components/layout'
import { HomePage } from './pages/home'
import { UserPage } from './pages/user'
import { TitlesPage } from './pages/titles'
import { AdminPage } from './pages/admin'
import { ManagePage } from './pages/manage'
import './styles.css'

function NotFound() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col items-center justify-center gap-4 rounded-3xl border border-dashed bg-card/60 px-6 py-16 text-center">
      <h1 className="text-2xl font-semibold tracking-tight">{t('messages.pageNotFound')}</h1>
      <Link to="/">{t('messages.backHome')}</Link>
    </div>
  )
}

function App() {
  return (
    <StrictMode>
      <IconContext.Provider value={{ weight: 'bold' }}>
        <ThemeProvider>
          <BrowserRouter basename={PLAYERS_BASE}>
            <SessionProvider>
              <LanguageProvider>
                <DocumentLanguage />
                <Routes>
                  <Route element={<Layout />}>
                    <Route index element={<HomePage />} />
                    <Route path="users/:id" element={<UserPage />} />
                    <Route path="titles" element={<TitlesPage />} />
                    <Route path="admin" element={<AdminPage />} />
                    <Route path="manage" element={<ManagePage />} />
                    <Route path="*" element={<NotFound />} />
                  </Route>
                </Routes>
              </LanguageProvider>
            </SessionProvider>
          </BrowserRouter>
        </ThemeProvider>
      </IconContext.Provider>
    </StrictMode>
  )
}

/** The app to render for a page of this area. */
export function players() {
  document.title = 'OurTaiko'
  return (
    <I18nextProvider i18n={i18n}>
      <App />
    </I18nextProvider>
  )
}
