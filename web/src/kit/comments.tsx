import { isChartOwner } from './chart-owners'
import { PlayerLink } from './components/player-link'
import { Suspense, createContext, lazy, useContext, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router-dom'
import { useInfiniteQuery } from '@tanstack/react-query'
import {
  ArrowBendUpLeftIcon,
  ArrowRightIcon,
  ChatCircleIcon,
  LinkSimpleIcon,
  MinusIcon,
  PencilSimpleIcon,
  PlusCircleIcon,
  PlusIcon,
  ShieldWarningIcon,
  TrashIcon,
} from '@phosphor-icons/react'
import { api, jsonRequest } from './api/client'
import { planReplies, type FoldPolicy } from './comment-folding'
import { endpoints, withQuery } from './api/endpoints'
import type { ChartBase, Comment, Dan, CommentSort, Vote, VoteResult } from './api/types'
import { formatLocale } from './i18n'
import { cn } from './lib/utils'
import { Button, buttonVariants } from './components/ui/button'
import { DialogClose } from './components/ui/dialog'
import { Textarea } from './components/ui/textarea'
import { Skeleton } from './components/ui/skeleton'
import { ChoiceSelect } from './components/choice-select'
import { UserAvatar } from './components/user-avatar'
import { ProfilePlate } from './components/profile-plate'
import { VoteControl, loginPath } from './components/vote-control'
import { Modal, Notice } from './notifications'
import { useNotification } from './notification-context'
import { useSession } from './session'
import { SOURCE } from './source'
import {
  commentQueries,
  invalidateSubjectComments,
  type CommentPages,
  type CommentsRead,
} from './queries/comments'
import { queryClient } from './query/client'

const maxCommentLength = 10000
// Deeper replies continue on the comment's own page, as on Reddit; a phone has
// no room for deep indentation and stops sooner.
const maxVisibleDepth = 8
const maxVisibleDepthNarrow = 5
const narrowScreen = '(max-width: 639px)'
// What each top-level thread shows first; the rest folds into "N replies".
const threadFold: FoldPolicy = { budget: 10, depth: 4, rootReplies: 4, replies: 2 }
const threadFoldNarrow: FoldPolicy = { budget: 8, depth: 2, rootReplies: 3, replies: 2 }
// A permalink shows a single thread, so more of it.
const focusFold: FoldPolicy = { budget: 40, depth: 6, rootReplies: 10, replies: 4 }
// What each click on a folded button reveals.
const expandFold: FoldPolicy = { budget: 10, depth: 3, rootReplies: 10, replies: 2 }
// Comments at or below this score start collapsed.
const collapseScore = -5

export const commentPath = (chartId: string, commentId: string) =>
  `/charts/${encodeURIComponent(chartId)}/comments/${encodeURIComponent(commentId)}`
export const danCommentPath = (danId: string, commentId: string) =>
  `/dans/${encodeURIComponent(danId)}/comments/${encodeURIComponent(commentId)}`

// What a thread hangs on: a chart or a 段位 course of a song source, or an announcement of
// the site. The pages and endpoints differ; the comments themselves do not.
export type CommentSubject = {
  /** Whose comments these are: a song source, or 'site'. It keeps their caches apart. */
  host: string
  /** The page the thread lives on, e.g. /charts/12 or /dans/3. */
  path: string
  commentCount: number
  comments: (query: { sort: string; page: number }) => string
  newComment: string
  /** One comment's own address (PATCH, DELETE), its vote's, and its thread's in an order. */
  comment: (commentId: string) => string
  commentVote: (commentId: string) => string
  commentThread: (commentId: string, sort: string) => string
  commentPath: (commentId: string) => string
  isOwner: (userId: string) => boolean
  /** Whether the viewer may remove others' comments here, when it is not their isAdmin. */
  moderator?: boolean
  /** The new-comment box's prompt names what the thread is about. */
  placeholder:
    | 'interactions.commentPlaceholder'
    | 'interactions.danCommentPlaceholder'
    | 'interactions.announcementCommentPlaceholder'
}

// A song source's addresses for single comments, taken now: a later request may be made after
// the visitor has moved to another area.
function sourceComments() {
  const { comment, commentVote, commentThread } = endpoints
  return { host: SOURCE, comment, commentVote, commentThread }
}

export function chartSubject(chart: ChartBase): CommentSubject {
  // Taken now: a later page may be asked for after the visitor has moved to another area.
  const address = endpoints.newComment(chart.id)
  return {
    ...sourceComments(),
    path: `/charts/${encodeURIComponent(chart.id)}`,
    commentCount: chart.commentCount,
    comments: (query) => withQuery(address, query),
    newComment: address,
    commentPath: (id) => commentPath(chart.id, id),
    isOwner: (userId) => isChartOwner(chart, userId),
    placeholder: 'interactions.commentPlaceholder',
  }
}

export function danSubject(dan: Dan): CommentSubject {
  // Taken now: a later page may be asked for after the visitor has moved to another area.
  const address = `${endpoints.dan(dan.id)}/comments`
  return {
    ...sourceComments(),
    path: `/dans/${encodeURIComponent(dan.id)}`,
    commentCount: dan.commentCount,
    comments: (query) => withQuery(address, query),
    newComment: address,
    commentPath: (id) => danCommentPath(dan.id, id),
    isOwner: (userId) => userId === dan.ownerId,
    placeholder: 'interactions.danCommentPlaceholder',
  }
}

// Where a comment outside its thread (a history, a notice) links to.
export function commentSubjectPath(comment: Pick<Comment, 'chartId' | 'danId'>) {
  return comment.danId
    ? `/dans/${encodeURIComponent(comment.danId)}`
    : `/charts/${encodeURIComponent(comment.chartId ?? '')}`
}
export function commentPermalink(comment: Pick<Comment, 'id' | 'chartId' | 'danId'>) {
  return comment.danId
    ? danCommentPath(comment.danId, comment.id)
    : commentPath(comment.chartId ?? '', comment.id)
}

export function RelativeTime({ value }: { value: string }) {
  const { i18n } = useTranslation()
  const date = new Date(value)
  const locale = formatLocale(i18n.language)
  const seconds = Math.round((date.getTime() - Date.now()) / 1000)
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ['year', 31536000],
    ['month', 2592000],
    ['week', 604800],
    ['day', 86400],
    ['hour', 3600],
    ['minute', 60],
  ]
  const [unit, size] = units.find(([, size]) => Math.abs(seconds) >= size) ?? ['second', 1]
  const text =
    unit === 'second'
      ? new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(0, 'second')
      : new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(
          Math.trunc(seconds / size),
          unit,
        )
  return (
    <time
      dateTime={value}
      title={date.toLocaleString(locale, { dateStyle: 'medium', timeStyle: 'short' })}
    >
      {text}
    </time>
  )
}

