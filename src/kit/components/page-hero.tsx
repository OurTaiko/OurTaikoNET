import type { ReactNode } from 'react'

/** The top of a main page: the heading, and what follows it. */
export function PageHero({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <header className="space-y-4">
      <h1 className="font-extrabold text-4xl sm:text-5xl leading-[1.1] tracking-tight">{title}</h1>
      {children}
    </header>
  )
}
