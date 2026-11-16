// A player's 3D Don (the one OurTaikoPlay draws for them): a live preview for everyone, and for the
// player themself the dressing room: a whole costume or a head and a body, a puchichara and the
// three colours, picked from the catalog's icons and saved to their profile (PUT /me/don). The
// models come from the public bucket's 3dchara folder, which the catalog's baseUrl names.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react'
import { api, jsonRequest } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import { readDonCatalog, type Catalog } from '@players/don-catalog'
import { Notice } from '@players/notice'
import { useSession } from '@players/session-context'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { cn } from '@sitekit/lib/utils'
import {
  DonPreview,
  catalogItem,
  catalogUrl,
  type DonCatalog,
  type DonCatalogItem,
  type DonLook,
  type DonMotionName,
  type DonPartKind,
} from '@sitekit/don3d'

type Answer = { look: DonLook }

// The motions the preview offers, in the order they are shown.
const motions = [
  'Normal',
  'Combo',
  'Sabi',
  'SelectLoop',
  'EntryLoop',
  'FullCombo',
  'Miss',
] as const satisfies readonly DonMotionName[]

const colorKeys = ['body', 'face', 'rim'] as const

/** The same look, for comparing a draft with what is saved. */
export function sameLook(a: DonLook, b: DonLook): boolean {
  return JSON.stringify(normalize(a)) === JSON.stringify(normalize(b))
}

function normalize(l: DonLook): DonLook {
  return {
    costume: l.costume,
    cos: l.cos,
    head: l.head,
    body: l.body,
    puchi: l.puchi,
    colors: l.colors ? (l.colors.map((c) => c.toUpperCase()) as DonLook['colors']) : null,
  }
}

/** An item's name in the reader's language, else Japanese (the cabinet's), else its number. */
export function itemName(item: DonCatalogItem | undefined, language: string, id: number): string {
  if (!item) return `#${id}`
  const short = language.split('-')[0]
  return item.name[language] ?? item.name[short] ?? item.name.ja ?? `#${id}`
}

