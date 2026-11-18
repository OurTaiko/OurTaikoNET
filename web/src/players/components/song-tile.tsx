import { useTranslation } from 'react-i18next'
import type { ChartBase } from '@sitekit/api/types'
import { chartText } from '@sitekit/chart-language'
import { LevelChips } from '@sitekit/components/difficulty-badges'
import { SongCover } from '@sitekit/components/song-cover'
import { SourceTag } from '@sitekit/components/source-tag'
import type { SourceId } from '@sitekit/source'

export { SourceTag }

// These pages belong to no source, so they say which source's API and pages they mean.
export const sources: SourceId[] = ['fanmade', 'official']
export const sourceApi = (source: SourceId) => `/api/v2/sources/${source}`
export const songPage = (source: SourceId, id: string) => `/${source}/charts/${id}`
export const coverOf = (chart: ChartBase) => chart.songCoverPath || undefined

/** A song of a named source: the tile of the song lists, linking to the song's page there. */
export function SongTile({
  source,
  chart,
  tag = false,
}: {
  source: SourceId
  chart: ChartBase
  /** Say the source on the tile: for a list that mixes both. */
  tag?: boolean
}) {
  const { i18n } = useTranslation()
  const text = chartText(chart, i18n.resolvedLanguage)
  return (
    <a
      href={songPage(source, chart.id)}
      data-testid="song-tile"
      className="group flex flex-col bg-card shadow-elev-1 hover:shadow-elev-2 rounded-[20px] outline-none focus-visible:ring-3 focus-visible:ring-ring focus-visible:ring-offset-2 min-w-0 overflow-hidden hover:-translate-y-0.5 motion-reduce:transform-none transition-[translate,box-shadow] motion-reduce:transition-none duration-200 ease-emphasized"
    >
      <SongCover
        image={coverOf(chart)}
        categoryIds={chart.categoryIds}
        bpm={chart.bpm}
        duration={chart.duration}
      />
      <div className="flex flex-col flex-1 gap-2 p-3 min-w-0">
        <div className="space-y-0.5 min-w-0">
          <h3 className="font-bold text-sm line-clamp-2 leading-snug" title={text.title}>
            {text.title}
          </h3>
          <p className="min-h-4 text-muted-foreground text-xs truncate" title={text.subtitle}>
            {text.subtitle}
          </p>
        </div>
        <LevelChips difficulties={chart.difficulties} />
        {tag && (
          <div className="mt-auto">
            <SourceTag source={source} />
          </div>
        )}
      </div>
    </a>
  )
}
