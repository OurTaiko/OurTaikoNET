// The pages of one song source: its charts, playlists and dan courses. The root (./root)
// shows them while the address is under /fanmade or /official.
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { BrowserRouter, Link, Navigate, Route, Routes, useParams } from 'react-router-dom'
import { CategoryProvider } from '@sitekit/categories'
import { ChartEditorPage } from '@sitekit/chart-editor'
import { Layout } from '@sitekit/components/layout'
import { DanEditorPage, DanPage, DansPage } from '@sitekit/dans'
import type { Language } from '@sitekit/i18n'
import { DocumentLanguage, LanguageProvider } from '@sitekit/language'
import { PageMeta } from '@sitekit/head'
import { NotificationProvider } from '@sitekit/notifications'
import { Auth, ChartManagerPage, Detail, Library } from '@sitekit/pages'
import { MyPlaylistsPage, PlaylistPage, PlaylistsPage } from '@sitekit/playlists'
import { ProfilePage } from '@sitekit/profile'
import { RequireSignIn } from '@sitekit/require-sign-in'
import { SessionProvider } from '@sitekit/session'
import { SiteProvider, type Site } from '@sitekit/site'
import type { SourceId } from '@sitekit/source'
import { RouterBridge, visit } from '@sitekit/navigation'
import type { ComponentType } from 'react'
import './styles.css'

// Each source's own parts are loaded only on that source's pages.
const load: Record<
  SourceId,
  () => Promise<{
    site: Site
    UpdatePage: ComponentType
    catalogues: Record<Language, object>
  }>
> = {
  fanmade: () => import('@fanmade/index'),
  official: () => import('@official/index'),
}

// Players are listed once for the whole site, outside any source.
function LeaveTo({ address }: { address: string }) {
  useEffect(() => visit(address, { replace: true }), [address])
  return null
}
/** A source's old page about one player: their page, at the charts they made. */
function LeaveToPlayer() {
  const { id = '' } = useParams()
  return <LeaveTo address={`/players/${encodeURIComponent(id)}?tab=charts`} />
}

function App({
  source,
  site,
  UpdatePage,
}: {
  source: SourceId
  site: Site
  UpdatePage: ComponentType
}) {
  const { t } = useTranslation()

  return (
    <BrowserRouter basename={`/${source}`}>
      <RouterBridge />
      <NotificationProvider>
        <SessionProvider>
          <LanguageProvider>
            <DocumentLanguage />
            <CategoryProvider>
              <SiteProvider value={site}>
                <Routes>
                  <Route element={<Layout />}>
                    <Route index element={<Library />} />
                    <Route path="users" element={<LeaveTo address="/players" />} />
                    <Route path="users/:id" element={<LeaveToPlayer />} />
                    <Route
                      path="me/profile"
                      element={
                        <>
                          <PageMeta title={t('messages.profile')} noindex />
                          <ProfilePage />
                        </>
                      }
                    />
                    <Route path="me/charts" element={<ChartManagerPage />} />
                    {/* Publishing and replacing files need a signed-in user. */}
                    <Route element={<RequireSignIn />}>
                      <Route path="me/charts/publish" element={<ChartManagerPage />} />
                      <Route
                        path="charts/:id/update"
                        element={
                          <>
                            <PageMeta title={t('messages.updateSongAndCharts')} noindex />
                            <UpdatePage />
                          </>
                        }
                      />
                    </Route>
                    <Route path="playlists" element={<PlaylistsPage />} />
                    <Route path="dans" element={<DansPage />} />
                    <Route path="dans/new" element={<DanEditorPage />} />
                    <Route path="dans/:id" element={<DanPage />} />
                    <Route path="dans/:id/comments/:commentId" element={<DanPage />} />
                    <Route path="dans/:id/edit" element={<DanEditorPage />} />
                    <Route path="playlists/:id" element={<PlaylistPage />} />
                    <Route path="me/playlists" element={<MyPlaylistsPage />} />
                    <Route path="upload" element={<Navigate to="/me/charts/publish" replace />} />
                    <Route
                      path="charts/:id/edit"
                      element={
                        <>
                          <PageMeta title={t('editor.title')} noindex />
                          <ChartEditorPage />
                        </>
                      }
                    />
                    <Route path="charts/:id" element={<Detail />} />
                    <Route path="charts/:id/comments/:commentId" element={<Detail />} />
                    <Route path="notifications" element={<LeaveTo address="/chats/system" />} />
                    <Route path="login" element={<Auth key="login" />} />
                    <Route path="register" element={<Auth key="register" register />} />
                    <Route
                      path="*"
                      element={
                        <div className="flex flex-col items-center justify-center gap-4 rounded-[20px] border border-dashed bg-card/60 px-6 py-16 text-center [&>p]:max-w-lg [&>p]:text-muted-foreground">
                          <PageMeta title={t('messages.pageNotFound')} noindex />
                          <h1 className="text-2xl font-extrabold sm:text-3xl">
                            {t('messages.pageNotFound')}
                          </h1>
                          <Link to="/">{t('messages.backToExplore')}</Link>
                        </div>
                      }
                    />
                  </Route>
                </Routes>
              </SiteProvider>
            </CategoryProvider>
          </LanguageProvider>
        </SessionProvider>
      </NotificationProvider>
    </BrowserRouter>
  )
}

/** A source's pages, ready to show: its own words, and the pages themselves. */
export async function songs(source: SourceId) {
  const { site, UpdatePage, catalogues } = await load[source]()
  return {
    catalogues,
    App: () => <App source={source} site={site} UpdatePage={UpdatePage} />,
  }
}