export function DonTab({ id, mine }: { id: string; mine: boolean }) {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [saved, setSaved] = useState<DonLook | null>(null)
  const [draft, setDraft] = useState<DonLook | null>(null)
  const [motion, setMotion] = useState<(typeof motions)[number]>('Normal')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [done, setDone] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    setSaved(null)
    setDraft(null)
    setError('')
    Promise.all([
      readDonCatalog(),
      api<Answer>(mine ? endpoints.myDon : endpoints.userDon(id), { signal: controller.signal }),
    ])
      .then(([c, answer]) => {
        setCatalog(c)
        setSaved(answer.look)
        setDraft(answer.look)
      })
      .catch((e: Error) => !controller.signal.aborted && setError(e.message))
    return () => controller.abort()
  }, [id, mine])

  const dirty = saved !== null && draft !== null && !sameLook(saved, draft)
  const change = (next: Partial<DonLook>) => {
    setDone(false)
    setDraft((d) => (d ? { ...d, ...next } : d))
  }
  const save = async () => {
    if (!draft) return
    setSaving(true)
    setError('')
    try {
      const answer = await api<Answer>(
        endpoints.myDon,
        jsonRequest('PUT', draft, session.csrfToken),
      )
      setSaved(answer.look)
      setDraft(answer.look)
      setDone(true)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  if (error && !draft) return <Notice>{error}</Notice>
  if (!catalog || !draft) return <Skeleton className="h-[480px] w-full rounded-3xl" />
  const colors = draft.colors ?? catalog.defaultColors.p1

  return (
    <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,400px)_minmax(0,1fr)]">
      <Card className="rounded-3xl lg:sticky lg:top-20">
        <CardContent className="space-y-4">
          <div className="relative aspect-[480/560] w-full overflow-hidden rounded-2xl bg-gradient-to-b from-muted/30 to-muted/70">
            <DonPreview
              catalog={catalog}
              baseUrl={catalog.baseUrl}
              look={draft}
              motion={motion}
              className="absolute inset-0"
            />
          </div>
          <div className="flex flex-wrap gap-1.5" role="group" aria-label={t('don.motion')}>
            {motions.map((m) => (
              <Button
                key={m}
                size="xs"
                variant={m === motion ? 'secondary' : 'ghost'}
                aria-pressed={m === motion}
                onClick={() => setMotion(m)}
              >
                {t(`don.motions.${m}`)}
              </Button>
            ))}
          </div>
          <p className="text-sm text-muted-foreground">{describe(catalog, draft, language, t)}</p>
        </CardContent>
      </Card>

      {mine ? (
        <Card className="rounded-3xl">
          <CardHeader>
            <CardTitle>{t('don.dress')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            <p className="text-sm text-muted-foreground">{t('don.about')}</p>
            <div
              className="inline-flex rounded-full border p-1"
              role="radiogroup"
              aria-label={t('don.wear')}
            >
              {[true, false].map((costume) => (
                <button
                  key={String(costume)}
                  type="button"
                  role="radio"
                  aria-checked={draft.costume === costume}
                  onClick={() => change({ costume })}
                  className={cn(
                    'rounded-full px-4 py-1.5 text-sm font-medium transition-colors',
                    draft.costume === costume
                      ? 'bg-primary text-primary-foreground'
                      : 'text-muted-foreground hover:text-foreground',
                  )}
                >
                  {t(costume ? 'don.costume' : 'don.parts')}
                </button>
              ))}
            </div>

            {draft.costume ? (
              <IconGrid
                title={t('don.kinds.cos')}
                kind="cos"
                catalog={catalog}
                value={draft.cos}
                language={language}
                onPick={(cos) => change({ cos })}
              />
            ) : (
              <>
                <IconGrid
                  title={t('don.kinds.head')}
                  kind="head"
                  catalog={catalog}
                  value={draft.head}
                  language={language}
                  onPick={(head) => change({ head })}
                />
                <IconGrid
                  title={t('don.kinds.body')}
                  kind="body"
                  catalog={catalog}
                  value={draft.body}
                  language={language}
                  onPick={(body) => change({ body })}
                />
              </>
            )}
            <IconGrid
              title={t('don.kinds.puchi')}
              kind="puchi"
              catalog={catalog}
              value={draft.puchi}
              language={language}
              onPick={(puchi) => change({ puchi })}
              none={t('don.noPuchi')}
            />

            <section className="space-y-3">
              <div className="flex items-center justify-between gap-3">
                <h3 className="text-sm font-semibold">{t('don.colors')}</h3>
                <Button
                  size="xs"
                  variant="ghost"
                  disabled={draft.colors === null}
                  onClick={() => change({ colors: null })}
                >
                  <ArrowCounterClockwiseIcon data-icon="inline-start" />
                  {t('don.defaultColors')}
                </Button>
              </div>
              <div className="flex flex-wrap gap-4">
                {colorKeys.map((key, i) => (
                  <label key={key} className="flex items-center gap-2 text-sm">
                    <input
                      type="color"
                      value={colors[i].toLowerCase()}
                      onChange={(e) => {
                        const next = [...colors] as [string, string, string]
                        next[i] = e.target.value.toUpperCase()
                        change({ colors: next })
                      }}
                      className="size-9 cursor-pointer rounded-lg border bg-transparent p-0.5"
                    />
                    {t(`don.color.${key}`)}
                  </label>
                ))}
              </div>
            </section>

            {error && <Notice>{error}</Notice>}
            {done && !dirty && <Notice kind="info">{t('don.saved')}</Notice>}
            <div className="flex flex-wrap gap-2">
              <Button disabled={!dirty || saving} onClick={() => void save()}>
                {saving ? t('don.saving') : t('don.save')}
              </Button>
              <Button variant="outline" disabled={!dirty || saving} onClick={() => setDraft(saved)}>
                {t('don.undo')}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}

function describe(
  catalog: DonCatalog,
  l: DonLook,
  language: string,
  t: (key: string, options?: Record<string, unknown>) => string,
): string {
  const name = (kind: DonPartKind, id: number) =>
    itemName(catalogItem(catalog, kind, id), language, id)
  const wears = l.costume
    ? name('cos', l.cos)
    : t('don.headAndBody', { head: name('head', l.head), body: name('body', l.body) })
  return l.puchi > 0 ? t('don.withPuchi', { wears, puchi: name('puchi', l.puchi) }) : wears
}

function IconGrid({
  title,
  kind,
  catalog,
  value,
  language,
  onPick,
  none,
}: {
  title: string
  kind: DonPartKind
  catalog: Catalog
  value: number
  language: string
  onPick: (id: number) => void
  none?: string
}) {
  const items = catalog.items[kind] ?? []
  const tile = (selected: boolean) =>
    cn(
      'relative flex aspect-square items-center justify-center overflow-hidden rounded-xl border bg-card transition',
      'hover:border-primary/60 focus-visible:outline-2 focus-visible:outline-primary',
      selected && 'border-primary ring-2 ring-primary/40',
    )
  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold">{title}</h3>
      <div
        className="grid max-h-72 grid-cols-[repeat(auto-fill,minmax(4rem,1fr))] gap-2 overflow-y-auto pr-1"
        role="radiogroup"
        aria-label={title}
      >
        {none !== undefined && (
          <button
            type="button"
            role="radio"
            aria-checked={value === 0}
            title={none}
            className={cn(tile(value === 0), 'text-xs text-muted-foreground')}
            onClick={() => onPick(0)}
          >
            {none}
          </button>
        )}
        {items.map((item) => {
          const name = itemName(item, language, item.id)
          return (
            <button
              key={item.id}
              type="button"
              role="radio"
              aria-checked={value === item.id}
              aria-label={name}
              title={name}
              className={tile(value === item.id)}
              onClick={() => onPick(item.id)}
            >
              {kind === 'puchi' ? (
                // The first of its two frames (stacked top to bottom, so twice as tall as wide).
                <img
                  src={catalogUrl(catalog.baseUrl, item.file, catalog.mirrorUrls)}
                  alt=""
                  loading="lazy"
                  className="absolute inset-x-0 top-0 w-full"
                />
              ) : item.icon ? (
                <img
                  src={catalogUrl(catalog.baseUrl, item.icon, catalog.mirrorUrls)}
                  alt=""
                  loading="lazy"
                  className="size-full object-contain p-1"
                />
              ) : (
                <span className="text-xs text-muted-foreground">#{item.id}</span>
              )}
            </button>
          )
        })}
      </div>
    </section>
  )
}
