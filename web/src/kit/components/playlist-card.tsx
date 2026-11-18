import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { HeartIcon, LockSimpleIcon, MusicNotesIcon, PlaylistIcon } from '@phosphor-icons/react'
import type { Playlist } from '../api/types'
import { CategoryLabels } from '../categories'
import { Badge } from './ui/badge'
import { Skeleton } from './ui/skeleton'
import { UserAvatar } from './user-avatar'
import { useCardCover } from './card-cover'

const systemIds = ['liked', 'popular', 'acclaimed', 'daily'] as const
export const emptyStateClassName =
  'flex flex-col items-center justify-center gap-4 rounded-[28px] bg-muted px-6 py-16 text-center [&>p]:max-w-lg [&>p]:text-muted-foreground'
// The server names system playlists in English; show them in the page language.
export function usePlaylistText() {
  const { t } = useTranslation()
  return (playlist: Pick<Playlist, 'id' | 'name' | 'description' | 'system'>) => {
    const id = systemIds.find((value) => playlist.system && value === playlist.id)
    return id
      ? {
          name: t(`playlists.system.${id}.name`),
          description: t(`playlists.system.${id}.description`),
        }
      : { name: playlist.name, description: playlist.description }
  }
}

// As coverSource (cover.ts).
export const playlistCoverSource = (playlist: Pick<Playlist, 'songListCoverPath'>) =>
  playlist.songListCoverPath || undefined

export function PlaylistFacts({ playlist }: { playlist: Playlist }) {
  const { t } = useTranslation()
  return (
    <CategoryLabels ids={[playlist.categoryId]}>
      <li>
        <Badge variant="outline" className="tabular-nums">
          <MusicNotesIcon aria-hidden="true" />
          {t('playlists.songCount', { count: playlist.chartCount })}
        </Badge>
      </li>
      {playlist.system ? (
        <li>
          <Badge variant="secondary">{t('playlists.systemBadge')}</Badge>
        </li>
      ) : (
        <>
          <li>
            <Badge variant="outline" className="tabular-nums">
              <HeartIcon aria-hidden="true" />
              {t('playlists.favoriteCount', { count: playlist.favoriteCount })}
            </Badge>
          </li>
          {!playlist.public && (
            <li>
              <Badge variant="secondary">
                <LockSimpleIcon aria-hidden="true" />
                {t('playlists.private')}
              </Badge>
            </li>
          )}
        </>
      )}
    </CategoryLabels>
  )
}

export function PlaylistCard({ playlist }: { playlist: Playlist }) {
  const { t } = useTranslation()
  const text = usePlaylistText()(playlist)
  const cover = useCardCover(playlistCoverSource(playlist))
  return (
    <Link
      className="group flex min-w-0 gap-4 rounded-[20px] bg-card p-4 shadow-elev-1 transition-[translate,box-shadow] duration-200 ease-emphasized outline-none hover:-translate-y-0.5 hover:shadow-elev-2 focus-visible:ring-3 focus-visible:ring-ring focus-visible:ring-offset-2 motion-reduce:transform-none motion-reduce:transition-none"
      data-testid="playlist-card"
      to={`/playlists/${encodeURIComponent(playlist.id)}`}
    >
      <div className="flex size-24 shrink-0 items-center justify-center overflow-hidden rounded-[16px] bg-secondary text-secondary-foreground">
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
          <PlaylistIcon className="size-8" aria-hidden="true" />
        )}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <h3 className="truncate font-bold" title={text.name}>
          {text.name}
        </h3>
        <p className="line-clamp-2 text-sm text-muted-foreground">{text.description}</p>
        <div className="mt-auto space-y-2">
          {playlist.owner && (
            <p className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
              <UserAvatar
                nickname={playlist.owner.nickname || t('messages.unknownMember')}
                avatarPath={playlist.owner.avatarPath}
                className="size-5 shrink-0"
                fallbackClassName="text-[10px]"
              />
              <span className="truncate">
                {playlist.owner.nickname || t('messages.unknownMember')}
              </span>
            </p>
          )}
          <div className="max-h-[1.375rem] overflow-hidden">
            <PlaylistFacts playlist={playlist} />
          </div>
        </div>
      </div>
    </Link>
  )
}

export function PlaylistGrid({ items, empty }: { items: Playlist[]; empty: ReactNode }) {
  if (!items.length) return <div className={emptyStateClassName}>{empty}</div>
  return (
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
      {items.map((playlist) => (
        <PlaylistCard key={playlist.id} playlist={playlist} />
      ))}
    </div>
  )
}

export function PlaylistGridSkeleton() {
  const { t } = useTranslation()
  return (
    <div
      className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3"
      role="status"
      aria-label={t('playlists.loading')}
    >
      {[0, 1, 2].map((n) => (
        <Skeleton className="h-32 rounded-[20px]" key={n} />
      ))}
    </div>
  )
}
