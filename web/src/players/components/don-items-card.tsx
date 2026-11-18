import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Catalog } from '@players/don-catalog'
import { donItemKey, type DonItems } from '@players/don-items'
import { ChoiceSelect } from '@players/components/choice-select'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Input } from '@sitekit/components/ui/input'
import { Label } from '@sitekit/components/ui/label'
import { catalogItem, donImage, donItemName, donSource, type DonPartKind } from '@sitekit/don3d'

const parts: DonPartKind[] = ['cos', 'head', 'body', 'puchi', 'paint']

// The parts of the Don that must be earned, for the administrators of titles and items: lock or
// unlock a part (locking gives it to whoever wears it), and give it to or take it back from a
// player. What gives a part by itself is in the rules' card.
export function DonItemsCard({
  catalog,
  items,
  onLock,
  onGrant,
}: {
  catalog: Catalog | null
  items: DonItems | null
  onLock: (part: DonPartKind, id: number, locked: boolean) => void
  onGrant: (part: DonPartKind, id: number, userId: string, give: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  const [part, setPart] = useState<DonPartKind>('cos')
  const [id, setId] = useState('')
  const [userId, setUserId] = useState('')
  const number = Number(id)
  const item = catalog && number > 0 ? catalogItem(catalog, part, number) : undefined
  const locked = items?.locked.has(donItemKey(part, number)) ?? false
  const picture = (p: DonPartKind, n: number) => {
    const it = catalog ? catalogItem(catalog, p, n) : undefined
    const file = it?.icon ?? (p === 'puchi' || p === 'paint' ? it?.file : undefined)
    return file && catalog ? (
      <img
        {...donImage(donSource(catalog.root, file))}
        alt=""
        className="size-12 rounded-lg border object-contain"
      />
    ) : (
      <span className="flex size-12 items-center justify-center rounded-lg border text-xs text-muted-foreground">
        #{n}
      </span>
    )
  }
  return (
    <Card className="rounded-3xl">
      <CardHeader>
        <CardTitle>{t('donItems.heading')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="flex flex-wrap items-end gap-3">
          <div className="space-y-2">
            <Label>{t('donItems.part')}</Label>
            <ChoiceSelect
              label={t('donItems.part')}
              value={part}
              onValueChange={(value) => setPart(value as DonPartKind)}
              items={parts.map((p) => ({ value: p, label: t(`don.kinds.${p}`) }))}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="don-item-id">{t('donItems.id')}</Label>
            <Input
              id="don-item-id"
              className="w-28"
              type="number"
              min={1}
              value={id}
              onChange={(e) => setId(e.target.value)}
            />
          </div>
          {number > 0 && (
            <div className="flex items-center gap-3" data-testid="don-item-chosen">
              {picture(part, number)}
              <div className="text-sm">
                <p className="font-medium">
                  {item ? donItemName(item, language, number) : t('donItems.notInCatalog')}
                </p>
                <p className="text-xs text-muted-foreground">
                  {locked ? t('donItems.locked') : t('donItems.free')}
                </p>
              </div>
            </div>
          )}
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <Button
            disabled={!item}
            variant={locked ? 'outline' : 'default'}
            onClick={() => onLock(part, number, !locked)}
          >
            {locked ? t('donItems.unlock') : t('donItems.lock')}
          </Button>
          <div className="space-y-2">
            <Label htmlFor="don-item-user">{t('admin.userId')}</Label>
            <Input
              id="don-item-user"
              className="w-80 font-mono"
              value={userId}
              onChange={(e) => setUserId(e.target.value.trim())}
            />
          </div>
          <Button disabled={!item || !userId} onClick={() => onGrant(part, number, userId, true)}>
            {t('admin.grantButton')}
          </Button>
          <Button
            variant="outline"
            disabled={!item || !userId}
            onClick={() => onGrant(part, number, userId, false)}
          >
            {t('admin.revokeButton')}
          </Button>
        </div>
        <section className="space-y-2">
          <h3 className="text-sm font-semibold">{t('donItems.lockedList')}</h3>
          {!items || items.locked.size === 0 ? (
            <p className="text-sm text-muted-foreground">{t('donItems.noneLocked')}</p>
          ) : (
            <ul className="flex flex-wrap gap-3" data-testid="don-locked-list">
              {[...items.locked].map((key) => {
                const [p, n] = key.split(':') as [DonPartKind, string]
                const it = catalog ? catalogItem(catalog, p, Number(n)) : undefined
                return (
                  <li key={key}>
                    <button
                      type="button"
                      className="flex items-center gap-2 rounded-xl border p-2 text-left text-xs hover:border-primary/60"
                      onClick={() => {
                        setPart(p)
                        setId(n)
                      }}
                    >
                      {picture(p, Number(n))}
                      <span>
                        {t(`don.kinds.${p}`)}
                        <br />
                        {donItemName(it, language, Number(n))}
                      </span>
                    </button>
                  </li>
                )
              })}
            </ul>
          )}
        </section>
      </CardContent>
    </Card>
  )
}
