// The pages that belong to no song source: home, players (their scores on both sources, titles,
// 段位 badges and nameplate), the title catalog, the site's announcements, messages (chats with other players and the
// site's notices), and for the site's administrator the manage panel.
import type { ReactNode } from 'react'
import { BrowserRouter, Link, Navigate, Route, Routes, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { DocumentLanguage, LanguageProvider } from '@sitekit/language'
import { PageMeta } from '@sitekit/head'
import { ChatsPage } from '@sitekit/chat-center'
import { EditorPage } from '@sitekit/editor-frame'
import { NotificationProvider } from '@sitekit/notifications'
import { RouterBridge } from '@sitekit/navigation'
import { UsersPage } from '@sitekit/users'
import { catalogues } from './i18n'
import { Layout } from '@players/components/layout'
import { HomePage } from './pages/home'
import { UserPage } from './pages/user'
import { TitlesPage } from './pages/titles'
import { AdminPage } from './pages/admin'
import { AnnouncementsPage } from './pages/announcements'
import { AnnouncementsAdminPage } from './pages/announcements-admin'
import { FeaturedPage } from './pages/featured'
import { ManagePage } from './pages/manage'
import { VisitsPage } from './pages/visits'
import '../styles.css'

function NotFound() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col items-center justify-center gap-4 rounded-[28px] border border-dashed bg-card/60 px-6 py-16 text-center">
      <PageMeta title={t('messages.pageNotFound')} noindex />
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

/** A page that names itself and is kept out of search results: the visitor's own or staff's. */
function Private({ title, children }: { title: string; children: ReactNode }) {
  return (
    <>
      <PageMeta title={title} noindex />
      {children}
    </>
  )
}

function App() {
  const { t } = useTranslation()
  return (
    <BrowserRouter>
      <RouterBridge />
      <NotificationProvider>
        <LanguageProvider>
          <DocumentLanguage />
          <Routes>
            <Route element={<Layout />}>
              <Route index element={<HomePage />} />
              <Route
                path="players"
                element={
                  <>
                    <PageMeta title={t('shell.directory')} description={t('meta.players')} />
                    <UsersPage />
                  </>
                }
              />
              <Route
                path="edit"
                element={
                  <>
                    <PageMeta title={t('shell.editor')} description={t('meta.editor')} />
                    <EditorPage />
                  </>
                }
              />
              <Route path="players/users/:id" element={<OldUserAddress />} />
              <Route path="players/:id" element={<UserPage />} />
              <Route
                path="titles"
                element={
                  <>
                    <PageMeta title={t('shell.titles')} description={t('meta.titles')} />
                    <TitlesPage />
                  </>
                }
              />
              <Route
                path="announcements"
                element={
                  <>
                    <PageMeta
                      title={t('announcements.heading')}
                      description={t('meta.announcements')}
                    />
                    <AnnouncementsPage />
                  </>
                }
              />
              <Route
                path="chats"
                element={
                  <Private title={t('chat.title')}>
                    <ChatsPage />
                  </Private>
                }
              />
              <Route
                path="chats/:id"
                element={
                  <Private title={t('chat.title')}>
                    <ChatsPage />
                  </Private>
                }
              />
              {/* The notices were a page of their own; they are a chat now. */}
              <Route path="notifications" element={<Navigate to="/chats/system" replace />} />
              <Route path="manage">
                <Route index element={<NotFound />} />
                <Route
                  path="titles"
                  element={
                    <Private title={t('shell.titleAdmin')}>
                      <AdminPage />
                    </Private>
                  }
                />
                <Route
                  path="groups"
                  element={
                    <Private title={t('manage.heading')}>
                      <ManagePage />
                    </Private>
                  }
                />
                <Route
                  path="featured"
                  element={
                    <Private title={t('featured.heading')}>
                      <FeaturedPage />
                    </Private>
                  }
                />
                <Route
                  path="announcements"
                  element={
                    <Private title={t('announcements.adminHeading')}>
                      <AnnouncementsAdminPage />
                    </Private>
                  }
                />
                <Route
                  path="visits"
                  element={
                    <Private title={t('visits.heading')}>
                      <VisitsPage />
                    </Private>
                  }
                />
                <Route path="*" element={<NotFound />} />
              </Route>
              <Route path="*" element={<NotFound />} />
            </Route>
          </Routes>
        </LanguageProvider>
      </NotificationProvider>
    </BrowserRouter>
  )
}

/** These pages, ready to show: their own words, and the pages themselves. */
export const players = { catalogues, App }
