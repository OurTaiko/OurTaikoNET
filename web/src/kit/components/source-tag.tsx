import { useTranslation } from 'react-i18next'
import type { SourceId } from '../source'

/** The tag that says which source something is from; only where both sources are mixed. */
export function SourceTag({ source }: { source: SourceId }) {
  const { t } = useTranslation()
  return (
    <span
      data-accent={source}
      className="inline-flex items-center bg-secondary px-2 rounded-[7px] h-[1.375rem] font-semibold text-[0.6875rem] text-secondary-foreground whitespace-nowrap"
    >
      {t(`shell.${source}`)}
    </span>
  )
}
