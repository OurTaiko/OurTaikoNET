import { useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { Title, TitleRule } from '@players/api/types'
import type { DonPartKind } from '@sitekit/don3d'
import { ChoiceSelect } from '@players/components/choice-select'
import { titleName } from '@players/dan'
import {
  categoryName,
  chartIdOf,
  ruleCategories,
  ruleCourses,
  ruleKinds,
  ruleSources,
  ruleText,
} from '@players/title-rules'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Input } from '@sitekit/components/ui/input'
import { Label } from '@sitekit/components/ui/label'

type Kind = TitleRule['kind']
type SourceId = (typeof ruleSources)[number]
/** A rule as the backend takes it (POST /api/v1/admin/titles/{id}/rules). */
export type RuleDraft = {
  kind: Kind
  source?: SourceId
  chartId?: string
  category?: string
  course?: string
  clear?: number
  count?: number
}

/** What a rule gives: a title, or a part of the 3D Don and its number. */
export type RuleTarget = { title: string } | { donPart: DonPartKind; donItem: number }

const donParts: DonPartKind[] = ['cos', 'head', 'body', 'puchi', 'paint']

// What gives an item by itself: the rules there are, and a form for one more. The item is a title
// (only the catalog's own, not the ones players made) or a part of the Don.
export function TitleRulesCard({
  titles,
  rules,
  donName,
  onAdd,
  onDelete,
}: {
  titles: Title[]
  rules: TitleRule[]
  // A Don part's name, from the catalog.
  donName: (part: DonPartKind, id: number) => string
  onAdd: (target: RuleTarget, rule: RuleDraft) => void
  onDelete: (rule: TitleRule) => void
}) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  const [form, setForm] = useState({
    target: 'title' as 'title' | 'don',
    titleId: '',
    donPart: 'cos' as DonPartKind,
    donItem: '',
    kind: 'chart_clear' as Kind,
    source: 'fanmade' as SourceId,
    chart: '',
    category: ruleCategories.fanmade[0],
    course: '',
    clear: '1',
    count: '',
  })
  const eligible = titles.filter((title) => !title.ownerId)
  const plays = form.kind === 'chart_clear' || form.kind === 'category_clear'
  const sourced = plays || form.kind === 'charts_uploaded'
  const counted = form.kind === 'category_clear' || form.kind === 'charts_uploaded'
  const submit = (event: FormEvent) => {
    event.preventDefault()
    const rule: RuleDraft = { kind: form.kind }
    if (sourced) rule.source = form.source
    if (form.kind === 'chart_clear') rule.chartId = chartIdOf(form.chart)
    if (form.kind === 'category_clear') rule.category = form.category
    if (plays) {
      rule.clear = Number(form.clear)
      if (form.course) rule.course = form.course
    }
    if (counted && form.count.trim()) rule.count = Number(form.count)
    onAdd(
      form.target === 'title'
        ? { title: form.titleId }
        : { donPart: form.donPart, donItem: Number(form.donItem) },
      rule,
    )
  }
  // What a rule gives, by name, and its key.
  const item = (rule: TitleRule): [string, string] => {
    if (rule.donPart && rule.donItem !== null)
      return [
        `${t(`don.kinds.${rule.donPart}`)} · ${donName(rule.donPart, rule.donItem)}`,
        `${rule.donPart}:${rule.donItem}`,
      ]
    const title = titles.find((candidate) => candidate.id === rule.titleId)
    return [title ? titleName(title, language) : (rule.titleId ?? ''), rule.titleId ?? '']
  }
  const ready =
    form.target === 'title'
      ? form.titleId !== ''
      : Number.isInteger(Number(form.donItem)) && Number(form.donItem) > 0
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
                  variant="destructive"
                  onClick={() => {
                    if (window.confirm(t('rules.confirmDelete'))) onDelete(rule)
                  }}
                >
                  {t('rules.delete')}
                </Button>
              </li>
            ))}
          </ul>
        )}
        <form className="flex flex-wrap items-end gap-3 rounded-2xl border p-4" onSubmit={submit}>
          <div className="space-y-2">
            <Label>{t('rules.target')}</Label>
            <ChoiceSelect
              label={t('rules.target')}
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
                  setForm({
                    ...form,
                    source: source as SourceId,
                    category: ruleCategories[source as SourceId][0],
                  })
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
          {form.kind === 'category_clear' && (
            <div className="space-y-2">
              <Label>{t('rules.category')}</Label>
              <ChoiceSelect
                label={t('rules.category')}
                value={form.category}
                onValueChange={(category) => setForm({ ...form, category })}
                items={ruleCategories[form.source].map((category) => ({
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
                  value={form.clear}
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
            {t('rules.add')}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
