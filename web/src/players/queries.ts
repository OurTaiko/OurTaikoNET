// What the server says of players: their profiles, scores, titles and Don, the title catalog, and
// the panels of the site's administrator. These pages belong to no source, so what comes from a
// song source names that source itself, in its key and in its address.
import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api as sourceRead } from '@sitekit/api/client'
import type { ChartBase, ChartList, PublicUser, UserSpace } from '@sitekit/api/types'
import { supportsChart } from '@sitekit/courses'
import { queryClient } from '@sitekit/query/client'
import type { SourceId } from '@sitekit/source'
import { api } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type {
  Activity,
  Dans,
  Group,
  GroupMember,
  HeldTitle,
  PublicProfile,
  ScorePage,
  ScoreSort,
  ScoreView,
  Title,
  TitleRule,
  VisitReport,
} from '@players/api/types'
import { sourceApi } from '@players/components/song-tile'
import { readDonCatalog } from '@players/don-catalog'
import type { DonLook } from '@sitekit/don3d'

/** One page of a list of a player's things. */
export type Page<T> = { items: T[]; total: number; page: number; pageSize: number }

export type ScoreFilter = {
  view: ScoreView
  sort: ScoreSort
  source: string
  course?: string
  page: number
}

export type DonAnswer = { look: DonLook }

export const playerKeys = {
  all: () => ['players'] as const,
  user: (id: string) => [...playerKeys.all(), 'user', id] as const,
  profile: (id: string) => [...playerKeys.user(id), 'profile'] as const,
  scores: (id: string, filter: ScoreFilter) =>
    // No course and an empty one are the same list.
    [...playerKeys.user(id), 'scores', { ...filter, course: filter.course || '' }] as const,
  heldTitles: (id: string) => [...playerKeys.user(id), 'titles'] as const,
  dans: (id: string) => [...playerKeys.user(id), 'dans'] as const,
  don: (id: string, mine: boolean) => [...playerKeys.user(id), 'don', mine] as const,
  donItems: (mine: boolean) => [...playerKeys.all(), 'don-items', mine] as const,
  titles: () => [...playerKeys.all(), 'titles'] as const,
  activity: () => [...playerKeys.all(), 'activity'] as const,
  manage: () => [...playerKeys.all(), 'manage'] as const,
  groups: () => [...playerKeys.manage(), 'groups'] as const,
  members: () => [...playerKeys.manage(), 'members'] as const,
  found: (text: string) => [...playerKeys.manage(), 'found', text] as const,
  visits: (period: number, zone: string) => [...playerKeys.all(), 'visits', period, zone] as const,
  // What a song source says, which the pages of that source can change.
  ofSource: (source: SourceId) => [source, 'players'] as const,
  sourceProfile: (source: SourceId, id: string) =>
    [...playerKeys.ofSource(source), 'profile', id] as const,
  ownerCharts: (source: SourceId, id: string) =>
    [...playerKeys.ofSource(source), 'charts', id] as const,
  works: (source: SourceId, address: string, page: number) =>
    [...playerKeys.ofSource(source), 'works', address, page] as const,
  library: (source: SourceId) => [...playerKeys.ofSource(source), 'library'] as const,
}

// Charts, playlists and comments are changed on the pages of a source, which do not know what is
// kept here: what those lists say is shown at once and asked again each time they are opened.
const askedOnEachOpening = 0

