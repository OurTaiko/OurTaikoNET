import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { CheckCircleIcon, TagIcon, UsersIcon } from '@phosphor-icons/react'
import type { ListedTitle, RewardRule, RuleCategories } from '@players/api/types'
import { ChoiceSelect } from '@players/components/choice-select'
import { TitleBand } from '@players/components/nameplate'
import { Notice } from '@players/notice'
import { playerQueries } from '@players/queries'
import { useSession } from '@players/session'
import { ruleText } from '@players/reward-rules'
import { EmptyState } from '@sitekit/components/empty-state'
import { PlayerLink } from '@sitekit/components/player-link'
import { SearchInput } from '@sitekit/components/search-input'
import { buttonVariants } from '@sitekit/components/ui/button'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { cn } from '@sitekit/lib/utils'

const noRules: RewardRule[] = []
const noCategories: RuleCategories = {}

/** The page of the signed-in player's own titles. */
export const myTitlesPage = '/me/titles'

/** The public titles, the rules that give them, and the categories a rule may name. */
export function useTitles() {
  const { data, error } = useQuery(playerQueries.titles())
  return {
    titles: data?.titles ?? null,
    rules: data?.rules ?? noRules,
    categories: data?.categories ?? noCategories,
    // What was read before stays on show when reading it again fails.
    error: data ? '' : (error?.message ?? ''),
  }
}

// As many cards across as fit.
const grid = 'grid grid-cols-[repeat(auto-fill,minmax(min(19rem,100%),1fr))] gap-3'

type Held = 'all' | 'held' | 'missing'

/** How a title is obtained: the managers' words for it, and the rules that give it by themselves. */
export function HowToGet({ title, rules }: { title: ListedTitle; rules: RewardRule[] }) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  const own = rules.filter((rule) => rule.titleId === title.id)
  if (!title.obtain && own.length === 0) return null
  return (
    <div className="space-y-1 text-sm" data-testid="title-rules">
      <p className="text-xs font-semibold text-muted-foreground">{t('rules.howTo')}</p>
      {title.obtain && <p className="whitespace-pre-line">{title.obtain}</p>}
      {own.length > 0 && (
        <ul className="list-disc pl-5">
          {own.map((rule) => (
            <li key={rule.id}>{ruleText(rule, t, language)}</li>
          ))}
        </ul>
      )}
    </div>
  )
}

function TitleCard({
  title,
  rules,
  held,
}: {
  title: ListedTitle
  rules: RewardRule[]
  held: boolean
}) {
  const { t } = useTranslation()
  return (
    <li
      className={cn(
        'flex flex-col gap-3 rounded-[20px] bg-card p-4 shadow-elev-1',
        held && 'ring-2 ring-primary/40',
      )}
    >
      {/* The band alone on a quiet stage, with room for what stands out of it. */}
      <div className="relative flex items-center justify-center rounded-2xl bg-muted/60 px-4 py-7">
        <TitleBand title={title} className="h-8" />
        {held && (
          <span className="absolute top-2 right-2 flex items-center gap-1 rounded-full bg-primary px-2 py-0.5 text-xs font-semibold text-primary-foreground">
            <CheckCircleIcon weight="fill" aria-hidden="true" className="size-3.5" />
            {t('titles.have')}
          </span>
        )}
      </div>
      {title.description && <p className="text-sm">{title.description}</p>}
      <HowToGet title={title} rules={rules} />
      <div className="mt-auto flex flex-wrap items-center gap-x-3 gap-y-1 pt-1 text-xs text-muted-foreground">
        <span className="flex items-center gap-1">
          <UsersIcon aria-hidden="true" className="size-3.5" />
          {t('titles.holders', { count: title.holders })}
        </span>
        {title.ownerId && (
          <span className="min-w-0 truncate">
            {t('titles.madeBy')}{' '}
            <PlayerLink id={title.ownerId} className="text-link hover:underline">
              {title.ownerName || t('profile.unknownPlayer')}
            </PlayerLink>
          </span>
        )}
      </div>
    </li>
  )
}

