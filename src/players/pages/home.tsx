import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router-dom'
import {
  ArrowRightIcon,
  ChartBarIcon,
  IdentificationCardIcon,
  MedalIcon,
} from '@phosphor-icons/react'
import { api } from '@sitekit/api/client'
import type { ChartBase, ChartList } from '@sitekit/api/types'
import { chartText } from '@sitekit/chart-language'
import { supportsChart } from '@sitekit/courses'
import { LevelChips } from '@sitekit/components/difficulty-badges'
import { SongCover } from '@sitekit/components/song-cover'
import { buttonVariants } from '@sitekit/components/ui/button'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { rememberedSource } from '@sitekit/shell/site'
import type { SourceId } from '@sitekit/source'
import { cn } from '@sitekit/lib/utils'
import { endpoints } from '@players/api/endpoints'
import type { PublicProfile } from '@players/api/types'
import { Nameplate } from '@players/components/nameplate'
import { useSession } from '@players/session-context'
import { Notice } from '@players/notice'
import { SongTile, coverOf, songPage, sourceApi, sources } from '@players/components/song-tile'

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

function SourceCard({ source, library }: { source: SourceId; library?: Library | 'failed' }) {
  const { t, i18n } = useTranslation()
  const name = t(`shell.${source}`)
  const ready = library && library !== 'failed' ? library : null
  return (
    <section
      data-accent={source}
      data-testid={`home-${source}`}
      className="flex flex-col gap-5 bg-card shadow-elev-1 p-6 rounded-[28px] min-w-0"
    >
      <div className="flex justify-between items-start gap-4">
        <div className="space-y-1.5 min-w-0">
          <p className="flex items-center gap-2 font-semibold text-primary text-sm">
            <span aria-hidden="true" className="bg-primary-container rounded-full size-2.5" />
            {source === 'fanmade' ? 'Ka' : 'Don'}
          </p>
          <h2 className="font-extrabold text-3xl tracking-tight">{name}</h2>
          <p className="text-muted-foreground text-sm">{t(`home.${source}Text`)}</p>
        </div>
        <span className="inline-flex items-center bg-secondary px-3 rounded-full h-8 font-semibold text-secondary-foreground text-sm tabular-nums whitespace-nowrap shrink-0">
          {ready ? (
            t('home.songs', { n: ready.total.toLocaleString(i18n.resolvedLanguage) })
          ) : (
            <span aria-hidden="true">…</span>
          )}
        </span>
      </div>
      <ul className="space-y-1.5">
        {ready
          ? ready.items.slice(0, 3).map((chart) => {
              const text = chartText(chart, i18n.resolvedLanguage)
              return (
                <li key={chart.id}>
                  <a
                    href={songPage(source, chart.id)}
                    className="group flex items-center gap-3 hover:bg-muted p-1.5 rounded-[14px] outline-none focus-visible:ring-3 focus-visible:ring-ring transition-colors"
                  >
                    <SongCover
                      image={coverOf(source, chart)}
                      categoryIds={chart.categoryIds}
                      className="rounded-[10px] size-11 shrink-0"
                    />
                    <span className="flex-1 min-w-0">
                      <span className="block font-semibold text-sm truncate">{text.title}</span>
                      <span className="block text-muted-foreground text-xs truncate">
                        {text.subtitle}
                      </span>
                    </span>
                    <LevelChips difficulties={chart.difficulties.slice(-1)} />
                  </a>
                </li>
              )
            })
          : library === 'failed'
            ? null
            : [0, 1, 2].map((row) => (
                <li key={row}>
                  <Skeleton className="rounded-[14px] h-14" />
                </li>
              ))}
      </ul>
      {library === 'failed' && <Notice>{t('home.loadFailed')}</Notice>}
      <div className="flex flex-wrap gap-2 mt-auto">
        <a className={buttonVariants()} href={`/${source}/`}>
          {t('home.browse', { name })}
          <ArrowRightIcon aria-hidden="true" />
        </a>
        <a
          className={buttonVariants({ variant: 'secondary' })}
          href={source === 'fanmade' ? '/fanmade/upload' : '/official/dans'}
        >
          {source === 'fanmade' ? t('home.upload') : t('home.dans')}
        </a>
      </div>
    </section>
  )
}

function You() {
  const { t } = useTranslation()
  const session = useSession()
  const id = session.user?.id
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
          <p className="text-muted-foreground text-sm">{t('home.signInText')}</p>
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
        <ul className="gap-3 grid grid-cols-1 sm:grid-cols-3">
          {features.map(({ icon: Icon, title, text }) => (
            <li key={title} className="space-y-1.5 bg-muted p-4 rounded-[20px]">
              <Icon className="size-6 text-primary" aria-hidden="true" />
              <h3 className="font-bold text-sm">{title}</h3>
              <p className="text-muted-foreground text-xs leading-relaxed">{text}</p>
            </li>
          ))}
        </ul>
      </section>
    )
  }
  const source = rememberedSource()
  const user = session.user
  return (
    <section className="flex sm:flex-row flex-col sm:items-center gap-6 min-w-0 bg-card shadow-elev-1 p-6 rounded-[28px]">
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
        <p className="text-muted-foreground text-sm">{t('home.plateNote')}</p>
      </div>
      <div className="flex sm:flex-col flex-wrap gap-2 shrink-0">
        <Link className={buttonVariants()} to={`/players/${user.id}`}>
          {t('home.openMyPage')}
          <ArrowRightIcon aria-hidden="true" />
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
  // Both sources' newest, the newest of all first.
  const newest = sources
    .flatMap((source) => {
      const library = libraries[source]
      return library && library !== 'failed'
        ? library.items.slice(0, 6).map((chart) => ({ source, chart }))
        : []
    })
    .sort((a, b) => b.chart.createdAt.localeCompare(a.chart.createdAt))
  const loading = sources.some((source) => !libraries[source])
  return (
    <div className="space-y-8">
      {params.get('error') === 'sso' && <Notice>{t('home.signInFailed')}</Notice>}
      <h1 className="sr-only">{t('shell.home')}</h1>
      <div className="gap-4 grid grid-cols-1 md:grid-cols-2">
        {sources.map((source) => (
          <SourceCard key={source} source={source} library={libraries[source]} />
        ))}
      </div>
      <You />
      <section className="space-y-4" aria-labelledby="newest-heading">
        <div className="flex flex-wrap items-baseline gap-x-3">
          <h2 id="newest-heading" className="font-extrabold text-2xl tracking-tight">
            {t('home.newest')}
          </h2>
          <p className="text-muted-foreground text-sm">{t('home.newestFrom')}</p>
        </div>
        <div className={cn('song-grid', loading && 'opacity-80')}>
          {newest.map(({ source, chart }) => (
            <SongTile key={`${source}/${chart.id}`} source={source} chart={chart} tag />
          ))}
          {loading &&
            [0, 1, 2, 3, 4, 5].map((tile) => (
              <Skeleton key={tile} className="rounded-[20px] aspect-[3/4]" />
            ))}
        </div>
      </section>
    </div>
  )
}
