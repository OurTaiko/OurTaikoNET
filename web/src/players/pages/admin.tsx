import { useMemo, useState } from 'react'
import { Navigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { PencilSimpleIcon, PlusIcon, TrashIcon, UsersIcon } from '@phosphor-icons/react'
import { api, jsonRequest } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { ListedTitle } from '@players/api/types'
import { ChoiceSelect } from '@players/components/choice-select'
import { DonItemsCard } from '@players/components/don-items-card'
import { TitleBand } from '@players/components/nameplate'
import { NoteField, TitleFields, VisibilityMark } from '@players/components/title-form'
import { TitleReviews } from '@players/components/title-reviews'
import { RewardRulesCard } from '@players/components/reward-rules-card'
import { titleName } from '@players/dan'
import { useDonItems } from '@players/don-items'
import { Notice } from '@players/notice'
import { donCatalogQuery, invalidatePlayers, playerQueries } from '@players/queries'
import { useSession } from '@players/session'
import { draftBody, draftOf, emptyDraft, type TitleDraft } from '@players/title-draft'
import type { PublicUser } from '@sitekit/api/types'
import { UserSearch } from '@sitekit/collaborations'
import { PlayerLink } from '@sitekit/components/player-link'
import { SearchInput } from '@sitekit/components/search-input'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { DialogClose } from '@sitekit/components/ui/dialog'
import { Input } from '@sitekit/components/ui/input'
import { Label } from '@sitekit/components/ui/label'
import { Skeleton } from '@sitekit/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@sitekit/components/ui/tabs'
import { UserAvatar } from '@sitekit/components/user-avatar'
import { catalogItem, donItemName } from '@sitekit/don3d'
import { cn } from '@sitekit/lib/utils'
import { Modal } from '@sitekit/notifications'
import { useNotification } from '@sitekit/notification-context'
import { useTitles } from './titles'

const tabs = ['reviews', 'titles', 'grants', 'don'] as const
const titleId = /^[a-z0-9][a-z0-9-]{0,63}$/

/**
 * Does something on the server and reads everything again, which shows most of what was done;
 * `done` says what the page does not show by itself. A failure is told over the page, or, with
 * `quiet`, only answered, for a dialog to show in itself.
 */
function useRun() {
  const { notify } = useNotification()
  const [busy, setBusy] = useState(false)
  const run = async <T,>(
    work: () => Promise<T>,
    done?: string | ((result: T) => string),
    quiet = false,
  ) => {
    setBusy(true)
    try {
      const result = await work()
      await invalidatePlayers()
      if (done) notify(typeof done === 'string' ? done : done(result), 'success')
      return ''
    } catch (e) {
      const failure = (e as Error).message
      if (!quiet) notify(failure, 'error')
      return failure
    } finally {
      setBusy(false)
    }
  }
  return { busy, run }
}

/**
 * /manage/titles, for the title managers: the requests for review that players sent, every
 * title there is, what gives titles to players (rules, and by hand), and the parts of the Don
 * that have to be earned.
 */
export function AdminPage() {
  const { t } = useTranslation()
  const session = useSession()
  const [params, setParams] = useSearchParams()
  const tab = tabs.find((name) => name === params.get('tab')) ?? 'reviews'
  const waiting = useQuery({
    ...playerQueries.reviews('pending', 1),
    enabled: !!session.user?.isAdmin,
  })
  if (session.loading) return null
  if (!session.user?.isAdmin) return <Navigate to="/" replace />
  const count = waiting.data?.total ?? 0
  return (
    <div className="space-y-5">
      <h1 className="text-2xl font-extrabold tracking-tight sm:text-3xl">
        {t('shell.titleAdmin')}
      </h1>
      <Tabs
        value={tab}
        onValueChange={(value) =>
          setParams(value === 'reviews' ? {} : { tab: String(value) }, { replace: true })
        }
        className="gap-5"
      >
        <div className="-m-1 max-w-full overflow-x-auto p-1">
          <TabsList aria-label={t('shell.titleAdmin')}>
            <TabsTrigger value="reviews">
              {t('admin.tabs.reviews')}
              {count > 0 && (
                <span
                  className="rounded-full bg-primary px-1.5 text-[11px] leading-5 font-semibold text-primary-foreground tabular-nums"
                  data-testid="review-count"
                >
                  {count}
                </span>
              )}
            </TabsTrigger>
            <TabsTrigger value="titles">{t('admin.tabs.titles')}</TabsTrigger>
            <TabsTrigger value="grants">{t('admin.tabs.grants')}</TabsTrigger>
            <TabsTrigger value="don">{t('admin.tabs.don')}</TabsTrigger>
          </TabsList>
        </div>
        <TabsContent value="reviews">
          <TitleReviews />
        </TabsContent>
        <TabsContent value="titles">
          <AllTitles />
        </TabsContent>
        <TabsContent value="grants">
          <Grants />
        </TabsContent>
        <TabsContent value="don">
          <DonItems />
        </TabsContent>
      </Tabs>
    </div>
  )
}

type Shown = 'all' | 'public' | 'private'
// The title a manager is writing: a new one of the site's own, or a public one being changed.
type Editing = { title: ListedTitle | null }

/**
 * Every title: the public ones, which a manager changes or removes, and the players' private
 * ones, which are worn in public and so can be removed here, but are their makers' to change.
 */
function AllTitles() {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const { busy, run } = useRun()
  const { confirm } = useNotification()
  const read = useQuery(playerQueries.adminTitles())
  const titles = read.data?.titles
  const error = titles ? '' : (read.error?.message ?? '')
  const [query, setQuery] = useState('')
  // What is typed is searched for a moment later; the list waits dimmed meanwhile.
  const [typing, setTyping] = useState(false)
  const [show, setShow] = useState<Shown>('all')
  const [editing, setEditing] = useState<Editing | null>(null)
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  const shown = useMemo(() => {
    const text = query.trim().toLowerCase()
    return (titles ?? []).filter(
      (title) =>
        (show === 'all' || title.visibility === show) &&
        (!text ||
          [title.id, title.name, ...Object.values(title.nameTranslations), title.ownerName].some(
            (value) => value.toLowerCase().includes(text),
          )),
    )
  }, [titles, query, show])
  const remove = async (title: ListedTitle) => {
    const yes = await confirm({
      message: t('admin.confirmDelete', { name: title.name, count: title.holders }),
      confirmLabel: t('admin.delete'),
      destructive: true,
    })
    if (!yes) return
    void run(() =>
      api(endpoints.adminTitle(title.id), jsonRequest('DELETE', {}, session.csrfToken)),
    )
  }
  return (
    <Card className="rounded-3xl">
      <CardHeader className="flex flex-row flex-wrap items-center gap-3">
        <CardTitle className="mr-auto">
          {t('admin.catalog')}
          {titles && (
            <span className="ml-2 text-sm font-normal text-muted-foreground">{shown.length}</span>
          )}
        </CardTitle>
        <div className="flex min-w-0 flex-wrap gap-2">
          <div className="flex w-56 min-w-0">
            <SearchInput
              value={query}
              onSearch={setQuery}
              onPendingChange={setTyping}
              label={t('titles.search')}
              placeholder={t('admin.searchPlaceholder')}
            />
          </div>
          <ChoiceSelect
            label={t('admin.show.label')}
            value={show}
            onValueChange={(value) => setShow(value as Shown)}
            items={(['all', 'public', 'private'] as const).map((value) => ({
              value,
              label: t(`admin.show.${value}`),
            }))}
          />
          <Button disabled={busy} onClick={() => setEditing({ title: null })}>
            <PlusIcon aria-hidden="true" />
            {t('admin.newTitle')}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {error && <Notice>{error}</Notice>}
        {!titles ? (
          !error && <Skeleton className="h-40 w-full rounded-2xl" />
        ) : shown.length === 0 ? (
          <p className="py-8 text-center text-muted-foreground">{t('titles.noMatch')}</p>
        ) : (
          <ul className={cn('divide-y', typing && 'opacity-60')} data-testid="all-titles">
            {shown.map((title) => (
              <li key={title.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 py-3">
                <TitleBand title={title} className="h-6" />
                <div className="min-w-0 flex-1 basis-56 space-y-1 text-xs text-muted-foreground">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium text-foreground">
                      {titleName(title, language)}
                    </span>
                    <VisibilityMark title={title} pending={title.pending} />
                  </div>
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                    <code className="max-w-40 truncate" title={title.id}>
                      {title.id}
                    </code>
                    <span className="flex items-center gap-1">
                      <UsersIcon aria-hidden="true" className="size-3.5" />
                      {t('titles.holders', { count: title.holders })}
                    </span>
                    {title.ownerId && (
                      <span>
                        {t('titles.madeBy')}{' '}
                        <PlayerLink id={title.ownerId} className="text-link hover:underline">
                          {title.ownerName || t('profile.unknownPlayer')}
                        </PlayerLink>
                      </span>
                    )}
                  </div>
                </div>
                <div className="flex gap-2">
                  {title.visibility === 'public' && (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={busy}
                      onClick={() => setEditing({ title })}
                    >
                      <PencilSimpleIcon aria-hidden="true" />
                      {t('admin.edit')}
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant="destructive"
                    disabled={busy}
                    onClick={() => void remove(title)}
                  >
                    <TrashIcon aria-hidden="true" />
                    {t('admin.delete')}
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
      {editing && (
        <TitleDialog
          title={editing.title}
          busy={busy}
          taken={(id) => !!titles?.some((title) => title.id === id)}
          onDismiss={() => setEditing(null)}
          onSave={async (id, draft, obtain) => {
            const failure = await run(
              () =>
                api(
                  endpoints.adminTitle(id),
                  jsonRequest('PUT', { ...draftBody(draft), obtain }, session.csrfToken),
                ),
              undefined,
              true,
            )
            if (!failure) setEditing(null)
            return failure
          }}
        />
      )}
    </Card>
  )
}

/** A title of the site's own to make, or a public one to change: ID, words, look, how to get it. */
function TitleDialog({
  title,
  busy,
  taken,
  onDismiss,
  onSave,
}: {
  title: ListedTitle | null
  busy: boolean
  taken: (id: string) => boolean
  onDismiss: () => void
  /** Saves the title; answers what went wrong, or nothing. */
  onSave: (id: string, draft: TitleDraft, obtain: string) => Promise<string>
}) {
  const { t } = useTranslation()
  const [id, setId] = useState(title?.id ?? '')
  const [draft, setDraft] = useState<TitleDraft>(title ? draftOf(title) : emptyDraft)
  const [obtain, setObtain] = useState(title?.obtain ?? '')
  const [error, setError] = useState('')
  // A new title needs an ID of its own, or saving would write over the title that has it.
  const idProblem = title
    ? ''
    : id && !titleId.test(id)
      ? t('admin.idInvalid')
      : taken(id)
        ? t('admin.idTaken')
        : ''
  return (
    <Modal
      title={title ? t('admin.editTitle') : t('admin.newTitle')}
      busy={busy}
      onDismiss={onDismiss}
      className="sm:max-w-2xl"
      actions={
        <>
          <DialogClose render={<Button type="button" variant="outline" />} disabled={busy}>
            {t('titles.cancel')}
          </DialogClose>
          <Button
            disabled={busy || !id || !!idProblem || !draft.name.trim()}
            onClick={() => void onSave(id, draft, obtain).then(setError)}
          >
            {t('admin.save')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {error && <Notice>{error}</Notice>}
        {!title && (
          <div className="space-y-2">
            <Label htmlFor="title-id">{t('admin.id')}</Label>
            <Input
              id="title-id"
              className="font-mono"
              value={id}
              onChange={(e) => setId(e.target.value.trim())}
              placeholder="taiko-master"
              aria-invalid={!!idProblem}
              disabled={busy}
              required
            />
            <p className={idProblem ? 'text-xs text-destructive' : 'text-xs text-muted-foreground'}>
              {idProblem || t('admin.idHint')}
            </p>
          </div>
        )}
        <TitleFields draft={draft} onChange={setDraft} prefix="title" disabled={busy} />
        <NoteField
          id="title-obtain"
          label={t('admin.obtain')}
          hint={t('admin.obtainHint')}
          value={obtain}
          onChange={setObtain}
          disabled={busy}
        />
      </div>
    </Modal>
  )
}

/** What gives titles to players: the rules that give by themselves, and a manager's own hand. */
function Grants() {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const { busy, run } = useRun()
  const { titles, rules, categories, error } = useTitles()
  const catalog = useQuery(donCatalogQuery).data ?? null
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  const [player, setPlayer] = useState<PublicUser | null>(null)
  const [title, setTitle] = useState('')
  const send = (method: 'POST' | 'DELETE') =>
    void run(
      () =>
        api(
          endpoints.adminGrants,
          jsonRequest(method, { userId: player?.id, titleId: title }, session.csrfToken),
        ),
      method === 'POST' ? t('admin.granted') : t('admin.revoked'),
    )
  return (
    <div className="space-y-6">
      {error && <Notice>{error}</Notice>}
      <RewardRulesCard
        titles={titles ?? []}
        rules={rules}
        categories={categories}
        donName={(part, id) =>
          donItemName(catalog ? catalogItem(catalog, part, id) : undefined, language, id)
        }
        onAdd={(target, rule) =>
          run(
            () =>
              api<{ granted: number }>(
                'title' in target
                  ? endpoints.adminTitleRules(target.title)
                  : endpoints.adminDonRules(target.donPart, target.donItem),
                jsonRequest('POST', rule, session.csrfToken),
              ),
            (result) => t('rules.added', { n: result.granted }),
          )
        }
        onUpdate={(rule, draft) =>
          run(
            () =>
              api<{ granted: number }>(
                endpoints.adminRule(rule.id),
                jsonRequest('PUT', draft, session.csrfToken),
              ),
            (result) => t('rules.updated', { n: result.granted }),
          )
        }
        onDelete={(rule) =>
          void run(() =>
            api(endpoints.adminRule(rule.id), jsonRequest('DELETE', {}, session.csrfToken)),
          )
        }
      />
      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>{t('admin.grant')}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-5 lg:grid-cols-2">
          <div className="space-y-3">
            <p className="text-sm font-medium">{t('admin.grantPlayer')}</p>
            <UserSearch
              pressed={(id) => player?.id === id}
              note={() => null}
              disabled={busy}
              onChoose={setPlayer}
            />
          </div>
          <div className="space-y-4">
            <p className="text-sm text-muted-foreground">{t('admin.grantHint')}</p>
            <div className="flex min-h-12 items-center gap-3 rounded-xl border px-3 py-2">
              {player ? (
                <>
                  <UserAvatar
                    nickname={player.nickname || '?'}
                    avatarPath={player.avatarPath}
                    className="size-8"
                  />
                  <PlayerLink
                    id={player.id}
                    className="min-w-0 truncate font-medium hover:underline"
                  >
                    {player.nickname || t('profile.unknownPlayer')}
                  </PlayerLink>
                </>
              ) : (
                <span className="text-sm text-muted-foreground">{t('admin.noPlayer')}</span>
              )}
            </div>
            <ChoiceSelect
              label={t('admin.title')}
              value={title}
              onValueChange={setTitle}
              items={(titles ?? []).map((item) => ({
                value: item.id,
                label: titleName(item, language),
              }))}
            />
            <div className="flex flex-wrap gap-2">
              <Button disabled={busy || !player || !title} onClick={() => send('POST')}>
                {t('admin.grantButton')}
              </Button>
              <Button
                variant="outline"
                disabled={busy || !player || !title}
                onClick={() => send('DELETE')}
              >
                {t('admin.revokeButton')}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

/** The parts of the Don that have to be earned: which are locked, and who was given one. */
function DonItems() {
  const { t } = useTranslation()
  const session = useSession()
  const { run } = useRun()
  // The Don's parts: the catalog names them, the backend says which are locked.
  const catalog = useQuery(donCatalogQuery).data ?? null
  const donItems = useDonItems(false)
  return (
    <DonItemsCard
      catalog={catalog}
      items={donItems}
      onLock={(part, id, locked) =>
        void run(
          () =>
            api<{ given: number }>(
              endpoints.adminDonLock(part, id),
              jsonRequest('PUT', { locked }, session.csrfToken),
            ),
          (result) =>
            locked ? t('donItems.lockedDone', { n: result.given }) : t('donItems.unlockedDone'),
        )
      }
      onGrant={(part, id, userId, give) =>
        void run(
          () =>
            api(
              endpoints.adminDonGrants,
              jsonRequest(give ? 'POST' : 'DELETE', { userId, part, id }, session.csrfToken),
            ),
          give ? t('admin.granted') : t('admin.revoked'),
        )
      }
    />
  )
}
