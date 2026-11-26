import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, jsonRequest } from '@players/api/client'
import { sourceApi, songPage, sources, SourceTag } from '@players/components/song-tile'
import { Notice } from '@players/notice'
import { useSession } from '@players/session'
import type { TagList, TagSuggestionStatus } from '@sitekit/api/types'
import { chartText } from '@sitekit/chart-language'
import { Badge } from '@sitekit/components/ui/badge'
import { Button } from '@sitekit/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@sitekit/components/ui/tabs'
import { UserAvatar } from '@sitekit/components/user-avatar'
import { formatLocale } from '@sitekit/i18n'
import { cn } from '@sitekit/lib/utils'
import type { SourceId } from '@sitekit/source'
import { queryClient } from '@sitekit/query/client'
import { tagName } from '@sitekit/tags'

type Person = { id: string; nickname: string; avatarPath: string }
/** Every suggestion of one tag for one chart (GET /tag-suggestions). */
type SuggestionGroup = {
  chart: {
    id: string
    numberId: number
    title: string
    subtitle: string
    titleTranslations: Record<string, string>
    subtitleTranslations: Record<string, string>
    tags: string[]
  }
  tag: string
  status: TagSuggestionStatus
  count: number
  suggesters: Person[]
  firstAt: string
  lastAt: string
  decidedAt: string | null
  decidedBy: Person | null
}
type SuggestionPage = { items: SuggestionGroup[]; total: number; page: number; pageSize: number }

const statuses: TagSuggestionStatus[] = ['pending', 'accepted', 'rejected']
// Who decides the suggestions of each source: TAG_MANAGER, or that source's managers.
const reviewers: Record<SourceId, string> = {
  fanmade: 'FANMADE_MANAGER',
  official: 'OFFICIAL_MANAGER',
}
/** The sources whose suggestions a player with these groups decides. */
export const reviewedSources = (groups: string[] = []) =>
  sources.filter((source) => groups.includes('TAG_MANAGER') || groups.includes(reviewers[source]))
const queueKey = (source: SourceId, status: string, page: number) =>
  ['tag-suggestions', source, status, page] as const

