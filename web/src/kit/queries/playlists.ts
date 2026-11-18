// What the song source being shown says of its playlists. As with charts, the address is taken when
// a query is made: by the time the answer comes the visitor may be in another area.
import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api } from '../api/client'
import { endpoints } from '../api/endpoints'
import type { MyPlaylists, Playlist, PlaylistPage } from '../api/types'
import { queryClient } from '../query/client'
import { SOURCE } from '../source'

export type PlaylistQuery = { q: string; sort: string; page: number; owner?: string }

export const playlistKeys = {
  all: () => [SOURCE, 'playlists'] as const,
  list: (query: PlaylistQuery) => [...playlistKeys.all(), 'list', query] as const,
  system: () => [...playlistKeys.all(), 'system'] as const,
  /** The viewer's playlists, each saying whether it holds `chart` when one is named. */
  mine: (chart = '') => [...playlistKeys.all(), 'mine', chart] as const,
  detail: (id: string) => [...playlistKeys.all(), 'detail', id] as const,
}

export const playlistQueries = {
  /** A page of the public list; the page before stays on show until the next one comes. */
  list: (query: PlaylistQuery) => {
    const address = endpoints.playlistList(query)
    return queryOptions({
      queryKey: playlistKeys.list(query),
      queryFn: ({ signal }) => api<PlaylistPage>(address, { signal }),
      placeholderData: keepPreviousData,
    })
  },
  system: () => {
    const address = endpoints.systemPlaylists
    return queryOptions({
      queryKey: playlistKeys.system(),
      queryFn: ({ signal }) =>
        api<{ items: Playlist[] }>(address, { signal }).then(({ items }) => items),
    })
  },
  mine: (chart?: string) => {
    const address = endpoints.myPlaylists(chart)
    return queryOptions({
      queryKey: playlistKeys.mine(chart),
      queryFn: ({ signal }) => api<MyPlaylists>(address, { signal }),
    })
  },
  detail: (id: string) => {
    const address = endpoints.playlist(id)
    return queryOptions({
      queryKey: playlistKeys.detail(id),
      queryFn: ({ signal }) => api<Playlist>(address, { signal }),
    })
  },
}

/** After a playlist's name, cover or favourites changed: every list may show it differently now. */
export const invalidatePlaylistLists = () =>
  queryClient.invalidateQueries({
    queryKey: playlistKeys.all(),
    predicate: ({ queryKey }) => queryKey[2] !== 'detail',
  })

/** After a playlist's songs changed: its own page is out of date as well as the lists. */
export const invalidatePlaylists = () =>
  queryClient.invalidateQueries({ queryKey: playlistKeys.all() })
