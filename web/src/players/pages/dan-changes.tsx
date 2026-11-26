import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { api, jsonRequest } from '@players/api/client'
import { sourceApi } from '@players/components/song-tile'
import { Notice } from '@players/notice'
import { useSession } from '@players/session'
import type { DanChangeReview, DanSong } from '@sitekit/api/types'
import { chartText } from '@sitekit/chart-language'
import { Button } from '@sitekit/components/ui/button'
import { Label } from '@sitekit/components/ui/label'
import { Textarea } from '@sitekit/components/ui/textarea'
import { ExamTable, danTitle } from '@sitekit/dans'
import { formatLocale } from '@sitekit/i18n'
import { queryClient } from '@sitekit/query/client'
import { sources, type SourceId } from '@sitekit/source'

// Who manages a source decides the changes asked for its fixed courses.
const managerOf: Record<SourceId, string> = {
  fanmade: 'FANMADE_MANAGER',
  official: 'OFFICIAL_MANAGER',
}
const queueKey = (source: SourceId) => ['dan-changes', 'queue', source] as const

// 段位审核: the requests to change the songs and conditions of fixed 段位 courses, for the managers
// of each source to approve, which gives the course what was asked and fixes it again, or to
// return with a reply. The course's maker is told either way.
export function DanChangesPage() {
  const { t } = useTranslation()
  const session = useSession()
  const managed = sources.filter((source) => session.user?.groups?.includes(managerOf[source.id]))
  const queues = useQueries({
    queries: managed.map((source) => ({
      queryKey: queueKey(source.id),
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        api<{ changes: DanChangeReview[] }>(`${sourceApi(source.id)}/dan-changes`, {
          signal,
        }).then((answer) => answer.changes),
    })),
  })
  if (session.loading) return null
  if (managed.length === 0) return <Navigate to="/" replace />
  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl font-extrabold tracking-tight sm:text-3xl">
          {t('danAdmin.heading')}
        </h1>
        <p className="text-sm text-muted-foreground">{t('danAdmin.intro')}</p>
      </header>
      {managed.map((source, i) => {
        const queue = queues[i]
        return (
          <section key={source.id} className="space-y-3" aria-labelledby={`dan-queue-${source.id}`}>
            <h2 id={`dan-queue-${source.id}`} className="text-lg font-extrabold">
              {t(`shell.${source.id}`)}
            </h2>
            {queue.error && <Notice>{queue.error.message}</Notice>}
            {queue.data?.length === 0 && (
              <p className="py-6 text-center text-muted-foreground">{t('danAdmin.empty')}</p>
            )}
            <ul className="space-y-4">
              {queue.data?.map((change) => (
                <Request key={change.id} source={source.id} change={change} />
              ))}
            </ul>
          </section>
        )
      })}
    </div>
  )
}

function Songs({ songs }: { songs: DanSong[] }) {
  const { t, i18n } = useTranslation()
  return (
    <ol className="list-inside list-decimal space-y-1 text-sm">
      {songs.map((song, i) => (
        <li key={i} className={song.available ? '' : 'text-destructive'}>
          {chartText(song, i18n.resolvedLanguage).title || song.chart.id} ·{' '}
          {t(`dans.courses.${song.chart.course}`)} · {t(`dans.sides.${song.chart.side}`)} ★
          {song.level}
          {song.hidden && ` · ${t('dans.hiddenTitle')}`}
          {!song.available && ` · ${t('dans.changed')}`}
        </li>
      ))}
    </ol>
  )
}

function Request({ source, change }: { source: SourceId; change: DanChangeReview }) {
  const { t, i18n } = useTranslation()
  const session = useSession()
  const [reply, setReply] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const decide = async (approve: boolean) => {
    setBusy(true)
    setError('')
    try {
      await api(
        `${sourceApi(source)}/dan-changes/${encodeURIComponent(change.id)}/decision`,
        jsonRequest('POST', { approve, reply: reply.trim() }, session.csrfToken),
      )
      await queryClient.invalidateQueries({ queryKey: queueKey(source) })
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  const course = change.course
  return (
    <li className="space-y-4 rounded-[20px] bg-card p-4 shadow-elev-1">
      <div className="flex flex-wrap items-baseline gap-2">
        <a
          className="font-bold underline-offset-2 hover:underline wrap-anywhere"
          href={`/${source}/dans/${encodeURIComponent(course.id)}`}
        >
          {danTitle(course.title, i18n.resolvedLanguage)}
        </a>
        <code className="text-xs">{course.code}</code>
        <span className="text-xs text-muted-foreground">
          {t('danAdmin.asked', {
            owner: course.owner || course.ownerId,
            at: new Date(change.createdAt).toLocaleString(formatLocale(i18n.language), {
              dateStyle: 'medium',
              timeStyle: 'short',
            }),
          })}
        </span>
      </div>
      <p className="rounded-[14px] bg-muted p-3 text-sm wrap-anywhere whitespace-pre-wrap">
        {change.request}
      </p>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="min-w-0 space-y-2">
          <h3 className="text-sm font-semibold text-muted-foreground">{t('danAdmin.now')}</h3>
          <Songs songs={course.songs} />
          <div className="overflow-x-auto">
            <ExamTable dan={course} />
          </div>
        </div>
        <div className="min-w-0 space-y-2">
          <h3 className="text-sm font-semibold text-muted-foreground">{t('danAdmin.asked_for')}</h3>
          <Songs songs={change.songs} />
          <div className="overflow-x-auto">
            <ExamTable dan={change} />
          </div>
        </div>
      </div>
      {error && <Notice>{error}</Notice>}
      <div className="space-y-2">
        <Label htmlFor={`reply-${change.id}`}>{t('danAdmin.reply')}</Label>
        <Textarea
          id={`reply-${change.id}`}
          maxLength={500}
          value={reply}
          onChange={(e) => setReply(e.target.value)}
        />
      </div>
      <div className="flex gap-2">
        <Button type="button" size="sm" disabled={busy} onClick={() => void decide(true)}>
          {t('danAdmin.approve')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={busy || !reply.trim()}
          onClick={() => void decide(false)}
        >
          {t('danAdmin.return')}
        </Button>
      </div>
    </li>
  )
}
