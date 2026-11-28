// Tags: what a song is beside its categories (東方Project, BMS…). The catalog is the site's, the
// same on both sources, and the players put the tags on: a chart has a tag while somebody's vote
// for it stands. A signed-in player adds a tag (one of the catalog, or a new name, which makes a
// pending tag), takes their own vote back, and reports a tag that does not belong; the source's
// managers decide the reports on the players' page 标签管理 (/manage/tags).
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, type QueryFunctionContext } from '@tanstack/react-query'
import { FlagIcon, HashIcon, PlusIcon, TagIcon } from '@phosphor-icons/react'
import { api, jsonRequest } from './api/client'
import { endpoints } from './api/endpoints'
import type { ChartBase, ChartTag, ChartTagState, Tag, TagList } from './api/types'
import { Badge } from './components/ui/badge'
import { Button } from './components/ui/button'
import { DialogClose } from './components/ui/dialog'
import { Input } from './components/ui/input'
import { normalizeLanguage } from './i18n'
import { cn } from './lib/utils'
import { Modal, Notice } from './notifications'
import { queryClient } from './query/client'
import { chartWritten } from './queries/chart-writes'
import { useSession } from './session'
import { SOURCE } from './source'

/** A tag name's length in characters; the server checks the same. */
export const maxTagName = 24
/** A report's reason in characters; the server checks the same. */
export const maxTagReason = 500

// The address is taken while the page of its source is shown: the source may change meanwhile.
const read =
  <T,>(address: string) =>
  ({ signal }: QueryFunctionContext) =>
    api<T>(address, { signal })

const tagsKey = () => [SOURCE, 'tags'] as const
const chartTagsKey = (id: string) => [SOURCE, 'chart-tags', id] as const

/** The tag catalog, with how many charts of the source being shown have each tag. */
export function useTags(enabled = true) {
  const { data, error } = useQuery({
    queryKey: tagsKey(),
    queryFn: read<TagList>(endpoints.tags),
    staleTime: 5 * 60_000,
    enabled,
  })
  return { items: data?.items ?? null, error: data ? '' : (error?.message ?? '') }
}

type Named = Pick<Tag, 'id' | 'names'>

/** A tag's name in the visitor's language, else in English, else its id. */
export function tagName(tag: Named | undefined, language: string, id = '') {
  return tag?.names[normalizeLanguage(language)] || tag?.names.en || tag?.id || id
}

const folded = (text: string) => text.trim().toLowerCase()

/** The tag a typed name is: its id or its name in any language, whatever the case. */
export function tagNamed<T extends Named>(tags: readonly T[], typed: string): T | undefined {
  const wanted = folded(typed)
  if (!wanted) return undefined
  return tags.find(
    (tag) => tag.id === wanted || Object.values(tag.names).some((name) => folded(name) === wanted),
  )
}

/** The tags a typed text finds: those with it in a name of any language. All for no text. */
export function tagsMatching<T extends Named>(tags: readonly T[], typed: string): T[] {
  const wanted = folded(typed)
  if (!wanted) return [...tags]
  return tags.filter((tag) =>
    Object.values(tag.names).some((name) => folded(name).includes(wanted)),
  )
}

/**
 * The picker's rows: the chart's own tags first, the most voted first, then the rest of the
 * catalog, official before pending and the most used first.
 */
export function pickerRows(catalog: readonly Tag[], onChart: readonly ChartTag[]) {
  const there = new Map(onChart.map((tag) => [tag.id, tag]))
  const counts = new Map(catalog.map((tag) => [tag.id, tag.chartCount]))
  const rest = catalog
    .filter((tag) => !there.has(tag.id))
    .sort(
      (a, b) =>
        Number(b.status === 'official') - Number(a.status === 'official') ||
        b.chartCount - a.chartCount,
    )
  return [
    ...onChart.map((tag) => ({ ...tag, chartCount: counts.get(tag.id) ?? 1 })),
    ...rest.map((tag) => ({ ...tag, count: 0, mine: false })),
  ]
}