const Markdown = lazy(() => import('./markdown'))

/**
 * A comment's text, which its writer may write in Markdown as an announcement is written; HTML in
 * it is never run. Until the parser has arrived it is shown as
 * written, with its web addresses as links.
 */
export function CommentBody({ text }: { text: string }) {
  return (
    <Suspense fallback={<PlainCommentBody text={text} />}>
      <Markdown text={text} by="member" className="max-w-[70ch]" />
    </Suspense>
  )
}

function PlainCommentBody({ text }: { text: string }) {
  const parts = text.split(/(https?:\/\/[^\s<>"']+[^\s<>"'.,;:!?)\]}，。！？）】])/g)
  return (
    <p className="max-w-[70ch] whitespace-pre-wrap text-sm leading-6 wrap-anywhere">
      {parts.map((part, i) =>
        i % 2 ? (
          <a
            key={i}
            href={part}
            target="_blank"
            rel="noopener noreferrer nofollow ugc"
            className="text-link hover:underline"
          >
            {part}
          </a>
        ) : (
          part
        ),
      )}
    </p>
  )
}

function Composer({
  initial = '',
  placeholder,
  submitLabel,
  autoFocus = false,
  onSubmit,
  onCancel,
}: {
  initial?: string
  placeholder: string
  submitLabel: string
  autoFocus?: boolean
  onSubmit: (body: string) => Promise<void>
  onCancel?: () => void
}) {
  const { t } = useTranslation()
  const { notify } = useNotification()
  const [body, setBody] = useState(initial)
  const [busy, setBusy] = useState(false)
  const length = Array.from(body.trim()).length
  const submit = async () => {
    if (busy || length === 0 || length > maxCommentLength) return
    setBusy(true)
    try {
      await onSubmit(body)
      setBody('')
    } catch (e) {
      notify((e as Error).message, 'error')
    } finally {
      setBusy(false)
    }
  }
  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <Textarea
        value={body}
        autoFocus={autoFocus}
        placeholder={placeholder}
        aria-label={placeholder}
        maxLength={maxCommentLength + 200}
        disabled={busy}
        className="min-h-24 bg-muted dark:bg-input/50"
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
            e.preventDefault()
            void submit()
          }
          if (e.key === 'Escape' && onCancel) onCancel()
        }}
      />
      <div className="flex flex-wrap items-center justify-end gap-2">
        <span
          className={cn(
            'mr-auto text-xs tabular-nums text-muted-foreground',
            length > maxCommentLength && 'text-destructive',
          )}
        >
          {length > maxCommentLength * 0.9
            ? t('interactions.characterCount', { count: length, max: maxCommentLength })
            : t('interactions.markdownHint')}
        </span>
        {onCancel && (
          <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={onCancel}>
            {t('messages.cancel')}
          </Button>
        )}
        <Button
          type="submit"
          size="sm"
          disabled={busy || length === 0 || length > maxCommentLength}
          className="active:scale-[0.96]"
        >
          {busy ? t('interactions.posting') : submitLabel}
        </Button>
      </div>
    </form>
  )
}

