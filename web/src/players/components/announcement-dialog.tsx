import { Suspense, lazy } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { withQuery } from '@sitekit/api/endpoints'
import { CommentSection, type CommentSubject } from '@sitekit/comments'
import { Dialog, DialogContent, DialogTitle } from '@sitekit/components/ui/dialog'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { formatLocale } from '@sitekit/i18n'
import { queryClient } from '@sitekit/query/client'
import { endpoints } from '@players/api/endpoints'
import type { Announcement } from '@players/api/types'
import { Notice } from '@players/notice'
import { playerKeys, playerQueries } from '@players/queries'
import { useSession } from '@players/session'
import { Pinned, announcementAddress, focus, open } from './announcements'

const Markdown = lazy(() => import('@sitekit/markdown'))

/**
 * An announcement's text, which the administrator writes in Markdown. Until the parser has
 * arrived it is shown as written, in as many lines.
 */
export function AnnouncementBody({ text }: { text: string }) {
  return (
    <Suspense
      fallback={<p className="text-sm leading-6 whitespace-pre-wrap wrap-anywhere">{text}</p>}
    >
      <Markdown text={text} className="max-w-[70ch]" />
    </Suspense>
  )
}

/** The announcement as what its comments hang on: they are the site's own, of no song source. */
function subjectOf(item: Announcement, moderator: boolean): CommentSubject {
  const address = endpoints.announcementComments(item.id)
  return {
    host: 'site',
    path: announcementAddress(item.id),
    commentCount: item.commentCount,
    comments: (query) => withQuery(address, query),
    newComment: address,
    comment: endpoints.comment,
    commentVote: endpoints.commentVote,
    commentThread: (id, sort) => withQuery(endpoints.comment(id), { sort }),
    commentPath: (id) => announcementAddress(item.id, id),
    // An announcement is the site's; nobody among the players owns it.
    isOwner: () => false,
    moderator,
    placeholder: 'interactions.announcementCommentPlaceholder',
  }
}

/**
 * The announcement the address names, over the page: what it says on the left and its comments
 * on the right, each scrolling by itself; on a narrow page one under the other. Pages that list
 * announcements show this once; it is nothing while the address names none.
 */
export function AnnouncementDialog() {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const [params, setParams] = useSearchParams()
  const id = params.get(open) ?? ''
  const read = useQuery({ ...playerQueries.announcement(id), enabled: !!id })
  if (!id) return null
  const item = read.data
  const close = () => {
    const rest = new URLSearchParams(params)
    rest.delete(open)
    rest.delete(focus)
    setParams(rest)
  }
  const locale = formatLocale(i18n.resolvedLanguage)
  return (
    <Dialog open onOpenChange={(shown) => !shown && close()}>
      <DialogContent
        data-testid="announcement-dialog"
        className="lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)] gap-0 p-0 sm:max-w-5xl max-h-[90svh] lg:min-h-[28rem] overflow-y-auto lg:overflow-hidden"
      >
        <div className="space-y-4 p-6 sm:p-8 min-w-0 lg:max-h-[90svh] lg:overflow-y-auto">
          {!item ? (
            <>
              <DialogTitle className="sr-only">{t('announcements.heading')}</DialogTitle>
              {read.error ? (
                <Notice>{read.error.message}</Notice>
              ) : (
                <Skeleton className="rounded-2xl w-full h-40" />
              )}
            </>
          ) : (
            <>
              <header className="space-y-2 pr-10 lg:pr-0">
                <DialogTitle className="font-extrabold text-xl leading-snug wrap-anywhere">
                  {item.title}
                </DialogTitle>
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
              {item.body && <AnnouncementBody text={item.body} />}
            </>
          )}
        </div>
        {item && (
          <div className="bg-muted/40 border-t lg:border-t-0 lg:border-l min-w-0 lg:max-h-[90svh] lg:overflow-y-auto">
            <CommentSection
              // Another announcement, or another comment singled out, starts its own list.
              key={`${item.id}/${params.get(focus) ?? ''}`}
              subject={subjectOf(item, !!session.user?.siteAdmin)}
              focusId={params.get(focus) || undefined}
              // The count is the announcement's, on every list that shows it.
              onCountChange={() =>
                void queryClient.invalidateQueries({ queryKey: playerKeys.announcements() })
              }
              className="bg-transparent shadow-none p-6 sm:p-8 lg:pr-14 border-0 rounded-none"
            />
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
