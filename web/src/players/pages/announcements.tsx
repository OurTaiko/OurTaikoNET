import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useSearchParams } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { CommentBody } from '@sitekit/comments'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { formatLocale } from '@sitekit/i18n'
import { AnnouncementPager, Pinned } from '@players/components/announcements'
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

// Everything the site's administrator has announced, whole: pinned ones first, then the
// newest. The home page's titles lead here, each to its own (#<id>).
export function AnnouncementsPage() {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const [page, turn] = usePageParam()
  const read = useQuery({
    ...playerQueries.announcements(page, announcementPageSize),
    placeholderData: keepPreviousData,
  })
  const list = read.data
  // The one the address names is brought into view once it is on the page.
  const named = useLocation().hash.slice(1)
  const present = !!list?.items.some((item) => item.id === named)
  useEffect(() => {
    if (present) document.getElementById(named)?.scrollIntoView({ block: 'start' })
  }, [present, named])
  const locale = formatLocale(i18n.resolvedLanguage)
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
      {list?.items.map((item) => (
        <article
          key={item.id}
          id={item.id}
          className="space-y-3 bg-card shadow-elev-1 p-6 rounded-[28px] scroll-mt-24"
        >
          <header className="space-y-1">
            <h2 className="font-bold text-lg wrap-anywhere">{item.title}</h2>
            <p className="flex items-center gap-2 text-muted-foreground text-xs">
              {item.pinned && <Pinned />}
              <time dateTime={item.publishedAt}>
                {new Date(item.publishedAt).toLocaleString(locale, {
                  dateStyle: 'medium',
                  timeStyle: 'short',
                })}
              </time>
            </p>
          </header>
          {item.body && <CommentBody text={item.body} />}
        </article>
      ))}
      {list && <AnnouncementPager list={list} onTurn={turn} />}
    </div>
  )
}
