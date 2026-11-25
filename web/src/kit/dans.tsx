import { useCallback, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { slotFromKey, slotKey } from './courses'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useQueries, useQuery } from '@tanstack/react-query'
import { CommentSection, danSubject } from './comments'
import { danTitle } from './dan-title'
export { danTitle }
import { VoteControl, VoteCounts } from './components/vote-control'
import {
  ChatCircleIcon,
  ArrowClockwiseIcon,
  MedalIcon,
  PencilSimpleIcon,
  PlusIcon,
  SealCheckIcon,
  TrashIcon,
  WarningIcon,
  XIcon,
} from '@phosphor-icons/react'
import { api, jsonRequest } from './api/client'
import { endpoints } from './api/endpoints'
import type {
  ChartBase,
  Dan,
  DanExam,
  DanInput,
  DanRewardTitle,
  Vote,
  VoteResult,
} from './api/types'
import { chartText } from './chart-language'
import { ChoiceSelect } from './components/choice-select'
import { SearchInput } from './components/search-input'
import { Badge } from './components/ui/badge'
import { Button, buttonVariants } from './components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from './components/ui/card'
import { Checkbox } from './components/ui/checkbox'
import { Input } from './components/ui/input'
import { Label } from './components/ui/label'
import { Modal, Notice } from './notifications'
import { TitleBand, profileTitlesPage, titleText, useProfileTitles } from './profile-titles'
import { useSession } from './session'
import { cn } from './lib/utils'
import { SourceTag } from './components/source-tag'
import { LevelBox } from './components/difficulty-badges'
import { SOURCE } from './source'
import { usePageMeta } from './head'
import { chartQueries } from './queries/charts'
import { danKeys, danQueries, invalidateDanList, invalidateDans } from './queries/dans'
import { queryClient } from './query/client'

// 段位 courses as OurTaikoPlay plays them (one to three songs with conditions). The game reads
// them from this server; passing an official one gives a 段位 badge on OurTaikoProfile.

export const danNames = [
  '初級',
  '十級',
  '九級',
  '八級',
  '七級',
  '六級',
  '五級',
  '四級',
  '三級',
  '二級',
  '一級',
  '初段',
  '二段',
  '三段',
  '四段',
  '五段',
  '六段',
  '七段',
  '八段',
  '九段',
  '十段',
  '玄人',
  '名人',
  '超人',
  '達人',
] as const
const titleLanguages = ['ja', 'en', 'zh', 'ko'] as const
const examTypes = ['gauge', 'good', 'ok', 'bad', 'hits', 'score', 'combo', 'rolls'] as const
const rewardLevels = ['red', 'red2', 'red3', 'gold', 'gold5', 'gold6'] as const
const courses = ['Easy', 'Normal', 'Hard', 'Oni', 'Edit'] as const
// The dan box colours of the Nijiiro skin's dan_select/box/folder, which the game draws the
// course's box, its yellow board and its loading plaque with. The skin's 1 is the same wood
// as 0, so it is not offered (the backend refuses it too).
const danColors = [
  { value: 0, name: 'wood', hex: '#FCCB73' },
  { value: 2, name: 'blue', hex: '#47A7B8' },
  { value: 3, name: 'red', hex: '#F15134' },
  { value: 4, name: 'silver', hex: '#D9DEE5' },
  { value: 5, name: 'gold', hex: '#DCC15F' },
  { value: 6, name: 'green', hex: '#499875' },
] as const

function ColorSwatch({ hex }: { hex: string }) {
  return (
    <span
      aria-hidden
      className="inline-block size-3.5 shrink-0 rounded-sm border border-black/20"
      style={{ backgroundColor: hex }}
    />
  )
}
// The game offers 五級 (6) .. 達人 (24); 0–5 exist in the numbering only.
const danFirst = 6

function Line({ value }: { value: number[] | null }) {
  const { t } = useTranslation()
  if (!value) return <span className="text-muted-foreground">—</span>
  return (
    <span className="tabular-nums">
      <span className="text-red-600 dark:text-red-400">{value[0]}</span>
      {value.length > 1 && (
        <>
          {' / '}
          <span className="text-amber-600 dark:text-amber-400">{value[1]}</span>
        </>
      )}
      <span className="sr-only">{value.length > 1 ? t('dans.redGold') : t('dans.red')}</span>
    </span>
  )
}

