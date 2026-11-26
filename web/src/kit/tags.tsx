// Tags: what a song is beside its categories (東方Project, BMS…). The catalog is the site's, the
// same on both sources; a chart's tags are set by a tag manager or the source's manager, and
// anyone else signed in suggests them for a tag manager to accept or reject.
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, type QueryFunctionContext } from '@tanstack/react-query'
import { CheckIcon, HashIcon, TagIcon } from '@phosphor-icons/react'
import { api, jsonRequest } from './api/client'
import { endpoints } from './api/endpoints'
import type { ChartBase, ChartTagState, Tag, TagList, TagSuggestionStatus } from './api/types'
import { Badge } from './components/ui/badge'
import { Button } from './components/ui/button'
import { Checkbox } from './components/ui/checkbox'
import { DialogClose } from './components/ui/dialog'
import { Label } from './components/ui/label'
import { normalizeLanguage } from './i18n'
import { Modal, Notice } from './notifications'
import { queryClient } from './query/client'
import { chartWritten } from './queries/chart-writes'
import { useSession } from './session'
import { SOURCE } from './source'

// The address is taken while the page of its source is shown: the source may change meanwhile.
const read =
  <T,>(address: string) =>
  ({ signal }: QueryFunctionContext) =>
    api<T>(address, { signal })

/** The tag catalog, with how many charts of the source being shown have each tag. */
export function useTags() {
  const { data, error } = useQuery({
    queryKey: [SOURCE, 'tags'],
    queryFn: read<TagList>(endpoints.tags),
    staleTime: 5 * 60_000,
  })
  return { items: data?.items ?? null, error: data ? '' : (error?.message ?? '') }
}

/** A tag's name in the visitor's language, else in English, else its id. */
export function tagName(tag: Pick<Tag, 'id' | 'names'> | undefined, language: string, id = '') {
  return tag?.names[normalizeLanguage(language)] || tag?.names.en || tag?.id || id
}

const chartTagsKey = (id: string) => [SOURCE, 'chart-tags', id] as const

/** A chart's tags under its title, and, for who is signed in, the way to set or suggest them. */
export function ChartTags({ chart }: { chart: Pick<ChartBase, 'id' | 'tags'> }) {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const { items } = useTags()
  const [open, setOpen] = useState(false)
  const ids = chart.tags ?? []
  if (ids.length === 0 && !session.user) return null
  return (
    <div className="flex flex-wrap items-center gap-2">
      {ids.length > 0 && (
        <ul aria-label={t('tags.heading')} className="flex flex-wrap items-center gap-2">
          {ids.map((id) => (
            <li key={id}>
              <Badge variant="secondary" className="gap-1">
                <HashIcon aria-hidden="true" className="size-3 opacity-60" />
                {tagName(
                  items?.find((tag) => tag.id === id),
                  i18n.language,
                  id,
                )}
              </Badge>
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
          {t(ids.length ? 'tags.manage' : 'tags.add')}
        </Button>
      )}
      {open && <TagDialog chartId={chart.id} onDismiss={() => setOpen(false)} />}
    </div>
  )
}

/**
 * The tags of a chart for whoever is signed in: a tag manager or the source's manager ticks
 * them and saves; anyone else suggests the ones it lacks and sees what became of them.
 */
function TagDialog({ chartId, onDismiss }: { chartId: string; onDismiss: () => void }) {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const { items, error: catalogError } = useTags()
  const state = useQuery({
    queryKey: chartTagsKey(chartId),
    queryFn: read<ChartTagState>(endpoints.chartTags(chartId)),
    staleTime: 0,
  })
  const [picked, setPicked] = useState<string[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const data = state.data
  const error = message || catalogError || (data ? '' : (state.error?.message ?? ''))

  const run = async (write: () => Promise<unknown>) => {
    setBusy(true)
    setMessage('')
    try {
      await write()
      await queryClient.invalidateQueries({ queryKey: chartTagsKey(chartId) })
      return true
    } catch (e) {
      setMessage((e as Error).message)
      return false
    } finally {
      setBusy(false)
    }
  }
  const save = async () => {
    const tags = picked ?? data?.tags ?? []
    const done = await run(() =>
      api(endpoints.chartTags(chartId), jsonRequest('PUT', { tags }, session.csrfToken)),
    )
    if (done) {
      await Promise.all([
        chartWritten(chartId),
        queryClient.invalidateQueries({ queryKey: [SOURCE, 'tags'] }),
      ])
      onDismiss()
    }
  }
  const suggest = (tag: string) =>
    run(() =>
      api(endpoints.tagSuggestions(chartId), jsonRequest('POST', { tag }, session.csrfToken)),
    )
  const withdraw = (tag: string) =>
    run(() =>
      api(endpoints.tagSuggestion(chartId, tag), jsonRequest('DELETE', {}, session.csrfToken)),
    )

  const statusText: Record<TagSuggestionStatus, string> = {
    pending: t('tags.pending'),
    accepted: t('tags.accepted'),
    rejected: t('tags.rejected'),
  }
  const editing = !!data?.canEdit
  const chosen = picked ?? data?.tags ?? []
  return (
    <Modal
      title={t(editing ? 'tags.editTitle' : 'tags.suggestTitle')}
      busy={busy}
      onDismiss={onDismiss}
      actions={
        <>
          <DialogClose
            render={<Button type="button" variant="outline" size="default" />}
            disabled={busy}
          >
            {t(editing ? 'messages.cancel' : 'tags.close')}
          </DialogClose>
          {editing && (
            <Button type="button" disabled={busy || picked === null} onClick={save}>
              {t('tags.save')}
            </Button>
          )}
        </>
      }
    >
      <p className="text-sm text-muted-foreground">
        {t(editing ? 'tags.editHint' : 'tags.suggestHint')}
      </p>
      {error && <Notice>{error}</Notice>}
      {!items || !data ? (
        !error && (
          <p role="status" className="text-sm text-muted-foreground">
            {t('tags.loading')}
          </p>
        )
      ) : editing ? (
        <div className="grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-3">
          {items.map((tag) => (
            <Label key={tag.id} className="inline-flex items-center gap-2 text-sm">
              <Checkbox
                disabled={busy}
                checked={chosen.includes(tag.id)}
                onCheckedChange={(checked) =>
                  setPicked(checked ? [...chosen, tag.id] : chosen.filter((id) => id !== tag.id))
                }
              />
              <span>{tagName(tag, i18n.language)}</span>
            </Label>
          ))}
        </div>
      ) : (
        <ul className="divide-y">
          {items.map((tag) => {
            const has = data.tags.includes(tag.id)
            const mine = data.suggestions.find((s) => s.tag === tag.id)
            return (
              <li key={tag.id} className="flex min-h-11 items-center justify-between gap-3 py-1.5">
                <span className="text-sm font-medium">{tagName(tag, i18n.language)}</span>
                {has ? (
                  <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                    <CheckIcon aria-hidden="true" className="size-3.5" />
                    {t('tags.onChart')}
                  </span>
                ) : mine?.status === 'pending' ? (
                  <span className="flex items-center gap-2">
                    <span className="text-xs text-muted-foreground">{statusText.pending}</span>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={busy}
                      onClick={() => withdraw(tag.id)}
                    >
                      {t('tags.withdraw')}
                    </Button>
                  </span>
                ) : mine ? (
                  <span className="text-xs text-muted-foreground">{statusText[mine.status]}</span>
                ) : (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={busy}
                    onClick={() => suggest(tag.id)}
                  >
                    {t('tags.suggest')}
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </Modal>
  )
}
