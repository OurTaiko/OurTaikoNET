import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { MegaphoneIcon, PushPinIcon } from '@phosphor-icons/react'
import { RelativeTime } from '@sitekit/comments'
import { Button } from '@sitekit/components/ui/button'
import type { Announcement } from '@players/api/types'
import { playerQueries, type Page } from '@players/queries'

// How many the home page shows; the rest are a link away.
const homeShown = 3

/** Where an announcement is on the page of them all. */
export const announcementAddress = (id: string) => `/announcements#${id}`

/** The mark of one that is kept before the others. */
export function Pinned() {
  const { t } = useTranslation()
  return (
    <span className="inline-flex items-center gap-1 bg-primary/10 px-2 py-0.5 rounded-full font-semibold text-primary text-xs shrink-0">
      <PushPinIcon weight="fill" aria-hidden="true" className="size-3" />
      {t('announcements.pinned')}
    </span>
  )
}

/** The buttons that turn the pages of a list of announcements; nothing when it is one page. */
export function AnnouncementPager({
  list,
  onTurn,
}: {
  list: Page<Announcement>
  onTurn: (page: number) => void
}) {
  const { t } = useTranslation()
  if (list.total <= list.pageSize) return null
  const turn = (page: number, label: ReactNode, off: boolean) => (
    <Button variant="outline" disabled={off} onClick={() => onTurn(page)}>
      {label}
    </Button>
  )
  return (
    <nav
      aria-label={t('announcements.pages')}
      className="flex flex-wrap justify-center items-center gap-4 text-sm"
    >
      {turn(list.page - 1, t('messages.previous'), list.page <= 1)}
      <span>
        {t('messages.pagination', {
          page: list.page,
          pages: Math.ceil(list.total / list.pageSize),
        })}
      </span>
      {turn(list.page + 1, t('messages.next'), list.page * list.pageSize >= list.total)}
    </nav>
  )
}

/**
 * The home page's corner for what the site's administrator announces: the first few titles,
 * pinned ones before the newest, each leading to its text on the page of them all. Nothing is
 * shown while there is none or they cannot be read: the home page is whole without it.
 */
export function Announcements({ className }: { className?: string }) {
  const { t } = useTranslation()
  const items = useQuery(playerQueries.announcements(1, homeShown)).data?.items ?? []
  if (!items.length) return null
  return (
    <section
      aria-labelledby="announcements-heading"
      data-testid="home-announcements"
      className={`bg-card shadow-elev-1 pt-4 pb-3 rounded-[28px] min-w-0 ${className ?? ''}`}
    >
      <div className="flex justify-between items-center gap-3 pr-5 pl-6">
        <h2
          id="announcements-heading"
          className="flex items-center gap-2 font-extrabold text-xl tracking-tight"
        >
          <MegaphoneIcon weight="fill" aria-hidden="true" className="size-5 text-primary" />
          {t('home.announcements')}
        </h2>
        <Link
          to="/announcements"
          className="rounded-md outline-none focus-visible:ring-3 focus-visible:ring-ring font-semibold text-muted-foreground hover:text-foreground text-sm"
        >
          {t('home.allAnnouncements')}
        </Link>
      </div>
      <ul className="px-3 pt-1">
        {items.map((item) => (
          <li key={item.id}>
            <Link
              to={announcementAddress(item.id)}
              className="flex items-center gap-2 hover:bg-muted px-3 py-2 rounded-[16px] outline-none focus-visible:ring-3 focus-visible:ring-ring transition-colors"
            >
              {item.pinned && <Pinned />}
              <span className="flex-1 font-bold text-sm truncate">{item.title}</span>
              <span className="text-muted-foreground text-xs shrink-0">
                <RelativeTime value={item.publishedAt} />
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}
