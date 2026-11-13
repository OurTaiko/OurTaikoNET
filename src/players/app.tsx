// The pages that belong to no song source: home, players (their scores on both sources, titles,
// 段位 badges and nameplate), the title catalog, and for the site's administrator the manage
// panel.
import { StrictMode } from 'react'
import { BrowserRouter, Link, Navigate, Route, Routes, useParams } from 'react-router-dom'
import { I18nextProvider, useTranslation } from 'react-i18next'
import { IconContext } from '@phosphor-icons/react'
import { DocumentLanguage, LanguageProvider } from '@sitekit/language'
import { NotificationsPage } from '@sitekit/notification-center'
import { NotificationProvider } from '@sitekit/notifications'
import { SessionProvider } from '@sitekit/session'
import { ThemeProvider } from '@sitekit/theme'
import { i18n } from './i18n'
import { endpoints } from './api/endpoints'
import { Layout } from '@players/components/layout'
import { HomePage } from './pages/home'
import { DirectoryPage } from './pages/directory'
import { UserPage } from './pages/user'
import { TitlesPage } from './pages/titles'
import { AdminPage } from './pages/admin'
import { ManagePage } from './pages/manage'
import '../styles.css'

function NotFound() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col items-center justify-center gap-4 rounded-[28px] border border-dashed bg-card/60 px-6 py-16 text-center">
      <h1 className="text-2xl font-extrabold tracking-tight">{t('messages.pageNotFound')}</h1>
      <Link to="/">{t('messages.backHome')}</Link>
    </div>
  )
}

// A player's page used to be /players/users/<id>.
function OldUserAddress() {
  const { id = '' } = useParams()
  return <Navigate to={`/players/${id}`} replace />
}

function App() {
  return (
    <StrictMode>
      <IconContext.Provider value={{ weight: 'bold' }}>
        <ThemeProvider storageKey="ourtaiko-net-theme">
          <BrowserRouter>
            <NotificationProvider>
              <SessionProvider me={endpoints.me}>
                <LanguageProvider>
                  <DocumentLanguage />
                  <Routes>
                    <Route element={<Layout />}>
                      <Route index element={<HomePage />} />
                      <Route path="players" element={<DirectoryPage />} />
                      <Route path="players/users/:id" element={<OldUserAddress />} />
                      <Route path="players/:id" element={<UserPage />} />
                      <Route path="titles" element={<TitlesPage />} />
                      <Route path="notifications" element={<NotificationsPage />} />
                      <Route path="admin" element={<AdminPage />} />
                      <Route path="manage" element={<ManagePage />} />
                      <Route path="*" element={<NotFound />} />
                    </Route>
                  </Routes>
                </LanguageProvider>
              </SessionProvider>
            </NotificationProvider>
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
