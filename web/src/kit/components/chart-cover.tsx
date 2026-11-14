import { chartText } from '../chart-language'
import { i18n } from '../i18n'
import { useTranslation } from 'react-i18next'
import { endpoints } from '../api/endpoints'
import { useState } from 'react'
import { cn } from '../lib/utils'
import { CoverUploadDialog } from './cover-dialog'
import type { ChartBase } from '../api/types'
import { coverSource } from '../cover'
import { categoryStripeColor } from '../category-colors'

export function ChartCover({ chart, className }: { chart: ChartBase; className?: string }) {
  const { t } = useTranslation()

  const [failedSource, setFailedSource] = useState('')
  const source = coverSource(chart)
  const available = source && failedSource !== source
  return (
    <div
      className={cn(
        'flex aspect-square items-center justify-center overflow-hidden rounded-[28px] bg-muted shadow-elev-1',
        className,
      )}
    >
      {available ? (
        <img
          src={source}
          alt={t('messages.songCoverAlt', {
            title: chartText(chart, i18n.resolvedLanguage).title,
          })}
          className="size-full object-contain"
          onError={() => setFailedSource(source)}
        />
      ) : (
        <div
          className="flex size-full items-center justify-center"
          style={{ background: categoryStripeColor(chart.categoryIds) }}
        >
          <span
            aria-hidden="true"
            className="aspect-square w-[44%] rounded-full border-[14px] border-white/75 shadow-[0_0_0_9px_rgb(255_255_255/0.3)] sm:border-[20px] sm:shadow-[0_0_0_13px_rgb(255_255_255/0.3)]"
          />
          <span className="sr-only">
            {source ? t('messages.coverTemporarilyUnavailable') : t('messages.noCover')}
          </span>
        </div>
      )}
    </div>
  )
}

// Owners and administrators replace covers from the chart page's management actions.
export function CoverDialog({
  chart,
  onSaved,
  onDismiss,
}: {
  chart: ChartBase
  onSaved: (coverHash: string) => void
  onDismiss: () => void
}) {
  const { t } = useTranslation()
  return (
    <CoverUploadDialog
      title={t('messages.changeSongCover')}
      endpoint={endpoints.cover(chart.id)}
      onSaved={onSaved}
      onDismiss={onDismiss}
    />
  )
}
