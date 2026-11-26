import { useState } from 'react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowUUpLeftIcon,
  PaperPlaneTiltIcon,
  PencilSimpleIcon,
  PlusIcon,
  TrashIcon,
  UsersIcon,
} from '@phosphor-icons/react'
import { api, jsonRequest } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { HeldTitle, Nameplate as Plate, OwnTitle, Review, Title } from '@players/api/types'
import { Nameplate, TitleBand } from '@players/components/nameplate'
import { NoteField, TitleFields, VisibilityMark } from '@players/components/title-form'
import { Notice } from '@players/notice'
import { invalidatePlayers, playerQueries } from '@players/queries'
import { useSession } from '@players/session'
import { draftBody, draftOf, draftTitle, emptyDraft, type TitleDraft } from '@players/title-draft'
import { myTitlesPage } from './titles'
import { RelativeTime } from '@sitekit/comments'
import { EmptyState } from '@sitekit/components/empty-state'
import { Button, buttonVariants } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Checkbox } from '@sitekit/components/ui/checkbox'
import { DialogClose } from '@sitekit/components/ui/dialog'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { cn } from '@sitekit/lib/utils'
import { Modal } from '@sitekit/notifications'
import { useNotification } from '@sitekit/notification-context'

// What the player is doing to one of their titles in a dialog.
type Doing =
  | { what: 'make' }
  | { what: 'change'; title: OwnTitle }
  | { what: 'publish'; title: OwnTitle }
  | { what: 'request'; title: OwnTitle }

/** How a held title came to the player. */
function useSource() {
  const { t } = useTranslation()
  return (held: HeldTitle, me: string) =>
    held.courseTitle ||
    (held.service === 'admin'
      ? t('titles.byAdmin')
      : held.service === 'rule'
        ? t('titles.byRule')
        : held.title.ownerId === me
          ? t('titles.byMe')
          : held.service)
}

/** The latest request for a title's review, as its maker reads it: what was asked and answered. */
function ReviewNote({ review }: { review: Review }) {
  const { t } = useTranslation()
  return (
    <div
      className={cn(
        'space-y-1 rounded-xl border px-3 py-2 text-sm',
        review.status === 'returned' && 'border-destructive/30 bg-destructive/5',
        review.status === 'approved' && 'border-primary/30 bg-primary/5',
      )}
    >
      <p className="flex flex-wrap items-baseline gap-x-2 text-xs font-semibold">
        {t(`titles.review.${review.kind}.${review.status}`)}
        <span className="font-normal text-muted-foreground">
          <RelativeTime value={review.decidedAt ?? review.createdAt} />
        </span>
      </p>
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
    </div>
  )
}

/**
 * /me/titles: the titles the signed-in player holds, with the one they wear, and the titles they
 * made. A title they make is private: theirs to wear and change, one of a few places. Asking for
 * its review, with how it should be obtained, lets the title managers make it public, after
 * which it changes only by a review of the change.
 */
