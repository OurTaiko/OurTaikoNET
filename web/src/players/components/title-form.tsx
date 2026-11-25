import { useTranslation } from 'react-i18next'
import { GlobeHemisphereWestIcon, HourglassMediumIcon, LockSimpleIcon } from '@phosphor-icons/react'
import type { Title } from '@players/api/types'
import { languages, noteLimit, type TitleDraft } from '@players/title-draft'
import { TitleLayersPicker } from '@players/components/title-layers-picker'
import { Badge } from '@sitekit/components/ui/badge'
import { Input } from '@sitekit/components/ui/input'
import { Label } from '@sitekit/components/ui/label'
import { Textarea } from '@sitekit/components/ui/textarea'

// What every form that writes a title shares: the fields of its draft (../title-draft), a text
// that goes with a review, and the marks that say what state a title is in.

/** The fields of a title: its name, the name in other languages, description and look. */
export function TitleFields({
  draft,
  onChange,
  prefix,
  disabled = false,
}: {
  draft: TitleDraft
  onChange: (draft: TitleDraft) => void
  /** Sets the fields' ids apart from another form's on the page. */
  prefix: string
  disabled?: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-name`}>{t('admin.name')}</Label>
        <Input
          id={`${prefix}-name`}
          value={draft.name}
          onChange={(e) => onChange({ ...draft, name: e.target.value })}
          maxLength={40}
          disabled={disabled}
          required
        />
      </div>
      {languages.map((language) => (
        <div className="space-y-2" key={language}>
          <Label htmlFor={`${prefix}-${language}`}>
            {t('admin.nameIn', { language: t(`languages.${language}`) })}
          </Label>
          <Input
            id={`${prefix}-${language}`}
            value={draft[language]}
            onChange={(e) => onChange({ ...draft, [language]: e.target.value })}
            maxLength={40}
            disabled={disabled}
          />
        </div>
      ))}
      <div className="space-y-2 sm:col-span-2">
        <Label htmlFor={`${prefix}-description`}>{t('admin.description')}</Label>
        <Input
          id={`${prefix}-description`}
          value={draft.description}
          onChange={(e) => onChange({ ...draft, description: e.target.value })}
          maxLength={500}
          disabled={disabled}
        />
      </div>
      <div className="sm:col-span-2">
        <TitleLayersPicker
          value={draft.layers}
          onChange={(layers) => onChange({ ...draft, layers })}
          name={draft.name.trim() || '—'}
          disabled={disabled}
        />
      </div>
    </div>
  )
}

/** A text of a review (what is asked, the reply, how a title is obtained), with its count. */
export function NoteField({
  id,
  label,
  hint,
  value,
  onChange,
  required = false,
  disabled = false,
}: {
  id: string
  label: string
  hint?: string
  value: string
  onChange: (value: string) => void
  required?: boolean
  disabled?: boolean
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-baseline justify-between gap-3">
        <Label htmlFor={id}>{label}</Label>
        <span className="text-xs text-muted-foreground tabular-nums">
          {[...value].length} / {noteLimit}
        </span>
      </div>
      <Textarea
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        maxLength={noteLimit}
        rows={3}
        required={required}
        disabled={disabled}
      />
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

/** Whether a title is private or public, and whether a review of it waits. */
export function VisibilityMark({
  title,
  pending = false,
}: {
  title: Pick<Title, 'visibility'>
  pending?: boolean
}) {
  const { t } = useTranslation()
  return (
    <>
      <Badge variant={title.visibility === 'public' ? 'default' : 'secondary'}>
        {title.visibility === 'public' ? (
          <GlobeHemisphereWestIcon aria-hidden="true" />
        ) : (
          <LockSimpleIcon aria-hidden="true" />
        )}
        {t(`titles.visibility.${title.visibility}`)}
      </Badge>
      {pending && (
        <Badge variant="outline">
          <HourglassMediumIcon aria-hidden="true" />
          {t('titles.review.pending')}
        </Badge>
      )}
    </>
  )
}