type Tree = {
  subject: CommentSubject
  focusId?: string
  narrow: boolean
  /** The deepest level shown; deeper replies get a continue-thread link. */
  maxDepth: number
  update: (id: string, change: (comment: Comment) => Comment) => void
  addReply: (parentId: string, reply: Comment) => void
  remove: (id: string, purged: boolean, removed: boolean) => void
}
const TreeContext = createContext<Tree | null>(null)
// How many direct replies each comment in a thread shows first, as planned by the
// top-level comment or the latest expansion above it.
const FoldContext = createContext<Map<string, number>>(new Map())

// A policy goes no deeper than the levels left below `level`.
function foldFrom(policy: FoldPolicy, level: number, maxDepth: number): FoldPolicy {
  return { ...policy, depth: Math.min(policy.depth, maxDepth - level) }
}

function mapTree(items: Comment[], id: string, change: (c: Comment) => Comment): Comment[] {
  return items.map((c) =>
    c.id === id ? change(c) : { ...c, replies: mapTree(c.replies, id, change) },
  )
}

// Mirrors the server: a purged comment disappears, then any deleted placeholder
// left without replies disappears as well.
function purge(items: Comment[], id: string): Comment[] {
  return items
    .filter((c) => c.id !== id)
    .map((c) => {
      const replies = purge(c.replies, id)
      return replies.length === c.replies.length
        ? c
        : { ...c, replies, replyCount: c.replyCount - 1 }
    })
    .filter((c) => !((c.deleted || c.removed) && c.replyCount === 0))
}

// Comments posted meanwhile can shift pages; skip what is already shown.
function shownComments(pages: CommentsRead[]) {
  const seen = new Set<string>()
  return pages
    .flatMap((page) => page.items)
    .filter((comment) => !seen.has(comment.id) && !!seen.add(comment.id))
}

function countTree(items: Comment[]): number {
  return items.reduce((n, c) => n + 1 + countTree(c.replies), 0)
}

