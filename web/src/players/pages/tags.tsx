import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, jsonRequest } from '@players/api/client'
import { sourceApi, songPage } from '@players/components/song-tile'
import { ChoiceSelect } from '@players/components/choice-select'
import { Notice } from '@players/notice'
import { useSession } from '@players/session'
import type { Tag, TagList } from '@sitekit/api/types'
import { chartText } from '@sitekit/chart-language'
import { Button } from '@sitekit/components/ui/button'
import { Input } from '@sitekit/components/ui/input'
import { Label } from '@sitekit/components/ui/label'
import { Tabs, TabsList, TabsTrigger } from '@sitekit/components/ui/tabs'
import { UserAvatar } from '@sitekit/components/user-avatar'
import { formatLocale, languageNames, type Language } from '@sitekit/i18n'
import { Modal } from '@sitekit/notifications'
import { useNotification } from '@sitekit/notification-context'
import { queryClient } from '@sitekit/query/client'
import { sources, type SourceId } from '@sitekit/source'
import { TagBadge, maxTagName, tagName } from '@sitekit/tags'

type Person = { id: string; nickname: string; avatarPath: string }
type ReportStatus = 'pending' | 'upheld' | 'dismissed'
/** Every report of one tag on one chart (GET /tag-reports). */
type ReportGroup = {
  chart: {
    id: string
    title: string
    subtitle: string
    titleTranslations: Record<string, string>
    subtitleTranslations: Record<string, string>
  }
  tag: Tag
  status: ReportStatus
  count: number
  reporters: (Person & { reason: string })[]
  votes: number
  firstAt: string
  decidedAt: string | null
  decidedBy: Person | null
}
type ReportPage = { items: ReportGroup[]; total: number; page: number; pageSize: number }

// Who manages a source decides the reports of its charts' tags, and keeps the catalog.
const managerOf: Record<SourceId, string> = {
  fanmade: 'FANMADE_MANAGER',
  official: 'OFFICIAL_MANAGER',
}
const statuses: ReportStatus[] = ['pending', 'upheld', 'dismissed']
// en first: it is the name a tag must have.
const languages: Language[] = ['en', 'zh-Hans', 'ja', 'ko']
const reportsKey = ['tag-admin', 'reports'] as const
const catalogKey = ['tag-admin', 'catalog'] as const
/** Everything of tags the site has read: these pages' and each source's own. */
const refreshTags = () =>
  Promise.all([
    queryClient.invalidateQueries({ queryKey: ['tag-admin'] }),
    ...sources.flatMap((source) => [
      queryClient.invalidateQueries({ queryKey: [source.id, 'tags'] }),
      queryClient.invalidateQueries({ queryKey: [source.id, 'chart-tags'] }),
    ]),
  ])

