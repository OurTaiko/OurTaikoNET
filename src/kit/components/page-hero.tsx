import type { ReactNode } from 'react'

/**
 * A heading with its last word marked (the whole of it where words are not spaced apart).
 * wholeUpTo: for text that may be long (a song's title), the most characters such a whole
 * is marked at; longer text with no spaces is left plain.
 */
export function Marked({ text, wholeUpTo = Infinity }: { text: string; wholeUpTo?: number }) {
  const cut = text.trimEnd().lastIndexOf(' ')
  if (cut < 0 && text.length > wholeUpTo) return text
  return cut < 0 ? (
    <span className="mark">{text}</span>
  ) : (
    <>
      {text.slice(0, cut + 1)}
      <span className="mark">{text.slice(cut + 1)}</span>
    </>
  )
}

/** The top of a main page: a line saying where this is, the heading, and what follows it. */
export function PageHero({
  title,
  kicker,
  children,
}: {
  title: string
  kicker?: ReactNode
  children?: ReactNode
}) {
  return (
    <header className="space-y-4">
      <div className="space-y-1.5">
        {kicker && <p className="font-semibold text-muted-foreground text-sm">{kicker}</p>}
        <h1 className="font-extrabold text-4xl sm:text-5xl leading-[1.1] tracking-tight">
          <Marked text={title} />
        </h1>
      </div>
      {children}
    </header>
  )
}