function CommentNode({ comment, level }: { comment: Comment; level: number }) {
  const { t } = useTranslation()
  const tree = useContext(TreeContext)!
  const { user, csrfToken } = useSession()
  const { notify } = useNotification()
  const location = useLocation()
  const [collapsed, setCollapsed] = useState(comment.score <= collapseScore)
  const inheritedFold = useContext(FoldContext)
  const [fold, setFold] = useState(() => {
    if (level > 0) return inheritedFold
    const policy = tree.focusId ? focusFold : tree.narrow ? threadFoldNarrow : threadFold
    return planReplies(comment, foldFrom(policy, level, tree.maxDepth))
  })
  const [shown, setShown] = useState(() => fold.get(comment.id) ?? 0)
  const [replying, setReplying] = useState(false)
  const [editing, setEditing] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const [busy, setBusy] = useState(false)
  const gone = comment.deleted || comment.removed
  const own = !!user && user.id === comment.authorId
  const canModerate = !!user && (tree.subject.moderator ?? user.isAdmin) && !own
  const focused = tree.focusId === comment.id
  const ref = useRef<HTMLElement>(null)
  useEffect(() => {
    if (!focused) return
    // The chart preview above loads later and grows the page. Keep the linked
    // comment in view while the layout settles, until the reader scrolls.
    const keep = () => ref.current?.scrollIntoView({ block: 'center' })
    keep()
    const observer = new ResizeObserver(keep)
    // body has a fixed height; the page content grows inside main.
    observer.observe(ref.current?.closest('main') ?? document.documentElement)
    const stop = () => observer.disconnect()
    const timer = window.setTimeout(stop, 4000)
    const events = ['wheel', 'touchstart', 'keydown', 'pointerdown'] as const
    events.forEach((e) => window.addEventListener(e, stop, { once: true, passive: true }))
    return () => {
      stop()
      window.clearTimeout(timer)
      events.forEach((e) => window.removeEventListener(e, stop))
    }
  }, [focused])
  const permalink = tree.subject.commentPath(comment.id)
  const author = gone ? '' : comment.author || t('messages.unknownMember')
  const hiddenReplies = comment.replyCount - comment.replies.length
  const folded = comment.replies.slice(shown)
  const expand = () => {
    const more = planReplies(comment, foldFrom(expandFold, level, tree.maxDepth), shown)
    setFold(new Map([...fold, ...more]))
    setShown(Math.max(more.get(comment.id) ?? 0, shown + 1))
  }
  const remove = async () => {
    setBusy(true)
    try {
      const result = await api<{ purged: boolean }>(
        tree.subject.comment(comment.id),
        jsonRequest('DELETE', {}, csrfToken),
      )
      setConfirm(false)
      tree.remove(comment.id, result.purged, canModerate)
      notify(
        canModerate ? t('interactions.commentRemoved') : t('interactions.commentDeleted'),
        'success',
      )
    } catch (e) {
      notify((e as Error).message, 'error')
    } finally {
      setBusy(false)
    }
  }
  return (
    <article
      ref={ref}
      id={`comment-${comment.id}`}
      aria-label={t('interactions.commentBy', {
        author: author || t('interactions.deletedAuthor'),
      })}
      className="min-w-0 scroll-mt-24"
    >
      <div className="flex gap-2">
        <div className="flex shrink-0 flex-col items-center">
          {gone ? (
            <span className="size-6 rounded-full bg-muted" aria-hidden="true" />
          ) : (
            <PlayerLink id={comment.authorId} tabIndex={-1} aria-hidden="true">
              <UserAvatar
                nickname={author}
                avatarPath={comment.authorAvatarPath}
                className="size-6"
                fallbackClassName="text-xs"
              />
            </PlayerLink>
          )}
          {!collapsed && comment.replies.length > 0 && (
            <button
              type="button"
              className="group mt-1 flex w-4 flex-1 justify-center outline-none"
              aria-label={t('interactions.collapseThread')}
              onClick={() => setCollapsed(true)}
            >
              <span className="w-px bg-border transition-colors group-hover:bg-link group-focus-visible:bg-link" />
            </button>
          )}
        </div>
        <div className="min-w-0 flex-1 pb-1">
          <header className="flex min-h-6 flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
            {gone ? (
              <span className="italic">{t('interactions.deletedAuthor')}</span>
            ) : (
              <PlayerLink
                id={comment.authorId}
                className="max-w-48 truncate font-medium text-foreground hover:underline"
              >
                {author}
              </PlayerLink>
            )}
            {/* The title and dan the author wears, as beside names on the leaderboard. */}
            {!gone && <ProfilePlate userId={comment.authorId} />}
            {!gone && tree.subject.isOwner(comment.authorId) && (
              <span
                className="rounded-md bg-link/10 px-1.5 py-0.5 font-medium text-link"
                title={t('collaboration.ownerBadgeTitle')}
              >
                {t('collaboration.owner')}
              </span>
            )}
            <span aria-hidden="true">·</span>
            <Link to={permalink} className="hover:underline">
              <RelativeTime value={comment.createdAt} />
            </Link>
            {comment.editedAt && !gone && (
              <span title={new Date(comment.editedAt).toLocaleString()}>
                · {t('interactions.edited')}
              </span>
            )}
            <button
              type="button"
              className="inline-flex items-center gap-1 rounded-md px-1 hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-expanded={!collapsed}
              aria-label={
                collapsed ? t('interactions.expandThread') : t('interactions.collapseThread')
              }
              onClick={() => setCollapsed((v) => !v)}
            >
              {collapsed ? <PlusIcon className="size-3" /> : <MinusIcon className="size-3" />}
              {collapsed && comment.replyCount > 0 && (
                <span>{t('interactions.replyCount', { count: comment.replyCount })}</span>
              )}
            </button>
          </header>
          {!collapsed && (
            <div
              className={cn(
                'mt-1 space-y-1',
                focused && '-mx-2 rounded-xl bg-link/5 px-2 py-1 ring-1 ring-ring/30',
              )}
            >
              {editing ? (
                <Composer
                  initial={comment.body}
                  autoFocus
                  placeholder={t('interactions.editPlaceholder')}
                  submitLabel={t('interactions.save')}
                  onCancel={() => setEditing(false)}
                  onSubmit={async (body) => {
                    const updated = await api<Comment>(
                      tree.subject.comment(comment.id),
                      jsonRequest('PATCH', { body }, csrfToken),
                    )
                    tree.update(comment.id, (c) => ({
                      ...c,
                      body: updated.body,
                      editedAt: updated.editedAt,
                    }))
                    setEditing(false)
                  }}
                />
              ) : gone ? (
                <p className="text-sm italic text-muted-foreground">
                  {comment.removed ? t('interactions.removedBody') : t('interactions.deletedBody')}
                </p>
              ) : (
                <CommentBody text={comment.body} />
              )}
              <div className="-ml-1.5 flex flex-wrap items-center gap-1 text-xs">
                <VoteControl
                  upvotes={comment.upvotes}
                  downvotes={comment.downvotes}
                  myVote={comment.myVote}
                  label={t('interactions.voteOnComment')}
                  disabled={gone && comment.myVote === 0}
                  onVote={(value: Vote) =>
                    api<VoteResult>(
                      tree.subject.commentVote(comment.id),
                      jsonRequest('PUT', { value }, csrfToken),
                    )
                  }
                  onResult={(r) => tree.update(comment.id, (c) => ({ ...c, ...r }))}
                />
                {!gone &&
                  (user ? (
                    <Button
                      variant="ghost"
                      size="xs"
                      aria-expanded={replying}
                      onClick={() => setReplying((v) => !v)}
                    >
                      <ArrowBendUpLeftIcon />
                      {t('interactions.reply')}
                    </Button>
                  ) : (
                    <Link
                      to={loginPath(location)}
                      className={buttonVariants({ variant: 'ghost', size: 'xs' })}
                    >
                      <ArrowBendUpLeftIcon />
                      {t('interactions.reply')}
                    </Link>
                  ))}
                <Button
                  variant="ghost"
                  size="xs"
                  onClick={() => {
                    const url = new URL(permalink, window.location.origin).href
                    navigator.clipboard?.writeText(url).then(
                      () => notify(t('interactions.linkCopied'), 'success'),
                      () => notify(url, 'info', t('interactions.share')),
                    )
                  }}
                >
                  <LinkSimpleIcon />
                  {t('interactions.share')}
                </Button>
                {own && !gone && !editing && (
                  <Button variant="ghost" size="xs" onClick={() => setEditing(true)}>
                    <PencilSimpleIcon />
                    {t('interactions.edit')}
                  </Button>
                )}
                {(own || canModerate) && !gone && (
                  <Button
                    variant="ghost"
                    size="xs"
                    className="text-destructive"
                    onClick={() => setConfirm(true)}
                  >
                    {canModerate ? <ShieldWarningIcon /> : <TrashIcon />}
                    {canModerate ? t('interactions.remove') : t('interactions.delete')}
                  </Button>
                )}
              </div>
              {replying && (
                <div className="pt-1">
                  <Composer
                    autoFocus
                    placeholder={t('interactions.replyPlaceholder', { author })}
                    submitLabel={t('interactions.reply')}
                    onCancel={() => setReplying(false)}
                    onSubmit={async (body) => {
                      const reply = await api<Comment>(
                        tree.subject.newComment,
                        jsonRequest('POST', { body, parentId: comment.id }, csrfToken),
                      )
                      tree.addReply(comment.id, reply)
                      // The new reply comes first; show one more so none already shown folds.
                      setShown((n) => n + 1)
                      setReplying(false)
                    }}
                  />
                </div>
              )}
            </div>
          )}
          {!collapsed &&
            (level >= tree.maxDepth && comment.replyCount > 0 ? (
              <Link
                to={permalink}
                className="mt-2 inline-flex items-center gap-1 text-xs text-link hover:underline"
              >
                {t('interactions.continueThread')}
                <ArrowRightIcon className="size-3" />
              </Link>
            ) : (
              (comment.replies.length > 0 || hiddenReplies > 0) && (
                <div className="mt-3 space-y-4">
                  <FoldContext.Provider value={fold}>
                    {comment.replies.slice(0, shown).map((reply) => (
                      <CommentNode key={reply.id} comment={reply} level={level + 1} />
                    ))}
                  </FoldContext.Provider>
                  {folded.length > 0 ? (
                    <button
                      type="button"
                      className="inline-flex items-center gap-1 rounded-md text-xs font-medium text-link hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      aria-expanded={false}
                      onClick={expand}
                    >
                      <PlusCircleIcon className="size-3.5" />
                      {shown === 0
                        ? t('interactions.replyCount', { count: countTree(folded) })
                        : t('interactions.moreReplies', { count: countTree(folded) })}
                    </button>
                  ) : (
                    hiddenReplies > 0 && (
                      <Link
                        to={permalink}
                        className="inline-flex items-center gap-1 text-xs text-link hover:underline"
                      >
                        {t('interactions.moreReplies', { count: hiddenReplies })}
                        <ArrowRightIcon className="size-3" />
                      </Link>
                    )
                  )}
                </div>
              )
            ))}
        </div>
      </div>
      {confirm && (
        <Modal
          title={canModerate ? t('interactions.removeTitle') : t('interactions.deleteTitle')}
          busy={busy}
          onDismiss={() => setConfirm(false)}
          actions={
            <>
              <DialogClose render={<Button type="button" variant="outline" />} disabled={busy}>
                {t('messages.cancel')}
              </DialogClose>
              <Button type="button" variant="destructive" disabled={busy} onClick={remove}>
                {canModerate ? t('interactions.remove') : t('interactions.delete')}
              </Button>
            </>
          }
        >
          {canModerate ? t('interactions.removeExplanation') : t('interactions.deleteExplanation')}
        </Modal>
      )}
    </article>
  )
}