// 标签管理: the reports players make of tags that do not belong on a song, for the managers of
// that song's source to uphold (the tag leaves the song for good) or dismiss; and the catalog of
// tags, the same for both sources, which any source's manager keeps.
export function TagsPage() {
  const { t } = useTranslation()
  const session = useSession()
  const managed = sources.filter((source) => session.user?.groups?.includes(managerOf[source.id]))
  const [params, setParams] = useSearchParams()
  if (session.loading) return null
  if (managed.length === 0) return <Navigate to="/" replace />
  const source = managed.find((entry) => entry.id === params.get('source'))?.id ?? managed[0].id
  return (
    <div className="space-y-8">
      <header className="space-y-3">
        <div className="space-y-1">
          <h1 className="text-2xl font-extrabold tracking-tight sm:text-3xl">
            {t('tagAdmin.heading')}
          </h1>
          <p className="text-sm text-muted-foreground">{t('tagAdmin.intro')}</p>
        </div>
        {managed.length > 1 && (
          <Tabs
            value={source}
            onValueChange={(value) => setParams({ source: String(value) }, { replace: true })}
          >
            <TabsList aria-label={t('tagAdmin.source')}>
              {managed.map((entry) => (
                <TabsTrigger key={entry.id} value={entry.id}>
                  {t(`shell.${entry.id}`)}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        )}
      </header>
      <Reports key={`reports-${source}`} source={source} />
      <Catalog key={`catalog-${source}`} source={source} />
    </div>
  )
}

function Reports({ source }: { source: SourceId }) {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const [status, setStatus] = useState<ReportStatus>('pending')
  const [page, setPage] = useState(1)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const queue = useQuery({
    queryKey: [...reportsKey, source, status, page],
    queryFn: ({ signal }) =>
      api<ReportPage>(`${sourceApi(source)}/tag-reports?status=${status}&page=${page}`, {
        signal,
      }),
  })
  const when = (at: string) =>
    new Date(at).toLocaleString(formatLocale(i18n.language), {
      dateStyle: 'medium',
      timeStyle: 'short',
    })
  const decide = async (group: ReportGroup, uphold: boolean) => {
    setBusy(true)
    setMessage('')
    try {
      await api(
        `${sourceApi(source)}/tag-reports/decision`,
        jsonRequest(
          'POST',
          { chartId: group.chart.id, tag: group.tag.id, uphold },
          session.csrfToken,
        ),
      )
      await refreshTags()
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
    <section className="space-y-4" aria-labelledby="tag-reports">
      <h2 id="tag-reports" className="text-lg font-extrabold">
        {t('tagAdmin.reports')}
      </h2>
      <Tabs
        value={status}
        onValueChange={(value) => {
          setStatus(value as ReportStatus)
          setPage(1)
        }}
      >
        <TabsList aria-label={t('tagAdmin.status')}>
          {statuses.map((id) => (
            <TabsTrigger key={id} value={id}>
              {t(`tagAdmin.statuses.${id}`)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      {error && <Notice>{error}</Notice>}
      {answer && answer.items.length === 0 && (
        <p className="py-8 text-center text-muted-foreground">{t('tagAdmin.noReports')}</p>
      )}
      <ul className="space-y-3">
        {answer?.items.map((group) => {
          const text = chartText(group.chart, i18n.resolvedLanguage)
          return (
            <li
              key={`${group.chart.id}/${group.tag.id}`}
              className="flex flex-col gap-3 rounded-[20px] bg-card p-4 shadow-elev-1 sm:flex-row sm:items-center"
            >
              <div className="min-w-0 flex-1 space-y-2">
                <div className="flex flex-wrap items-center gap-2">
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
                  <TagBadge tag={group.tag} />
                  <span className="text-muted-foreground">
                    {t('tagAdmin.reportedBy', { count: group.count })}
                    {' · '}
                    {t('tagAdmin.votes', { count: group.votes })}
                  </span>
                </div>
                <ul className="space-y-1">
                  {group.reporters.map((person) => (
                    <li key={person.id} className="flex items-start gap-2 text-sm">
                      <UserAvatar
                        nickname={person.nickname}
                        avatarPath={person.avatarPath}
                        className="size-5"
                      />
                      <span className="min-w-0 wrap-anywhere">
                        <span className="font-medium">{person.nickname}</span>
                        {person.reason && (
                          <span className="text-muted-foreground">：{person.reason}</span>
                        )}
                      </span>
                    </li>
                  ))}
                </ul>
                <p className="text-xs text-muted-foreground tabular-nums">
                  {group.decidedAt
                    ? t('tagAdmin.decided', {
                        at: when(group.decidedAt),
                        by: group.decidedBy?.nickname ?? t('tagAdmin.someone'),
                      })
                    : t('tagAdmin.since', { at: when(group.firstAt) })}
                </p>
              </div>
              {status === 'pending' && (
                <div className="flex shrink-0 gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="destructive"
                    disabled={busy}
                    onClick={() => decide(group, true)}
                  >
                    {t('tagAdmin.uphold')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => decide(group, false)}
                  >
                    {t('tagAdmin.dismiss')}
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
            onClick={() => setPage(page - 1)}
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
            onClick={() => setPage(page + 1)}
          >
            {t('scores.next')}
          </Button>
        </nav>
      )}
    </section>
  )
}

type Names = Partial<Record<Language, string>>

/** The fields of a tag's names, one a language; en is required. */
function NameFields({
  names,
  disabled,
  onChange,
}: {
  names: Names
  disabled: boolean
  onChange: (names: Names) => void
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {languages.map((language) => (
        <Label key={language} className="grid gap-1 text-xs text-muted-foreground">
          {languageNames[language]}
          <Input
            value={names[language] ?? ''}
            required={language === 'en'}
            maxLength={maxTagName}
            disabled={disabled}
            onChange={(event) => onChange({ ...names, [language]: event.target.value })}
          />
        </Label>
      ))}
    </div>
  )
}

function Catalog({ source }: { source: SourceId }) {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const { confirm } = useNotification()
  const catalog = useQuery({
    queryKey: [...catalogKey, source],
    queryFn: ({ signal }) => api<TagList>(`${sourceApi(source)}/tags`, { signal }),
  })
  const [draft, setDraft] = useState<Names>({})
  const [editing, setEditing] = useState<Tag | null>(null)
  const [merging, setMerging] = useState<Tag | null>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')

  const write = async (address: string, method: string, body: unknown) => {
    setBusy(true)
    setMessage('')
    try {
      await api(address, jsonRequest(method, body, session.csrfToken))
      await refreshTags()
      return true
    } catch (e) {
      setMessage((e as Error).message)
      return false
    } finally {
      setBusy(false)
    }
  }
  const address = (tag: Tag) => `${sourceApi(source)}/tags/${encodeURIComponent(tag.id)}`
  const remove = async (tag: Tag) => {
    const yes = await confirm({
      message: t('tagAdmin.confirmDelete', { name: tagName(tag, i18n.language) }),
      confirmLabel: t('tagAdmin.delete'),
      destructive: true,
    })
    if (yes) void write(address(tag), 'DELETE', {})
  }
  const items = catalog.data?.items
  const error = message || (items ? '' : (catalog.error?.message ?? ''))
  return (
    <section className="space-y-4" aria-labelledby="tag-catalog">
      <div className="space-y-1">
        <h2 id="tag-catalog" className="text-lg font-extrabold">
          {t('tagAdmin.catalog')}
        </h2>
        <p className="text-sm text-muted-foreground">{t('tagAdmin.catalogIntro')}</p>
      </div>
      {error && <Notice>{error}</Notice>}
      <form
        className="space-y-3 rounded-[20px] bg-card p-4 shadow-elev-1"
        onSubmit={async (event) => {
          event.preventDefault()
          if (await write(`${sourceApi(source)}/tags`, 'POST', { names: draft })) setDraft({})
        }}
      >
        <h3 className="text-sm font-bold">{t('tagAdmin.newTag')}</h3>
        <NameFields names={draft} disabled={busy} onChange={setDraft} />
        <Button type="submit" size="sm" disabled={busy || !draft.en?.trim()}>
          {t('tagAdmin.create')}
        </Button>
      </form>
      <ul className="divide-y rounded-[20px] bg-card px-4 shadow-elev-1">
        {items?.map((tag) => (
          <li key={tag.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
            <span className="flex min-w-0 flex-wrap items-center gap-2">
              <TagBadge tag={tag} />
              <span className="text-xs text-muted-foreground tabular-nums">
                {t('tagAdmin.songs', { count: tag.chartCount })}
              </span>
            </span>
            <span className="flex shrink-0 flex-wrap gap-1">
              {tag.status === 'pending' && (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => write(address(tag), 'PATCH', { official: true })}
                >
                  {t('tagAdmin.makeOfficial')}
                </Button>
              )}
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => setEditing(tag)}
              >
                {t('tagAdmin.rename')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => setMerging(tag)}
              >
                {t('tagAdmin.merge')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => remove(tag)}
              >
                {t('tagAdmin.delete')}
              </Button>
            </span>
          </li>
        ))}
      </ul>
      {editing && (
        <RenameDialog
          tag={editing}
          busy={busy}
          onDismiss={() => setEditing(null)}
          onSave={async (names) => {
            if (await write(address(editing), 'PATCH', { names })) setEditing(null)
          }}
        />
      )}
      {merging && items && (
        <MergeDialog
          tag={merging}
          others={items.filter((tag) => tag.id !== merging.id)}
          busy={busy}
          onDismiss={() => setMerging(null)}
          onMerge={async (into) => {
            if (await write(`${address(merging)}/merge`, 'POST', { into })) setMerging(null)
          }}
        />
      )}
    </section>
  )
}

function RenameDialog({
  tag,
  busy,
  onDismiss,
  onSave,
}: {
  tag: Tag
  busy: boolean
  onDismiss: () => void
  onSave: (names: Names) => void
}) {
  const { t } = useTranslation()
  const [names, setNames] = useState<Names>(tag.names)
  return (
    <Modal
      title={t('tagAdmin.rename')}
      busy={busy}
      onDismiss={onDismiss}
      actions={
        <>
          <Button type="button" variant="outline" disabled={busy} onClick={onDismiss}>
            {t('tagAdmin.cancel')}
          </Button>
          <Button type="button" disabled={busy || !names.en?.trim()} onClick={() => onSave(names)}>
            {t('tagAdmin.save')}
          </Button>
        </>
      }
    >
      <p className="text-sm text-muted-foreground">{t('tagAdmin.renameHint')}</p>
      <NameFields names={names} disabled={busy} onChange={setNames} />
    </Modal>
  )
}

function MergeDialog({
  tag,
  others,
  busy,
  onDismiss,
  onMerge,
}: {
  tag: Tag
  others: Tag[]
  busy: boolean
  onDismiss: () => void
  onMerge: (into: string) => void
}) {
  const { t, i18n } = useTranslation()
  const [into, setInto] = useState('')
  return (
    <Modal
      title={t('tagAdmin.mergeTitle', { name: tagName(tag, i18n.language) })}
      busy={busy}
      onDismiss={onDismiss}
      actions={
        <>
          <Button type="button" variant="outline" disabled={busy} onClick={onDismiss}>
            {t('tagAdmin.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={busy || !into}
            onClick={() => onMerge(into)}
          >
            {t('tagAdmin.merge')}
          </Button>
        </>
      }
    >
      <p className="text-sm text-muted-foreground">{t('tagAdmin.mergeHint')}</p>
      <ChoiceSelect
        label={t('tagAdmin.mergeInto')}
        value={into}
        items={[
          { value: '', label: t('tagAdmin.choose') },
          ...others.map((other) => ({ value: other.id, label: tagName(other, i18n.language) })),
        ]}
        onValueChange={setInto}
      />
    </Modal>
  )
}