/** A tag as a label: its name, a mark while it is pending, and what follows (a count). */
export function TagBadge({
  tag,
  children,
  className,
}: {
  tag: Pick<Tag, 'id' | 'names' | 'status'>
  children?: React.ReactNode
  className?: string
}) {
  const { t, i18n } = useTranslation()
  const pending = tag.status === 'pending'
  return (
    <Badge
      variant={pending ? 'outline' : 'secondary'}
      className={cn('gap-1', pending && 'border-dashed', className)}
      title={pending ? t('tags.pendingHint') : undefined}
    >
      <HashIcon aria-hidden="true" className="size-3 opacity-60" />
      <span className="wrap-anywhere">{tagName(tag, i18n.language)}</span>
      {pending && <span className="text-[10px] text-muted-foreground">{t('tags.pending')}</span>}
      {children}
    </Badge>
  )
}

/** A chart's tags under its categories with their votes, and the way to add to them. */
export function ChartTags({ chart }: { chart: Pick<ChartBase, 'id'> }) {
  const { t } = useTranslation()
  const session = useSession()
  const [open, setOpen] = useState(false)
  const state = useQuery({
    queryKey: chartTagsKey(chart.id),
    queryFn: read<ChartTagState>(endpoints.chartTags(chart.id)),
  })
  const tags = state.data?.tags ?? []
  if (tags.length === 0 && !session.user) return null
  return (
    <div className="flex flex-wrap items-center gap-2">
      {tags.length > 0 && (
        <ul aria-label={t('tags.heading')} className="flex flex-wrap items-center gap-2">
          {tags.map((tag) => (
            <li key={tag.id}>
              <TagBadge tag={tag}>
                <span
                  className="text-xs text-muted-foreground tabular-nums"
                  aria-label={t('tags.votes', { count: tag.count })}
                >
                  {tag.count}
                </span>
              </TagBadge>
            </li>
          ))}
        </ul>
      )}
      {session.user && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-6 gap-1 px-2 text-xs text-muted-foreground"
          onClick={() => setOpen(true)}
        >
          <TagIcon aria-hidden="true" className="size-3.5" />
          {t('tags.open')}
        </Button>
      )}
      {open && state.data && (
        <TagDialog chartId={chart.id} state={state.data} onDismiss={() => setOpen(false)} />
      )}
    </div>
  )
}

/**
 * Every tag there is with how many songs have it, for a signed-in player to add to the chart or
 * take their own vote back, to name a new one, and to report one that is on the chart.
 */
