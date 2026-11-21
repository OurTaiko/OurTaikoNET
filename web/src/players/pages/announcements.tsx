import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ChatCircleIcon } from '@phosphor-icons/react'
import { RelativeTime } from '@sitekit/comments'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { AnnouncementDialog } from '@players/components/announcement-dialog'
import { AnnouncementPager, Pinned, announcementOver } from '@players/components/announcements'
import { Notice } from '@players/notice'
import { playerQueries } from '@players/queries'
import { useSession } from '@players/session'

/** As many as the server gives at a time when it is not asked for fewer. */
export const announcementPageSize = 20

/** The page ?page= names, the first when it names none. */
export function usePageParam() {
  const [params, setParams] = useSearchParams()
  const asked = Number(params.get('page'))
  const page = Number.isInteger(asked) && asked >= 1 && asked <= 10000 ? asked : 1
  return [page, (next: number) => setParams({ page: String(next) })] as const
}

// Everything the site's administrator has announced: pinned ones first, then the newest. Each
// opens over the page with its text and its comments (AnnouncementDialog).
export function AnnouncementsPage() {
  const { t } = useTranslation()
  const session = useSession()
  const [page, turn] = usePageParam()
  const read = useQuery({
    ...playerQueries.announcements(page, announcementPageSize),
    placeholderData: keepPreviousData,
  })
  const list = read.data
  return (
    <div className="space-y-6 mx-auto max-w-3xl" data-testid="announcements">
      <div className="flex flex-wrap justify-between items-baseline gap-3">
        <h1 className="font-extrabold text-2xl tracking-tight">{t('announcements.heading')}</h1>
        {session.user?.siteAdmin && (
          <Link to="/manage/announcements" className="font-semibold text-sm underline">
            {t('announcements.manage')}
          </Link>
        )}
      </div>
      {read.error && !list && <Notice>{read.error.message}</Notice>}
      {read.isPending && <Skeleton className="rounded-[28px] w-full h-40" />}
      {list && !list.items.length && (
        <p className="text-muted-foreground text-sm">{t('announcements.none')}</p>
      )}
      {!!list?.items.length && (
        <ul className="bg-card shadow-elev-1 p-3 rounded-[28px]">
          {list.items.map((item) => (
            <li key={item.id}>
              <Link
                to={`${announcementOver(item.id)}${page > 1 ? `&page=${page}` : ''}`}
                className="flex flex-wrap items-center gap-x-3 gap-y-1 hover:bg-muted px-4 py-3 rounded-[18px] outline-none focus-visible:ring-3 focus-visible:ring-ring transition-colors"
              >
                {item.pinned && <Pinned />}
                <span className="flex-[1_1_12rem] font-bold wrap-anywhere">{item.title}</span>
                <span className="flex items-center gap-3 text-muted-foreground text-xs shrink-0">
                  <span className="flex items-center gap-1">
                    <ChatCircleIcon aria-hidden="true" className="size-3.5" />
                    <span className="sr-only">{t('announcements.comments')}</span>
                    {item.commentCount}
                  </span>
                  <RelativeTime value={item.publishedAt} />
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
      {list && <AnnouncementPager list={list} onTurn={turn} />}
      <AnnouncementDialog />
    </div>
  )
}
