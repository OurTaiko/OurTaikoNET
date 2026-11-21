import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { ArrowClockwiseIcon, UsersIcon } from '@phosphor-icons/react'
import { Button } from './components/ui/button'
import { Card } from './components/ui/card'
import { Skeleton } from './components/ui/skeleton'
import { ChoiceSelect } from './components/choice-select'
import { SearchInput } from './components/search-input'
import { PublicUserCard } from './components/public-user-card'
import { directoryQuery, directorySorts } from './queries/directory'
const directoryPageSize = 24

// As many cards across as fit, so the list is as dense on a wide page as on a phone.
const grid = 'grid grid-cols-[repeat(auto-fill,minmax(min(18rem,100%),1fr))] gap-3'

/** The site's players, whichever source they play. */
export function UsersPage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const q = params.get('q') || ''
  const sort = directorySorts.find((value) => value === params.get('sort')) ?? 'active'
  const rawPage = Number(params.get('page'))
  const page = Number.isInteger(rawPage) && rawPage >= 1 && rawPage <= 10000 ? rawPage : 1
  // Each page starts at its top: the buttons that turn it are below the list.
  useEffect(() => {
    window.scrollTo(0, 0)
  }, [page])
  const directory = useQuery(directoryQuery({ q, sort, page, pageSize: directoryPageSize }))
  const result = directory.data
  // What was read before stays on show when reading it again fails.
  const error = result ? '' : (directory.error?.message ?? '')
  // The list of another page or search is on its way; the one before is not shown meanwhile.
  const loading = directory.isPending || directory.isPlaceholderData
  const [searchPending, setSearchPending] = useState(false)
  const search = useCallback(
    (query: string) => {
      setParams(
        (current) => {
          const next = new URLSearchParams(current)
          if (query) next.set('q', query)
          else next.delete('q')
          next.set('page', '1')
          return next
        },
        { replace: true },
      )
    },
    [setParams],
  )
  const update = (values: Record<string, string>) => setParams({ q, sort, page: '1', ...values })
  const busy = loading || searchPending
  return (
    <section className="space-y-4" aria-labelledby="users-heading">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
        <div className="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1 lg:mr-auto">
          <h1 id="users-heading" className="text-2xl font-extrabold tracking-tight sm:text-3xl">
            {t('messages.userSquare')}
          </h1>
          <p className="text-sm text-muted-foreground" role="status" aria-live="polite">
            {busy
              ? t('messages.loadingMembers')
              : error
                ? t('messages.membersUnavailable')
                : t('messages.membersTotal', { total: result?.total ?? 0 })}
          </p>
        </div>
        <div
          role="search"
          aria-label={t('messages.searchMembers')}
          className="flex min-w-0 flex-wrap gap-2 lg:w-[30rem] lg:shrink-0"
        >
          {/* On a phone the box has a row to itself, so what it asks for can be read. */}
          <div className="flex min-w-0 basis-full sm:basis-0 sm:grow">
            <SearchInput
              value={q}
              onSearch={search}
              onPendingChange={setSearchPending}
              label={t('messages.searchMembers')}
              placeholder={t('messages.searchMembersPlaceholder')}
            />
          </div>
          <ChoiceSelect
            label={t('messages.sortMembers')}
            value={sort}
            onValueChange={(value) => update({ sort: value })}
            items={[
              { value: 'active', label: t('messages.recentlyActiveMembers') },
              { value: 'newest', label: t('messages.newestMembers') },
              { value: 'karma', label: t('messages.highestKarmaMembers') },
            ]}
          />
          <Button
            variant="outline"
            size="icon"
            onClick={() => void directory.refetch()}
            disabled={directory.isFetching}
            aria-label={t('messages.refreshUsers')}
            title={t('messages.refreshUsers')}
          >
            <ArrowClockwiseIcon aria-hidden="true" />
          </Button>
        </div>
      </div>
      {error ? (
        <Card role="alert" className="border-destructive p-6 text-destructive shadow-none">
          {error}
        </Card>
      ) : busy ? (
        <div className={grid} aria-busy="true" aria-label={t('messages.loadingMembers')}>
          {Array.from({ length: 12 }, (_, n) => (
            <Skeleton key={n} className="h-[9.8125rem] rounded-[20px]" />
          ))}
        </div>
      ) : (
        <>
          {result && !result.profilesAvailable && (
            <p role="status" className="text-sm text-muted-foreground">
              {t('messages.memberNamesUnavailable')}
            </p>
          )}
          {result?.items.length ? (
            <div className={grid}>
              {result.items.map((user) => (
                <article key={user.id} aria-label={user.nickname ?? t('messages.unknownMember')}>
                  <PublicUserCard user={user} sort={sort} />
                </article>
              ))}
            </div>
          ) : (
            <Card className="items-center gap-3 border border-dashed border-input bg-cream px-6 py-12 text-center shadow-none">
              <UsersIcon className="mb-2 size-8 text-muted-foreground" aria-hidden="true" />
              <h2 className="text-lg font-extrabold">
                {q ? t('messages.noMatchingMembers') : t('messages.noMembersOnThisPage')}
              </h2>
              {(q || page > 1) && (
                <Button variant="outline" onClick={() => setParams({ sort })}>
                  {t('messages.showAllMembers')}
                </Button>
              )}
            </Card>
          )}
          {result && result.total > result.pageSize && (
            <nav
              aria-label={t('messages.memberPagination')}
              className="flex flex-wrap items-center justify-center gap-4 text-sm"
            >
              <Button
                variant="outline"
                disabled={page <= 1}
                onClick={() => update({ page: String(page - 1) })}
              >
                {t('messages.previous')}
              </Button>
              <span>
                {t('messages.pagination', {
                  page,
                  pages: Math.ceil(result.total / result.pageSize),
                })}
              </span>
              <Button
                variant="outline"
                disabled={page * result.pageSize >= result.total}
                onClick={() => update({ page: String(page + 1) })}
              >
                {t('messages.next')}
              </Button>
            </nav>
          )}
        </>
      )}
    </section>
  )
}
