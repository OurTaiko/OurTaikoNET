import {
  useEffect,
  useRef,
  useState,
  type FocusEvent,
  type PointerEvent,
  type RefObject,
  type TouchEvent,
} from 'react'
import { CaretLeftIcon, CaretRightIcon, PauseIcon, PlayIcon } from '@phosphor-icons/react'
import { Button } from '@sitekit/components/ui/button'
import type { SourceId } from '@sitekit/source'

/** Whether the visitor asked their system for less motion: nothing then moves until they start it. */
export const prefersStill = () =>
  typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

/**
 * The home page's way of showing several things in one place: one at a time, moving on by
 * itself, and waiting while the pointer is over it, the keyboard is in it, or the visitor
 * paused it. `count` is how many there are; with none, nothing here is to be called.
 */
export function useCarousel(count: number) {
  const [index, setIndex] = useState(0)
  const [paused, setPaused] = useState(prefersStill)
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const swiped = useRef<number | null>(null)
  const current = count ? index % count : 0
  const go = (to: number) => setIndex((to + count) % count)
  return {
    current,
    go,
    paused,
    setPaused,
    /** Nothing is to move by itself now: paused, or the visitor is at it. */
    still: paused || hovered || focused,
    moving: !paused && !hovered && !focused && count > 1,
    /** For the element around all of it. */
    hold: {
      onPointerEnter: (event: PointerEvent) => event.pointerType === 'mouse' && setHovered(true),
      onPointerLeave: () => setHovered(false),
      onFocus: () => setFocused(true),
      onBlur: (event: FocusEvent) =>
        !event.currentTarget.contains(event.relatedTarget) && setFocused(false),
    },
    /** For the element the slides are in: a swipe goes on or back. */
    swipe: {
      onTouchStart: (event: TouchEvent) => {
        swiped.current = event.touches[0].clientX
      },
      onTouchEnd: (event: TouchEvent) => {
        const moved = event.changedTouches[0].clientX - (swiped.current ?? 0)
        swiped.current = null
        if (Math.abs(moved) > 48) go(current + (moved < 0 ? 1 : -1))
      },
    },
  }
}

export type Carousel = ReturnType<typeof useCarousel>

/**
 * The way around a carousel, under its slides: back and on, a note for each slide (red, don,
 * for one of the official source and teal, ka, for a fanmade one), and pause. The note of
 * the slide shown stretches into a roll that fills until the next takes over.
 */
export function CarouselBar({
  carousel,
  notes,
  labels,
}: {
  carousel: Carousel
  /** One for each slide, in order; `label` says which it is to a screen reader. */
  notes: { key: string; source: SourceId; label: string }[]
  labels: { previous: string; next: string; pause: string; resume: string }
}) {
  const { current, go, paused, setPaused, moving } = carousel
  return (
    <div className="flex items-center gap-1 px-3 py-2 border-t">
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={labels.previous}
        onClick={() => go(current - 1)}
      >
        <CaretLeftIcon weight="bold" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={labels.next}
        onClick={() => go(current + 1)}
      >
        <CaretRightIcon weight="bold" />
      </Button>
      <ol className="flex flex-1 justify-center items-center min-w-0 note-lane">
        {notes.map((note, i) => (
          <li key={note.key}>
            <button
              type="button"
              data-accent={note.source}
              aria-current={i === current}
              aria-label={note.label}
              className="group place-items-center grid px-[3px] sm:px-1 rounded-full outline-none h-8"
              onClick={() => go(i)}
            >
              <span className="note">
                {i === current && (
                  <span
                    className="note-roll"
                    style={{ animationPlayState: moving ? 'running' : 'paused' }}
                    onAnimationEnd={() => go(current + 1)}
                  />
                )}
              </span>
            </button>
          </li>
        ))}
      </ol>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={paused ? labels.resume : labels.pause}
        onClick={() => setPaused(!paused)}
      >
        {paused ? <PlayIcon weight="fill" /> : <PauseIcon weight="fill" />}
      </Button>
    </div>
  )
}

/**
 * Scrolls box down by itself while `moving`, a line every second or so. The list is in it
 * twice (`loop` is the second copy), so that reaching the copy and jumping back by its
 * distance shows no seam. The visitor's own scrolling always wins, and holds it a moment.
 */
export function useDrift(
  box: RefObject<HTMLElement | null>,
  loop: RefObject<HTMLElement | null>,
  moving: boolean,
) {
  useEffect(() => {
    const element = box.current
    if (!element) return
    let touched = 0
    const touch = () => (touched = performance.now())
    // The seam, for the visitor's scrolling as for ours.
    const wrap = () => {
      const length = loop.current?.offsetTop
      if (length && element.scrollTop >= length) element.scrollTop -= length
    }
    element.addEventListener('wheel', touch, { passive: true })
    element.addEventListener('touchstart', touch, { passive: true })
    element.addEventListener('touchmove', touch, { passive: true })
    element.addEventListener('scroll', wrap, { passive: true })
    let frame = 0
    if (moving) {
      let last = performance.now()
      let at = element.scrollTop
      const step = (now: number) => {
        // A tab that was in the background comes back with one long frame.
        const passed = Math.min(now - last, 100)
        last = now
        if (now - touched > 2000) {
          // Browsers round scrollTop; ours is kept apart unless the visitor moved it.
          if (Math.abs(element.scrollTop - at) > 2) at = element.scrollTop
          at += (28 * passed) / 1000
          const length = loop.current?.offsetTop
          if (length && at >= length) at -= length
          element.scrollTop = at
        }
        frame = requestAnimationFrame(step)
      }
      frame = requestAnimationFrame(step)
    }
    return () => {
      cancelAnimationFrame(frame)
      element.removeEventListener('wheel', touch)
      element.removeEventListener('touchstart', touch)
      element.removeEventListener('touchmove', touch)
      element.removeEventListener('scroll', wrap)
    }
  }, [box, loop, moving])
}
