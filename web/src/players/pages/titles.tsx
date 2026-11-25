import { useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { api, jsonRequest } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { Title, TitleRule } from '@players/api/types'
import { Nameplate } from '@players/components/nameplate'
import { TitleLayersPicker } from '@players/components/title-layers-picker'
import { plainLook, type TitleLayers } from '@sitekit/nameplate-layers'
import { Badge } from '@sitekit/components/ui/badge'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Input } from '@sitekit/components/ui/input'
import { Label } from '@sitekit/components/ui/label'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { Notice } from '@players/notice'
import { invalidatePlayers, playerQueries } from '@players/queries'
import { useSession } from '@players/session'
import { titleName } from '@players/dan'
import { ruleText } from '@players/title-rules'

// A player keeps at most this many community titles (official ones do not count).
const titleLimit = 5
const languages = ['en', 'ja', 'ko'] as const
const noRules: TitleRule[] = []

export function useTitles() {
  const { data, error } = useQuery(playerQueries.titles())
  return {
    titles: data?.titles ?? null,
    rules: data?.rules ?? noRules,
    // What was read before stays on show when reading it again fails.
    error: data ? '' : (error?.message ?? ''),
  }
}

type Draft = {
  id: string | null
  official: boolean
  name: string
  description: string
  layers: TitleLayers
  en: string
  ja: string
  ko: string
}
const emptyDraft: Draft = {
  id: null,
  official: false,
  name: '',
  description: '',
  layers: plainLook,
  en: '',
  ja: '',
  ko: '',
}

