import { t } from './i18n'
import type { ChartBase } from './api/types'

export const maxCoverBytes = 8 * 1024 * 1024
export function coverFileError(file: File): string {
  if (!/\.(jpe?g|png|webp)$/i.test(file.name)) return t('messages.coversMustBeJpgPngOrWebpFiles')
  if (!file.size || file.size > maxCoverBytes) return t('messages.coverMustNotBeEmptyOrExceed8Mib')
  return ''
}
// The cover's path under the distribution root; there is no API route to read a cover.
// Components read it from the visitor's root (card-cover.tsx).
export function coverSource(chart: Pick<ChartBase, 'songCoverPath'>): string | undefined {
  return chart.songCoverPath || undefined
}

/**
 * What a cover upload answers: the new cover's hash and path, both empty once removed. A chart's
 * cover answers songCoverPath, a playlist's songListCoverPath.
 */
export type SavedCover = { coverHash: string; songCoverPath?: string; songListCoverPath?: string }
