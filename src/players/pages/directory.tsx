import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { UsersPage } from '@sitekit/users'
import { rememberedSource } from '@sitekit/shell/site'
import type { SourceId } from '@sitekit/source'
import { cn } from '@sitekit/lib/utils'

const sources: SourceId[] = ['fanmade', 'official']

/**
 * The players, by what they did in one source's songs. Which source is a view of this page
 * only: it starts at the one last browsed and choosing here changes nothing elsewhere.
 */
export function DirectoryPage() {
  const { t } = useTranslation()
  const [source, setSource] = useState<SourceId>(rememberedSource)
  return (
    <UsersPage source={source}>
      <div role="group" aria-label={t('shell.source')} className="flex flex-wrap gap-2 pt-1">
        {sources.map((item) => (
          <button
            key={item}
            type="button"
            aria-pressed={item === source}
            data-accent={item}
            onClick={() => setSource(item)}
            className={cn(
              'inline-flex items-center gap-2 px-4 border rounded-full outline-none focus-visible:ring-3 focus-visible:ring-ring h-9 font-medium text-sm transition-colors',
              item === source
                ? 'border-transparent bg-secondary font-semibold text-secondary-foreground'
                : 'border-input text-muted-foreground hover:bg-muted',
            )}
          >
            <span aria-hidden="true" className="bg-primary-container rounded-full size-2" />
            {t(`shell.${item}`)}
          </button>
        ))}
      </div>
    </UsersPage>
  )
}
