import { useTranslation } from 'react-i18next'
import { TitleBandArt, useNameplateSet } from '@sitekit/components/nameplate-art'
import { itemName, partsOf, slots, type Slot, type TitleLayers } from '@sitekit/nameplate-layers'
import { ChoiceSelect } from './choice-select'

const custom = ''

/**
 * How a title looks, picked layer by layer from the parts the game's titles are made of (a band,
 * and an effect and an icon or none), or all three at once from one of those titles; with the
 * title on its band as it will be drawn.
 */
export function TitleLayersPicker({
  value,
  onChange,
  name,
  disabled = false,
}: {
  value: TitleLayers
  onChange: (value: TitleLayers) => void
  name: string
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const set = useNameplateSet()
  if (!set) {
    return <p className="text-sm text-muted-foreground">{t('admin.layers.loading')}</p>
  }
  const titles = set.catalog.titles
  const preset =
    Object.entries(titles).find(
      ([, look]) =>
        look.band === value.band && look.effect === value.effect && look.icon === value.icon,
    )?.[0] ?? custom
  const label = (slot: Slot) => t(`admin.layers.${slot}`)
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm font-medium">{t('admin.look')}</span>
        <ChoiceSelect
          label={t('admin.layers.preset')}
          value={preset}
          onValueChange={(label) => {
            if (titles[label]) onChange(titles[label])
          }}
          items={[
            { value: custom, label: t('admin.layers.custom') },
            ...Object.keys(titles).map((label) => ({ value: label, label })),
          ]}
          disabled={disabled}
        />
      </div>
      <div className="flex flex-wrap gap-3">
        {slots.map((slot) => {
          const parts = partsOf(set.catalog, slot)
          const current = value[slot] ?? ''
          const inTitles = parts.find((part) => part.id === current)?.titles ?? []
          return (
            <div key={slot} className="space-y-1">
              <span className="block text-xs text-muted-foreground">{label(slot)}</span>
              <ChoiceSelect
                label={label(slot)}
                value={current}
                onValueChange={(id) =>
                  onChange({ ...value, [slot]: id || (slot === 'band' ? value.band : null) })
                }
                items={[
                  ...(slot === 'band' ? [] : [{ value: '', label: t('admin.layers.none') }]),
                  ...parts.map((part) => ({ value: part.id, label: itemName(part.id) })),
                ]}
                disabled={disabled}
              />
              {inTitles.length > 0 && (
                <span
                  className="block max-w-48 truncate text-xs text-muted-foreground"
                  title={inTitles.join(', ')}
                >
                  {t('admin.layers.inTitles', { titles: inTitles.join(', ') })}
                </span>
              )}
            </div>
          )
        })}
      </div>
      <TitleBandArt layers={value} name={name} className="h-10" />
    </div>
  )
}