/**
 * The catalog: every public title on its band, with how it is obtained and how many hold it.
 * A title a player made for themselves is private and not here until a review makes it public;
 * the player's own are on their page of titles.
 */
export function TitlesPage() {
  const { t } = useTranslation()
  const session = useSession()
  const { titles, rules, error } = useTitles()
  const user = session.user
  const mine = useQuery({ ...playerQueries.heldTitles(user?.id ?? ''), enabled: !!user })
  const held = useMemo(() => new Set(mine.data?.titles.map((h) => h.title.id) ?? []), [mine.data])
  const [query, setQuery] = useState('')
  // What is typed is searched for a moment later; the list waits dimmed meanwhile.
  const [typing, setTyping] = useState(false)
  const [show, setShow] = useState<Held>('all')
  const shown = useMemo(() => {
    const text = query.trim().toLowerCase()
    return (titles ?? []).filter((title) => {
      if (show !== 'all' && held.has(title.id) !== (show === 'held')) return false
      if (!text) return true
      return [
        title.name,
        ...Object.values(title.nameTranslations),
        title.description,
        title.obtain,
        title.ownerName,
      ].some((value) => value.toLowerCase().includes(text))
    })
  }, [titles, query, show, held])
  return (
    <section className="space-y-4" aria-labelledby="titles-heading">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
        <div className="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1 lg:mr-auto">
          <h1 id="titles-heading" className="text-2xl font-extrabold tracking-tight sm:text-3xl">
            {t('titles.catalog')}
          </h1>
          {titles && (
            <p className="text-sm text-muted-foreground" role="status" aria-live="polite">
              {user
                ? t('titles.totalHeld', {
                    total: titles.length,
                    held: titles.filter((title) => held.has(title.id)).length,
                  })
                : t('titles.total', { total: titles.length })}
            </p>
          )}
        </div>
        <div
          role="search"
          aria-label={t('titles.search')}
          className="flex min-w-0 flex-wrap gap-2 lg:w-[30rem] lg:shrink-0"
        >
          <div className="flex min-w-0 basis-full sm:basis-0 sm:grow">
            <SearchInput
              value={query}
              onSearch={setQuery}
              onPendingChange={setTyping}
              label={t('titles.search')}
              placeholder={t('titles.searchPlaceholder')}
            />
          </div>
          {user && (
            <ChoiceSelect
              label={t('titles.show.label')}
              value={show}
              onValueChange={(value) => setShow(value as Held)}
              items={(['all', 'held', 'missing'] as const).map((value) => ({
                value,
                label: t(`titles.show.${value}`),
              }))}
            />
          )}
        </div>
      </div>
      {user && (
        <div className="flex flex-wrap items-center gap-3 rounded-[20px] bg-card px-4 py-3 text-sm shadow-elev-1">
          <TagIcon aria-hidden="true" className="size-5 shrink-0 text-primary" />
          <p className="min-w-0 flex-1 basis-56 text-muted-foreground">{t('titles.makeYourOwn')}</p>
          <Link to={myTitlesPage} className={buttonVariants({ variant: 'outline', size: 'sm' })}>
            {t('shell.myTitles')}
          </Link>
        </div>
      )}
      {error && <Notice>{error}</Notice>}
      {!titles ? (
        !error && (
          <div className={grid} aria-busy="true">
            {Array.from({ length: 6 }, (_, n) => (
              <Skeleton key={n} className="h-44 rounded-[20px]" />
            ))}
          </div>
        )
      ) : shown.length === 0 ? (
        <EmptyState title={titles.length === 0 ? t('titles.catalogEmpty') : t('titles.noMatch')} />
      ) : (
        <ul className={cn(grid, typing && 'opacity-60')} data-testid="title-catalog">
          {shown.map((title) => (
            <TitleCard key={title.id} title={title} rules={rules} held={held.has(title.id)} />
          ))}
        </ul>
      )}
    </section>
  )
}
