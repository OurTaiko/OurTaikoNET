import { useTranslation } from 'react-i18next'
import { FileDashedIcon } from '@phosphor-icons/react'
import type { Chart } from '@official/api/types'
import { Badge } from '@sitekit/components/ui/badge'
import {
  ChartCard as SharedChartCard,
  CommentCountBadge,
  PlayCountBadge,
  UpvoteCountBadge,
} from '@sitekit/components/chart-card'
import { coverChip } from '@sitekit/components/song-cover'
import { cn } from '@sitekit/lib/utils'

export function ChartCard({ chart }: { chart: Chart }) {
  const { t } = useTranslation()
  return (
    <SharedChartCard
      chart={chart}
      // A song without files has no length to show: the cover says so in its place.
      coverNote={
        !chart.hasFiles && (
          <span className={cn(coverChip, 'gap-1')}>
            <FileDashedIcon className="size-3" aria-hidden="true" />
            {t('messages.noFiles')}
          </span>
        )
      }
      facts={
        <li>
          {chart.nijiiroId === null ? (
            <Badge variant="outline" className="text-muted-foreground tabular-nums">
              #{chart.numberId}
            </Badge>
          ) : (
            <Badge
              variant="outline"
              className="gap-0 p-0 tabular-nums"
              title={t('db.nijiiroIdWithValue', { id: chart.nijiiroId })}
            >
              <span className="flex items-center bg-muted px-2 border-r h-full text-muted-foreground shrink-0">
                Nijiiro
              </span>
              <span className="px-2">{chart.nijiiroId}</span>
            </Badge>
          )}
        </li>
      }
      footer={
        <>
          <UpvoteCountBadge count={chart.upvotes} />
          <PlayCountBadge count={chart.playCount} />
          <CommentCountBadge count={chart.commentCount} />
        </>
      }
    />
  )
}
