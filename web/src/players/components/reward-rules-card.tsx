import { useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { RewardRule, RuleCategories, Title } from '@players/api/types'
import type { DonPartKind } from '@sitekit/don3d'
import { ChoiceSelect } from '@players/components/choice-select'
import { titleName } from '@players/dan'
import {
  categoryName,
  chartIdOf,
  danVerdicts,
  ruleCourses,
  ruleKinds,
  ruleSources,
  ruleText,
} from '@players/reward-rules'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Input } from '@sitekit/components/ui/input'
import { Label } from '@sitekit/components/ui/label'
import { useNotification } from '@sitekit/notification-context'

type Kind = RewardRule['kind']
type SourceId = (typeof ruleSources)[number]
/** A rule's conditions as the backend takes them (POST /api/v2/admin/titles/{id}/rules, PUT
 * /api/v2/admin/rules/{id}). */
export type RuleDraft = {
  kind: Kind
  source?: SourceId
  chartId?: string
  danId?: string
  category?: string
  course?: string
  clear?: number
  count?: number
}

/** What a rule gives: a title, or a part of the 3D Don and its number. */
export type RuleTarget = { title: string } | { donPart: DonPartKind; donItem: number }

const donParts: DonPartKind[] = ['cos', 'head', 'body', 'puchi', 'paint']

const blank = {
  target: 'title' as 'title' | 'don',
  titleId: '',
  donPart: 'cos' as DonPartKind,
  donItem: '',
  kind: 'chart_clear' as Kind,
  source: 'fanmade' as SourceId,
  chart: '',
  dan: '',
  // '': the source's first category.
  category: '',
  course: '',
  clear: '1',
  count: '',
}

/** The form filled in with a rule, to change it. */
const filled = (rule: RewardRule): typeof blank => ({
  target: rule.item === 'title' ? 'title' : 'don',
  titleId: rule.titleId ?? '',
  donPart: rule.donPart ?? 'cos',
  donItem: rule.donItem === null ? '' : String(rule.donItem),
  kind: rule.kind,
  source: rule.source ?? 'fanmade',
  chart: rule.chartId ?? '',
  dan: rule.danId ?? '',
  category: rule.category ?? '',
  course: rule.course ?? '',
  clear: String(rule.clear ?? 1),
  count: rule.count === null ? '' : String(rule.count),
})

