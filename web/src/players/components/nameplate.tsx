import { useTranslation } from 'react-i18next'
import type { Nameplate as Plate } from '@players/api/types'
import { danName, titleName } from '@players/dan'
import { NameplateArt, TitleBandArt } from '@sitekit/components/nameplate-art'
import { layersOf } from '@sitekit/nameplate-layers'

/** A title as its band alone: the strip a nameplate wears, its layers as the game draws them. */
export function TitleBand({
  title,
  className,
}: {
  title: NonNullable<Plate['title']>
  className?: string
}) {
  const { i18n } = useTranslation()
  return (
    <TitleBandArt
      layers={layersOf(title)}
      name={titleName(title, i18n.resolvedLanguage ?? 'zh-Hans')}
      className={className}
    />
  )
}

// The nameplate as the game draws it, from the game's own art: the 段位 chip on the left, the
// title band over the name. Without title or 段位 it is the plain guest plate.
export function Nameplate({ plate, className }: { plate: Plate; className?: string }) {
  const { t, i18n } = useTranslation()
  return (
    <NameplateArt
      name={plate.nickname || t('profile.unknownPlayer')}
      title={
        plate.title
          ? {
              name: titleName(plate.title, i18n.resolvedLanguage ?? 'zh-Hans'),
              layers: layersOf(plate.title),
            }
          : null
      }
      dan={
        plate.dan
          ? {
              index: plate.dan.index,
              gold: plate.dan.gold,
              combo: plate.dan.combo,
              label: danName(plate.dan.index),
            }
          : null
      }
      className={className}
    />
  )
}
