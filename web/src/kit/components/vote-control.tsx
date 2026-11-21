import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { ArrowFatUpIcon, QuestionMarkIcon } from '@phosphor-icons/react'
import { cn } from '../lib/utils'
import { formatLocale } from '../i18n'
import { useSession } from '../session'
import { useNotification } from '../notification-context'
import type { Vote, VoteResult } from '../api/types'

export function compactNumber(value: number, language: string) {
  return new Intl.NumberFormat(formatLocale(language), {
    notation: 'compact',
    maximumFractionDigits: 1,
  }).format(value)
}

export function loginPath(location: { pathname: string; search: string; hash: string }) {
  return `/login?returnTo=${encodeURIComponent(location.pathname + location.search + location.hash)}`
}

type Tally = { upvotes: number; downvotes: number }

/** The two counts side by side, read-only: upvotes and 何意味 ("what?"). They are never netted. */
export function VoteCounts({ upvotes, downvotes }: Tally) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? ''
  return (
    <>
      <span
        className="inline-flex items-center gap-1"
        title={t('interactions.upvotesExact', { count: upvotes })}
      >
        <ArrowFatUpIcon className="size-3" aria-hidden="true" />
        <span className="sr-only">{t('interactions.upvote')}</span>
        {compactNumber(upvotes, language)}
      </span>
      <span
        className="inline-flex items-center gap-1"
        title={t('interactions.whatsExact', { count: downvotes })}
      >
        <QuestionMarkIcon className="size-3" weight="bold" aria-hidden="true" />
        <span className="sr-only">{t('interactions.what')}</span>
        {compactNumber(downvotes, language)}
      </span>
    </>
  )
}

/** What the tally becomes when the viewer's own vote changes from one value to another. */
export function tallyAfterVote(tally: Tally, from: Vote, to: Vote): Tally {
  return {
    upvotes: tally.upvotes - (from === 1 ? 1 : 0) + (to === 1 ? 1 : 0),
    downvotes: tally.downvotes - (from === -1 ? 1 : 0) + (to === -1 ? 1 : 0),
  }
}

// An upvote and a 何意味 ("what?", stored as -1), each with its own count. Pressing the
// active one withdraws the vote. The counts update immediately and roll back if the server
// rejects the vote.
export function VoteControl({
  upvotes,
  downvotes,
  myVote,
  onVote,
  onResult,
  label,
  layout = 'horizontal',
  disabled = false,
  className,
}: Tally & {
  myVote: Vote
  onVote: (value: Vote) => Promise<VoteResult>
  onResult: (result: VoteResult) => void
  label: string
  layout?: 'horizontal' | 'vertical'
  disabled?: boolean
  className?: string
}) {
  const { t, i18n } = useTranslation()
  const { user } = useSession()
  const { notify } = useNotification()
  const navigate = useNavigate()
  const location = useLocation()
  const [pending, setPending] = useState<(Tally & { myVote: Vote }) | null>(null)
  const shown = pending ?? { upvotes, downvotes, myVote }
  const cast = async (direction: 1 | -1) => {
    if (!user) {
      navigate(loginPath(location))
      return
    }
    if (pending) return
    const next: Vote = shown.myVote === direction ? 0 : direction
    setPending({ ...tallyAfterVote({ upvotes, downvotes }, myVote, next), myVote: next })
    try {
      onResult(await onVote(next))
    } catch (e) {
      notify((e as Error).message, 'error')
    } finally {
      setPending(null)
    }
  }
  const button =
    'inline-flex h-7 min-w-7 items-center justify-center gap-1 rounded-full px-1.5 text-sm font-semibold tabular-nums text-muted-foreground transition-all duration-200 ease-[cubic-bezier(0.25,0.1,0.25,1)] hover:bg-muted active:scale-[0.96] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 motion-reduce:transition-none motion-reduce:active:scale-100'
  const language = i18n.resolvedLanguage ?? ''
  return (
    <div
      role="group"
      aria-label={label}
      className={cn(
        'inline-flex shrink-0 items-center gap-0.5',
        layout === 'vertical' && 'flex-col',
        className,
      )}
    >
      <button
        type="button"
        className={cn(button, shown.myVote === 1 && 'text-don hover:text-don')}
        aria-pressed={shown.myVote === 1}
        aria-label={t('interactions.upvotesExact', { count: shown.upvotes })}
        title={t('interactions.upvote')}
        disabled={disabled}
        onClick={() => void cast(1)}
      >
        <ArrowFatUpIcon
          className="size-4"
          weight={shown.myVote === 1 ? 'fill' : 'bold'}
          aria-hidden="true"
        />
        <span aria-live="polite">{compactNumber(shown.upvotes, language)}</span>
      </button>
      <button
        type="button"
        className={cn(button, shown.myVote === -1 && 'text-link hover:text-link')}
        aria-pressed={shown.myVote === -1}
        aria-label={t('interactions.whatsExact', { count: shown.downvotes })}
        title={t('interactions.what')}
        disabled={disabled}
        onClick={() => void cast(-1)}
      >
        <QuestionMarkIcon className="size-4" weight="bold" aria-hidden="true" />
        <span aria-live="polite">{compactNumber(shown.downvotes, language)}</span>
      </button>
    </div>
  )
}
