import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import type { ChartBase } from '@sitekit/api/types'
import type { SourceId } from '@sitekit/source'
import { newestOf } from '@players/newest'
import { Notice } from '@players/notice'
import { AnnouncementDialog } from '@players/components/announcement-dialog'
import { Announcements } from '@players/components/announcements'
import { CommunityActivity } from '@players/components/community-activity'
import { FeaturedPlaylists } from '@players/components/featured-playlists'
import { Friends } from '@players/components/friends'
import { RecentWorks } from '@players/components/recent-works'
import { sources } from '@players/components/song-tile'
import { playerQueries } from '@players/queries'

type Library = { total: number; items: ChartBase[] }

/** Each source's newest songs and how many it has; absent while loading, 'failed' when it broke. */
function useLibraries() {
  const answers = useQueries({ queries: sources.map((source) => playerQueries.library(source)) })
  const libraries: Partial<Record<SourceId, Library | 'failed'>> = {}
  sources.forEach((source, index) => {
    const answer = answers[index]
    if (answer.data) libraries[source] = answer.data
    else if (answer.isError) libraries[source] = 'failed'
  })
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
        {/* Beside the newest charts on a wide page, the announcements over the activity; on a
            narrow one each is a row of its own and the announcements come first. */}
        <div className="xl:flex xl:flex-col xl:gap-4 contents min-w-0 xl:min-h-[23.5rem]">
          <Announcements className="order-first xl:order-none" />
          <CommunityActivity />
        </div>
      </div>
      <FeaturedPlaylists />
      <Friends />
      <AnnouncementDialog />
    </div>
  )
}
