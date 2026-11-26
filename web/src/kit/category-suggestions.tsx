// Suggesting categories (Fanmade): a player who does not set a chart's categories themselves
// suggests the ones it should be in, for the source's managers to accept or reject on the
// players' page 乐曲管理 (/manage/songs). Owners and managers change the categories by editing
// the chart.
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { CheckIcon, LightbulbIcon } from '@phosphor-icons/react'
import { api, jsonRequest } from './api/client'
import { endpoints } from './api/endpoints'
import type { ChartBase, ChartSuggestions, SuggestionStatus } from './api/types'
import { categoryName, useCategories } from './categories'
import { Button } from './components/ui/button'
import { DialogClose } from './components/ui/dialog'
import { Modal, Notice } from './notifications'
import { queryClient } from './query/client'
import { useSession } from './session'
import { SOURCE } from './source'

/** Whether the source being shown takes suggestions of categories. */
export const takesSuggestions = () => SOURCE === 'fanmade'

const suggestionsKey = (id: string) => [SOURCE, 'category-suggestions', id] as const

/** The button that opens the suggestions, for a signed-in player who does not edit the chart. */
export function SuggestCategories({ chart }: { chart: Pick<ChartBase, 'id' | 'categoryIds'> }) {
  const { t } = useTranslation()
  const session = useSession()
  const [open, setOpen] = useState(false)
  const shown = takesSuggestions() && !!session.user
  const state = useQuery({
    queryKey: suggestionsKey(chart.id),
    queryFn: ({ signal }) =>
      api<ChartSuggestions>(endpoints.categorySuggestions(chart.id), { signal }),
    enabled: shown,
  })
  if (!shown || !state.data || state.data.canEdit) return null
  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-6 gap-1 px-2 text-xs text-muted-foreground"
        onClick={() => setOpen(true)}
      >
        <LightbulbIcon aria-hidden="true" className="size-3.5" />
        {t('categorySuggestions.open')}
      </Button>
      {open && (
        <SuggestionDialog chart={chart} state={state.data} onDismiss={() => setOpen(false)} />
      )}
    </>
  )
}

function SuggestionDialog({
  chart,
  state,
  onDismiss,
}: {
  chart: Pick<ChartBase, 'id' | 'categoryIds'>
  state: ChartSuggestions
  onDismiss: () => void
}) {
  const { t } = useTranslation()
  const session = useSession()
  const { items, error: catalogError } = useCategories()
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const run = async (write: () => Promise<unknown>) => {
    setBusy(true)
    setMessage('')
    try {
      await write()
      await queryClient.invalidateQueries({ queryKey: suggestionsKey(chart.id) })
    } catch (e) {
      setMessage((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  const suggest = (category: string) =>
    run(() =>
      api(
        endpoints.categorySuggestions(chart.id),
        jsonRequest('POST', { category }, session.csrfToken),
      ),
    )
  const withdraw = (category: string) =>
    run(() =>
      api(
        endpoints.categorySuggestion(chart.id, category),
        jsonRequest('DELETE', {}, session.csrfToken),
      ),
    )
  const statusText: Record<SuggestionStatus, string> = {
    pending: t('categorySuggestions.pending'),
    accepted: t('categorySuggestions.accepted'),
    rejected: t('categorySuggestions.rejected'),
  }
  const error = message || catalogError
  return (
    <Modal
      title={t('categorySuggestions.title')}
      busy={busy}
      onDismiss={onDismiss}
      actions={
        <DialogClose
          render={<Button type="button" variant="outline" size="default" />}
          disabled={busy}
        >
          {t('categorySuggestions.close')}
        </DialogClose>
      }
    >
      <p className="text-sm text-muted-foreground">{t('categorySuggestions.hint')}</p>
      {error && <Notice>{error}</Notice>}
      {!items ? (
        !error && (
          <p role="status" className="text-sm text-muted-foreground">
            {t('messages.loadingCategories')}
          </p>
        )
      ) : (
        <ul className="divide-y">
          {items.map((category) => {
            const mine = state.suggestions.find((s) => s.category === category.id)
            return (
              <li
                key={category.id}
                className="flex min-h-11 items-center justify-between gap-3 py-1.5"
              >
                <span className="text-sm font-medium">{categoryName(category)}</span>
                {chart.categoryIds.includes(category.id) ? (
                  <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                    <CheckIcon aria-hidden="true" className="size-3.5" />
                    {t('categorySuggestions.inCategory')}
                  </span>
                ) : mine?.status === 'pending' ? (
                  <span className="flex items-center gap-2">
                    <span className="text-xs text-muted-foreground">{statusText.pending}</span>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={busy}
                      onClick={() => withdraw(category.id)}
                    >
                      {t('categorySuggestions.withdraw')}
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
                    onClick={() => suggest(category.id)}
                  >
                    {t('categorySuggestions.suggest')}
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