function ExamTable({ dan }: { dan: Dan }) {
  const { t } = useTranslation()
  return (
    <table className="w-full text-sm">
      <thead className="text-left text-muted-foreground">
        <tr>
          <th className="py-1 font-normal">{t('dans.exam')}</th>
          <th className="py-1 font-normal">{t('dans.whole')}</th>
          {dan.songs.map((_, i) => (
            <th key={i} className="py-1 font-normal">
              {t('dans.songNumber', { number: i + 1 })}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {dan.exams.map((exam, i) => {
          const perSong = Array.isArray(exam.value[0]) || exam.value[0] === null
          return (
            <tr key={i} className="border-t">
              <td className="py-2">
                {t(`dans.examTypes.${exam.type}`)} {t(`dans.ranges.${exam.range}`)}
              </td>
              <td className="py-2">{perSong ? '—' : <Line value={exam.value as number[]} />}</td>
              {dan.songs.map((_, song) => (
                <td key={song} className="py-2">
                  {perSong ? (
                    <Line value={(exam.value as (number[] | null)[])[song] ?? null} />
                  ) : (
                    '—'
                  )}
                </td>
              ))}
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

// Garbled text that stands in for a hidden title: the same for a song every time, and its
// length says nothing about the real title.
const garbleGlyphs = '縺繧譁蟄怜喧縺代ｦ繝ｼ繧ｹ蜷榊燕螟ｱ謨鈴擇蛻'
function garble(seed: string) {
  let h = 2166136261
  for (const c of seed) h = Math.imul(h ^ c.charCodeAt(0), 16777619)
  let text = ''
  for (let i = 0; i < 8; i++) {
    // mulberry32
    h = (h + 0x6d2b79f5) | 0
    let x = Math.imul(h ^ (h >>> 15), h | 1)
    x ^= x + Math.imul(x ^ (x >>> 7), x | 61)
    text += garbleGlyphs[((x ^ (x >>> 14)) >>> 0) % garbleGlyphs.length]
  }
  return text
}

function HiddenTitle({ seed }: { seed: string }) {
  const { t } = useTranslation()
  return (
    <span className="font-medium text-muted-foreground" aria-label={t('dans.hiddenTitle')}>
      <span aria-hidden="true">{garble(seed)}</span>
    </span>
  )
}

// A song's place in a course: the course and side in words, then its level box.
function SongLevel({ song }: { song: Dan['songs'][number] }) {
  const { t } = useTranslation()
  return (
    <span className="ml-auto inline-flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
      {t(`dans.courses.${song.chart.course}`)}
      {song.chart.side !== 'single' && ` · ${t(`dans.sides.${song.chart.side}`)}`}
      <LevelBox course={song.chart.course} level={song.level} name={null} />
    </span>
  )
}

function DanCard({ dan }: { dan: Dan }) {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const isOwner = session.user?.id === dan.ownerId
  const unavailable = dan.songs.some((s) => !s.available)
  return (
    <Link
      to={`/dans/${dan.id}`}
      className="flex flex-col gap-4 rounded-[20px] bg-card p-5 shadow-elev-1 transition-[translate,box-shadow] duration-200 ease-emphasized outline-none hover:-translate-y-0.5 hover:shadow-elev-2 focus-visible:ring-3 focus-visible:ring-ring focus-visible:ring-offset-2 motion-reduce:transform-none motion-reduce:transition-none"
    >
      <div className="flex items-start gap-3">
        {/* The colour of the course's box in the game. */}
        <span
          aria-hidden="true"
          className="flex size-11 shrink-0 items-center justify-center rounded-[14px] text-black/60"
          style={{ background: (danColors.find((c) => c.value === dan.color) ?? danColors[0]).hex }}
        >
          <MedalIcon className="size-5" />
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-lg font-extrabold tracking-tight">
            {danTitle(dan.title, i18n.resolvedLanguage)}
          </h2>
          <p className="truncate text-xs text-muted-foreground">
            {dan.owner || t('dans.unknownOwner')}
            <span aria-hidden="true"> · </span>
            <span className="inline-flex gap-2 align-middle tabular-nums">
              <VoteCounts upvotes={dan.upvotes} downvotes={dan.downvotes} />
            </span>
            <span aria-hidden="true"> · </span>
            {t('interactions.commentCount', { count: dan.commentCount })}
          </p>
        </div>
        <div className="flex shrink-0 gap-1">
          {dan.official && (
            <Badge>
              <SealCheckIcon aria-hidden="true" />
              {t('dans.official')}
            </Badge>
          )}
          {unavailable && (
            <Badge variant="destructive">
              <WarningIcon aria-hidden="true" />
              {t('dans.unavailable')}
            </Badge>
          )}
        </div>
      </div>
      <ol className="space-y-1.5 text-sm">
        {dan.songs.map((song, i) => (
          <li key={i} className="flex min-w-0 items-center gap-2">
            <span className="w-5 shrink-0 text-muted-foreground tabular-nums">{i + 1}</span>
            {song.hidden && !isOwner ? (
              <span className="truncate">
                <HiddenTitle seed={`${dan.id}/${i}`} />
              </span>
            ) : (
              <span className="truncate font-medium">
                {chartText(song, i18n.resolvedLanguage).title}
              </span>
            )}
            {song.hidden && (
              <Badge variant="outline" className="shrink-0">
                {t('dans.hiddenTitle')}
              </Badge>
            )}
            <SongLevel song={song} />
          </li>
        ))}
      </ol>
    </Link>
  )
}

export function DansPage() {
  const { t } = useTranslation()
  const session = useSession()
  const list = useQuery(danQueries.list())
  const dans = list.data
  // What was read before stays on show when reading it again fails.
  const error = dans ? '' : (list.error?.message ?? '')
  usePageMeta({ title: t('dans.heading'), description: t('meta.dans') })
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="space-y-1.5">
          <h1 className="text-4xl leading-[1.1] font-extrabold tracking-tight sm:text-5xl">
            {t('dans.heading')}
          </h1>
        </div>
        {session.user && (
          <Link className={buttonVariants()} to="/dans/new">
            <PlusIcon aria-hidden="true" />
            {t('dans.create')}
          </Link>
        )}
      </div>
      {error && <Notice>{error}</Notice>}
      {!dans && !error && <p role="status">{t('dans.loading')}</p>}
      {dans?.length === 0 && (
        <p className="rounded-[28px] bg-muted py-12 text-center text-muted-foreground">
          {t('dans.empty')}
        </p>
      )}
      <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
        {dans?.map((dan) => (
          <DanCard key={dan.id} dan={dan} />
        ))}
      </div>
    </div>
  )
}

export function DanPage() {
  const { id = '' } = useParams()
  // What went wrong on one course is not carried to the next.
  return <DanView key={id} id={id} />
}

function DanView({ id }: { id: string }) {
  const { t, i18n } = useTranslation()
  const { commentId } = useParams()
  const session = useSession()
  const navigate = useNavigate()
  const [actionError, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [deleted, setDeleted] = useState(false)
  const { titles: profileTitles } = useProfileTitles()
  const read = useQuery({ ...danQueries.detail(id), enabled: !deleted })
  const dan = read.data ?? null
  // What was read before stays on show when reading it again fails.
  const error = actionError || (dan ? '' : (read.error?.message ?? ''))
  const setDan = (change: (current: Dan) => Dan) => {
    queryClient.setQueryData<Dan>(danKeys.detail(id), (current) => current && change(current))
    void invalidateDanList()
  }
  const metaTitle = dan ? danTitle(dan.title, i18n.resolvedLanguage) : undefined
  usePageMeta({
    title: metaTitle,
    description: dan
      ? t('meta.dan', {
          title: metaTitle,
          owner: dan.owner || t('dans.unknownOwner'),
          // The songs by name, unless some are hidden: then only how many.
          songs: dan.songs.some((s) => s.hidden)
            ? t('playlists.songCount', { count: dan.songs.length })
            : dan.songs.map((s) => chartText(s, i18n.resolvedLanguage).title).join(' / '),
        })
      : undefined,
  })
  if (error) return <Notice>{error}</Notice>
  if (!dan) return <p role="status">{t('dans.loading')}</p>
  const canEdit = session.user && (session.user.id === dan.ownerId || session.user.isAdmin)
  const isOwner = session.user?.id === dan.ownerId
  const run = async (work: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await work()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="space-y-6">
      <header className="space-y-2.5">
        <div className="flex flex-wrap items-center gap-2">
          <SourceTag source={SOURCE} />
          {dan.official ? (
            <Badge>
              <SealCheckIcon aria-hidden="true" />
              {t('dans.official')}
            </Badge>
          ) : (
            <Badge variant="secondary">{t('dans.community')}</Badge>
          )}
        </div>
        <h1 className="text-3xl leading-[1.18] font-extrabold tracking-tight wrap-anywhere sm:text-4xl">
          {danTitle(dan.title, i18n.resolvedLanguage)}
        </h1>
        <p className="text-sm text-muted-foreground">
          {t('dans.by', { owner: dan.owner || t('dans.unknownOwner') })}
        </p>
      </header>
      {dan.songs.some((s) => !s.available) && <Notice>{t('dans.unavailableExplained')}</Notice>}
      <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,20rem)]">
        <div className="flex min-w-0 flex-col gap-5">
          <section className={panelClassName} aria-labelledby="dan-songs-heading">
            <h2 id="dan-songs-heading" className="text-lg font-extrabold">
              {t('dans.songs')}
            </h2>
            <ol className="divide-y">
              {dan.songs.map((song, i) => {
                const text = chartText(song, i18n.resolvedLanguage)
                return (
                  <li
                    key={i}
                    className={cn(
                      'flex flex-wrap items-center gap-3 py-3',
                      !song.available && 'opacity-60',
                    )}
                  >
                    <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-secondary text-sm font-bold text-secondary-foreground tabular-nums">
                      {i + 1}
                    </span>
                    {song.hidden && !isOwner ? (
                      <HiddenTitle seed={`${dan.id}/${i}`} />
                    ) : (
                      <Link
                        to={`/charts/${song.chart.id}`}
                        className="min-w-0 font-medium hover:underline"
                      >
                        {text.title}
                        {text.subtitle && (
                          <span className="ml-2 text-xs text-muted-foreground">
                            {text.subtitle}
                          </span>
                        )}
                      </Link>
                    )}
                    <SongLevel song={song} />
                    {song.hidden && <Badge variant="outline">{t('dans.hiddenTitle')}</Badge>}
                    {!song.available && <Badge variant="destructive">{t('dans.changed')}</Badge>}
                  </li>
                )
              })}
            </ol>
          </section>
          <section className={cn(panelClassName, 'space-y-3')} aria-labelledby="dan-exams-heading">
            <h2 id="dan-exams-heading" className="text-lg font-extrabold">
              {t('dans.exams')}
            </h2>
            <div className="overflow-x-auto">
              <ExamTable dan={dan} />
            </div>
          </section>
        </div>
        <aside className="flex min-w-0 flex-col gap-5">
          {/* The code is what a player types into the game to play this course. */}
          <section className={cn(panelClassName, 'space-y-4')}>
            <div className="space-y-1.5">
              <h2 className="text-sm font-semibold text-muted-foreground">{t('dans.code')}</h2>
              <code className="block rounded-[14px] bg-muted px-3 py-2 text-center font-mono text-2xl font-bold tracking-[0.3em]">
                {dan.code}
              </code>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <VoteControl
                className="rounded-full bg-muted px-1 py-0.5"
                upvotes={dan.upvotes}
                downvotes={dan.downvotes}
                myVote={dan.myVote}
                label={t('interactions.voteOnDan')}
                onVote={(value: Vote) =>
                  api<VoteResult>(
                    endpoints.danVote(dan.id),
                    jsonRequest('PUT', { value }, session.csrfToken),
                  )
                }
                onResult={(result) => setDan((current) => ({ ...current, ...result }))}
              />
              <a
                href="#comments"
                className="inline-flex h-11 items-center gap-1.5 rounded-full px-3 text-sm text-muted-foreground hover:text-foreground"
              >
                <ChatCircleIcon size={18} aria-hidden="true" />
                {t('interactions.commentCount', { count: dan.commentCount })}
              </a>
            </div>
          </section>
          <section
            className={cn(panelClassName, 'space-y-3')}
            aria-labelledby="dan-rewards-heading"
          >
            <h2 id="dan-rewards-heading" className="text-lg font-extrabold">
              {t('dans.rewards')}
            </h2>
            <ul className="space-y-1 text-sm">
              {dan.rewards.dan !== undefined && (
                <li className="flex items-center gap-2">
                  <MedalIcon className="size-4 shrink-0" aria-hidden="true" />
                  {t('dans.rewardDan', { dan: danNames[dan.rewards.dan] })}
                  {!dan.official && (
                    <span className="text-muted-foreground">
                      {t('dans.rewardDanNeedsOfficial')}
                    </span>
                  )}
                </li>
              )}
              {(dan.rewards.titles ?? []).map((reward) => {
                const title = profileTitles?.find((p) => p.id === reward.id)
                return (
                  <li key={reward.id + reward.on} className="flex flex-wrap items-center gap-2">
                    {title ? (
                      <TitleBand title={title} language={i18n.resolvedLanguage} />
                    ) : (
                      <code className="text-xs">{reward.id}</code>
                    )}
                    <span>{t(`dans.levels.${reward.on}`)}</span>
                    {title && !dan.official && (
                      <span className="text-muted-foreground">
                        {t('dans.rewardDanNeedsOfficial')}
                      </span>
                    )}
                    {profileTitles && !title && (
                      <span className="text-destructive">{t('dans.titleMissing')}</span>
                    )}
                  </li>
                )
              })}
              {dan.rewards.dan === undefined && !dan.rewards.titles?.length && (
                <li className="text-muted-foreground">{t('dans.noRewards')}</li>
              )}
            </ul>
          </section>
          {(canEdit || session.user?.isAdmin) && (
            <section
              className={cn(panelClassName, 'space-y-3')}
              aria-labelledby="dan-manage-heading"
            >
              <h2 id="dan-manage-heading" className="text-lg font-extrabold">
                {t('messages.manageChart')}
              </h2>
              <div className="flex flex-col gap-2 [&>*]:justify-start">
                {canEdit && (
                  <Link
                    className={buttonVariants({ variant: 'outline' })}
                    to={`/dans/${dan.id}/edit`}
                  >
                    <PencilSimpleIcon aria-hidden="true" />
                    {t('dans.edit')}
                  </Link>
                )}
                {session.user?.isAdmin && (
                  <Button
                    variant="outline"
                    disabled={busy}
                    onClick={() =>
                      void run(async () => {
                        await api(
                          endpoints.danOfficial(dan.id),
                          jsonRequest('PUT', { official: !dan.official }, session.csrfToken),
                        )
                        setDan((current) => ({ ...current, official: !dan.official }))
                      })
                    }
                  >
                    <SealCheckIcon aria-hidden="true" />
                    {dan.official ? t('dans.unmarkOfficial') : t('dans.markOfficial')}
                  </Button>
                )}
                {canEdit && (
                  <Button
                    variant="destructive"
                    disabled={busy}
                    onClick={() => {
                      if (!window.confirm(t('dans.confirmDelete'))) return
                      void run(async () => {
                        await api(endpoints.dan(dan.id), {
                          ...jsonRequest('DELETE', {}, session.csrfToken),
                        })
                        // Nothing more is asked of it, and no later visit is shown it from memory.
                        setDeleted(true)
                        queryClient.removeQueries({ queryKey: danKeys.detail(id) })
                        void invalidateDanList()
                        navigate('/dans')
                      })
                    }}
                  >
                    <TrashIcon aria-hidden="true" />
                    {t('dans.delete')}
                  </Button>
                )}
              </div>
            </section>
          )}
        </aside>
      </div>
      <CommentSection
        subject={danSubject(dan)}
        focusId={commentId}
        onCountChange={(delta) =>
          setDan((current) => ({
            ...current,
            commentCount: Math.max(0, current.commentCount + delta),
          }))
        }
      />
    </div>
  )
}

const panelClassName = 'rounded-[28px] bg-card p-6 shadow-elev-1'

/** The heading of one part of the course form, with its place in the order. */
function StepTitle({ number, children }: { number: number; children: ReactNode }) {
  return (
    <CardTitle className="flex items-center gap-2.5 text-lg font-extrabold">
      <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-secondary text-sm text-secondary-foreground tabular-nums">
        {number}
      </span>
      {children}
    </CardTitle>
  )
}

// Picks one public song (and shows its courses) for a course's slot.
function SongPicker({
  onPick,
  onDismiss,
}: {
  onPick: (chart: ChartBase) => void
  onDismiss: () => void
}) {
  const { t, i18n } = useTranslation()
  const [query, setQuery] = useState('')
  const search = useCallback((value: string) => setQuery(value), [])
  const list = useQuery(chartQueries.list({ q: query, page: 1 }))
  const result = list.data
  const error = result ? '' : (list.error?.message ?? '')
  return (
    <Modal title={t('dans.pickSong')} onDismiss={onDismiss}>
      <SearchInput value={query} onSearch={search} onPendingChange={() => {}} />
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
      {!result && !error && <p role="status">{t('dans.loading')}</p>}
      <ul
        className={cn(
          'max-h-80 space-y-2 overflow-y-auto transition-opacity',
          list.isPlaceholderData && 'opacity-60',
        )}
      >
        {result?.items.map((chart) => {
          const text = chartText(chart, i18n.resolvedLanguage)
          return (
            <li key={chart.id}>
              <button
                type="button"
                className="w-full rounded-[20px] border p-3 text-left hover:bg-accent"
                onClick={() => onPick(chart)}
              >
                <span className="block font-medium">{text.title}</span>
                <span className="block text-xs text-muted-foreground">
                  {[text.subtitle, chart.maker || chart.author].filter(Boolean).join(' · ')}
                </span>
              </button>
            </li>
          )
        })}
      </ul>
    </Modal>
  )
}

type SongBlock = { course: string; side: string; level: number }
type SongSlot = {
  chartId: string
  title: string
  /** The blocks the chart has; the maker must pick one (no default). */
  blocks: SongBlock[]
  course: string
  side: string
  hidden: boolean
}
type ExamRow = {
  type: DanExam['type']
  range: 'more' | 'less'
  perSong: boolean
  values: string[][]
}

const sides = ['single', 'p1', 'p2'] as const
/** Every block of the chart, Easy to Edit and single, 1P, 2P: the game plays exactly one. */
function slotBlocks(chart: ChartBase): SongBlock[] {
  return courses.flatMap((course) =>
    sides.flatMap((side) => {
      const d = chart.difficulties.find((x) => x.course === course && x.side === side)
      return d ? [{ course: d.course, side: d.side, level: d.level }] : []
    }),
  )
}

function toExamRows(exams: DanExam[], songs: number): ExamRow[] {
  return exams.map((exam) => {
    const perSong = Array.isArray(exam.value[0]) || exam.value[0] === null
    const line = (v: number[] | null) =>
      v ? [String(v[0]), v.length > 1 ? String(v[1]) : ''] : ['', '']
    return {
      type: exam.type,
      range: exam.range,
      perSong,
      values: perSong
        ? Array.from({ length: songs }, (_, i) =>
            line((exam.value as (number[] | null)[])[i] ?? null),
          )
        : [line(exam.value as number[])],
    }
  })
}

function fromExamRows(rows: ExamRow[], songs: number): DanExam[] {
  const line = ([red, gold]: string[]) =>
    red.trim() === '' ? null : gold.trim() === '' ? [Number(red)] : [Number(red), Number(gold)]
  return rows.map((row) => ({
    type: row.type,
    range: row.range,
    value: row.perSong
      ? Array.from({ length: songs }, (_, i) => line(row.values[i] ?? ['', '']))
      : (line(row.values[0]) ?? []),
  }))
}

export function DanEditorPage() {
  const { id } = useParams()
  // Each course is edited in a form of its own.
  return <DanEditor key={id ?? ''} id={id} />
}

function DanEditor({ id }: { id?: string }) {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const navigate = useNavigate()
  const [titles, setTitles] = useState<Record<string, string>>({ ja: '' })
  const [color, setColor] = useState(0)
  const [code, setCode] = useState('')
  const [songs, setSongs] = useState<SongSlot[]>([])
  const [exams, setExams] = useState<ExamRow[]>([
    { type: 'gauge', range: 'more', perSong: false, values: [['100', '']] },
  ])
  const [rewardDan, setRewardDan] = useState('none')
  const [rewardTitles, setRewardTitles] = useState<DanRewardTitle[]>([])
  // Whether the course being changed is official: only then does it give its titles.
  const [made, setMade] = useState<{ official: boolean } | null>(null)
  const [picking, setPicking] = useState<number | null>(null)
  const [saveError, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const profile = useProfileTitles()
  usePageMeta({ title: t(id ? 'dans.editHeading' : 'dans.createHeading'), noindex: true })
  // The form starts from what the server holds now, not from what an earlier page remembers, and
  // once filled it is the maker's: nothing read later is written over it.
  const [seeded, setSeeded] = useState(false)
  const read = useQuery({
    ...danQueries.detail(id ?? ''),
    enabled: !!id,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const dan = read.isSuccess && read.isFetchedAfterMount ? read.data : undefined
  // A chart that cannot be read leaves its song with the one block the course names.
  const charts = useQueries({
    queries: (dan?.songs ?? []).map((song) => ({
      ...chartQueries.detail<ChartBase>(song.chart.id),
      retry: false,
    })),
  })
  if (dan && !seeded && charts.every((chart) => !chart.isPending)) {
    setSeeded(true)
    setMade({ official: dan.official })
    setTitles(dan.title)
    setColor(dan.color)
    setCode(dan.code)
    setExams(toExamRows(dan.exams, dan.songs.length))
    setRewardDan(dan.rewards.dan === undefined ? 'none' : String(dan.rewards.dan))
    setRewardTitles(dan.rewards.titles ?? [])
    setSongs(
      dan.songs.map((s, i) => {
        const chart = charts[i].data
        return {
          chartId: String(s.chart.id),
          title: chartText(s, i18n.resolvedLanguage).title,
          blocks: chart
            ? slotBlocks(chart)
            : [{ course: s.chart.course, side: s.chart.side, level: s.level }],
          course: s.chart.course,
          side: s.chart.side,
          hidden: s.hidden,
        }
      }),
    )
  }
  const error = saveError || (seeded ? '' : (read.error?.message ?? ''))
  if (session.loading) return null
  if (!session.user) return <Notice>{t('dans.signInToEdit')}</Notice>
  const official = made?.official ?? false
  // The titles a course may give, the public ones, and any this course already names that is
  // not among them (kept so that saving shows why).
  const usable = profile.titles ?? []
  const titleItems = (current: string) => {
    const items = usable.map((title) => ({
      value: title.id,
      label: titleText(title, i18n.resolvedLanguage),
    }))
    return current && !usable.some((title) => title.id === current)
      ? [{ value: current, label: current }, ...items]
      : items
  }
  const setSongCount = (next: SongSlot[]) => {
    setSongs(next)
    setExams((rows) =>
      rows.map((row) => ({
        ...row,
        values: row.perSong
          ? Array.from({ length: next.length }, (_, i) => row.values[i] ?? ['', ''])
          : row.values,
      })),
    )
  }
  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (songs.some((s) => !s.side)) {
      setError(t('dans.sideRequired'))
      return
    }
    setBusy(true)
    setError('')
    const body: DanInput = {
      title: Object.fromEntries(Object.entries(titles).filter(([, v]) => v.trim())),
      color,
      ...(!id && code.trim() ? { code: code.trim().toUpperCase() } : {}),
      songs: songs.map((s) => ({
        chartId: s.chartId,
        course: s.course,
        side: s.side,
        hidden: s.hidden,
      })),
      exams: fromExamRows(exams, songs.length),
      rewards: {
        ...(rewardDan === 'none' ? {} : { dan: Number(rewardDan) }),
        titles: rewardTitles.filter((r) => r.id.trim()),
      },
    }
    try {
      const saved = await api<Dan>(
        id ? endpoints.dan(id) : endpoints.dans,
        jsonRequest(id ? 'PUT' : 'POST', body, session.csrfToken),
      )
      queryClient.setQueryData(danKeys.detail(saved.id), saved)
      void invalidateDans()
      navigate(`/dans/${saved.id}`)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <form className="space-y-6" onSubmit={(e) => void save(e)}>
      <div className="space-y-1.5">
        <h1 className="text-4xl leading-[1.1] font-extrabold tracking-tight sm:text-5xl">
          {id ? t('dans.editHeading') : t('dans.createHeading')}
        </h1>
      </div>
      {error && <Notice>{error}</Notice>}
      <Card className="rounded-[28px]">
        <CardHeader>
          <StepTitle number={1}>{t('dans.basics')}</StepTitle>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          {titleLanguages.map((language) => (
            <div key={language} className="space-y-2">
              <Label htmlFor={`dan-title-${language}`}>
                {t('dans.titleIn', { language: t(`dans.languages.${language}`) })}
              </Label>
              <Input
                id={`dan-title-${language}`}
                maxLength={40}
                value={titles[language] ?? ''}
                onChange={(e) => setTitles({ ...titles, [language]: e.target.value })}
              />
            </div>
          ))}
          <ChoiceSelect
            label={t('dans.color')}
            value={String(color)}
            onValueChange={(v) => setColor(Number(v))}
            prefix={
              <ColorSwatch hex={(danColors.find((c) => c.value === color) ?? danColors[0]).hex} />
            }
            items={danColors.map((c) => ({
              value: String(c.value),
              label: t('dans.colorNamed', { name: t(`dans.colors.${c.name}`), number: c.value }),
              icon: <ColorSwatch hex={c.hex} />,
            }))}
          />
          <div className="space-y-2">
            <Label htmlFor="dan-code">{t('dans.code')}</Label>
            <Input
              id="dan-code"
              value={code}
              readOnly={!!id}
              maxLength={6}
              autoCapitalize="characters"
              autoComplete="off"
              spellCheck={false}
              pattern="[A-Za-z0-9]{6}"
              placeholder={id ? '' : t('dans.codePlaceholder')}
              className="font-mono uppercase tracking-[0.2em]"
              onChange={(e) => setCode(e.target.value.replace(/[^A-Za-z0-9]/g, '').toUpperCase())}
            />
            <p className="text-xs text-muted-foreground">
              {id ? t('dans.codeFixed') : t('dans.codeHint')}
            </p>
          </div>
        </CardContent>
      </Card>
      <Card className="rounded-[28px]">
        <CardHeader>
          <StepTitle number={2}>{t('dans.songs')}</StepTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {songs.map((song, i) => (
            <div key={i} className="flex flex-wrap items-center gap-3 rounded-[20px] border p-3">
              <span className="text-lg font-bold tabular-nums text-muted-foreground">{i + 1}</span>
              <span className="min-w-0 flex-1 truncate font-medium">{song.title}</span>
              <ChoiceSelect
                label={t('dans.side')}
                value={song.side ? slotKey(song) : ''}
                onValueChange={(v) => {
                  const { course, side } = slotFromKey(v)
                  setSongs(songs.map((s, j) => (j === i ? { ...s, course, side } : s)))
                }}
                items={song.blocks.map((b) => ({
                  value: slotKey(b),
                  label: `${t(`dans.courses.${b.course as (typeof courses)[number]}`)} ★${b.level} · ${t(`dans.sides.${b.side as (typeof sides)[number]}`)}`,
                }))}
              />
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={song.hidden}
                  onCheckedChange={(v) =>
                    setSongs(songs.map((s, j) => (j === i ? { ...s, hidden: Boolean(v) } : s)))
                  }
                />
                {t('dans.hidden')}
              </label>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t('dans.removeSong')}
                onClick={() => setSongCount(songs.filter((_, j) => j !== i))}
              >
                <XIcon aria-hidden="true" />
              </Button>
            </div>
          ))}
          {songs.length < 3 && (
            <Button type="button" variant="outline" onClick={() => setPicking(songs.length)}>
              <PlusIcon aria-hidden="true" />
              {t('dans.addSong')}
            </Button>
          )}
        </CardContent>
      </Card>
      <Card className="rounded-[28px]">
        <CardHeader>
          <StepTitle number={3}>{t('dans.exams')}</StepTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {exams.map((row, i) => (
            <div key={i} className="space-y-3 rounded-[20px] bg-muted p-4">
              <div className="flex flex-wrap items-center gap-3">
                <ChoiceSelect
                  label={t('dans.exam')}
                  value={row.type}
                  onValueChange={(v) =>
                    setExams(
                      exams.map((r, j) =>
                        j === i
                          ? {
                              ...r,
                              type: v as DanExam['type'],
                              perSong: v === 'gauge' ? false : r.perSong,
                              values: v === 'gauge' ? [r.values[0] ?? ['', '']] : r.values,
                            }
                          : r,
                      ),
                    )
                  }
                  items={examTypes.map((type) => ({
                    value: type,
                    label: t(`dans.examTypes.${type}`),
                  }))}
                />
                <ChoiceSelect
                  label={t('dans.range')}
                  value={row.range}
                  onValueChange={(v) =>
                    setExams(
                      exams.map((r, j) => (j === i ? { ...r, range: v as 'more' | 'less' } : r)),
                    )
                  }
                  items={(['more', 'less'] as const).map((range) => ({
                    value: range,
                    label: t(`dans.ranges.${range}`),
                  }))}
                />
                {row.type !== 'gauge' && (
                  <label className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={row.perSong}
                      onCheckedChange={(v) =>
                        setExams(
                          exams.map((r, j) =>
                            j === i
                              ? {
                                  ...r,
                                  perSong: Boolean(v),
                                  values: v
                                    ? Array.from({ length: songs.length }, () => [
                                        ...(r.values[0] ?? ['', '']),
                                      ])
                                    : [r.values[0] ?? ['', '']],
                                }
                              : r,
                          ),
                        )
                      }
                    />
                    {t('dans.perSong')}
                  </label>
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="ml-auto"
                  aria-label={t('dans.removeExam')}
                  onClick={() => setExams(exams.filter((_, j) => j !== i))}
                >
                  <XIcon aria-hidden="true" />
                </Button>
              </div>
              <div className="grid gap-2 sm:grid-cols-3">
                {row.values.map((value, k) => (
                  <div key={k} className="flex items-center gap-2">
                    {row.perSong && (
                      <span className="w-12 shrink-0 text-xs text-muted-foreground">
                        {t('dans.songNumber', { number: k + 1 })}
                      </span>
                    )}
                    <Input
                      inputMode="numeric"
                      aria-label={t('dans.red')}
                      placeholder={t('dans.red')}
                      value={value[0]}
                      onChange={(e) =>
                        setExams(
                          exams.map((r, j) =>
                            j === i
                              ? {
                                  ...r,
                                  values: r.values.map((v, m) =>
                                    m === k ? [e.target.value, v[1]] : v,
                                  ),
                                }
                              : r,
                          ),
                        )
                      }
                    />
                    <Input
                      inputMode="numeric"
                      aria-label={t('dans.gold')}
                      placeholder={t('dans.gold')}
                      value={value[1]}
                      onChange={(e) =>
                        setExams(
                          exams.map((r, j) =>
                            j === i
                              ? {
                                  ...r,
                                  values: r.values.map((v, m) =>
                                    m === k ? [v[0], e.target.value] : v,
                                  ),
                                }
                              : r,
                          ),
                        )
                      }
                    />
                  </div>
                ))}
              </div>
            </div>
          ))}
          {exams.length < 4 && (
            <Button
              type="button"
              variant="outline"
              onClick={() =>
                setExams([
                  ...exams,
                  { type: 'good', range: 'more', perSong: false, values: [['', '']] },
                ])
              }
            >
              <PlusIcon aria-hidden="true" />
              {t('dans.addExam')}
            </Button>
          )}
        </CardContent>
      </Card>
      <Card className="rounded-[28px]">
        <CardHeader>
          <StepTitle number={4}>{t('dans.rewards')}</StepTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <ChoiceSelect
            label={t('dans.rewardDanLabel')}
            value={rewardDan}
            onValueChange={setRewardDan}
            items={[
              { value: 'none', label: t('dans.noDan') },
              ...danNames.map((name, i) => ({ value: String(i), label: name })).slice(danFirst),
            ]}
          />
          {rewardTitles.map((reward, i) => {
            const title = profile.titles?.find((p) => p.id === reward.id)
            return (
              <div key={i} className="flex flex-wrap items-center gap-3">
                {profile.titles ? (
                  <ChoiceSelect
                    label={t('dans.rewardTitleLabel')}
                    value={reward.id}
                    onValueChange={(v) =>
                      setRewardTitles(rewardTitles.map((r, j) => (j === i ? { ...r, id: v } : r)))
                    }
                    items={titleItems(reward.id)}
                  />
                ) : (
                  <Input
                    className="w-64 font-mono"
                    aria-label={t('dans.titleId')}
                    placeholder="taiko-master"
                    value={reward.id}
                    onChange={(e) =>
                      setRewardTitles(
                        rewardTitles.map((r, j) =>
                          j === i ? { ...r, id: e.target.value.trim() } : r,
                        ),
                      )
                    }
                  />
                )}
                <ChoiceSelect
                  label={t('dans.rewardLevel')}
                  value={reward.on}
                  onValueChange={(v) =>
                    setRewardTitles(
                      rewardTitles.map((r, j) =>
                        j === i ? { ...r, on: v as DanRewardTitle['on'] } : r,
                      ),
                    )
                  }
                  items={rewardLevels.map((level) => ({
                    value: level,
                    label: t(`dans.levels.${level}`),
                  }))}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={t('dans.removeReward')}
                  onClick={() => setRewardTitles(rewardTitles.filter((_, j) => j !== i))}
                >
                  <XIcon aria-hidden="true" />
                </Button>
                {title && <TitleBand title={title} language={i18n.resolvedLanguage} />}
                {title && !official && (
                  <span className="text-xs text-muted-foreground">
                    {t('dans.officialTitleLater')}
                  </span>
                )}
                {profile.titles && reward.id && !title && (
                  <span className="text-xs text-destructive">{t('dans.titleMissing')}</span>
                )}
              </div>
            )
          })}
          {profile.failed && <Notice>{t('dans.titlesUnavailable')}</Notice>}
          <div className="flex flex-wrap items-center gap-3">
            {rewardTitles.length < 10 && (
              <Button
                type="button"
                variant="outline"
                disabled={!!profile.titles && usable.length === 0}
                onClick={() =>
                  setRewardTitles([...rewardTitles, { id: usable[0]?.id ?? '', on: 'red' }])
                }
              >
                <PlusIcon aria-hidden="true" />
                {t('dans.addReward')}
              </Button>
            )}
            <Button type="button" variant="ghost" onClick={profile.reload}>
              <ArrowClockwiseIcon aria-hidden="true" />
              {t('dans.reloadTitles')}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            {profile.titles && usable.length === 0 && `${t('dans.noUsableTitles')} `}
            <a
              href={profileTitlesPage}
              target="_blank"
              rel="noopener noreferrer"
              className="underline"
            >
              {t('dans.titleCatalog')}
            </a>
          </p>
        </CardContent>
      </Card>
      {/* Stays in reach while the form scrolls; on a phone it sits above the bottom bar. */}
      <div className="sticky bottom-24 z-10 flex w-fit gap-2 rounded-full bg-card p-2 shadow-elev-2 lg:bottom-4">
        <Button type="submit" disabled={busy || songs.length === 0}>
          {t('dans.save')}
        </Button>
        <Link className={buttonVariants({ variant: 'ghost' })} to={id ? `/dans/${id}` : '/dans'}>
          {t('dans.cancel')}
        </Link>
      </div>
      {picking !== null && (
        <SongPicker
          onDismiss={() => setPicking(null)}
          onPick={(chart) => {
            const blocks = slotBlocks(chart)
            if (blocks.length === 0) {
              setError(t('dans.noPlayableCourse'))
              setPicking(null)
              return
            }
            const text = chartText(chart, i18n.resolvedLanguage)
            // No block is chosen for the maker: the course names the one it plays.
            setSongCount([
              ...songs,
              {
                chartId: String(chart.id),
                title: text.title,
                blocks,
                course: '',
                side: '',
                hidden: false,
              },
            ])
            setPicking(null)
          }}
        />
      )}
    </form>
  )
}
