import type { ReactNode } from 'react'
import { useCardCover } from './card-cover'
import { categoryStripeColor } from '../category-colors'
import { cn } from '../lib/utils'

/** A small dark label that reads over any cover. */
export const coverChip =
  'inline-flex items-center bg-black/75 px-1.5 rounded-md h-5 font-semibold text-[0.6875rem] text-white tabular-nums'

export function formatDuration(seconds: number) {
  return `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, '0')}`
}

/**
 * The square top of a song tile: the song's cover, or its first category's colour with a drum
 * face when it has none, and the tempo and length over it. Sits inside a `group` link.
 * What a tile says beyond those goes over the cover too, so the tile below it keeps one height:
 * `children` under the tempo, `corner` in place of the length, `marks` at the bottom right.
 */
export function SongCover({
  image,
  categoryIds,
  bpm,
  duration,
  className,
  children,
  corner,
  marks,
}: {
  image?: string
  categoryIds?: string[]
  bpm?: number
  /** Seconds; left out when the song has no files yet. */
  duration?: number
  className?: string
  children?: ReactNode
  corner?: ReactNode
  marks?: ReactNode
}) {
  const cover = useCardCover(image)
  return (
    <div
      className={cn(
        'relative flex justify-center items-center aspect-square overflow-hidden',
        className,
      )}
      style={{ background: categoryStripeColor(categoryIds) }}
    >
      {cover.source ? (
        <img
          src={cover.source}
          alt=""
          loading="lazy"
          decoding="async"
          onError={cover.onError}
          className="size-full object-cover motion-reduce:transform-none group-hover:scale-105 transition-transform motion-reduce:transition-none duration-500 ease-emphasized"
        />
      ) : (
        <span
          aria-hidden="true"
          className="shadow-[0_0_0_6px_rgb(255_255_255/0.3)] border-[9px] border-white/75 rounded-full w-[44%] aspect-square"
        />
      )}
      <div className="top-2 left-2 absolute flex flex-col items-start gap-1 max-w-[calc(100%-1rem)]">
        {!!bpm && bpm > 0 && <span className={coverChip}>{bpm} BPM</span>}
        {children}
      </div>
      <div className="top-2 right-2 absolute">
        {corner ??
          (!!duration && duration > 0 && (
            <span className={coverChip}>{formatDuration(duration)}</span>
          ))}
      </div>
      {marks && <div className="right-2 bottom-2 absolute">{marks}</div>}
    </div>
  )
}
