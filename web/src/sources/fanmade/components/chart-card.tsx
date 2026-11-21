import type { Chart } from '@fanmade/api/types'
import {
  ChartCard as SharedChartCard,
  CommentCountBadge,
  PlayCountBadge,
  MakerBadge,
  VoteCountBadge,
} from '@sitekit/components/chart-card'

export function ChartCard({ chart }: { chart: Chart }) {
  return (
    <SharedChartCard
      chart={chart}
      facts={<MakerBadge name={chart.maker || chart.author} />}
      footer={
        <>
          <VoteCountBadge upvotes={chart.upvotes} downvotes={chart.downvotes} />
          <PlayCountBadge count={chart.playCount} />
          <CommentCountBadge count={chart.commentCount} />
        </>
      }
    />
  )
}
