import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate } from 'react-router-dom'
import { useQueries, useQuery } from '@tanstack/react-query'
import { ArrowDownIcon, ArrowUpIcon, PlusIcon, XIcon } from '@phosphor-icons/react'
import type { Playlist } from '@sitekit/api/types'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Input } from '@sitekit/components/ui/input'
import { Label } from '@sitekit/components/ui/label'
import { useFirst } from '@sitekit/queries/chart-writes'
import { queryClient } from '@sitekit/query/client'
import type { SourceId } from '@sitekit/source'
import { api, jsonRequest } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { FeaturedPlaylist, FeaturedPlaylists } from '@players/api/types'
import { SourceTag, sources } from '@players/components/song-tile'
import {
  PlaylistCover,
  isSource,
  playlistInAddress,
  playlistKey,
  playlistPage,
  usePlaylistsNamed,
} from '@players/featured'
import { Notice } from '@players/notice'
import { playerKeys, playerQueries } from '@players/queries'
import { useSession } from '@players/session'

// As the server counts a line about a playlist: in characters.
const maxNote = 40
// How many of a source's playlists a search shows.
const foundShown = 6

type Found = { source: SourceId; playlist: Playlist }

/** A playlist as the panel names one: cover, name, source, how many songs and whose it is. */
function Named({ source, playlist }: { source: string; playlist: Playlist | undefined }) {
  const { t } = useTranslation()
  if (!playlist || !isSource(source))
    return <span className="text-muted-foreground text-sm">{t('featured.unavailable')}</span>
  const count = t('playlists.songCount', { count: playlist.chartCount })
  return (
    <span data-accent={source} className="flex items-center gap-3 min-w-0">
      <PlaylistCover playlist={playlist} className="rounded-[14px] size-12" />
      <span className="min-w-0">
        <a
          href={playlistPage(source, playlist.id)}
          className="block font-semibold underline truncate"
        >
          {playlist.name}
        </a>
        <span className="flex items-center gap-2 text-muted-foreground text-xs">
          <SourceTag source={source} />
          <span className="truncate">
            {playlist.owner ? `${count} · ${playlist.owner.nickname}` : count}
          </span>
        </span>
      </span>
    </span>
  )
}

