import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { TitleBandArt } from '@sitekit/components/nameplate-art'
import { Checkbox } from '@sitekit/components/ui/checkbox'
import {
  itemName,
  slots,
  type LayerIds,
  type Slot,
  type TitleLayers,
} from '@sitekit/nameplate-layers'
import type { TitlePart } from '@players/api/types'
import { playerQueries } from '@players/queries'
import { ChoiceSelect } from './choice-select'

const custom = ''

/** A part as a title's layers name it. */
const layerOf = (part: TitlePart) => `${part.type}/${part.name}`

/**
 * How a title looks, picked layer by layer from the parts the site keeps for a player's plate (a
 * band, and an effect and an icon or none), or all at once from a preset; whether the effect runs
 * over the whole plate or is cut to the band; with the title on its band as it will be drawn.
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
  const { t, i18n } = useTranslation()
  const read = useQuery(playerQueries.titleParts())
  const parts = read.data?.items
  // A look that came without numbers (a new title's) gets those of the parts it names.
  useEffect(() => {
    if (!parts || value.ids) return
    const number = (id: string | null) => parts.find((part) => layerOf(part) === id)?.id ?? null
    const band = number(value.band)
    if (band !== null)
      onChange({ ...value, ids: { band, effect: number(value.effect), icon: number(value.icon) } })
  }, [parts, value, onChange])
  if (!read.data || !parts) {
    return (
      <p className="text-sm text-muted-foreground">
        {t(read.isError ? 'admin.layers.unavailable' : 'admin.layers.loading')}
      </p>
    )
  }
  const presets = read.data.presets.filter((preset) => preset.kind === 'player')
  const offered = parts.filter((part) => part.kind === 'player')
  const ids = value.ids
  const same = (look: TitleLayers) =>
    look.ids?.band === ids?.band &&
    look.ids?.effect === ids?.effect &&
    look.ids?.icon === ids?.icon &&
    look.effectOverflow === value.effectOverflow
  const preset = presets.find((preset) => same(preset.layers))?.name ?? custom
  const label = (slot: Slot) => t(`admin.layers.${slot}`)
  const partName = (part: TitlePart) => part.nameTranslations[i18n.language] || itemName(part.name)
  // The look of these parts: their numbers for the request, their names for the picture.
  const lookOf = (picked: LayerIds, effectOverflow: boolean): TitleLayers => {
    const named = (id: number | null) => {
      const part = parts.find((part) => part.id === id)
      return part ? layerOf(part) : null
    }
    return {
      band: named(picked.band) ?? value.band,
      effect: named(picked.effect),
      icon: named(picked.icon),
      // Nothing overflows without an effect.
      effectOverflow: effectOverflow && picked.effect !== null,
      ids: picked,
    }
  }
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm font-medium">{t('admin.look')}</span>
        <ChoiceSelect
          label={t('admin.layers.preset')}
          value={preset}
          onValueChange={(name) => {
            const look = presets.find((preset) => preset.name === name)?.layers
            if (look?.ids) onChange(lookOf(look.ids, look.effectOverflow))
          }}
          items={[
            { value: custom, label: t('admin.layers.custom') },
            ...presets.map((preset) => ({ value: preset.name, label: preset.name })),
          ]}
          disabled={disabled}
        />
      </div>
      <div className="flex flex-wrap gap-3">
        {slots.map((slot) => {
          const current = ids?.[slot] ?? null
          const inPresets = presets
            .filter((preset) => current !== null && preset.layers.ids?.[slot] === current)
            .map((preset) => preset.name)
          return (
            <div key={slot} className="space-y-1">
              <span className="block text-xs text-muted-foreground">{label(slot)}</span>
              <ChoiceSelect
                label={label(slot)}
                value={current === null ? '' : String(current)}
                onValueChange={(picked) => {
                  if (!ids) return
                  const id = picked ? Number(picked) : null
                  if (slot === 'band' && id === null) return
                  onChange(lookOf({ ...ids, [slot]: id }, value.effectOverflow))
                }}
                items={[
                  ...(slot === 'band' ? [] : [{ value: '', label: t('admin.layers.none') }]),
                  ...offered
                    .filter((part) => part.type === slot)
                    .map((part) => ({ value: String(part.id), label: partName(part) })),
                ]}
                disabled={disabled || !ids}
              />
              {inPresets.length > 0 && (
                <span
                  className="block max-w-48 truncate text-xs text-muted-foreground"
                  title={inPresets.join(', ')}
                >
                  {t('admin.layers.inTitles', { titles: inPresets.join(', ') })}
                </span>
              )}
            </div>
          )
        })}
      </div>
      {value.effect && (
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={value.effectOverflow}
            disabled={disabled}
            onCheckedChange={(checked) => onChange({ ...value, effectOverflow: Boolean(checked) })}
          />
          {t('admin.layers.overflow')}
        </label>
      )}
      <TitleBandArt layers={value} name={name} className="h-7" />
    </div>
  )
}
