import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate } from 'react-router-dom'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Checkbox } from '@sitekit/components/ui/checkbox'
import { Input } from '@sitekit/components/ui/input'
import { Label } from '@sitekit/components/ui/label'
import { Textarea } from '@sitekit/components/ui/textarea'
import { RelativeTime } from '@sitekit/comments'
import { queryClient } from '@sitekit/query/client'
import { api, jsonRequest } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { Announcement, AnnouncementDraft } from '@players/api/types'
import { AnnouncementPager, Pinned, announcementAddress } from '@players/components/announcements'
import { Notice } from '@players/notice'
import { playerKeys, playerQueries } from '@players/queries'
import { useSession } from '@players/session'
import { announcementPageSize, usePageParam } from './announcements'

// As the server counts them: in characters.
const maxTitle = 80
const maxBody = 4000
const blank: AnnouncementDraft = { title: '', body: '', pinned: false }

// The manage panel's part about what the site announces: write a new one, change or take down
// those there are. Only the site's administrator sees it; the server decides.
export function AnnouncementsAdminPage() {
  const { t } = useTranslation()
  const session = useSession()
  const allowed = !!session.user?.siteAdmin
  const [page, turn] = usePageParam()
  const read = useQuery({
    ...playerQueries.announcements(page, announcementPageSize),
    enabled: allowed,
    placeholderData: keepPreviousData,
  })
  // What is being written, and which announcement it changes (none: a new one).
  const [draft, setDraft] = useState(blank)
  const [editing, setEditing] = useState<Announcement | null>(null)
  const [message, setMessage] = useState<{ kind: 'error' | 'info'; text: string } | null>(null)
  const [busy, setBusy] = useState(false)

  if (session.loading) return null
  if (!allowed) return <Navigate to="/" replace />

  const write = (change: Partial<AnnouncementDraft>) => {
    setMessage(null)
    setDraft({ ...draft, ...change })
  }
  const start = (item: Announcement | null) => {
    setMessage(null)
    setEditing(item)
    setDraft(item ? { title: item.title, body: item.body, pinned: item.pinned } : blank)
  }
  // One request to the server at a time; afterwards the lists here and on the home page are
  // read anew.
  const send = async (request: () => Promise<unknown>, done: string) => {
    setBusy(true)
    setMessage(null)
    try {
      await request()
      setMessage({ kind: 'info', text: done })
      void queryClient.invalidateQueries({ queryKey: playerKeys.announcements() })
      return true
    } catch (e) {
      setMessage({ kind: 'error', text: (e as Error).message })
      return false
    } finally {
      setBusy(false)
    }
  }
  const save = async () => {
    const saved = await send(
      () =>
        editing
          ? api(
              endpoints.adminAnnouncement(editing.id),
              jsonRequest('PUT', draft, session.csrfToken),
            )
          : api(endpoints.adminAnnouncements, jsonRequest('POST', draft, session.csrfToken)),
      t(editing ? 'announcements.saved' : 'announcements.published'),
    )
    if (saved) {
      setEditing(null)
      setDraft(blank)
    }
  }
  const remove = async (item: Announcement) => {
    if (!window.confirm(t('announcements.confirmDelete', { title: item.title }))) return
    const removed = await send(
      () =>
        api(
          endpoints.adminAnnouncement(item.id),
          jsonRequest('DELETE', undefined, session.csrfToken),
        ),
      t('announcements.deleted'),
    )
    if (removed && editing?.id === item.id) {
      setEditing(null)
      setDraft(blank)
    }
  }
  const list = read.data
  const notice =
    message ?? (read.error && !list && { kind: 'error' as const, text: read.error.message })

  return (
    <div className="space-y-6" data-testid="announcements-panel">
      <div className="space-y-1">
        <h1 className="font-semibold text-2xl tracking-tight">{t('announcements.adminHeading')}</h1>
        <p className="text-muted-foreground text-sm">{t('announcements.adminIntro')}</p>
      </div>
      {notice && <Notice kind={notice.kind}>{notice.text}</Notice>}

      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>
            {t(editing ? 'announcements.editHeading' : 'announcements.newHeading')}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault()
              void save()
            }}
          >
            <div className="space-y-1.5">
              <Label htmlFor="announcement-title">
                {t('announcements.title', { max: maxTitle })}
              </Label>
              <Input
                id="announcement-title"
                value={draft.title}
                maxLength={maxTitle}
                required
                autoComplete="off"
                disabled={busy}
                onChange={(event) => write({ title: event.target.value })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="announcement-body">{t('announcements.body', { max: maxBody })}</Label>
              <Textarea
                id="announcement-body"
                value={draft.body}
                maxLength={maxBody}
                rows={5}
                disabled={busy}
                onChange={(event) => write({ body: event.target.value })}
              />
            </div>
            <div className="flex items-center gap-3">
              <Checkbox
                id="announcement-pinned"
                checked={draft.pinned}
                disabled={busy}
                onCheckedChange={(on) => write({ pinned: on === true })}
              />
              <Label htmlFor="announcement-pinned">{t('announcements.pin')}</Label>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <Button type="submit" disabled={busy || !draft.title.trim()}>
                {t(editing ? 'announcements.save' : 'announcements.publish')}
              </Button>
              {editing && (
                <Button type="button" variant="outline" disabled={busy} onClick={() => start(null)}>
                  {t('announcements.cancel')}
                </Button>
              )}
            </div>
          </form>
        </CardContent>
      </Card>

      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>{t('announcements.listHeading', { n: list?.total ?? 0 })}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          {list && !list.items.length && (
            <p className="text-muted-foreground text-sm" data-testid="announcements-none">
              {t('announcements.none')}
            </p>
          )}
          {!!list?.items.length && (
            <ul className="divide-y" data-testid="announcements-list">
              {list.items.map((item) => (
                <li key={item.id} className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
                  <span className="flex flex-[1_1_14rem] items-center gap-2 min-w-0">
                    {item.pinned && <Pinned />}
                    <a
                      href={announcementAddress(item.id)}
                      className="font-semibold underline truncate"
                    >
                      {item.title}
                    </a>
                  </span>
                  <span className="text-muted-foreground text-xs">
                    <RelativeTime value={item.publishedAt} />
                  </span>
                  <span className="flex items-center gap-1">
                    <Button variant="outline" size="sm" disabled={busy} onClick={() => start(item)}>
                      {t('announcements.edit')}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:text-destructive"
                      disabled={busy}
                      onClick={() => void remove(item)}
                    >
                      {t('announcements.delete')}
                    </Button>
                  </span>
                </li>
              ))}
            </ul>
          )}
          {list && <AnnouncementPager list={list} onTurn={turn} />}
        </CardContent>
      </Card>
    </div>
  )
}
