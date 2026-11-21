import { fetchChartFile } from './resource-download'
import { chartText } from './chart-language'
import { i18n } from './i18n'
import { useTranslation } from 'react-i18next'
import { endpoints } from './api/endpoints'
import { Progress } from './components/ui/progress'
import { useEffect, useRef, useState } from 'react'
import { queryOptions, skipToken, useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { CaretLeftIcon, WarningCircleIcon } from '@phosphor-icons/react'
import { previewReplacement, uploadChart } from './api/client'
import type { ChartBase } from './api/types'
import { isChartOwner } from './chart-owners'
import { EditorFrame, tellEditor, type FromEditor } from './editor-frame'
import { useSession } from './session'
import { Notice } from './notifications'
import { useNotification } from './notification-context'
import { chartWritten, useChartSeed, useFirst } from './queries/chart-writes'
import { chartKeys } from './queries/charts'
import { maxTja } from './tja-limits'
import { withChartTitles } from './tja-titles'
import { replacementFields, reviewReplacement } from './replacement-diff'
import type { Proposal, ReplacementPreview, Review } from './replacement-diff'
import { ReplacementReview } from './replacement-review'

function createRequestKey() {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

// The line a TJA validation error points at, when the API gave one.
function errorLine(e: unknown): number | undefined {
  const cause = (e as { cause?: { line?: unknown; errors?: { line?: unknown }[] } }).cause
  const line = cause?.line ?? cause?.errors?.[0]?.line
  return typeof line === 'number' && line > 0 ? line : undefined
}

// The text a chart is edited from. It is read once for each opening of the editor: what is being
// typed starts from this, so nothing may bring a newer one.
function chartSource(chart: ChartBase | null) {
  const address = chart ? endpoints.resources(chart.id) : ''
  return queryOptions({
    queryKey: [...chartKeys.all(), 'tja', chart?.id ?? ''],
    queryFn: chart
      ? async ({ signal }) => {
          const response = await fetchChartFile(
            chart.id,
            'tja',
            { signal, credentials: 'omit' },
            address,
          )
          if (!response.ok)
            throw new Error(i18n.t('messages.chartHttpError', { status: response.status }))
          const bytes = await response.arrayBuffer()
          if (bytes.byteLength > maxTja)
            throw new Error(i18n.t('messages.chartExceedsThePreviewSizeLimit'))
          // The editor keeps lines apart by "\n", so other line breaks are
          // normalized before anything compares. The title headers are the
          // chart's own names, so saving keeps them.
          return withChartTitles(
            new TextDecoder('utf-8', { fatal: true }).decode(bytes).replace(/\r\n?/g, '\n'),
            chart,
          )
        }
      : skipToken,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}

// The name the text gives its song (WAVE, before the first #START): the stored text names the
// file by its hash, not by the chart's audio name, and the editor finds the song by this.
function songFile(tja: string) {
  return /^\s*WAVE\s*:\s*(.*?)\s*$/im.exec(tja.split(/#START/i)[0])?.[1] ?? ''
}

// The chart's audio, handed to the editor with the text (null: the chart has none).
function chartAudio(chart: ChartBase | null) {
  const address = chart ? endpoints.resources(chart.id) : ''
  return queryOptions({
    queryKey: [...chartKeys.all(), 'audio-bytes', chart?.id ?? '', chart?.audioHash ?? ''],
    queryFn: chart
      ? async ({ signal }) => {
          const response = await fetchChartFile(
            chart.id,
            'audio',
            { signal, credentials: 'omit' },
            address,
          )
          if (response.status === 404) return null
          if (!response.ok)
            throw new Error(i18n.t('messages.chartHttpError', { status: response.status }))
          return response.arrayBuffer()
        }
      : skipToken,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}

/** Reads the chart, its text and its audio once; the editor below keeps them from then on. */
export function ChartEditorPage() {
  const { t } = useTranslation()
  const { id } = useParams()
  const session = useSession()
  const { chart, error } = useChartSeed<ChartBase>(id)
  const read = useQuery(chartSource(chart))
  const sound = useQuery(chartAudio(chart))
  const source = useFirst(chart?.id, read.data)
  const audio = useFirst(chart?.id, sound.data)
  if (!id) return <Notice>{t('messages.missingSongId')}</Notice>
  if (chart && source !== undefined && audio !== undefined)
    return <ChartEditor key={id} loaded={chart} source={source} audio={audio} />
  if (session.loading) return <p role="status">{t('messages.loadingAccount')}</p>
  if (error) return <Notice>{error}</Notice>
  if (read.error) return <Notice>{read.error.message || t('messages.failedToLoadChart')}</Notice>
  if (sound.error) return <Notice>{sound.error.message || t('messages.failedToLoadChart')}</Notice>
  return <p className="text-sm text-muted-foreground">{t('editor.loading')}</p>
}

/**
 * Edits a chart in OurTaikoEdit, the chart editor (./editor-frame), with its text and audio.
 * Saving in the editor sends the text here, and it becomes the chart's new TJA through the same
 * replacement as the update page, with the audio and movie kept, after the same review of what it
 * changes: blocks the new text no longer has go with their scores, and older scores leave the
 * leaderboard. The editor is told whether it was stored.
 */
function ChartEditor({
  loaded,
  source,
  audio,
}: {
  loaded: ChartBase
  source: string
  audio: ArrayBuffer | null
}) {
  const { t } = useTranslation()
  const { notify } = useNotification()
  const navigate = useNavigate()
  const session = useSession()
  const [chart, setChart] = useState(loaded)
  const [opened, setOpened] = useState(false)
  const [openError, setOpenError] = useState('')
  const [dirty, setDirty] = useState(false)
  // Open while the server reads the text (review null) and until the update is confirmed.
  const [reviewing, setReviewing] = useState<{
    file: File
    text: string
    /** The text drops the movie the chart has. */
    dropsMovie: boolean
    preview: ReplacementPreview | null
    review: Review | null
  } | null>(null)
  const attempted = useRef('')
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState(0)
  const [saveError, setSaveError] = useState('')
  const requestKey = useRef(createRequestKey())
  const controller = useRef<AbortController | null>(null)
  const frame = useRef<HTMLIFrameElement>(null)
  // The editor's save being answered.
  const asking = useRef<number | null>(null)

  useEffect(() => {
    if (!dirty) return
    const warn = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])
  useEffect(() => () => controller.current?.abort(), [])

  /** Tells the editor how its save went: stored, called off (no message) or refused. */
  const answer = (saved: boolean, message?: string, line?: number) => {
    const request = asking.current
    if (request === null) return
    asking.current = null
    tellEditor(frame.current, { type: 'save-result', request, saved, message, line })
  }

  // The site fields go back as the chart has them unless the review lets the new text change them.
  const proposal = (current: ChartBase): Proposal => ({
    description: current.description,
    categoryIds: current.categoryIds,
    demoStart: current.demoStart,
    demoEnd: current.demoEnd,
    typedMakers: {},
    typedDemo: false,
  })
  const fail = (e: unknown) => {
    const message = e instanceof Error ? e.message : t('messages.requestFailed')
    setSaveError(message)
    answer(false, message, errorLine(e))
  }
  const startReview = async (text: string) => {
    if (busy) return
    setSaveError('')
    const abort = new AbortController()
    controller.current = abort
    // The server checks the text as it would an uploaded file, under the site's rules.
    const file = new File([text], chart.tjaName, { type: 'text/plain;charset=utf-8' })
    // A stored movie stays while the text still declares BGMOVIE before its first #START.
    const dropsMovie = !!chart.movieHash && !/^\s*BGMOVIE\s*:\s*\S/m.test(text.split(/#START/i)[0])
    setReviewing({ file, text, dropsMovie, preview: null, review: null })
    try {
      const preview = await previewReplacement(
        chart.id,
        file,
        undefined,
        session.csrfToken,
        abort.signal,
      )
      if (abort.signal.aborted) return
      setReviewing({
        file,
        text,
        dropsMovie,
        preview,
        review: reviewReplacement(chart, preview, proposal(chart), false, {
          demo: true,
          makers: true,
        }),
      })
    } catch (e) {
      setReviewing(null)
      if (!abort.signal.aborted) fail(e)
      else answer(false)
    }
  }
  const save = async (kept: Set<string>) => {
    if (!reviewing?.preview || busy) return
    const { file, text, preview } = reviewing
    setReviewing(null)
    setBusy(true)
    setProgress(0)
    controller.current = new AbortController()
    try {
      const fields = replacementFields(chart, preview, proposal(chart), kept)
      // Other text or choices are a different request; the same ones retry the last.
      const attempt = [...kept].sort().join(',') + '\n' + text
      if (attempt !== attempted.current) requestKey.current = createRequestKey()
      attempted.current = attempt
      const form = new FormData()
      form.append('tja', file)
      form.append('confirmReset', 'true')
      if (fields.keepTitles) form.append('keepTitles', 'true')
      form.append('description', fields.description)
      // Only a kept range is sent; otherwise the server takes the new text's DEMOSTART.
      if (kept.has('demo')) {
        form.append('demoStart', String(fields.demoStart))
        form.append('demoEnd', String(fields.demoEnd))
      }
      form.append('categoryIds', JSON.stringify(fields.categoryIds))
      form.append('difficultyMakers', JSON.stringify(fields.difficultyMakers))
      const updated = await uploadChart<ChartBase>(
        form,
        session.csrfToken,
        requestKey.current,
        controller.current.signal,
        setProgress,
        chart.id,
        true,
      )
      if ('status' in updated) throw new Error(t('messages.requestFailed'))
      requestKey.current = createRequestKey()
      attempted.current = ''
      setChart(updated)
      void chartWritten(updated.id, updated)
      answer(true)
      notify(t('editor.saved'), 'success')
    } catch (e) {
      fail(e)
    } finally {
      setBusy(false)
    }
  }

  const hear = (message: FromEditor) => {
    switch (message.type) {
      case 'ready':
        // Again if the editor was loaded anew: the audio is copied, since sending moves it.
        tellEditor(frame.current, {
          type: 'open',
          chart: {
            id: chart.id,
            name: chartText(chart, i18n.resolvedLanguage).title,
            tjaName: chart.tjaName,
            tja: source,
            audioName: songFile(source) || chart.audioName,
            audio: audio ? audio.slice(0) : null,
          },
        })
        break
      case 'opened':
        setOpened(true)
        setOpenError('')
        break
      case 'open-failed':
        setOpenError(message.message)
        break
      case 'save':
        // One save at a time: a second while one is under way is turned away.
        if (asking.current !== null || busy || reviewing) {
          tellEditor(frame.current, { type: 'save-result', request: message.request, saved: false })
          break
        }
        asking.current = message.request
        void startReview(message.tja)
        break
      case 'dirty':
        setDirty(message.dirty)
        break
      case 'leave':
        navigate(`/charts/${chart.id}`)
        break
    }
  }

  if (session.loading) return <p role="status">{t('messages.loadingAccount')}</p>
  const canManage = isChartOwner(chart, session.user?.id) || !!session.user?.isAdmin
  if (!canManage) return <Notice>{t('editor.onlyManagers')}</Notice>
  const title = chartText(chart, i18n.resolvedLanguage).title
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <Link
          to={`/charts/${chart.id}`}
          className="inline-flex min-w-0 items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <CaretLeftIcon size={14} aria-hidden="true" />
          <span className="truncate">{title}</span>
        </Link>
        <h1 className="text-sm font-extrabold">{t('editor.title')}</h1>
        {dirty && (
          <span className="text-sm text-muted-foreground" role="status">
            {t('messages.unsavedChanges')}
          </span>
        )}
      </div>
      {busy && <Progress value={progress} aria-label={t('messages.saving')} />}
      {(saveError || openError) && (
        <Notice>
          <span className="inline-flex items-center gap-2">
            <WarningCircleIcon size={16} className="shrink-0" aria-hidden="true" />
            {openError ? t('editor.openFailed', { message: openError }) : saveError}
          </span>
        </Notice>
      )}
      <div className="relative">
        <EditorFrame embed frameRef={frame} onMessage={hear} className="h-[calc(100svh-9rem)]" />
        {!opened && !openError && (
          <p
            role="status"
            className="absolute inset-0 flex items-center justify-center rounded-[28px] bg-card text-sm text-muted-foreground"
          >
            {t('editor.loading')}
          </p>
        )}
      </div>
      {reviewing && (
        <ReplacementReview
          review={reviewing.review}
          files={{
            tja: chart.tjaName,
            audio: null,
            keptAudio: chart.audioName,
            cover: null,
            movie: reviewing.dropsMovie ? 'remove' : null,
          }}
          onCancel={() => {
            controller.current?.abort()
            setReviewing(null)
            answer(false)
          }}
          onConfirm={(kept) => void save(kept)}
        />
      )}
    </div>
  )
}
