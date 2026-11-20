// What the home page's recommended playlists and the panel that sets them share: reading the
// playlists a list names from their sources, a playlist's page, and its cover.
import { useQueries } from '@tanstack/react-query'
import { PlaylistIcon } from '@phosphor-icons/react'
import type { Playlist } from '@sitekit/api/types'
import { useCardCover } from '@sitekit/components/card-cover'
import { playlistCoverSource } from '@sitekit/components/playlist-card'
import { cn } from '@sitekit/lib/utils'
import type { SourceId } from '@sitekit/source'
import { sources } from '@players/components/song-tile'
import { playerQueries } from '@players/queries'

/** A playlist of a source, as the site tells playlists of both sources apart. */
export const playlistKey = (source: string, id: string) => `${source}/${id}`
export const playlistPage = (source: SourceId, id: string) =>
  `/${source}/playlists/${encodeURIComponent(id)}`
export const isSource = (source: string): source is SourceId =>
  (sources as string[]).includes(source)

/**
 * The playlist whose page an address is, wherever in the text the address stands: what the
 * panel's search box takes for "this one". null when the text has no playlist's address.
 */
export function playlistInAddress(text: string): { source: SourceId; id: string } | null {
  const found = /(?:^|\/)(\w+)\/playlists\/([0-9a-f]{32})(?![0-9a-f])/.exec(text)
  return found && isSource(found[1]) ? { source: found[1], id: found[2] } : null
}

/**
 * The playlists a list names, read from their sources: one request a source, without their
 * songs. `found` has each by its playlistKey; one that is private or gone is not in it.
 */
export function usePlaylistsNamed(named: { source: string; playlistId: string }[]) {
  const asked = sources.map((source) => ({
    source,
    ids: named.filter((item) => item.source === source).map((item) => item.playlistId),
  }))
  const answers = useQueries({
    queries: asked.map(({ source, ids }) => ({
      ...playerQueries.playlistsNamed(source, ids),
      enabled: ids.length > 0,
    })),
  })
  const found = new Map<string, Playlist>()
  asked.forEach(({ source }, index) =>
    answers[index].data?.forEach((playlist) =>
      found.set(playlistKey(source, playlist.id), playlist),
    ),
  )
  return {
    found,
    loading: asked.some(({ ids }, index) => ids.length > 0 && answers[index].isPending),
    error: answers.find((answer) => !answer.data && answer.error)?.error ?? null,
  }
}

/** A playlist's cover, or the playlist mark on its source's colour where it has none. */
export function PlaylistCover({ playlist, className }: { playlist: Playlist; className?: string }) {
  const cover = useCardCover(playlistCoverSource(playlist))
  return (
    <span
      className={cn(
        'flex justify-center items-center bg-primary-container overflow-hidden text-on-primary-container shrink-0',
        className,
      )}
    >
      {cover.source ? (
        <img
          src={cover.source}
          alt=""
          loading="lazy"
          decoding="async"
          onError={cover.onError}
          className="size-full object-cover"
        />
      ) : (
        <PlaylistIcon className="size-2/5" aria-hidden="true" />
      )}
    </span>
  )
}
