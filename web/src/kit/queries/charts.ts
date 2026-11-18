// What the song source being shown says of its charts. The address is taken when a query is made,
// on a page of that source: by the time the answer comes the visitor may be in another area.
import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '../api/client'
import { endpoints, type ChartQuery } from '../api/endpoints'
import type { ChartBase, ChartList } from '../api/types'
import { queryClient } from '../query/client'
import { SOURCE } from '../source'

export const chartKeys = {
  all: () => [SOURCE, 'charts'] as const,
  lists: () => [...chartKeys.all(), 'list'] as const,
  list: (query: ChartQuery, mine: boolean) => [...chartKeys.lists(), { query, mine }] as const,
  detail: (id: string, by: string) => [...chartKeys.all(), 'detail', id, by] as const,
}

export const chartQueries = {
  /** A page of the list; the page before stays on show until the next one comes. */
  list: (query: ChartQuery, mine = false) => {
    const address = endpoints.chartList(query, mine)
    return queryOptions({
      queryKey: chartKeys.list(query, mine),
      queryFn: ({ signal }) => api<ChartList>(address, { signal }),
      placeholderData: keepPreviousData,
      // Shown at once, and asked again each time: charts are published from other pages and
      // finish processing on the server later.
      refetchOnMount: 'always',
    })
  },
  detail: <C extends ChartBase>(id: string, by = 'id') => {
    const address = endpoints.chartDetail(id, by)
    return queryOptions({
      queryKey: chartKeys.detail(id, by),
      queryFn: ({ signal }) => api<C>(address, { signal }),
      refetchOnMount: 'always',
    })
  },
}

/** After a chart was published, changed or deleted: every list may show it differently now. */
export const invalidateChartLists = () =>
  queryClient.invalidateQueries({ queryKey: chartKeys.lists() })