export function MyTitlesPage() {
  const { t } = useTranslation()
  const session = useSession()
  const { notify, confirm } = useNotification()
  const source = useSource()
  const user = session.user
  const id = user?.id ?? ''
  const profile = useQuery({ ...playerQueries.profile(id), enabled: !!user })
  const heldRead = useQuery({ ...playerQueries.heldTitles(id), enabled: !!user })
  const madeRead = useQuery({ ...playerQueries.myTitles(), enabled: !!user })
  const [doing, setDoing] = useState<Doing | null>(null)
  const [busy, setBusy] = useState(false)
  // A request for review was just sent: where its answer will come is said once.
  const [sent, setSent] = useState(false)
  if (session.loading) return <p role="status">{t('messages.loadingAccount')}</p>
  if (!user)
    return (
      <EmptyState title={t('messages.signInRequired')}>
        <a
          className={buttonVariants({ variant: 'default', size: 'default' })}
          href={endpoints.login(myTitlesPage)}
        >
          {t('messages.signIn')}
        </a>
      </EmptyState>
    )
  const plate = profile.data?.nameplate
  const held = heldRead.data?.titles
  const made = madeRead.data
  const error =
    (held ? '' : (heldRead.error?.message ?? '')) || (made ? '' : (madeRead.error?.message ?? ''))
  const full = !!made && made.used >= made.limit

  /**
   * Does something on the server and reads everything again, which shows what it did. It
   * answers what went wrong, or nothing.
   */
  const run = async (work: () => Promise<unknown>) => {
    setBusy(true)
    setSent(false)
    try {
      await work()
      await invalidatePlayers()
      return ''
    } catch (e) {
      return (e as Error).message
    } finally {
      setBusy(false)
    }
  }
  /** The same for what is done on the page itself, where a failure is told over it. */
  const act = async (work: () => Promise<unknown>) => {
    const failure = await run(work)
    if (failure) notify(failure, 'error')
  }
  const wear = (titleId: string | null, showDan: boolean) =>
    act(() =>
      api<Plate>(endpoints.nameplate, jsonRequest('PUT', { titleId, showDan }, session.csrfToken)),
    )
  const remove = async (title: Title) => {
    const yes = await confirm({
      message: t('titles.confirmDelete', { name: title.name }),
      confirmLabel: t('titles.delete'),
      destructive: true,
    })
    if (!yes) return
    void act(() => api(endpoints.myTitle(title.id), jsonRequest('DELETE', {}, session.csrfToken)))
  }
  const withdraw = (review: Review) =>
    void act(() =>
      api(endpoints.myTitleReview(review.id), jsonRequest('DELETE', {}, session.csrfToken)),
    )

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h1 className="text-2xl font-extrabold tracking-tight sm:text-3xl">{t('titles.mine')}</h1>
        <Link to="/titles" className="text-sm text-link hover:underline">
          {t('titles.catalogLink')}
        </Link>
      </div>
      {error && <Notice>{error}</Notice>}

      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>{t('titles.held')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
            {plate ? (
              <Nameplate plate={plate} className="max-w-xs" />
            ) : (
              <Skeleton className="aspect-[329/81] w-full max-w-xs rounded-2xl" />
            )}
            <div className="space-y-2 text-sm">
              <p className="text-muted-foreground">{t('titles.plateHint')}</p>
              {plate && (
                <label className="flex items-center gap-2">
                  <Checkbox
                    checked={plate.showDan}
                    disabled={busy}
                    onCheckedChange={(checked) =>
                      void wear(plate.title?.id ?? null, Boolean(checked))
                    }
                  />
                  {t('titles.showDan')}
                </label>
              )}
            </div>
          </div>
          {!held ? (
            !error && <Skeleton className="h-24 w-full rounded-2xl" />
          ) : held.length === 0 ? (
            <p className="py-6 text-center text-muted-foreground">{t('titles.none')}</p>
          ) : (
            <ul
              className="grid grid-cols-[repeat(auto-fill,minmax(min(17rem,100%),1fr))] gap-3"
              data-testid="held-titles"
            >
              {held.map((h) => {
                const worn = plate?.title?.id === h.title.id
                return (
                  <li
                    key={h.title.id}
                    className={cn(
                      'flex flex-col gap-3 rounded-2xl border p-3',
                      worn && 'border-primary ring-2 ring-primary/30',
                    )}
                  >
                    <div className="flex justify-center rounded-xl bg-muted/60 px-3 py-5">
                      <TitleBand title={h.title} className="h-7" />
                    </div>
                    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      {h.title.visibility === 'private' && <VisibilityMark title={h.title} />}
                      <span>{source(h, user.id)}</span>
                      <span>·</span>
                      <RelativeTime value={h.grantedAt} />
                    </div>
                    <Button
                      size="sm"
                      className="mt-auto self-start"
                      variant={worn ? 'secondary' : 'default'}
                      disabled={busy || !plate}
                      onClick={() => void wear(worn ? null : h.title.id, plate?.showDan ?? true)}
                    >
                      {worn ? t('titles.takeOff') : t('titles.wear')}
                    </Button>
                  </li>
                )
              })}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card className="rounded-3xl">
        <CardHeader className="flex flex-row flex-wrap items-center gap-3">
          <CardTitle className="mr-auto">{t('titles.made')}</CardTitle>
          {made && (
            <span className="text-sm text-muted-foreground" data-testid="title-quota">
              {t('titles.quota', { used: made.used, limit: made.limit })}
            </span>
          )}
          <Button
            size="sm"
            disabled={!made || full || busy}
            onClick={() => setDoing({ what: 'make' })}
          >
            <PlusIcon aria-hidden="true" />
            {t('titles.create')}
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {full ? t('titles.limitReached', { limit: made.limit }) : t('titles.madeHint')}
          </p>
          {sent && <Notice kind="info">{t('titles.review.sent')}</Notice>}
          {!made ? (
            !error && <Skeleton className="h-24 w-full rounded-2xl" />
          ) : made.titles.length === 0 ? (
            <p className="py-6 text-center text-muted-foreground">{t('titles.noneMade')}</p>
          ) : (
            <ul className="divide-y rounded-2xl border" data-testid="made-titles">
              {made.titles.map((title) => {
                const waiting = title.pending && title.review
                const actions: ReactNode =
                  waiting && title.review ? (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={busy}
                      onClick={() => title.review && withdraw(title.review)}
                    >
                      <ArrowUUpLeftIcon aria-hidden="true" />
                      {t('titles.review.withdraw')}
                    </Button>
                  ) : title.visibility === 'public' ? (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={busy}
                      onClick={() => setDoing({ what: 'request', title })}
                    >
                      <PencilSimpleIcon aria-hidden="true" />
                      {t('titles.review.askChange')}
                    </Button>
                  ) : (
                    <>
                      <Button
                        size="sm"
                        disabled={busy}
                        onClick={() => setDoing({ what: 'publish', title })}
                      >
                        <PaperPlaneTiltIcon aria-hidden="true" />
                        {t('titles.review.ask')}
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={busy}
                        onClick={() => setDoing({ what: 'change', title })}
                      >
                        <PencilSimpleIcon aria-hidden="true" />
                        {t('titles.edit')}
                      </Button>
                      <Button
                        size="sm"
                        variant="destructive"
                        disabled={busy}
                        onClick={() => void remove(title)}
                      >
                        <TrashIcon aria-hidden="true" />
                        {t('titles.delete')}
                      </Button>
                    </>
                  )
                return (
                  <li key={title.id} className="space-y-3 p-4">
                    <div className="flex flex-wrap items-center gap-x-4 gap-y-3">
                      <TitleBand title={title} className="h-7" />
                      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                        <VisibilityMark title={title} pending={title.pending} />
                        {title.visibility === 'public' && (
                          <span className="flex items-center gap-1">
                            <UsersIcon aria-hidden="true" className="size-3.5" />
                            {t('titles.holders', { count: title.holders })}
                          </span>
                        )}
                      </div>
                      <div className="ml-auto flex flex-wrap gap-2">{actions}</div>
                    </div>
                    {title.description && <p className="text-sm">{title.description}</p>}
                    {title.visibility === 'public' && title.obtain && (
                      <p className="text-sm whitespace-pre-line">
                        <span className="text-xs font-semibold text-muted-foreground">
                          {t('rules.howTo')}{' '}
                        </span>
                        {title.obtain}
                      </p>
                    )}
                    {title.review && title.review.status !== 'withdrawn' && (
                      <ReviewNote review={title.review} />
                    )}
                  </li>
                )
              })}
            </ul>
          )}
        </CardContent>
      </Card>

      {doing && (doing.what === 'make' || doing.what === 'change') && (
        <TitleDialog
          title={doing.what === 'change' ? doing.title : null}
          plate={plate}
          busy={busy}
          onDismiss={() => setDoing(null)}
          onSave={async (draft) => {
            const failure = await run(() =>
              doing.what === 'change'
                ? api(
                    endpoints.myTitle(doing.title.id),
                    jsonRequest('PUT', draftBody(draft), session.csrfToken),
                  )
                : api(endpoints.myTitles, jsonRequest('POST', draftBody(draft), session.csrfToken)),
            )
            if (!failure) setDoing(null)
            return failure
          }}
        />
      )}
      {doing && (doing.what === 'publish' || doing.what === 'request') && (
        <ReviewDialog
          title={doing.title}
          busy={busy}
          onDismiss={() => setDoing(null)}
          onAsk={async (request, draft) => {
            const failure = await run(() =>
              api(
                endpoints.myTitleReviews(doing.title.id),
                jsonRequest(
                  'POST',
                  draft ? { ...draftBody(draft), request } : { request },
                  session.csrfToken,
                ),
              ),
            )
            if (!failure) {
              setDoing(null)
              setSent(true)
            }
            return failure
          }}
        />
      )}
    </div>
  )
}

/** Makes a title, or changes a private one: its words and look, on the player's own plate. */
function TitleDialog({
  title,
  plate,
  busy,
  onDismiss,
  onSave,
}: {
  title: OwnTitle | null
  plate: Plate | undefined
  busy: boolean
  onDismiss: () => void
  /** Saves the draft; answers what went wrong, or nothing. */
  onSave: (draft: TitleDraft) => Promise<string>
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<TitleDraft>(title ? draftOf(title) : emptyDraft)
  const [error, setError] = useState('')
  return (
    <Modal
      title={title ? t('titles.editing') : t('titles.create')}
      busy={busy}
      onDismiss={onDismiss}
      className="sm:max-w-2xl"
      actions={
        <>
          <DialogClose render={<Button type="button" variant="outline" />} disabled={busy}>
            {t('titles.cancel')}
          </DialogClose>
          <Button
            disabled={busy || !draft.name.trim()}
            onClick={() => void onSave(draft).then(setError)}
          >
            {title ? t('titles.save') : t('titles.createButton')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <p className="text-sm text-muted-foreground">{t('titles.privateHint')}</p>
        {error && <Notice>{error}</Notice>}
        <TitleFields draft={draft} onChange={setDraft} prefix="my-title" disabled={busy} />
        {plate && (
          <Nameplate
            plate={{ ...plate, title: draftTitle(draft, title ?? undefined) }}
            className="max-w-xs"
          />
        )}
      </div>
    </Modal>
  )
}

/**
 * Asks for a review: of a private title, that it be made public as it is; of a public one, that
 * it be changed to what the form says. Either way with a few words for the title managers.
 */
function ReviewDialog({
  title,
  busy,
  onDismiss,
  onAsk,
}: {
  title: OwnTitle
  busy: boolean
  onDismiss: () => void
  /** Sends the request; answers what went wrong, or nothing. */
  onAsk: (request: string, draft: TitleDraft | null) => Promise<string>
}) {
  const { t } = useTranslation()
  const change = title.visibility === 'public'
  const [draft, setDraft] = useState<TitleDraft>(draftOf(title))
  const [request, setRequest] = useState('')
  const [error, setError] = useState('')
  return (
    <Modal
      title={change ? t('titles.review.askChange') : t('titles.review.ask')}
      busy={busy}
      onDismiss={onDismiss}
      className="sm:max-w-2xl"
      actions={
        <>
          <DialogClose render={<Button type="button" variant="outline" />} disabled={busy}>
            {t('titles.cancel')}
          </DialogClose>
          <Button
            disabled={busy || !request.trim() || (change && !draft.name.trim())}
            onClick={() => void onAsk(request.trim(), change ? draft : null).then(setError)}
          >
            <PaperPlaneTiltIcon aria-hidden="true" />
            {t('titles.review.send')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {change ? t('titles.review.changeHint') : t('titles.review.publishHint')}
        </p>
        {error && <Notice>{error}</Notice>}
        {change ? (
          <TitleFields draft={draft} onChange={setDraft} prefix="review-title" disabled={busy} />
        ) : (
          <div className="flex justify-center rounded-2xl bg-muted/60 px-4 py-6">
            <TitleBand title={title} className="h-8" />
          </div>
        )}
        <NoteField
          id="review-request"
          label={change ? t('titles.review.changeRequest') : t('titles.review.publishRequest')}
          hint={t('titles.review.requestHint')}
          value={request}
          onChange={setRequest}
          disabled={busy}
          required
        />
      </div>
    </Modal>
  )
}
