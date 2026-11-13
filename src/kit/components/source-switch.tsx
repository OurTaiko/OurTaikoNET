import { useTranslation } from 'react-i18next'
import { SOURCE, sources } from '../source'
import { cn } from '../lib/utils'

// The two song sources, side by side in the header. The other one is a plain link: its pages
// are loaded anew, with its own charts, text and colours.
export function SourceSwitch() {
  const { t } = useTranslation()
  return (
    <nav
      className="flex items-center p-0.5 border-[1.5px] rounded-lg text-sm font-bold shrink-0"
      aria-label={t('messages.songSource')}
      data-testid="source-switch"
    >
      {sources.map((source) =>
        source.id === SOURCE ? (
          <span
            key={source.id}
            aria-current="page"
            className="px-2.5 py-1 rounded-md bg-primary text-primary-foreground"
          >
            {source.name}
          </span>
        ) : (
          <a
            key={source.id}
            href={`/${source.id}/`}
            className={cn(
              'px-2.5 py-1 rounded-md text-muted-foreground transition-colors hover:text-primary',
              'outline-none focus-visible:ring-3 focus-visible:ring-ring/40',
            )}
          >
            {source.name}
          </a>
        ),
      )}
    </nav>
  )
}
