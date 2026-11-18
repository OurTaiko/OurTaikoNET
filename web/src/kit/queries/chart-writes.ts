// What pages that change a chart share: telling the cache about the change, and holding still the
// chart a form was filled from.
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { ChartBase } from '../api/types'
import { queryClient } from '../query/client'
import { chartKeys, chartQueries, invalidateChartLists } from './charts'

/**
 * After a chart was published or changed: every list may show it differently now, and so may its
 * page, however that page named it. `saved` is the chart as the server answered, when it did; a
 * chart still being processed has only its id, and a new one none yet.
 */
export async function chartWritten(id: string | undefined, saved?: ChartBase) {
  if (!id) return invalidateChartLists()
  const details = {
    queryKey: [...chartKeys.all(), 'detail'],
    predicate: ({ queryKey, state }: { queryKey: readonly unknown[]; state: { data: unknown } }) =>
      queryKey[3] === id || (state.data as ChartBase | undefined)?.id === id,
  }
  if (saved) queryClient.setQueriesData<ChartBase>(details, (known) => ({ ...known, ...saved }))
  await Promise.all([
    invalidateChartLists(),
    queryClient.invalidateQueries(details),
    queryClient.invalidateQueries({ queryKey: [...chartKeys.all(), 'file', id] }),
  ])
}

/** After a chart was deleted: its page must not be shown again from what was kept of it. */
export function chartDeleted(id: string) {
  queryClient.removeQueries({
    queryKey: [...chartKeys.all(), 'detail'],
    predicate: ({ queryKey, state }) =>
      queryKey[3] === id || (state.data as ChartBase | undefined)?.id === id,
  })
  return invalidateChartLists()
}

/** The first value given for a key, kept whatever is given later and until the key changes. */
export function useFirst<T>(key: string | undefined, value: T | undefined) {
  const [kept, keep] = useState<{ key: string; value: T } | null>(null)
  if (kept && kept.key === key) return kept.value
  if (key === undefined || value === undefined) return undefined
  keep({ key, value })
  return value
}

/**
 * The chart a form starts from: read anew when the form opens, then held as it was. A later read
 * must not change a form that is being filled in, and neither may a change of who is signed in.
 */
export function useChartSeed<C extends ChartBase>(id: string | undefined) {
  const read = useQuery({
    ...chartQueries.detail<C>(id ?? ''),
    enabled: !!id,
    staleTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const fresh = read.isFetchedAfterMount && !read.isFetching && !read.error
  const chart = useFirst(id, fresh ? read.data : undefined) ?? null
  return { chart, error: chart ? '' : (read.error?.message ?? '') }
}
