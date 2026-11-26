import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, jsonRequest } from '@players/api/client'
import { sourceApi, songPage } from '@players/components/song-tile'
import { Notice } from '@players/notice'
import { useSession } from '@players/session'
import type { SuggestionStatus } from '@sitekit/api/types'
import { categoryColor } from '@sitekit/category-colors'
import { chartText } from '@sitekit/chart-language'
import { Badge } from '@sitekit/components/ui/badge'
import { Button } from '@sitekit/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@sitekit/components/ui/tabs'
import { UserAvatar } from '@sitekit/components/user-avatar'
import { formatLocale } from '@sitekit/i18n'
import { queryClient } from '@sitekit/query/client'
import { categoryName } from '@players/reward-rules'

type Person = { id: string; nickname: string; avatarPath: string }
/** Every suggestion of one category for one chart (GET /category-suggestions). */
type SuggestionGroup = {
  chart: {
    id: string
    numberId: number
    title: string
    subtitle: string
    titleTranslations: Record<string, string>
    subtitleTranslations: Record<string, string>
    categoryIds: string[]
  }
  category: string
  status: SuggestionStatus
  count: number
  suggesters: Person[]
  firstAt: string
  lastAt: string
  decidedAt: string | null
  decidedBy: Person | null
}
type SuggestionPage = { items: SuggestionGroup[]; total: number; page: number; pageSize: number }

// Only Fanmade takes suggestions; its managers decide them.
const source = 'fanmade'
const statuses: SuggestionStatus[] = ['pending', 'accepted', 'rejected']
const queueKey = (status: string, page: number) => ['category-suggestions', status, page] as const

// 乐曲管理: the categories players suggest for Fanmade's songs, for its managers to accept or
// reject. A decision answers every suggestion of that category for that song, and those made
// after it.
export function SongsPage() {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const allowed = !!session.user?.groups?.includes('FANMADE_MANAGER')
  const [params, setParams] = useSearchParams()
  const status = (statuses as string[]).includes(params.get('status') ?? '')
    ? (params.get('status') as SuggestionStatus)
    : 'pending'
  const page = Math.max(1, Math.floor(Number(params.get('page')) || 1))
  const show = (next: { status?: string; page?: number }) => {
    const values = { status, page, ...next }
    setParams(
      {
        ...(values.status !== 'pending' && { status: values.status }),
        ...(values.page > 1 && { page: String(values.page) }),
      },
      { replace: true },
    )
  }
  const queue = useQuery({
    queryKey: queueKey(status, page),
    queryFn: ({ signal }) =>
      api<SuggestionPage>(
        `${sourceApi(source)}/category-suggestions?status=${status}&page=${page}`,
        { signal },
      ),
    enabled: allowed,
  })
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')

  if (session.loading) return null
  if (!allowed) return <Navigate to="/" replace />

  const name = (id: string) => categoryName(t, source, id)
  const when = (at: string) =>
    new Date(at).toLocaleString(formatLocale(i18n.language), {
      dateStyle: 'medium',
      timeStyle: 'short',
    })
  const decide = async (group: SuggestionGroup, accept: boolean) => {
    setBusy(true)
    setMessage('')
    try {
      await api(
        `${sourceApi(source)}/category-suggestions/decision`,
        jsonRequest(
          'POST',
          { chartId: group.chart.id, category: group.category, accept },
          session.csrfToken,
        ),
      )
      await queryClient.invalidateQueries({ queryKey: ['category-suggestions'] })
    } catch (e) {
      setMessage((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const answer = queue.data
  const error = message || (answer ? '' : (queue.error?.message ?? ''))
  const pages = answer ? Math.max(1, Math.ceil(answer.total / answer.pageSize)) : 1
  return (
    <div className="space-y-5">
      <header className="space-y-1">
        <h1 className="text-2xl font-extrabold tracking-tight sm:text-3xl">
          {t('songAdmin.heading')}
        </h1>
        <p className="text-sm text-muted-foreground">{t('songAdmin.intro')}</p>
      </header>
      <Tabs value={status} onValueChange={(value) => show({ status: String(value), page: 1 })}>
        <TabsList aria-label={t('songAdmin.status')}>
          {statuses.map((id) => (
            <TabsTrigger key={id} value={id}>
              {t(`songAdmin.statuses.${id}`)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      {error && <Notice>{error}</Notice>}
      {answer && answer.items.length === 0 && (
        <p className="py-10 text-center text-muted-foreground">{t('songAdmin.empty')}</p>
      )}
      <ul className="space-y-3">
        {answer?.items.map((group) => {
          const text = chartText(group.chart, i18n.resolvedLanguage)
          return (
            <li
              key={`${group.chart.id}/${group.category}`}
              className="flex flex-col gap-3 rounded-[20px] bg-card p-4 shadow-elev-1 sm:flex-row sm:items-center"
            >
              <div className="min-w-0 flex-1 space-y-2">
                <div className="flex flex-wrap items-center gap-2">
                  <a
                    className="font-bold underline-offset-2 hover:underline wrap-anywhere"
                    href={songPage(source, group.chart.id)}
                  >
                    {text.title || t('songAdmin.unknownChart')}
                  </a>
                  {text.subtitle && (
                    <span className="truncate text-xs text-muted-foreground">{text.subtitle}</span>
                  )}
                </div>
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <Badge className="gap-1.5 font-semibold">
                    {categoryColor(group.category) && (
                      <span
                        aria-hidden="true"
                        className="size-2 rounded-full"
                        style={{ background: categoryColor(group.category) }}
                      />
                    )}
                    {name(group.category)}
                  </Badge>
                  <span className="text-muted-foreground">
                    {t('songAdmin.suggestedBy', { count: group.count })}
                  </span>
                  <span className="flex items-center -space-x-1.5">
                    {group.suggesters.map((person) => (
                      <UserAvatar
                        key={person.id}
                        nickname={person.nickname}
                        avatarPath={person.avatarPath}
                        className="size-6 ring-2 ring-card"
                      />
                    ))}
                  </span>
                  <span className="truncate text-xs text-muted-foreground">
                    {group.suggesters.map((person) => person.nickname).join('、')}
                  </span>
                </div>
                {group.chart.categoryIds.length > 0 && (
                  <p className="text-xs text-muted-foreground">
                    {t('songAdmin.current', {
                      categories: group.chart.categoryIds.map(name).join(' · '),
                    })}
                  </p>
                )}
                <p className="text-xs text-muted-foreground tabular-nums">
                  {group.decidedAt
                    ? t('songAdmin.decided', {
                        at: when(group.decidedAt),
                        by: group.decidedBy?.nickname ?? t('songAdmin.earlier'),
                      })
                    : t('songAdmin.since', { at: when(group.firstAt) })}
                </p>
              </div>
              {status === 'pending' && (
                <div className="flex shrink-0 gap-2">
                  <Button
                    type="button"
                    size="sm"
                    disabled={busy}
                    onClick={() => decide(group, true)}
                  >
                    {t('songAdmin.accept')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => decide(group, false)}
                  >
                    {t('songAdmin.reject')}
                  </Button>
                </div>
              )}
            </li>
          )
        })}
      </ul>
      {pages > 1 && (
        <nav className="flex items-center justify-center gap-3" aria-label={t('songAdmin.pages')}>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => show({ page: page - 1 })}
          >
            {t('scores.previous')}
          </Button>
          <span className="text-sm tabular-nums">
            {page} / {pages}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={page >= pages}
            onClick={() => show({ page: page + 1 })}
          >
            {t('scores.next')}
          </Button>
        </nav>
      )}
    </div>
  )
}
