// Which parts of the 3D Don must be earned (the backend's internal/donitems): an administrator
// locks a part, and then it is worn only by the players who own it. Everything else is anyone's.
// The dressing room asks only `barred` and `rulesFor`; the manage page changes them.
import { queryOptions, useQuery } from '@tanstack/react-query'
import { api } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { RewardRule } from '@players/api/types'
import { playerKeys } from '@players/queries'
import type { DonPartKind } from '@sitekit/don3d'

export type DonItem = { part: DonPartKind; id: number }
export const donItemKey = (part: string, id: number) => `${part}:${id}`

export type DonItems = {
  locked: Set<string>
  owned: Set<string>
  rules: RewardRule[]
  /** A locked part the player does not own (none and taken-off never are). */
  barred: (part: DonPartKind, id: number) => boolean
  /** The rules that give a part. */
  rulesFor: (part: DonPartKind, id: number) => RewardRule[]
}

type Known = Pick<DonItems, 'locked' | 'owned' | 'rules'>

/** Reads what is locked and the rules; with `mine`, what the signed-in player owns. Not known:
 * nothing is barred (the server still says no when a part is). */
export async function readDonItems(mine: boolean, signal?: AbortSignal): Promise<Known> {
  try {
    const [items, owned] = await Promise.all([
      api<{ locked: DonItem[]; rules: RewardRule[] }>(endpoints.donItems, { signal }),
      mine
        ? api<{ owned: DonItem[] }>(endpoints.myDonItems, { signal })
        : Promise.resolve({ owned: [] as DonItem[] }),
    ])
    return {
      locked: new Set(items.locked.map((i) => donItemKey(i.part, i.id))),
      owned: new Set(owned.owned.map((i) => donItemKey(i.part, i.id))),
      rules: items.rules,
    }
  } catch (e) {
    if (signal?.aborted) throw e
    return { locked: new Set(), owned: new Set(), rules: [] }
  }
}

/** The questions the pages ask of what is known. */
export function donItemsOf(known: Known): DonItems {
  return {
    ...known,
    barred: (part, id) =>
      id > 0 && known.locked.has(donItemKey(part, id)) && !known.owned.has(donItemKey(part, id)),
    rulesFor: (part, id) => known.rules.filter((r) => r.donPart === part && r.donItem === id),
  }
}

/** What is locked and which rules give parts; with `mine`, also what the signed-in player owns. */
export function useDonItems(mine: boolean): DonItems | null {
  const { data: known } = useQuery(
    queryOptions({
      queryKey: playerKeys.donItems(mine),
      queryFn: ({ signal }) => readDonItems(mine, signal),
    }),
  )
  return known ? donItemsOf(known) : null
}