function TagDialog({
  chartId,
  state,
  onDismiss,
}: {
  chartId: string
  state: ChartTagState
  onDismiss: () => void
}) {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const { items, error: catalogError } = useTags()
  const [typed, setTyped] = useState('')
  // The tag whose report is being written, and its reason.
  const [reporting, setReporting] = useState('')
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')

  const run = async (write: () => Promise<unknown>) => {
    setBusy(true)
    setMessage('')
    try {
      await write()
      // The chart names its official tags, and the catalog counts its charts.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: chartTagsKey(chartId) }),
        queryClient.invalidateQueries({ queryKey: tagsKey() }),
        chartWritten(chartId),
      ])
      return true
    } catch (e) {
      setMessage((e as Error).message)
      return false
    } finally {
      setBusy(false)
    }
  }
  const vote = (tag: string, value: 0 | 1) =>
    run(() =>
      api(endpoints.chartTag(chartId, tag), jsonRequest('PUT', { vote: value }, session.csrfToken)),
    )
  const create = async () => {
    const done = await run(() =>
      api(
        endpoints.chartTags(chartId),
        jsonRequest('POST', { name: typed.trim() }, session.csrfToken),
      ),
    )
    if (done) setTyped('')
  }
  const report = async () => {
    const done = await run(() =>
      api(
        endpoints.chartTagReports(chartId, reporting),
        jsonRequest('POST', { reason: reason.trim() }, session.csrfToken),
      ),
    )
    if (done) {
      setReporting('')
      setReason('')
    }
  }

  const rows = items ? pickerRows(items, state.tags) : null
  const shown = rows ? tagsMatching(rows, typed) : []
  const name = typed.trim()
  // A name no tag has yet makes a new one.
  const fresh = !!rows && !!name && [...name].length <= maxTagName && !tagNamed(rows, name)
  const error = message || catalogError
  return (
    <Modal
      title={t('tags.title')}
      busy={busy}
      onDismiss={onDismiss}
      actions={
        <DialogClose
          render={<Button type="button" variant="outline" size="default" />}
          disabled={busy}
        >
          {t('tags.close')}
        </DialogClose>
      }
    >
      <p className="text-sm text-muted-foreground">{t('tags.hint')}</p>
      <Input
        value={typed}
        maxLength={maxTagName * 2}
        placeholder={t('tags.find')}
        aria-label={t('tags.find')}
        onChange={(event) => setTyped(event.target.value)}
      />
      {error && <Notice>{error}</Notice>}
      {!rows ? (
        !error && (
          <p role="status" className="text-sm text-muted-foreground">
            {t('tags.loading')}
          </p>
        )
      ) : (
        <ul className="max-h-[50dvh] divide-y overflow-y-auto">
          {fresh && (
            <li className="flex min-h-11 items-center justify-between gap-3 py-1.5">
              <span className="min-w-0 text-sm">
                <span className="font-medium wrap-anywhere">{name}</span>
                <span className="block text-xs text-muted-foreground">
                  {t(state.canReview ? 'tags.newOfficialHint' : 'tags.newHint')}
                </span>
              </span>
              <Button type="button" size="sm" disabled={busy} onClick={create}>
                <PlusIcon aria-hidden="true" />
                {t('tags.create')}
              </Button>
            </li>
          )}
          {shown.map((tag) => (
            <li key={tag.id} className="py-1.5">
              <div className="flex min-h-8 items-center justify-between gap-3">
                <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                  <TagBadge tag={tag} />
                  <span className="text-xs text-muted-foreground tabular-nums">
                    {t('tags.songs', { count: tag.chartCount })}
                    {tag.count > 0 && ` · ${t('tags.votesHere', { count: tag.count })}`}
                  </span>
                </span>
                <span className="flex shrink-0 items-center gap-1">
                  {tag.count > 0 &&
                    (state.reported.includes(tag.id) ? (
                      <span className="px-2 text-xs text-muted-foreground">
                        {t('tags.reported')}
                      </span>
                    ) : (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={busy}
                        aria-label={t('tags.reportOf', { name: tagName(tag, i18n.language) })}
                        title={t('tags.report')}
                        onClick={() => {
                          setReporting(reporting === tag.id ? '' : tag.id)
                          setReason('')
                        }}
                      >
                        <FlagIcon aria-hidden="true" />
                      </Button>
                    ))}
                  {tag.mine ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={busy}
                      onClick={() => vote(tag.id, 0)}
                    >
                      {t('tags.remove')}
                    </Button>
                  ) : (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={busy}
                      onClick={() => vote(tag.id, 1)}
                    >
                      {t('tags.add')}
                    </Button>
                  )}
                </span>
              </div>
              {reporting === tag.id && (
                <form
                  className="mt-2 flex items-center gap-2"
                  onSubmit={(event) => {
                    event.preventDefault()
                    void report()
                  }}
                >
                  <Input
                    autoFocus
                    value={reason}
                    maxLength={maxTagReason}
                    placeholder={t('tags.reason')}
                    aria-label={t('tags.reason')}
                    onChange={(event) => setReason(event.target.value)}
                  />
                  <Button type="submit" size="sm" disabled={busy}>
                    {t('tags.sendReport')}
                  </Button>
                </form>
              )}
            </li>
          ))}
          {shown.length === 0 && !fresh && (
            <li className="py-6 text-center text-sm text-muted-foreground">{t('tags.none')}</li>
          )}
        </ul>
      )}
    </Modal>
  )
}
