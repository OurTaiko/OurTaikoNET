import { t } from './i18n'
import type { ChartBase } from './api/types'

export const maxCoverBytes = 8 * 1024 * 1024
export function coverFileError(file: File): string {
  if (!/\.(jpe?g|png|webp)$/i.test(file.name)) return t('messages.coversMustBeJpgPngOrWebpFiles')
  if (!file.size || file.size > maxCoverBytes) return t('messages.coverMustNotBeEmptyOrExceed8Mib')
  return ''
}
// The cover's address in the public bucket; there is no API route to read a cover. Components
// move it to the visitor's regional root (card-cover.tsx).
export function coverSource(chart: Pick<ChartBase, 'coverUrl'>): string | undefined {
  return chart.coverUrl || undefined
}

/** What a cover upload answers: the new cover's hash and own address, both empty once removed. */
export type SavedCover = { coverHash: string; coverUrl?: string }
