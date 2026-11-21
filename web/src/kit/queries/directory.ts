// The site's players, with what they did on all sources together. The list belongs to no source:
// it is the same from whichever area it is asked.
import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '../api/client'
import { SITE_API_ROOT } from '../api/config'
import { withQuery } from '../api/endpoints'
import type { UserDirectory } from '../api/types'

export const directorySorts = ['active', 'newest', 'karma'] as const
export type DirectorySort = (typeof directorySorts)[number]
export type DirectoryQuery = {
  q: string
  sort: DirectorySort
  page: number
  /** Players in a page, 1 to 100; left out, 24. */
  pageSize?: number
}

export const directoryAddress = (query: DirectoryQuery) =>
  withQuery(
    `${SITE_API_ROOT}/users`,
    Object.fromEntries(
      Object.entries(query).filter(
        (entry): entry is [string, string | number] => entry[1] !== undefined,
      ),
    ),
  )

export const directoryQuery = (query: DirectoryQuery) =>
  queryOptions({
    queryKey: ['players', 'directory', query] as const,
    queryFn: ({ signal }) => api<UserDirectory>(directoryAddress(query), { signal }),
    placeholderData: keepPreviousData,
    // Shown at once, and asked again each time: the pages of the sources change these numbers.
    refetchOnMount: 'always',
  })
