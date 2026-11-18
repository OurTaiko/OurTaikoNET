import { useTranslation } from 'react-i18next'
import { endpoints } from './api/endpoints'
import { Button } from './components/ui/button'
import { Label } from './components/ui/label'
import { Checkbox } from './components/ui/checkbox'
import { Badge } from './components/ui/badge'
import { ChoiceSelect } from './components/choice-select'
import { useQuery, type QueryFunctionContext } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { api } from './api/client'
import { categoryColor } from './category-colors'
import type { Category, CategoryList } from './api/types'
import { i18n } from './i18n'
import { SOURCE } from './source'

// The address is taken while the page of its source is shown: the source may change meanwhile.
const categoriesOf =
  (address: string) =>
  ({ signal }: QueryFunctionContext) =>
    api<CategoryList>(address, { signal })

/** The catalog of the source being shown, and how many charts it has published. */
export function useCategories() {
  const { data, error, refetch } = useQuery({
    queryKey: [SOURCE, 'categories'],
    queryFn: categoriesOf(endpoints.categories),
    staleTime: 5 * 60_000,
  })
  return {
    items: data?.items ?? null,
    chartCount: data?.chartCount ?? 0,
    // What was read before stays good when reading it again fails.
    error: data ? '' : (error?.message ?? ''),
    retry: () => void refetch(),
  }
}
// Category names are the site's text, categories.<id>; the API's title is the fallback.
export function categoryName(category: Pick<Category, 'id' | 'title'>): string {
  const key = `categories.${category.id}`
  return i18n.exists(key) ? i18n.t(key as 'categories.variety') : category.title
}
export function CategoryLabels({ ids = [], children }: { ids?: string[]; children?: ReactNode }) {
  const { t } = useTranslation()
  const { items } = useCategories()
  if (ids.length === 0 && !children) return null
  const uniqueIds = [...new Set(ids)]
  return (
    <ul
      className="flex min-w-0 flex-wrap items-center gap-2"
      aria-label={children ? t('messages.chartInformation') : t('messages.categories')}
    >
      {children}
      {uniqueIds.map((id) => (
        <li key={id}>
          <Badge variant="outline" className="gap-1.5">
            {categoryColor(id) && (
              <span
                aria-hidden="true"
                className="rounded-full size-2 shrink-0"
                style={{ background: categoryColor(id) }}
              />
            )}
            {categoryName(items?.find((category) => category.id === id) ?? { id, title: id })}
          </Badge>
        </li>
      ))}
    </ul>
  )
}
export function CategoryPicker({
  value,
  onChange,
  disabled = false,
}: {
  value: string[]
  onChange: (ids: string[]) => void
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const { items, error, retry } = useCategories()
  return (
    <fieldset
      className="min-w-0 space-y-3 [&>legend]:mb-2 [&>legend]:text-sm [&>legend]:font-medium"
      disabled={disabled}
    >
      <legend>{t('messages.chartCategories')}</legend>
      <p className="text-sm text-muted-foreground">{t('messages.categoriesHint')}</p>
      {error ? (
        <div role="alert" className="flex flex-wrap items-center gap-3 text-sm text-destructive">
          <span>{t('messages.categoryLoadError', { message: error })}</span>
          <Button type="button" variant="outline" size="sm" onClick={retry}>
            {t('messages.reloadCategories')}
          </Button>
        </div>
      ) : items === null ? (
        <p role="status" className="text-sm text-muted-foreground">
          {t('messages.loadingCategories')}
        </p>
      ) : (
        <div className="flex flex-wrap gap-x-6 gap-y-4">
          {items.map((category) => (
            <Label key={category.id} className="inline-flex items-center gap-2 text-sm">
              <Checkbox
                disabled={disabled}
                checked={value.includes(category.id)}
                onCheckedChange={(checked) => {
                  onChange(
                    checked
                      ? [...value, category.id].sort()
                      : value.filter((id) => id !== category.id),
                  )
                }}
              />
              <span>{categoryName(category)}</span>
            </Label>
          ))}
        </div>
      )}
    </fieldset>
  )
}

export function CategorySelect({
  label,
  value,
  onChange,
  disabled = false,
}: {
  label: string
  value: string
  onChange: (id: string) => void
  disabled?: boolean
}) {
  const { items } = useCategories()
  return (
    <ChoiceSelect
      label={label}
      value={value}
      onValueChange={onChange}
      disabled={disabled || !items}
      items={(items ?? []).map((category) => ({
        value: category.id,
        label: categoryName(category),
      }))}
    />
  )
}
