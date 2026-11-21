import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { cn } from 'cn'

/**
 * One line of text that never wraps, so what holds it keeps its height. Text longer than its box
 * slides to its end and back by itself; for a visitor who asked for less motion it stays still
 * and can be scrolled sideways instead (styles.css).
 */
export function ScrollingText({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  const box = useRef<HTMLSpanElement>(null)
  const text = useRef<HTMLSpanElement>(null)
  // How far the text reaches past its box, in pixels; 0 when it fits.
  const [shift, setShift] = useState(0)
  useEffect(() => {
    const outer = box.current
    const inner = text.current
    if (!outer || !inner) return
    // Observing reports the first sizes too. Under two pixels is rounding, not text cut off.
    const watch = new ResizeObserver(() => {
      const over = inner.offsetWidth - outer.clientWidth
      setShift(over >= 2 ? over : 0)
    })
    watch.observe(outer)
    watch.observe(inner)
    return () => watch.disconnect()
  }, [])
  return (
    <span ref={box} className={cn('scrolling-text block', className)}>
      <span
        ref={text}
        className={cn('inline-block', shift > 0 && 'scrolling-text-moving')}
        style={
          shift > 0
            ? ({
                '--shift': `${shift}px`,
                // The same pace whatever the length, with a rest at each end.
                animationDuration: `${Math.max(4, shift / 18 + 2)}s`,
              } as CSSProperties)
            : undefined
        }
      >
        {children}
      </span>
    </span>
  )
}