// 管理 / TAG: the tag managers' queue of the tags players suggest, one source at a time. A
// decision answers every suggestion of that tag for that chart, and those made after it.
export function TagsPage() {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const mine = reviewedSources(session.user?.groups)
  const allowed = mine.length > 0
  const [params, setParams] = useSearchParams()
  const source = (mine as string[]).includes(params.get('source') ?? '')
    ? (params.get('source') as SourceId)
    : (mine[0] ?? sources[0])
  const status = (statuses as string[]).includes(params.get('status') ?? '')
    ? (params.get('status') as TagSuggestionStatus)
    : 'pending'
  const page = Math.max(1, Math.floor(Number(params.get('page')) || 1))
  const show = (next: { source?: string; status?: string; page?: number }) => {
    const values = { source, status, page, ...next }
    setParams(
      {
        ...(values.source !== mine[0] && { source: values.source }),
        ...(values.status !== 'pending' && { status: values.status }),
        ...(values.page > 1 && { page: String(values.page) }),
      },
      { replace: true },
    )
  }
  const catalog = useQuery({
    queryKey: ['tags', source],
    queryFn: ({ signal }) => api<TagList>(`${sourceApi(source)}/tags`, { signal }),
    enabled: allowed,
    staleTime: 5 * 60_000,
  })
  const queue = useQuery({
    queryKey: queueKey(source, status, page),
    queryFn: ({ signal }) =>
      api<SuggestionPage>(`${sourceApi(source)}/tag-suggestions?status=${status}&page=${page}`, {
        signal,
      }),
    enabled: allowed,
  })
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')

  if (session.loading) return null
  if (!allowed) return <Navigate to="/" replace />

  const name = (id: string) =>
    tagName(
      catalog.data?.items.find((tag) => tag.id === id),
      i18n.language,
      id,
    )
  const when = (at: string) =>
    new Date(at).toLocaleString(formatLocale(i18n.language), {
      dateStyle: 'medium',
      timeStyle: 'short',
    })
  const decide = async (group: SuggestionGroup, accept: boolean) => {
    const key = `${group.chart.id}/${group.tag}`
    setBusy(key)
    setMessage('')
    try {
      await api(
        `${sourceApi(source)}/tag-suggestions/decision`,
        jsonRequest('POST', { chartId: group.chart.id, tag: group.tag, accept }, session.csrfToken),
      )
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['tag-suggestions', source] }),
        queryClient.invalidateQueries({ queryKey: ['tags', source] }),
      ])
    } catch (e) {
      setMessage((e as Error).message)
    } finally {
      setBusy('')
    }
  }

  const answer = queue.data
  const error = message || (answer ? '' : (queue.error?.message ?? ''))
  const pages = answer ? Math.max(1, Math.ceil(answer.total / answer.pageSize)) : 1
  return (
    <div className="space-y-5">
      <header className="space-y-1">
        <h1 className="text-2xl font-extrabold tracking-tight sm:text-3xl">
          {t('tagAdmin.heading')}
        </h1>
        <p className="text-sm text-muted-foreground">{t('tagAdmin.intro')}</p>
      </header>
      <div className="flex flex-wrap items-center gap-3">
        <div role="group" aria-label={t('tagAdmin.source')} className="flex gap-1.5">
          {mine.map((id) => (
            <Button
              key={id}
              type="button"
              size="sm"
              variant={id === source ? 'default' : 'outline'}
              aria-pressed={id === source}
              onClick={() => show({ source: id, page: 1 })}
            >
              {t(`tagAdmin.sources.${id}`)}
            </Button>
          ))}
        </div>
        <Tabs value={status} onValueChange={(value) => show({ status: String(value), page: 1 })}>
          <TabsList aria-label={t('tagAdmin.status')}>
            {statuses.map((id) => (
              <TabsTrigger key={id} value={id}>
                {t(`tagAdmin.statuses.${id}`)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>
      {error && <Notice>{error}</Notice>}
      {answer && answer.items.length === 0 && (
        <p className="py-10 text-center text-muted-foreground">{t('tagAdmin.empty')}</p>
      )}
      <ul className="space-y-3">
        {answer?.items.map((group) => {
          const key = `${group.chart.id}/${group.tag}`
          const text = chartText(group.chart, i18n.resolvedLanguage)
          return (
            <li
              key={key}
              className="flex flex-col gap-3 rounded-[20px] bg-card p-4 shadow-elev-1 sm:flex-row sm:items-center"
            >
              <div className="min-w-0 flex-1 space-y-2">
                <div className="flex flex-wrap items-center gap-2">
                  <SourceTag source={source} />
                  <a
                    className="font-bold underline-offset-2 hover:underline wrap-anywhere"
                    href={songPage(source, group.chart.id)}
                  >
                    {text.title || t('tagAdmin.unknownChart')}
                  </a>
                  {text.subtitle && (
                    <span className="truncate text-xs text-muted-foreground">{text.subtitle}</span>
                  )}
                </div>
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <Badge className="font-semibold"># {name(group.tag)}</Badge>
                  <span className="text-muted-foreground">
                    {t('tagAdmin.suggestedBy', { count: group.count })}
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
                {group.chart.tags.length > 0 && (
                  <p className="text-xs text-muted-foreground">
                    {t('tagAdmin.current', {
                      tags: group.chart.tags.map(name).join(' · '),
                    })}
                  </p>
                )}
                <p className="text-xs text-muted-foreground tabular-nums">
                  {group.decidedAt
                    ? t('tagAdmin.decided', {
                        at: when(group.decidedAt),
                        by: group.decidedBy?.nickname ?? t('tagAdmin.earlier'),
                      })
                    : t('tagAdmin.since', { at: when(group.firstAt) })}
                </p>
              </div>
              {status === 'pending' && (
                <div className="flex shrink-0 gap-2">
                  <Button
                    type="button"
                    size="sm"
                    disabled={busy !== ''}
                    onClick={() => decide(group, true)}
                  >
                    {t('tagAdmin.accept')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={busy !== ''}
                    className={cn(busy === key && 'opacity-60')}
                    onClick={() => decide(group, false)}
                  >
                    {t('tagAdmin.reject')}
                  </Button>
                </div>
              )}
            </li>
          )
        })}
      </ul>
      {pages > 1 && (
        <nav className="flex items-center justify-center gap-3" aria-label={t('tagAdmin.pages')}>
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
