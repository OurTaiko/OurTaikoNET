// The site's players, with what they did on all sources together. The list belongs to no source:
// it is the same from whichever area it is asked.
import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '../api/client'
import { SITE_API_ROOT } from '../api/config'
import { withQuery } from '../api/endpoints'
import type { UserDirectory } from '../api/types'

export type DirectoryQuery = { q: string; sort: 'active' | 'newest'; page: number }

export const directoryAddress = (query: DirectoryQuery) =>
  withQuery(`${SITE_API_ROOT}/users`, query)

export const directoryQuery = (query: DirectoryQuery) =>
  queryOptions({
    queryKey: ['players', 'directory', query] as const,
    queryFn: ({ signal }) => api<UserDirectory>(directoryAddress(query), { signal }),
    placeholderData: keepPreviousData,
    // Shown at once, and asked again each time: the pages of the sources change these numbers.
    refetchOnMount: 'always',
  })