// The titles the player made for their own 段位 courses: up to five community ones, which they
// change or delete; of the ones an administrator made official they change the look only.
function MyTitles({ titles }: { titles: Title[] }) {
  const { t } = useTranslation()
  const session = useSession()
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ kind: 'error' | 'info'; text: string } | null>(null)
  const user = session.user
  if (!user) return null
  const mine = titles.filter((title) => title.ownerId === user.id)
  const community = mine.filter((title) => !title.official).length
  const run = async (work: () => Promise<unknown>, done: string) => {
    setBusy(true)
    setMessage(null)
    try {
      await work()
      setMessage({ kind: 'info', text: done })
      setDraft(emptyDraft)
      void invalidatePlayers()
    } catch (e) {
      setMessage({ kind: 'error', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }
  const save = (event: FormEvent) => {
    event.preventDefault()
    const nameTranslations = Object.fromEntries(
      languages.filter((l) => draft[l].trim()).map((l) => [l, draft[l].trim()]),
    )
    const body = {
      name: draft.name,
      nameTranslations,
      description: draft.description,
      layers: draft.layers,
    }
    void run(
      () =>
        api(
          draft.id ? endpoints.myTitle(draft.id) : endpoints.myTitles,
          jsonRequest(draft.id ? 'PUT' : 'POST', body, session.csrfToken),
        ),
      t('titles.saved'),
    )
  }
  const editing = draft.id !== null
  const full = !editing && community >= titleLimit
  return (
    <Card className="rounded-3xl">
      <CardHeader>
        <CardTitle>{t('titles.mine')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        {message && <Notice kind={message.kind}>{message.text}</Notice>}
        <p className="text-sm text-muted-foreground" data-testid="title-quota">
          {t('titles.quota', { used: community, limit: titleLimit })}
        </p>
        {mine.length > 0 && (
          <ul className="divide-y rounded-2xl border">
            {mine.map((title) => (
              <li key={title.id} className="flex flex-wrap items-center gap-3 p-3">
                <Nameplate
                  plate={{
                    userId: user.id,
                    nickname: user.nickname,
                    avatarPath: '',
                    title,
                    dan: null,
                    showDan: false,
                  }}
                  className="h-16 max-w-xs [&_.text-2xl]:text-lg"
                />
                <Badge variant={title.official ? 'default' : 'secondary'}>
                  {title.official ? t('titles.official') : t('titles.community')}
                </Badge>
                <div className="ml-auto flex gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => {
                      setMessage(null)
                      setDraft({
                        id: title.id,
                        official: title.official,
                        name: title.name,
                        description: title.description,
                        layers: title.layers,
                        en: title.nameTranslations.en ?? '',
                        ja: title.nameTranslations.ja ?? '',
                        ko: title.nameTranslations.ko ?? '',
                      })
                    }}
                  >
                    {title.official ? t('titles.changeBand') : t('titles.edit')}
                  </Button>
                  {!title.official && (
                    <Button
                      size="sm"
                      variant="destructive"
                      disabled={busy}
                      onClick={() => {
                        if (window.confirm(t('titles.confirmDelete', { name: title.name })))
                          void run(
                            () =>
                              api(endpoints.myTitle(title.id), {
                                ...jsonRequest('DELETE', {}, session.csrfToken),
                              }),
                            t('titles.deleted'),
                          )
                      }}
                    >
                      {t('titles.delete')}
                    </Button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
        <form className="grid gap-4 rounded-2xl border p-4 sm:grid-cols-2" onSubmit={save}>
          <h3 className="font-semibold sm:col-span-2">
            {editing ? t('titles.editing') : t('titles.create')}
          </h3>
          {draft.official && (
            <p className="text-sm text-muted-foreground sm:col-span-2">
              {t('titles.officialBandOnly')}
            </p>
          )}
          {full && (
            <p className="text-sm text-muted-foreground sm:col-span-2">
              {t('titles.limitReached', { limit: titleLimit })}
            </p>
          )}
          <div className="space-y-2">
            <Label htmlFor="my-title-name">{t('admin.name')}</Label>
            <Input
              id="my-title-name"
              value={draft.name}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              maxLength={40}
              disabled={draft.official || full}
              required
            />
          </div>
          {languages.map((language) => (
            <div className="space-y-2" key={language}>
              <Label htmlFor={`my-title-${language}`}>
                {t('admin.nameIn', { language: t(`languages.${language}`) })}
              </Label>
              <Input
                id={`my-title-${language}`}
                value={draft[language]}
                onChange={(e) => setDraft({ ...draft, [language]: e.target.value })}
                maxLength={40}
                disabled={draft.official || full}
              />
            </div>
          ))}
          <div className="space-y-2 sm:col-span-2">
            <Label htmlFor="my-title-description">{t('admin.description')}</Label>
            <Input
              id="my-title-description"
              value={draft.description}
              onChange={(e) => setDraft({ ...draft, description: e.target.value })}
              maxLength={500}
              disabled={draft.official || full}
            />
          </div>
          <div className="sm:col-span-2">
            <TitleLayersPicker
              value={draft.layers}
              onChange={(layers) => setDraft({ ...draft, layers })}
              name={draft.name || '—'}
              disabled={full}
            />
          </div>
          <div className="sm:col-span-2">
            <Nameplate
              plate={{
                userId: user.id,
                nickname: user.nickname,
                avatarPath: '',
                dan: null,
                showDan: false,
                title: {
                  id: draft.id ?? '',
                  name: draft.name || '—',
                  nameTranslations: {},
                  description: '',
                  style: 'wood',
                  layers: draft.layers,
                  official: draft.official,
                  ownerId: user.id,
                },
              }}
            />
          </div>
          <div className="flex gap-2 sm:col-span-2">
            <Button type="submit" disabled={busy || full}>
              {editing ? t('titles.save') : t('titles.createButton')}
            </Button>
            {editing && (
              <Button type="button" variant="ghost" onClick={() => setDraft(emptyDraft)}>
                {t('titles.cancel')}
              </Button>
            )}
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

// The catalog: every title there is, as it looks on a nameplate. Signed in, the player's own
// titles come first.
export function TitlesPage() {
  const { t, i18n } = useTranslation()
  const { titles, rules, error } = useTitles()
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  return (
    <div className="space-y-6">
      {titles && <MyTitles titles={titles} />}
      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>{t('titles.catalog')}</CardTitle>
        </CardHeader>
        <CardContent>
          {error && <Notice>{error}</Notice>}
          {!titles ? (
            <Skeleton className="h-48 w-full" />
          ) : titles.length === 0 ? (
            <p className="py-8 text-center text-muted-foreground">{t('titles.catalogEmpty')}</p>
          ) : (
            <ul className="grid gap-3 sm:grid-cols-2">
              {titles.map((title) => (
                <li key={title.id} className="space-y-2 rounded-2xl border p-4">
                  <Nameplate
                    plate={{
                      userId: '',
                      nickname: titleName(title, language),
                      avatarPath: '',
                      title,
                      dan: null,
                      showDan: false,
                    }}
                    className="h-16 [&_.text-2xl]:hidden"
                  />
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Badge variant={title.official ? 'default' : 'secondary'}>
                      {title.official ? t('titles.official') : t('titles.community')}
                    </Badge>
                    <code>{title.id}</code>
                  </div>
                  {title.description && <p className="text-sm">{title.description}</p>}
                  {rules.some((rule) => rule.titleId === title.id) && (
                    <div className="text-sm" data-testid="title-rules">
                      <p className="text-xs text-muted-foreground">{t('rules.howTo')}</p>
                      <ul className="list-disc pl-5">
                        {rules
                          .filter((rule) => rule.titleId === title.id)
                          .map((rule) => (
                            <li key={rule.id}>{ruleText(rule, t, language)}</li>
                          ))}
                      </ul>
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