export function CommentSection({
  subject,
  focusId,
  onCountChange,
  className,
}: {
  subject: CommentSubject
  focusId?: string
  onCountChange: (delta: number) => void
  /** Over the card it is on a page: where it is a part of something else, such as a dialog. */
  className?: string
}) {
  const { t } = useTranslation()
  const { user, csrfToken } = useSession()
  const location = useLocation()
  const [sort, setSort] = useState<CommentSort>('best')
  // Read once: rotating a phone does not fold comments already on screen.
  const [narrow] = useState(() => !!window.matchMedia?.(narrowScreen).matches)
  const read = commentQueries.section(subject, focusId, sort)
  const list = useInfiniteQuery(read)
  const pages = list.data?.pages
  const items = pages ? shownComments(pages) : null
  const total = pages?.at(-1)?.total ?? 0
  const loadingMore = list.isFetchingNextPage
  // What was read before stays on show when reading it again fails, but more of it that could not
  // be read is said; asking again shows the wait.
  const error =
    list.isFetching || (items && !list.isFetchNextPageError) ? '' : (list.error?.message ?? '')
  const section = useRef<HTMLElement>(null)
  useEffect(() => {
    if (location.hash === '#comments') section.current?.scrollIntoView()
  }, [location.hash])
  // The server answered each change with what it now holds, so the loaded pages are changed here.
  const write = async (change: (page: CommentsRead, index: number) => CommentsRead) => {
    // Nothing is held yet of an order still on its way: what comes will have the change.
    if (!queryClient.getQueryData(read.queryKey)) return
    // An answer already on its way was read before this change, and must not undo it.
    await queryClient.cancelQueries({ queryKey: read.queryKey })
    queryClient.setQueryData<CommentPages>(
      read.queryKey,
      (data) => data && { ...data, pages: data.pages.map(change) },
    )
    void invalidateSubjectComments(subject)
  }
  const tree: Tree = {
    subject,
    focusId,
    narrow,
    maxDepth: narrow ? maxVisibleDepthNarrow : maxVisibleDepth,
    update: (id, change) =>
      void write((page) => ({ ...page, items: mapTree(page.items, id, change) })),
    addReply: (parentId, reply) => {
      void write((page) => ({
        ...page,
        items: mapTree(page.items, parentId, (c) => ({
          ...c,
          replyCount: c.replyCount + 1,
          replies: [reply, ...c.replies],
        })),
      }))
      onCountChange(1)
    },
    remove: (id, purged, removed) => {
      if (!purged) {
        void write((page) => ({
          ...page,
          items: mapTree(page.items, id, (c) => ({
            ...c,
            body: '',
            author: '',
            authorId: '',
            authorAvatarPath: '',
            deleted: !removed,
            removed,
          })),
        }))
        return
      }
      if (!items) return
      const next = purge(items, id)
      const gone = items.length - next.length
      void write((page) => ({ ...page, items: purge(page.items, id), total: page.total - gone }))
      onCountChange(countTree(next) - countTree(items))
    },
  }
  const sorts: { value: CommentSort; label: string }[] = [
    { value: 'best', label: t('interactions.sortBest') },
    { value: 'top', label: t('interactions.sortTop') },
    { value: 'new', label: t('interactions.sortNew') },
    { value: 'old', label: t('interactions.sortOld') },
    { value: 'controversial', label: t('interactions.sortControversial') },
  ]
  return (
    <section
      ref={section}
      id="comments"
      aria-labelledby="comments-heading"
      className={cn(
        'scroll-mt-24 space-y-6 rounded-[20px] border border-border bg-card p-6 shadow-drop sm:p-8',
        className,
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2
          id="comments-heading"
          className="flex shrink-0 items-center gap-2 text-xl font-extrabold whitespace-nowrap"
        >
          <ChatCircleIcon aria-hidden="true" />
          {t('interactions.commentsHeading', { count: subject.commentCount })}
        </h2>
        <div className="shrink-0">
          <ChoiceSelect
            label={t('interactions.sortComments')}
            value={sort}
            items={sorts}
            onValueChange={(value) => setSort(value as CommentSort)}
          />
        </div>
      </div>
      {focusId ? (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl bg-muted px-4 py-3 text-sm">
          <span className="text-muted-foreground">{t('interactions.singleThread')}</span>
          <Link to={`${subject.path}#comments`} className="text-link hover:underline">
            {t('interactions.viewAllComments')}
          </Link>
          {items?.[0]?.parentId && (
            <Link to={subject.commentPath(items[0].parentId)} className="text-link hover:underline">
              {t('interactions.viewParent')}
            </Link>
          )}
        </div>
      ) : user ? (
        <Composer
          placeholder={t(subject.placeholder)}
          submitLabel={t('interactions.comment')}
          onSubmit={async (body) => {
            const created = await api<Comment>(
              subject.newComment,
              jsonRequest('POST', { body }, csrfToken),
            )
            void write((page, index) => ({
              ...page,
              items: index === 0 ? [created, ...page.items] : page.items,
              total: page.total + 1,
            }))
            onCountChange(1)
          }}
        />
      ) : (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-muted px-4 py-3 text-sm">
          <span className="text-muted-foreground">{t('interactions.signInToComment')}</span>
          <Link to={loginPath(location)} className={buttonVariants({ size: 'sm' })}>
            {t('messages.signInRegister')}
          </Link>
        </div>
      )}
      {error ? (
        <div className="flex flex-col items-center gap-3 py-6 text-center">
          <Notice>{error}</Notice>
          <Button variant="outline" onClick={() => void list.refetch()}>
            {t('messages.retry')}
          </Button>
        </div>
      ) : !items ? (
        <div className="space-y-4" role="status" aria-label={t('interactions.loadingComments')}>
          <Skeleton className="h-16 rounded-xl" />
          <Skeleton className="h-16 rounded-xl" />
        </div>
      ) : items.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          {t('interactions.noComments')}
        </p>
      ) : (
        <TreeContext.Provider value={tree}>
          <div
            className={cn('space-y-6 transition-opacity', list.isPlaceholderData && 'opacity-60')}
          >
            {items.map((comment) => (
              <CommentNode key={comment.id} comment={comment} level={0} />
            ))}
          </div>
        </TreeContext.Provider>
      )}
      {!focusId && items && items.length < total && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            disabled={loadingMore || list.isPlaceholderData}
            onClick={() => void list.fetchNextPage()}
          >
            {loadingMore ? t('interactions.loadingComments') : t('interactions.loadMoreComments')}
          </Button>
        </div>
      )}
    </section>
  )
}
