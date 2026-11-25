import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { ArrowRightIcon, ArrowUUpLeftIcon, SealCheckIcon } from '@phosphor-icons/react'
import { api, jsonRequest } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { QueuedReview, Title } from '@players/api/types'
import { TitleBand } from '@players/components/nameplate'
import { NoteField } from '@players/components/title-form'
import { Notice } from '@players/notice'
import { invalidatePlayers, playerQueries } from '@players/queries'
import { useSession } from '@players/session'
import { languages } from '@players/title-draft'
import { RelativeTime } from '@sitekit/comments'
import { PlayerLink } from '@sitekit/components/player-link'
import { Badge } from '@sitekit/components/ui/badge'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { useNotification } from '@sitekit/notification-context'

/** A request's title as asked for, to draw: the title with the request's words and look. */
const asked = (review: QueuedReview): Title => ({
  ...review.title,
  name: review.name,
  nameTranslations: review.nameTranslations,
  description: review.description,
  layers: review.layers,
})

/** A title's words: its name in each language it has one in, and its description. */
function Words({ title }: { title: Pick<Title, 'name' | 'nameTranslations' | 'description'> }) {
  const { t } = useTranslation()
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-sm">
      <dt className="text-muted-foreground">{t('admin.name')}</dt>
      <dd className="font-medium">{title.name}</dd>
      {languages
        .filter((language) => title.nameTranslations[language])
        .map((language) => (
          <div key={language} className="contents">
            <dt className="text-muted-foreground">{t(`languages.${language}`)}</dt>
            <dd>{title.nameTranslations[language]}</dd>
          </div>
        ))}
      {title.description && (
        <>
          <dt className="text-muted-foreground">{t('admin.description')}</dt>
          <dd>{title.description}</dd>
        </>
      )}
    </dl>
  )
}

function Who({ id, name }: { id: string; name: string }) {
  const { t } = useTranslation()
  return (
    <PlayerLink id={id} className="font-medium text-link hover:underline">
      {name || t('profile.unknownPlayer')}
    </PlayerLink>
  )
}

/** One request waiting for a title manager: what is asked, and the form that decides it. */
function PendingReview({ review }: { review: QueuedReview }) {
  const { t } = useTranslation()
  const session = useSession()
  const { notify } = useNotification()
  // The words the catalog will give for how the title is obtained: the manager's, starting
  // from what the title says now (a change) or from what its maker asks for (a new one).
  const [obtain, setObtain] = useState(review.title.obtain || review.request)
  const [reply, setReply] = useState('')
  const [busy, setBusy] = useState(false)
  const decide = async (approve: boolean) => {
    setBusy(true)
    try {
      await api(
        endpoints.adminTitleReview(review.id),
        jsonRequest('POST', { approve, reply, obtain: approve ? obtain : '' }, session.csrfToken),
      )
      await invalidatePlayers()
    } catch (e) {
      notify((e as Error).message, 'error')
    } finally {
      setBusy(false)
    }
  }
  const change = review.kind === 'change'
  return (
    <li className="space-y-4 rounded-2xl border p-4" data-testid="pending-review">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
        <Badge variant={change ? 'secondary' : 'default'}>{t(`reviews.kind.${review.kind}`)}</Badge>
        <span>
          <Who id={review.userId} name={review.userName} />
        </span>
        <span className="text-muted-foreground">
          <RelativeTime value={review.createdAt} />
        </span>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <div className="space-y-3">
          <div className="flex flex-wrap items-center justify-center gap-x-3 gap-y-5 rounded-2xl bg-muted/60 px-4 py-6">
            {change && (
              <>
                <TitleBand title={review.title} className="h-7 opacity-70" />
                <ArrowRightIcon aria-label={t('reviews.becomes')} className="size-5 shrink-0" />
              </>
            )}
            <TitleBand title={asked(review)} className="h-7" />
          </div>
          {change && (
            <div className="space-y-1 opacity-70">
              <p className="text-xs font-semibold text-muted-foreground">{t('reviews.now')}</p>
              <Words title={review.title} />
            </div>
          )}
          <div className="space-y-1">
            {change && (
              <p className="text-xs font-semibold text-muted-foreground">{t('reviews.asked')}</p>
            )}
            <Words title={review} />
          </div>
          <div className="space-y-1 rounded-xl border bg-card px-3 py-2 text-sm">
            <p className="text-xs font-semibold text-muted-foreground">{t('reviews.request')}</p>
            <p className="whitespace-pre-line">{review.request}</p>
          </div>
        </div>
        <div className="space-y-4">
          <NoteField
            id={`obtain-${review.id}`}
            label={t('admin.obtain')}
            hint={t('reviews.obtainHint')}
            value={obtain}
            onChange={setObtain}
            disabled={busy}
          />
          <NoteField
            id={`reply-${review.id}`}
            label={t('reviews.reply')}
            hint={t('reviews.replyHint')}
            value={reply}
            onChange={setReply}
            disabled={busy}
          />
          <div className="flex flex-wrap gap-2">
            <Button disabled={busy} onClick={() => void decide(true)}>
              <SealCheckIcon aria-hidden="true" />
              {t('reviews.approve')}
            </Button>
            <Button variant="outline" disabled={busy} onClick={() => void decide(false)}>
              <ArrowUUpLeftIcon aria-hidden="true" />
              {t('reviews.return')}
            </Button>
          </div>
        </div>
      </div>
    </li>
  )
}

