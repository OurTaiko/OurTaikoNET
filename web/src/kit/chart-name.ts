import { queryOptions, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api } from './api/client'
import { SITE_API_ROOT } from './api/config'
import type { ChartBase } from './api/types'
import { chartText } from './chart-language'
import type { SourceId } from './source'

/**
 * A chart of a source by its ID, for what knows only which song it is about (a comment, a
 * notice): its names are the chart's own, in every language. The key is the chart page's
 * (chartKeys.detail), so the two share what they read; kept a while, as a list may name the
 * same song many times.
 */
export const chartNameQuery = (source: SourceId, id: string) =>
  queryOptions({
    queryKey: [source, 'charts', 'detail', id, 'id'] as const,
    queryFn: ({ signal }) =>
      api<ChartBase>(`${SITE_API_ROOT}/sources/${source}/charts/${encodeURIComponent(id)}?by=id`, {
        signal,
      }),
    staleTime: 10 * 60_000,
    retry: false,
  })

/**
 * The song's name in the visitor's language (chartText): null while its chart is being read,
 * a stand-in when it cannot be, and '' without a chart.
 */
export function useChartTitle(source: SourceId, id: string | undefined): string | null {
  const { t, i18n } = useTranslation()
  const chart = useQuery({ ...chartNameQuery(source, id ?? ''), enabled: !!id })
  if (!id) return ''
  if (chart.data) return chartText(chart.data, i18n.resolvedLanguage).title
  return chart.isError ? t('messages.unknownSong') : null
}