export const playerQueries = {
  profile: (id: string) =>
    queryOptions({
      queryKey: playerKeys.profile(id),
      queryFn: ({ signal }) => api<PublicProfile>(endpoints.user(id), { signal }),
    }),
  /** A page of a player's scores; the page before stays on show until the next one comes. */
  scores: (id: string, filter: ScoreFilter) => {
    const { view, sort, source, course, page } = filter
    const query = new URLSearchParams({ view, sort, source, page: String(page) })
    if (course) query.set('course', course)
    const address = endpoints.userScores(id, query)
    return queryOptions({
      queryKey: playerKeys.scores(id, filter),
      queryFn: ({ signal }) => api<ScorePage>(address, { signal }),
      placeholderData: keepPreviousData,
    })
  },
  heldTitles: (id: string) =>
    queryOptions({
      queryKey: playerKeys.heldTitles(id),
      queryFn: ({ signal }) => api<{ titles: HeldTitle[] }>(endpoints.userTitles(id), { signal }),
    }),
  dans: (id: string) =>
    queryOptions({
      queryKey: playerKeys.dans(id),
      queryFn: ({ signal }) => api<Dans>(endpoints.userDans(id), { signal }),
    }),
  /**
   * A player's Don, which seeds the dressing room: read afresh each time the tab is opened, and
   * not again while it is open, where the player may be halfway through changing it.
   */
  don: (id: string, mine: boolean) => {
    const address = mine ? endpoints.myDon : endpoints.userDon(id)
    return queryOptions({
      queryKey: playerKeys.don(id, mine),
      queryFn: ({ signal }) => api<DonAnswer>(address, { signal }),
      staleTime: Infinity,
      gcTime: 0,
    })
  },
  titles: () =>
    queryOptions({
      queryKey: playerKeys.titles(),
      queryFn: ({ signal }) =>
        api<{ titles: Title[]; rules?: TitleRule[] }>(endpoints.titles, { signal }),
    }),
  activity: () =>
    queryOptions({
      queryKey: playerKeys.activity(),
      queryFn: ({ signal }) => api<{ items: Activity[] }>(endpoints.activity, { signal }),
    }),
  groups: () =>
    queryOptions({
      queryKey: playerKeys.groups(),
      queryFn: ({ signal }) => api<{ groups: Group[] }>(endpoints.groups, { signal }),
    }),
  members: () =>
    queryOptions({
      queryKey: playerKeys.members(),
      queryFn: ({ signal }) => api<{ users: GroupMember[] }>(endpoints.groupMembers, { signal }),
    }),
  found: (text: string) =>
    queryOptions({
      queryKey: playerKeys.found(text),
      queryFn: ({ signal }) =>
        api<{ users: GroupMember[] }>(endpoints.findPlayers(text), { signal }),
    }),
  /** A period's report, with the period it is of: the one before stays up while the next is read. */
  visits: (period: number, zone: string) =>
    queryOptions({
      queryKey: playerKeys.visits(period, zone),
      queryFn: async ({ signal }) => ({
        period,
        report: await api<VisitReport>(endpoints.visitReport(period, zone), { signal }),
      }),
      placeholderData: keepPreviousData,
    }),
  /** What a source knows of a player; a source that does not know them is not asked twice. */
  sourceProfile: (source: SourceId, id: string) =>
    queryOptions({
      queryKey: playerKeys.sourceProfile(source, id),
      queryFn: async ({ signal }): Promise<PublicUser | null> => {
        const address = `${sourceApi(source)}/users/${encodeURIComponent(id)}`
        return (await sourceRead<UserSpace>(address, { signal })).user
      },
      staleTime: askedOnEachOpening,
      retry: false,
    }),
  /** The charts a player uploaded to a source that this site can show. */
  ownerCharts: (source: SourceId, id: string) =>
    queryOptions({
      queryKey: playerKeys.ownerCharts(source, id),
      queryFn: async ({ signal }) => {
        const address = `${sourceApi(source)}/charts?owner=${encodeURIComponent(id)}`
        const list = await sourceRead<ChartList>(address, { signal })
        return list.items.filter(supportsChart).map((chart: ChartBase) => ({ source, chart }))
      },
      staleTime: askedOnEachOpening,
    }),
  /** One page of a list of a player's things in a source; `address` is the list's, without it. */
  works: <T>(source: SourceId, address: string, page: number) =>
    queryOptions({
      queryKey: playerKeys.works(source, address, page),
      queryFn: ({ signal }) =>
        sourceRead<Page<T>>(`${address}${address.includes('?') ? '&' : '?'}page=${page}`, {
          signal,
        }),
      staleTime: askedOnEachOpening,
    }),
  /** A source's newest songs and how many it has. */
  library: (source: SourceId) =>
    queryOptions({
      queryKey: playerKeys.library(source),
      queryFn: async ({ signal }) => {
        const list = await sourceRead<ChartList>(`${sourceApi(source)}/charts`, { signal })
        return { total: list.total, items: list.items.filter(supportsChart) }
      },
      staleTime: askedOnEachOpening,
    }),
}

// The catalog is the same for every page and every player, and readDonCatalog reads it once.
export const donCatalogQuery = queryOptions({
  queryKey: ['don', 'catalog'] as const,
  queryFn: () => readDonCatalog(),
  staleTime: Infinity,
})

/**
 * After a title or a part of the Don was changed, given or taken back: the catalog, the nameplates
 * that show it and what each player holds may all read differently now.
 */
export const invalidatePlayers = () => queryClient.invalidateQueries({ queryKey: playerKeys.all() })

/** After a player's groups were set; rejects when the panel's lists could not be read again. */
export const invalidateManage = async () => {
  // A search may be given up while it is read again, which says nothing of what was saved.
  void queryClient.invalidateQueries({ queryKey: [...playerKeys.manage(), 'found'] })
  await Promise.all(
    [playerKeys.groups(), playerKeys.members()].map((queryKey) =>
      queryClient.invalidateQueries({ queryKey }, { throwOnError: true }),
    ),
  )
}
