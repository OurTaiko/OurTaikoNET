import type { Chart } from '@fanmade/api/types'
import {
  ChartCard as SharedChartCard,
  CommentCountBadge,
  PlayCountBadge,
  MakerBadge,
  UpvoteCountBadge,
} from '@sitekit/components/chart-card'

export function ChartCard({ chart }: { chart: Chart }) {
  return (
    <SharedChartCard
      chart={chart}
      facts={<MakerBadge name={chart.maker || chart.author} />}
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