// What gives an item by itself: the rules there are, each to change or delete, and a form for
// one more. The item is a title (a public one: a private title is its maker's alone) or a part of
// the Don; a rule's item does not change. The categories are the backend's (GET /api/v2/titles).
// onAdd and onUpdate answer '' once done, else what went wrong.
export function RewardRulesCard({
  titles,
  rules,
  categories,
  donName,
  onAdd,
  onUpdate,
  onDelete,
}: {
  titles: Title[]
  rules: RewardRule[]
  categories: RuleCategories
  // A Don part's name, from the catalog.
  donName: (part: DonPartKind, id: number) => string
  onAdd: (target: RuleTarget, rule: RuleDraft) => Promise<string>
  onUpdate: (rule: RewardRule, draft: RuleDraft) => Promise<string>
  onDelete: (rule: RewardRule) => void
}) {
  const { t, i18n } = useTranslation()
  const { confirm } = useNotification()
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  const [form, setForm] = useState(blank)
  const [editing, setEditing] = useState<RewardRule | null>(null)
  const sourceCategories = categories[form.source] ?? []
  const category = form.category || sourceCategories[0] || ''
  const eligible = titles.filter((title) => title.visibility === 'public')
  const plays = form.kind === 'chart_clear' || form.kind === 'category_clear'
  // What names a source: plays, and passing a 段位 course of it.
  const sourced = plays || form.kind === 'dan_clear'
  const counted = form.kind === 'category_clear' || form.kind === 'charts_uploaded'
  const submit = (event: FormEvent) => {
    event.preventDefault()
    const rule: RuleDraft = { kind: form.kind }
    // An upload rule names no source: players upload to the fanmade one alone.
    if (sourced) rule.source = form.source
    if (form.kind === 'chart_clear') rule.chartId = chartIdOf(form.chart)
    if (form.kind === 'dan_clear') {
      rule.danId = chartIdOf(form.dan)
      rule.clear = Number(form.clear)
    }
    if (form.kind === 'category_clear') rule.category = category
    if (plays) {
      // A crown, 1–3: what a 段位 verdict left in the form counts as a clear.
      rule.clear = Math.min(Number(form.clear), 3)
      if (form.course) rule.course = form.course
    }
    if (counted && form.count.trim()) rule.count = Number(form.count)
    const sent = editing
      ? onUpdate(editing, rule)
      : onAdd(
          form.target === 'title'
            ? { title: form.titleId }
            : { donPart: form.donPart, donItem: Number(form.donItem) },
          rule,
        )
    void sent.then((failure) => {
      if (!failure && editing) stop()
    })
  }
  const stop = () => {
    setEditing(null)
    setForm(blank)
  }
  // What a rule gives, by name, and its key.
  const item = (rule: RewardRule): [string, string] => {
    if (rule.item === 'don_item' && rule.donPart && rule.donItem !== null)
      return [
        `${t(`don.kinds.${rule.donPart}`)} · ${donName(rule.donPart, rule.donItem)}`,
        `${rule.donPart}:${rule.donItem}`,
      ]
    const title = titles.find((candidate) => candidate.id === rule.titleId)
    return [title ? titleName(title, language) : (rule.titleId ?? ''), rule.titleId ?? '']
  }
  const ready =
    (form.target === 'title'
      ? form.titleId !== ''
      : Number.isInteger(Number(form.donItem)) && Number(form.donItem) > 0) &&
    (form.kind !== 'category_clear' || category !== '')
  return (
    <Card className="rounded-3xl">
      <CardHeader>
        <CardTitle>{t('rules.heading')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        {rules.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('rules.none')}</p>
        ) : (
          <ul className="divide-y" data-testid="rule-list">
            {rules.map((rule) => (
              <li key={rule.id} className="flex flex-wrap items-center gap-3 py-3">
                <span className="font-medium">{item(rule)[0]}</span>
                <code className="text-xs">{item(rule)[1]}</code>
                <span className="text-sm">{ruleText(rule, t, language)}</span>
                <Button
                  className="ml-auto"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setEditing(rule)
                    setForm(filled(rule))
                  }}
                >
                  {t('rules.edit')}
                </Button>
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={async () => {
                    const yes = await confirm({
                      message: t('rules.confirmDelete'),
                      confirmLabel: t('rules.delete'),
                      destructive: true,
                    })
                    if (yes) onDelete(rule)
                  }}
                >
                  {t('rules.delete')}
                </Button>
              </li>
            ))}
          </ul>
        )}
        <form className="flex flex-wrap items-end gap-3 rounded-2xl border p-4" onSubmit={submit}>
          {editing && (
            <p className="w-full text-sm text-muted-foreground" data-testid="rule-editing">
              {t('rules.editing', { id: editing.id })}
            </p>
          )}
          <div className="space-y-2">
            <Label>{t('rules.target')}</Label>
            <ChoiceSelect
              label={t('rules.target')}
              disabled={editing !== null}
              value={form.target}
              onValueChange={(target) => setForm({ ...form, target: target as 'title' | 'don' })}
              items={[
                { value: 'title', label: t('rules.targets.title') },
                { value: 'don', label: t('rules.targets.don') },
              ]}
            />
          </div>
          {form.target === 'title' ? (
            <div className="space-y-2">
              <Label>{t('rules.title')}</Label>
              <ChoiceSelect
                label={t('rules.title')}
                disabled={editing !== null}
                value={form.titleId}
                onValueChange={(titleId) => setForm({ ...form, titleId })}
                items={eligible.map((title) => ({
                  value: title.id,
                  label: `${title.name} (${title.id})`,
                }))}
              />
            </div>
          ) : (
            <>
              <div className="space-y-2">
                <Label>{t('rules.donPart')}</Label>
                <ChoiceSelect
                  label={t('rules.donPart')}
                  disabled={editing !== null}
                  value={form.donPart}
                  onValueChange={(donPart) => setForm({ ...form, donPart: donPart as DonPartKind })}
                  items={donParts.map((part) => ({ value: part, label: t(`don.kinds.${part}`) }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="rule-don-item">{t('rules.donItem')}</Label>
                <Input
                  id="rule-don-item"
                  className="w-28"
                  type="number"
                  min={1}
                  disabled={editing !== null}
                  value={form.donItem}
                  onChange={(e) => setForm({ ...form, donItem: e.target.value })}
                  required
                />
                {Number(form.donItem) > 0 && (
                  <p className="text-xs text-muted-foreground">
                    {donName(form.donPart, Number(form.donItem))}
                  </p>
                )}
              </div>
            </>
          )}
          <div className="space-y-2">
            <Label>{t('rules.kind')}</Label>
            <ChoiceSelect
              label={t('rules.kind')}
              value={form.kind}
              onValueChange={(kind) => setForm({ ...form, kind: kind as Kind })}
              items={ruleKinds.map((kind) => ({ value: kind, label: t(`rules.kinds.${kind}`) }))}
            />
          </div>
          {sourced && (
            <div className="space-y-2">
              <Label>{t('rules.source')}</Label>
              <ChoiceSelect
                label={t('rules.source')}
                value={form.source}
                onValueChange={(source) =>
                  setForm({ ...form, source: source as SourceId, category: '' })
                }
                items={ruleSources.map((source) => ({
                  value: source,
                  label: t(`shell.${source}`),
                }))}
              />
            </div>
          )}
          {form.kind === 'chart_clear' && (
            <div className="space-y-2">
              <Label htmlFor="rule-chart">{t('rules.chart')}</Label>
              <Input
                id="rule-chart"
                className="w-80 font-mono"
                value={form.chart}
                onChange={(e) => setForm({ ...form, chart: e.target.value })}
                required
              />
            </div>
          )}
          {form.kind === 'dan_clear' && (
            <>
              <div className="space-y-2">
                <Label htmlFor="rule-dan">{t('rules.dan')}</Label>
                <Input
                  id="rule-dan"
                  className="w-80 font-mono"
                  value={form.dan}
                  onChange={(e) => setForm({ ...form, dan: e.target.value })}
                  required
                />
                <p className="max-w-80 text-xs text-muted-foreground">{t('rules.danHint')}</p>
              </div>
              <div className="space-y-2">
                <Label>{t('rules.verdict')}</Label>
                <ChoiceSelect
                  label={t('rules.verdict')}
                  value={danVerdicts.find((v) => v === form.clear) ?? '1'}
                  onValueChange={(clear) => setForm({ ...form, clear })}
                  items={danVerdicts.map((verdict) => ({
                    value: verdict,
                    label: t(`rules.verdicts.${verdict}`),
                  }))}
                />
              </div>
            </>
          )}
          {form.kind === 'category_clear' && (
            <div className="space-y-2">
              <Label>{t('rules.category')}</Label>
              <ChoiceSelect
                label={t('rules.category')}
                value={category}
                onValueChange={(category) => setForm({ ...form, category })}
                items={sourceCategories.map((category) => ({
                  value: category,
                  label: categoryName(t, form.source, category),
                }))}
              />
            </div>
          )}
          {plays && (
            <>
              <div className="space-y-2">
                <Label>{t('rules.course')}</Label>
                <ChoiceSelect
                  label={t('rules.course')}
                  value={form.course || 'any'}
                  onValueChange={(course) =>
                    setForm({ ...form, course: course === 'any' ? '' : course })
                  }
                  items={[
                    { value: 'any', label: t('rules.anyCourse') },
                    ...ruleCourses.map((course) => ({
                      value: course,
                      label: t(`courses.${course}`),
                    })),
                  ]}
                />
              </div>
              <div className="space-y-2">
                <Label>{t('rules.clear')}</Label>
                <ChoiceSelect
                  label={t('rules.clear')}
                  value={['1', '2', '3'].includes(form.clear) ? form.clear : '1'}
                  onValueChange={(clear) => setForm({ ...form, clear })}
                  items={(['1', '2', '3'] as const).map((clear) => ({
                    value: clear,
                    label: t(`rules.clears.${clear}`),
                  }))}
                />
              </div>
            </>
          )}
          {counted && (
            <div className="space-y-2">
              <Label htmlFor="rule-count">{t('rules.count')}</Label>
              <Input
                id="rule-count"
                className="w-36"
                type="number"
                min={1}
                max={100000}
                value={form.count}
                onChange={(e) => setForm({ ...form, count: e.target.value })}
                placeholder={form.kind === 'category_clear' ? t('rules.countAll') : undefined}
                required={form.kind === 'charts_uploaded'}
              />
            </div>
          )}
          <Button type="submit" disabled={!ready}>
            {editing ? t('rules.save') : t('rules.add')}
          </Button>
          {editing && (
            <Button type="button" variant="outline" onClick={stop}>
              {t('rules.cancel')}
            </Button>
          )}
        </form>
      </CardContent>
    </Card>
  )
}
