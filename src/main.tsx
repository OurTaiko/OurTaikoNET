import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { IconContext } from '@phosphor-icons/react'
import { I18nextProvider, useTranslation } from 'react-i18next'
import { BrowserRouter, Link, Navigate, Route, Routes } from 'react-router-dom'
import { CategoryProvider } from '@sitekit/categories'
import { ChartEditorPage } from '@sitekit/chart-editor'
import { Layout } from '@sitekit/components/layout'
import { DanEditorPage, DanPage, DansPage } from '@sitekit/dans'
import { addSourceCatalogues, i18n } from '@sitekit/i18n'
import { DocumentLanguage, LanguageProvider } from '@sitekit/language'
import { NotificationsPage } from '@sitekit/notification-center'
import { NotificationProvider } from '@sitekit/notifications'
import { Auth, ChartManagerPage, Detail, Library } from '@sitekit/pages'
import { MyPlaylistsPage, PlaylistPage, PlaylistsPage } from '@sitekit/playlists'
import { ProfilePage } from '@sitekit/profile'
import { RequireSignIn } from '@sitekit/require-sign-in'
import { SessionProvider } from '@sitekit/session'
import { SiteProvider, type Site } from '@sitekit/site'
import { BASE, SOURCE, sourceOf, type SourceId } from '@sitekit/source'
import { ThemeProvider } from '@sitekit/theme'
import { UserSpacePage } from '@sitekit/user-space'
import { UsersPage } from '@sitekit/users'
import type { ComponentType } from 'react'
import './styles.css'

// Each source's own parts are loaded only on that source's pages.
const load: Record<
  SourceId,
  () => Promise<{
    site: Site
    UpdatePage: ComponentType
    catalogues: Parameters<typeof addSourceCatalogues>[0]
  }>
> = {
  fanmade: () => import('@fanmade/index'),
  official: () => import('@official/index'),
}

const lastSource = 'ourtaiko-net-source'

function App({ site, UpdatePage }: { site: Site; UpdatePage: ComponentType }) {
  const { t } = useTranslation()

  return (
    <StrictMode>
      <IconContext.Provider value={{ weight: 'bold' }}>
        <ThemeProvider storageKey="ourtaiko-net-theme">
          <BrowserRouter basename={BASE}>
            <NotificationProvider>
              <SessionProvider>
                <LanguageProvider>
                  <DocumentLanguage />
                  <CategoryProvider>
                    <SiteProvider value={site}>
                      <Routes>
                        <Route element={<Layout />}>
                          <Route index element={<Library />} />
                          <Route path="users" element={<UsersPage />} />
                          <Route path="users/:id" element={<UserSpacePage />} />
                          <Route path="me/profile" element={<ProfilePage />} />
                          <Route path="me/charts" element={<ChartManagerPage />} />
                          {/* Publishing and replacing files need a signed-in user. */}
                          <Route element={<RequireSignIn />}>
                            <Route path="me/charts/publish" element={<ChartManagerPage />} />
                            <Route path="charts/:id/update" element={<UpdatePage />} />
                          </Route>
                          <Route path="playlists" element={<PlaylistsPage />} />
                          <Route path="dans" element={<DansPage />} />
                          <Route path="dans/new" element={<DanEditorPage />} />
                          <Route path="dans/:id" element={<DanPage />} />
                          <Route path="dans/:id/comments/:commentId" element={<DanPage />} />
                          <Route path="dans/:id/edit" element={<DanEditorPage />} />
                          <Route path="playlists/:id" element={<PlaylistPage />} />
                          <Route path="me/playlists" element={<MyPlaylistsPage />} />
                          <Route
                            path="upload"
                            element={<Navigate to="/me/charts/publish" replace />}
                          />
                          <Route path="charts/:id/edit" element={<ChartEditorPage />} />
                          <Route path="charts/:id" element={<Detail />} />
                          <Route path="charts/:id/comments/:commentId" element={<Detail />} />
                          <Route path="notifications" element={<NotificationsPage />} />
                          <Route path="login" element={<Auth key="login" />} />
                          <Route path="register" element={<Auth key="register" register />} />
                          <Route
                            path="*"
                            element={
                              <div className="flex flex-col items-center justify-center gap-4 rounded-[14px] border border-dashed bg-card/60 px-6 py-16 text-center [&>p]:max-w-lg [&>p]:text-muted-foreground">
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
        </ThemeProvider>
      </IconContext.Provider>
    </StrictMode>
  )
}

async function start() {
  // An address outside both sources goes to the source used last (Fanmade the first time),
  // keeping the rest of the address: /charts/x becomes /fanmade/charts/x.
  if (!sourceOf(location.pathname)) {
    let source: SourceId = 'fanmade'
    try {
      source = sourceOf(`/${localStorage.getItem(lastSource)}`) ?? source
    } catch {
      // No storage: Fanmade.
    }
    location.replace(`/${source}${location.pathname}${location.search}${location.hash}`)
    return
  }
  try {
    localStorage.setItem(lastSource, SOURCE)
  } catch {
    // The choice is simply not remembered.
  }
  document.documentElement.dataset.source = SOURCE
  const { site, UpdatePage, catalogues } = await load[SOURCE]()
  addSourceCatalogues(catalogues)
  document.title = `OurTaiko · ${site.brand}`
  createRoot(document.getElementById('root')!).render(
    <I18nextProvider i18n={i18n}>
      <App site={site} UpdatePage={UpdatePage} />
    </I18nextProvider>,
  )
}
void start()
