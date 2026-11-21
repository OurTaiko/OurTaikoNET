import type { Chart } from '@fanmade/api/types'
import {
  ChartCard as SharedChartCard,
  CommentCountBadge,
  PlayCountBadge,
  MakerBadge,
  ScoreBadge,
} from '@sitekit/components/chart-card'

export function ChartCard({ chart }: { chart: Chart }) {
  return (
    <SharedChartCard
      chart={chart}
      facts={<MakerBadge name={chart.maker || chart.author} />}
      footer={
        <>
          <ScoreBadge score={chart.score} />
          <PlayCountBadge count={chart.playCount} />
          <CommentCountBadge count={chart.commentCount} />
        </>
      }
    />
  )
}
