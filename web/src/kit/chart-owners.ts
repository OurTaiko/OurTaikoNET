import type { ChartBase } from './api/types'

/** The built-in account that owns every official chart. It is never shown as a person. */
export const SYSTEM_OWNER_ID = '0'.repeat(32)

/** The people who own the chart: none when it belongs to the built-in account. */
export function chartOwners(chart: ChartBase) {
  const owners = chart.owners ?? [
    { id: chart.ownerId, nickname: chart.author, avatarPath: chart.authorAvatarPath },
  ]
  return owners.filter((owner) => owner.id !== SYSTEM_OWNER_ID)
}

export function isChartOwner(chart: ChartBase, userId?: string) {
  return !!userId && (chart.ownerIds ?? [chart.ownerId]).includes(userId)
}