/**
 * What the title managers review: the requests that wait, oldest first, each with the form
 * that approves or returns it, and below them what was decided, newest first.
 */
export function TitleReviews() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const waiting = useQuery(playerQueries.reviews('pending', 1))
  const decided = useQuery(playerQueries.reviews('decided', page))
  const error = waiting.data ? '' : (waiting.error?.message ?? '')
  return (
    <div className="space-y-6">
      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>
            {t('reviews.waiting')}
            {waiting.data && waiting.data.total > 0 && (
              <span className="ml-2 text-sm font-normal text-muted-foreground">
                {waiting.data.total}
              </span>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">{t('reviews.hint')}</p>
          {error && <Notice>{error}</Notice>}
          {!waiting.data ? (
            !error && <Skeleton className="h-40 w-full rounded-2xl" />
          ) : waiting.data.items.length === 0 ? (
            <p className="py-8 text-center text-muted-foreground">{t('reviews.none')}</p>
          ) : (
            <ul className="space-y-4">
              {waiting.data.items.map((review) => (
                <PendingReview key={review.id} review={review} />
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>{t('reviews.decided')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {!decided.data ? (
            !decided.error && <Skeleton className="h-24 w-full rounded-2xl" />
          ) : decided.data.items.length === 0 ? (
            <p className="py-6 text-center text-muted-foreground">{t('reviews.noneDecided')}</p>
          ) : (
            <ul className="divide-y" data-testid="decided-reviews">
              {decided.data.items.map((review) => (
                <li key={review.id} className="space-y-2 py-3 text-sm">
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
                    <TitleBand title={asked(review)} className="h-5" />
                    <Badge
                      variant={
                        review.status === 'approved'
                          ? 'default'
                          : review.status === 'returned'
                            ? 'destructive'
                            : 'outline'
                      }
                    >
                      {t(`reviews.status.${review.status}`)}
                    </Badge>
                    <span className="text-muted-foreground">
                      {t(`reviews.kind.${review.kind}`)}
                    </span>
                    <Who id={review.userId} name={review.userName} />
                    <span className="text-muted-foreground">
                      <RelativeTime value={review.decidedAt ?? review.createdAt} />
                      {review.reviewerName &&
                        ` · ${t('reviews.by', { name: review.reviewerName })}`}
                    </span>
                  </div>
                  <p className="whitespace-pre-line text-muted-foreground">
                    {t('titles.review.asked')}
                    {review.request}
                  </p>
                  {review.reply && (
                    <p className="whitespace-pre-line">
                      {t('titles.review.replied')}
                      {review.reply}
                    </p>
                  )}
                </li>
              ))}
            </ul>
          )}
          {decided.data && decided.data.total > decided.data.pageSize && (
            <nav className="flex flex-wrap items-center justify-center gap-4 text-sm">
              <Button variant="outline" disabled={page <= 1} onClick={() => setPage(page - 1)}>
                {t('messages.previous')}
              </Button>
              <span>
                {t('messages.pagination', {
                  page,
                  pages: Math.ceil(decided.data.total / decided.data.pageSize),
                })}
              </span>
              <Button
                variant="outline"
                disabled={page * decided.data.pageSize >= decided.data.total}
                onClick={() => setPage(page + 1)}
              >
                {t('messages.next')}
              </Button>
            </nav>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
