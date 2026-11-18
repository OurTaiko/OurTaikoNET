import type { Chart } from '@fanmade/api/types'
import {
  BranchingBadge,
  ChartCard as SharedChartCard,
  CommentCountBadge,
  DoubleBadge,
  MakerBadge,
  ScoreBadge,
} from '@sitekit/components/chart-card'

export function ChartCard({ chart }: { chart: Chart }) {
  return (
    <SharedChartCard
      chart={chart}
      facts={
        <>
          <MakerBadge name={chart.maker || chart.author} />
        </>
      }
      footer={
        <>
          <DoubleBadge difficulties={chart.difficulties} />
          <BranchingBadge difficulties={chart.difficulties} />
          <ScoreBadge score={chart.score} />
          <CommentCountBadge count={chart.commentCount} />
        </>
      }
    />
  )
}
