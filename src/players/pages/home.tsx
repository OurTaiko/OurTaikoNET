import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router-dom'
import { ChartBarIcon, IdentificationCardIcon, MedalIcon } from '@phosphor-icons/react'
import { api } from '@sitekit/api/client'
import type { ChartBase, ChartList } from '@sitekit/api/types'
import { supportsChart } from '@sitekit/courses'
import { buttonVariants } from '@sitekit/components/ui/button'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { useRememberedSource } from '@sitekit/shell/site'
import type { SourceId } from '@sitekit/source'
import { cn } from '@sitekit/lib/utils'
import { endpoints } from '@players/api/endpoints'
import type { PublicProfile } from '@players/api/types'
import { Nameplate } from '@players/components/nameplate'
import { inTurns } from '@players/in-turns'
import { useSession } from '@players/session-context'
import { Notice } from '@players/notice'
import { CommunityActivity } from '@players/components/community-activity'
import { RecentWorks } from '@players/components/recent-works'
import { SongTile, sourceApi, sources } from '@players/components/song-tile'

type Library = { total: number; items: ChartBase[] }

/** Each source's newest songs and how many it has; null while loading, 'failed' when it broke. */
function useLibraries() {
  const [libraries, setLibraries] = useState<Partial<Record<SourceId, Library | 'failed'>>>({})
  useEffect(() => {
    const controller = new AbortController()
    for (const source of sources)
      api<ChartList>(`${sourceApi(source)}/charts`, { signal: controller.signal })
        .then((list) =>
          setLibraries((current) => ({
            ...current,
            [source]: { total: list.total, items: list.items.filter(supportsChart) },
          })),
        )
        .catch(
          () =>
            !controller.signal.aborted &&
            setLibraries((current) => ({ ...current, [source]: 'failed' })),
        )
    return () => controller.abort()
  }, [])
  return libraries
}

function You() {
  const { t } = useTranslation()
  const session = useSession()
  const id = session.user?.id
  const source = useRememberedSource()
  const [profile, setProfile] = useState<PublicProfile | null>(null)
  useEffect(() => {
    if (!id) return
    const controller = new AbortController()
    api<PublicProfile>(endpoints.user(id), { signal: controller.signal })
      .then(setProfile)
      .catch(() => undefined) // The plate is an extra here; the player's page reports what failed.
    return () => controller.abort()
  }, [id])
  if (session.loading) return <Skeleton className="rounded-[28px] w-full h-40" />
  if (!session.user) {
    const features = [
      { icon: ChartBarIcon, title: t('home.scoresTitle'), text: t('home.scoresText') },
      { icon: MedalIcon, title: t('home.titlesTitle'), text: t('home.titlesText') },
      { icon: IdentificationCardIcon, title: t('home.plateTitle'), text: t('home.plateText') },
    ]
    return (
      <section className="gap-6 grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] bg-card shadow-elev-1 p-6 rounded-[28px]">
        <div className="space-y-4">
          <h2 className="font-extrabold text-2xl tracking-tight">{t('home.signInTitle')}</h2>
          <div className="flex flex-wrap gap-2">
            <a className={buttonVariants()} href={endpoints.login('/')}>
              {t('nav.signIn')}
            </a>
            <a
              className={buttonVariants({ variant: 'outline' })}
              href={endpoints.account('register')}
            >
              {t('home.register')}
            </a>
          </div>
        </div>
        <ul className="space-y-3">
          {features.map(({ icon: Icon, title, text }) => (
            <li key={title} className="flex gap-3 text-sm">
              <Icon className="mt-0.5 size-5 text-primary shrink-0" aria-hidden="true" />
              <p>
                <span className="block font-bold">{title}</span>
                <span className="text-muted-foreground">{text}</span>
              </p>
            </li>
          ))}
        </ul>
      </section>
    )
  }
  const user = session.user
  return (
    <section className="flex sm:flex-row flex-col sm:items-center gap-6 bg-card shadow-elev-1 p-6 rounded-[28px] min-w-0">
      <div className="flex-1 space-y-3 min-w-0">
        <h2 className="font-extrabold text-2xl tracking-tight">{t('home.plate')}</h2>
        <Nameplate
          plate={{
            userId: user.id,
            title: null,
            dan: null,
            showDan: false,
            ...profile?.nameplate,
            nickname: user.nickname,
            avatarUrl: user.avatarUrl ?? '',
          }}
        />
      </div>
      <div className="flex sm:flex-col flex-wrap gap-2 shrink-0">
        <Link className={buttonVariants()} to={`/players/${user.id}`}>
          {t('home.openMyPage')}
        </Link>
        <a className={buttonVariants({ variant: 'secondary' })} href={`/${source}/me/playlists`}>
          {t('home.myPlaylists')}
        </a>
        <a className={buttonVariants({ variant: 'secondary' })} href="/fanmade/me/charts">
          {t('home.myCharts')}
        </a>
      </div>
    </section>
  )
}

export function HomePage() {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const libraries = useLibraries()
  // Each source's newest, as many as `each`.
  const newestOf = (each: number) =>
    sources.map((source) => {
      const library = libraries[source]
      return library && library !== 'failed'
        ? library.items.slice(0, each).map((chart) => ({ source, chart }))
        : []
    })
  // Both sources' newest, the newest of all first.
  const newest = newestOf(6)
    .flat()
    .sort((a, b) => b.chart.createdAt.localeCompare(a.chart.createdAt))
  const loading = sources.some((source) => !libraries[source])
  return (
    <div className="space-y-8">
      {params.get('error') === 'sso' && <Notice>{t('home.signInFailed')}</Notice>}
      <h1 className="sr-only">{t('shell.home')}</h1>
      <div className="gap-4 grid grid-cols-1 xl:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]">
        <RecentWorks
          works={inTurns(newestOf(10), 10, (work) => work.chart.createdAt)}
          loading={loading}
          failed={sources.every((source) => libraries[source] === 'failed')}
        />
        <CommunityActivity />
      </div>
      <You />
      <section className="space-y-4" aria-labelledby="newest-heading">
        <h2 id="newest-heading" className="font-extrabold text-2xl tracking-tight">
          {t('home.newest')}
        </h2>
        <div className={cn('song-grid', loading && 'opacity-80')}>
          {newest.map(({ source, chart }) => (
            <SongTile key={`${source}/${chart.id}`} source={source} chart={chart} tag />
          ))}
          {loading &&
            [0, 1, 2, 3, 4, 5].map((tile) => (
              <Skeleton key={tile} className="rounded-[20px] aspect-3/4" />
            ))}
        </div>
      </section>
    </div>
  )
}