// The manage panel's part about the home page: which playlists it recommends, in which order,
// and a line about each. Only the site's administrator sees it. Any public playlist of either
// source can be recommended; the list is what is sent, whole, when it is saved.
export function FeaturedPage() {
  const { t } = useTranslation()
  const session = useSession()
  const allowed = !!session.user?.siteAdmin
  // What is being arranged starts from the list as it was when the panel opened, and is not
  // changed under the administrator's hands by a later reading of it.
  const read = useQuery({
    ...playerQueries.featured(),
    enabled: allowed,
    staleTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const opened = useFirst(
    'featured',
    read.isFetchedAfterMount && !read.isFetching && !read.error ? read.data : undefined,
  )
  // The list as last saved here, and as it is on the page.
  const [saved, setSaved] = useState<FeaturedPlaylist[] | null>(null)
  const [draft, setDraft] = useState<FeaturedPlaylist[] | null>(null)
  const kept = saved ?? opened?.items ?? []
  const rows = draft ?? kept
  const max = opened?.max ?? 0
  // Those added here are known from the search that found them.
  const [added, setAdded] = useState<Map<string, Playlist>>(() => new Map())
  const named = usePlaylistsNamed(opened?.items ?? [])
  const playlistOf = (item: FeaturedPlaylist) => {
    const key = playlistKey(item.source, item.playlistId)
    return added.get(key) ?? named.found.get(key)
  }

  // What is typed in the search box, and the text last searched for, a moment after typing
  // stops. A playlist's address finds that playlist; anything else is looked for in the names.
  const [lookup, setLookup] = useState('')
  const [asked, setAsked] = useState('')
  const text = lookup.trim()
  useEffect(() => {
    const timer = window.setTimeout(() => setAsked(text), 250)
    return () => window.clearTimeout(timer)
  }, [text])
  const current = asked === text && !!asked
  const address = playlistInAddress(asked)
  const searches = useQueries({
    queries: sources.map((source) =>
      address
        ? {
            ...playerQueries.playlistsNamed(source, [address.id]),
            enabled: allowed && current && address.source === source,
          }
        : { ...playerQueries.playlistsFound(source, asked), enabled: allowed && current },
    ),
  })
  const searching = current && searches.some((search) => search.isFetching)
  const found: Found[] = current
    ? sources.flatMap((source, index) =>
        (searches[index].data ?? []).slice(0, foundShown).map((playlist) => ({ source, playlist })),
      )
    : []
  const searchError = current
    ? (searches.find((search) => !search.data && search.error)?.error ?? null)
    : null

  const [message, setMessage] = useState<{ kind: 'error' | 'info'; text: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const unread = (opened ? null : read.error) ?? named.error ?? searchError
  const notice = message ?? (unread && { kind: 'error' as const, text: unread.message })

  if (session.loading) return null
  if (!allowed) return <Navigate to="/" replace />

  const change = (next: FeaturedPlaylist[]) => {
    setMessage(null)
    setDraft(next)
  }
  const add = ({ source, playlist }: Found) => {
    setAdded((known) => new Map(known).set(playlistKey(source, playlist.id), playlist))
    change([...rows, { source, playlistId: playlist.id, note: '' }])
  }
  const move = (index: number, offset: number) => {
    const next = [...rows]
    ;[next[index], next[index + offset]] = [next[index + offset], next[index]]
    change(next)
  }
  const save = async () => {
    setBusy(true)
    setMessage(null)
    try {
      const answer = await api<FeaturedPlaylists>(
        endpoints.adminFeaturedPlaylists,
        jsonRequest('PUT', { items: rows }, session.csrfToken),
      )
      setSaved(answer.items)
      setDraft(null)
      setMessage({ kind: 'info', text: t('featured.saved') })
      // The home page reads it anew.
      void queryClient.invalidateQueries({ queryKey: playerKeys.featured() })
    } catch (e) {
      setMessage({ kind: 'error', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }
  const chosen = new Set(rows.map((row) => playlistKey(row.source, row.playlistId)))
  const full = rows.length >= max
  const changed = JSON.stringify(rows) !== JSON.stringify(kept)

  return (
    <div className="space-y-6" data-testid="featured-panel">
      <div className="space-y-1">
        <h1 className="font-semibold text-2xl tracking-tight">{t('featured.heading')}</h1>
        <p className="text-muted-foreground text-sm">{t('featured.intro', { max })}</p>
      </div>
      {notice && <Notice kind={notice.kind}>{notice.text}</Notice>}

      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>{t('featured.chosenHeading', { n: rows.length, max })}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          {!opened ? null : rows.length === 0 ? (
            <p className="text-muted-foreground text-sm" data-testid="featured-none">
              {t('featured.none')}
            </p>
          ) : (
            <ol className="divide-y" data-testid="featured-chosen">
              {rows.map((row, index) => {
                const key = playlistKey(row.source, row.playlistId)
                const name = playlistOf(row)?.name ?? t('featured.unavailable')
                return (
                  <li key={key} className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
                    <span className="flex-[1_1_14rem] min-w-0">
                      <Named source={row.source} playlist={playlistOf(row)} />
                    </span>
                    <span className="flex-[2_1_16rem] space-y-1 min-w-0">
                      <Label htmlFor={`note-${key}`} className="text-muted-foreground text-xs">
                        {t('featured.note', { max: maxNote })}
                      </Label>
                      <Input
                        id={`note-${key}`}
                        value={row.note}
                        maxLength={maxNote}
                        autoComplete="off"
                        disabled={busy}
                        onChange={(event) =>
                          change(
                            rows.map((other, i) =>
                              i === index ? { ...other, note: event.target.value } : other,
                            ),
                          )
                        }
                      />
                    </span>
                    <span className="flex items-center self-end">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        disabled={busy || index === 0}
                        aria-label={t('featured.moveUp', { name })}
                        onClick={() => move(index, -1)}
                      >
                        <ArrowUpIcon aria-hidden="true" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        disabled={busy || index === rows.length - 1}
                        aria-label={t('featured.moveDown', { name })}
                        onClick={() => move(index, 1)}
                      >
                        <ArrowDownIcon aria-hidden="true" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        className="text-destructive hover:text-destructive"
                        disabled={busy}
                        aria-label={t('featured.remove', { name })}
                        onClick={() => change(rows.filter((_, i) => i !== index))}
                      >
                        <XIcon aria-hidden="true" />
                      </Button>
                    </span>
                  </li>
                )
              })}
            </ol>
          )}
          <div className="flex flex-wrap items-center gap-3">
            <Button onClick={() => void save()} disabled={busy || !opened || !changed}>
              {t('featured.save')}
            </Button>
            {changed && <p className="text-muted-foreground text-sm">{t('featured.unsaved')}</p>}
          </div>
        </CardContent>
      </Card>

      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>{t('featured.addHeading')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="space-y-1.5">
            <Label htmlFor="featured-search">{t('featured.search')}</Label>
            <Input
              id="featured-search"
              type="search"
              value={lookup}
              onChange={(event) => setLookup(event.target.value)}
              placeholder={t('featured.searchPlaceholder')}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          {full && <p className="text-muted-foreground text-sm">{t('featured.full', { max })}</p>}
          {current && !searching && !searchError && found.length === 0 && (
            <p className="text-muted-foreground text-sm" data-testid="featured-no-match">
              {t('featured.noMatch')}
            </p>
          )}
          {found.length > 0 && (
            <ul className="px-4 border rounded-2xl divide-y" data-testid="featured-found">
              {found.map((item) => {
                const taken = chosen.has(playlistKey(item.source, item.playlist.id))
                return (
                  <li
                    key={playlistKey(item.source, item.playlist.id)}
                    className="flex flex-wrap items-center gap-3 py-3"
                  >
                    <span className="flex-1 min-w-0">
                      <Named source={item.source} playlist={item.playlist} />
                    </span>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={busy || taken || full}
                      onClick={() => add(item)}
                    >
                      <PlusIcon aria-hidden="true" />
                      {t(taken ? 'featured.added' : 'featured.add')}
                    </Button>
                  </li>
                )
              })}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
