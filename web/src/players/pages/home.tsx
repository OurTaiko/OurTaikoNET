import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { api } from '@sitekit/api/client'
import type { ChartBase, ChartList } from '@sitekit/api/types'
import { supportsChart } from '@sitekit/courses'
import type { SourceId } from '@sitekit/source'
import { newestOf } from '@players/newest'
import { Notice } from '@players/notice'
import { CommunityActivity } from '@players/components/community-activity'
import { Friends } from '@players/components/friends'
import { RecentWorks } from '@players/components/recent-works'
import { sourceApi, sources } from '@players/components/song-tile'

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

export function HomePage() {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const libraries = useLibraries()
  // Each source's ten newest: the ten newest of all are among them.
  const newest = sources.map((source) => {
    const library = libraries[source]
    return library && library !== 'failed'
      ? library.items.slice(0, 10).map((chart) => ({ source, chart }))
      : []
  })
  const loading = sources.some((source) => !libraries[source])
  return (
    <div className="space-y-8">
      {params.get('error') === 'sso' && <Notice>{t('home.signInFailed')}</Notice>}
      <h1 className="sr-only">{t('shell.home')}</h1>
      <div className="gap-4 grid grid-cols-1 xl:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]">
        <RecentWorks
          works={newestOf(newest, 10, (work) => work.chart.createdAt)}
          loading={loading}
          failed={sources.every((source) => libraries[source] === 'failed')}
        />
        <CommunityActivity />
      </div>
      <Friends />
    </div>
  )
}
