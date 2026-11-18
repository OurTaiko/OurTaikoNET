// What the song source being shown says of its 段位 courses. As with charts, the address is taken
// when a query is made: by the time the answer comes the visitor may be in another area.
import { queryOptions } from '@tanstack/react-query'
import { api } from '../api/client'
import { endpoints } from '../api/endpoints'
import type { Dan } from '../api/types'
import { queryClient } from '../query/client'
import { SOURCE } from '../source'

export const danKeys = {
  all: () => [SOURCE, 'dans'] as const,
  list: () => [...danKeys.all(), 'list'] as const,
  detail: (id: string) => [...danKeys.all(), 'detail', id] as const,
}

export const danQueries = {
  list: () => {
    const address = endpoints.dans
    return queryOptions({
      queryKey: danKeys.list(),
      queryFn: ({ signal }) => api<{ dans: Dan[] }>(address, { signal }).then(({ dans }) => dans),
    })
  },
  detail: (id: string) => {
    const address = endpoints.dan(id)
    return queryOptions({
      queryKey: danKeys.detail(id),
      queryFn: ({ signal }) => api<Dan>(address, { signal }),
    })
  },
}

/** After a course's score, comments or standing changed: its card in the list shows them. */
export const invalidateDanList = () => queryClient.invalidateQueries({ queryKey: danKeys.list() })

/** After a course was saved: everything said of it before is out of date. */
export const invalidateDans = () => queryClient.invalidateQueries({ queryKey: danKeys.all() })
