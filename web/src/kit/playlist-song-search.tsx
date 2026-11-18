import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import type { ChartBase, ChartList } from './api/types'
import { chartText } from './chart-language'
import { SearchInput } from './components/search-input'
import { ToggleRow } from './components/toggle-row'
import { Button } from './components/ui/button'
import { cn } from './lib/utils'
import { Modal } from './notifications'
import { chartQueries } from './queries/charts'

// Searches every public song for the owner of a playlist. Each result shows
// whether the playlist holds it; selecting one adds it or takes it out.
/** C is the site's Chart: the search answers with whole charts, and onToggle hands one back. */
export function PlaylistSongSearchDialog<C extends ChartBase>({
  charts,
  disabled,
  onToggle,
  onDismiss,
}: {
  /** The playlist's current songs. */
  charts: C[]
  /** A change is being saved. */
  disabled: boolean
  onToggle: (chart: C, listed: boolean) => void
  onDismiss: () => void
}) {
  const { t, i18n } = useTranslation()
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const [pending, setPending] = useState(false)
  const search = useCallback((value: string) => {
    setQuery(value)
    setPage(1)
  }, [])
  const list = useQuery(chartQueries.list({ q: query, page }))
  const result = list.data as ChartList<C> | undefined
  const error = result ? '' : (list.error?.message ?? '')
  const listed = new Set(charts.map((chart) => chart.id))
  return (
    <Modal title={t('playlists.addSongs')} onDismiss={onDismiss}>
      <SearchInput value={query} onSearch={search} onPendingChange={setPending} />
      <p className="text-xs text-muted-foreground" role="status" aria-live="polite">
        {t('playlists.songCount', { count: charts.length })}
      </p>
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
      {!result && !error && <p role="status">{t('messages.loadingCharts')}</p>}
      {result && (
        <div
          className={cn(
            'max-h-80 space-y-2 overflow-y-auto transition-opacity',
            list.isPlaceholderData && 'opacity-60',
          )}
          role="group"
          aria-label={t('messages.searchCharts')}
        >
          {result.items.length === 0 && (
            <p className="text-muted-foreground">{t('messages.noMatchingCharts')}</p>
          )}
          {result.items.map((chart) => {
            const text = chartText(chart, i18n.resolvedLanguage)
            return (
              <ToggleRow
                key={chart.id}
                title={text.title}
                detail={[text.subtitle, chart.maker || chart.author].filter(Boolean).join(' · ')}
                pressed={listed.has(chart.id)}
                pressedLabel={t('playlists.inPlaylist')}
                idleLabel={
                  <>
                    <PlusIcon className="size-3.5 shrink-0" aria-hidden="true" />
                    {t('playlists.add')}
                  </>
                }
                disabled={disabled || pending}
                onToggle={() => onToggle(chart, listed.has(chart.id))}
              />
            )
          })}
        </div>
      )}
      {result && result.total > result.pageSize && (
        <div className="flex items-center justify-between gap-3">
          <Button
            variant="outline"
            disabled={page <= 1 || pending}
            onClick={() => setPage(page - 1)}
          >
            {t('messages.previous')}
          </Button>
          <span className="text-xs text-muted-foreground">
            {t('messages.pagination', { page, pages: Math.ceil(result.total / result.pageSize) })}
          </span>
          <Button
            variant="outline"
            disabled={page * result.pageSize >= result.total || pending}
            onClick={() => setPage(page + 1)}
          >
            {t('messages.next')}
          </Button>
        </div>
      )}
    </Modal>
  )
}
