// Shows what the player was given (./store) when there is something, over any page. The frame
// mounts it once.
//
// Each kind of reward is drawn by its entry in `views`; a kind with no entry is not drawn, and
// is still marked seen with the rest. A new kind of reward is one more entry here.
import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '../components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../components/ui/dialog'
import { DanEmblem, TitleBandArt, type TitleStyle } from '../components/nameplate-art'
import { danNames } from '../components/profile-plate'
import { catalogItem, donImage, donItemName, donSource, type DonPartKind } from '../don3d'
import { readDonCatalog, type ServedDonCatalog } from '../don3d/catalog-request'
import { visit } from '../navigation'
import { titleText, type ProfileTitle } from '../profile-titles'
import { dismissRewards, useRewards, type Reward } from './store'

type View = (reward: Reward, language: string) => { node: ReactNode; wear?: boolean } | null

type TitleData = { title?: ProfileTitle; service?: string; courseTitle?: string }
type DanData = { index?: number; gold?: boolean; combo?: number }
type DonItemData = { part?: DonPartKind; itemId?: number; service?: string; courseTitle?: string }

function useViews(catalog: ServedDonCatalog | null): Record<string, View> {
  const { t } = useTranslation()
  return {
    title: (reward, language) => {
      const { title, service, courseTitle } = reward.data as TitleData
      if (!title?.name) return null
      const name = titleText(title, language)
      const from = courseTitle
        ? t('rewards.fromCourse', { course: courseTitle })
        : service === 'rule'
          ? t('rewards.fromRule')
          : service === 'admin'
            ? t('rewards.fromAdmin')
            : ''
      return {
        wear: true,
        node: (
          <div className="flex flex-col items-center gap-2 text-center">
            <span className="text-xs text-muted-foreground">{t('rewards.title')}</span>
            <TitleBandArt style={title.style as TitleStyle} name={name} className="h-9" />
            {title.description && <p>{title.description}</p>}
            {from && <p className="text-xs text-muted-foreground">{from}</p>}
          </div>
        ),
      }
    },
    // A part of the 3D Don, drawn from the catalog (its picture when it has one).
    don_item: (reward, language) => {
      const { part, itemId } = reward.data as DonItemData
      if (!part || typeof itemId !== 'number') return null
      const item = catalog ? catalogItem(catalog, part, itemId) : undefined
      const file = item?.icon ?? (part === 'puchi' || part === 'paint' ? item?.file : undefined)
      return {
        node: (
          <div className="flex flex-col items-center gap-2 text-center">
            <span className="text-xs text-muted-foreground">{t('rewards.donItem')}</span>
            {file && catalog ? (
              <img
                {...donImage(donSource(catalog.baseUrl, file, catalog))}
                alt=""
                className="size-20 rounded-xl border object-contain"
              />
            ) : null}
            <span className="font-medium">
              {t(`rewards.donPart.${part}`)} · {donItemName(item, language, itemId)}
            </span>
          </div>
        ),
      }
    },
    dan: (reward) => {
      const { index, gold, combo } = reward.data as DanData
      if (typeof index !== 'number') return null
      const name = danNames[index] ?? ''
      return {
        node: (
          <div className="flex flex-col items-center gap-2 text-center">
            <span className="text-xs text-muted-foreground">{t('rewards.dan')}</span>
            <DanEmblem
              index={index}
              gold={!!gold}
              combo={combo ?? 0}
              label={name}
              className="h-14"
            />
            <span className="font-medium">{name}</span>
          </div>
        ),
      }
    },
  }
}

export function RewardsDialog() {
  const { t, i18n } = useTranslation()
  const { rewards, player } = useRewards()
  // The Don's catalog, read only when a part of it is to be shown.
  const [catalog, setCatalog] = useState<ServedDonCatalog | null>(null)
  const needsCatalog = rewards.some((reward) => reward.kind === 'don_item')
  useEffect(() => {
    if (needsCatalog && !catalog) readDonCatalog().then(setCatalog, () => {})
  }, [needsCatalog, catalog])
  const views = useViews(catalog)
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  const shown = rewards
    .map((reward) => ({ reward, view: views[reward.kind]?.(reward, language) ?? null }))
    .filter((item) => item.view !== null)
  const nothingToDraw = rewards.length > 0 && shown.length === 0
  // Only kinds this page cannot draw: nothing to show, but they were delivered.
  useEffect(() => {
    if (nothingToDraw) void dismissRewards()
  }, [nothingToDraw])
  if (shown.length === 0) return null
  const wear = player && shown.some((item) => item.view?.wear)
  return (
    <Dialog
      open
      // A click meant for the page as the dialog appears must not make the player miss it.
      disablePointerDismissal
      onOpenChange={(open) => {
        if (!open) void dismissRewards()
      }}
    >
      <DialogContent data-testid="rewards-dialog">
        <DialogHeader>
          <DialogTitle className="text-center">{t('rewards.heading')}</DialogTitle>
        </DialogHeader>
        <ul className="max-h-[60svh] space-y-5 overflow-y-auto">
          {shown.map(({ reward, view }) => (
            <li key={reward.id}>{view?.node}</li>
          ))}
        </ul>
        <DialogFooter>
          {wear && (
            <Button
              variant="outline"
              onClick={() => {
                void dismissRewards()
                visit(`/players/${encodeURIComponent(player)}?tab=titles`)
              }}
            >
              {t('rewards.wear')}
            </Button>
          )}
          <Button onClick={() => void dismissRewards()}>{t('rewards.gotIt')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
