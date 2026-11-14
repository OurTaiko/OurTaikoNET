import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate } from 'react-router-dom'
import { api, jsonRequest } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { Group, GroupMember } from '@players/api/types'
import { Button } from '@sitekit/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@sitekit/components/ui/card'
import { Checkbox } from '@sitekit/components/ui/checkbox'
import { Input } from '@sitekit/components/ui/input'
import { Label } from '@sitekit/components/ui/label'
import { UserAvatar } from '@sitekit/components/user-avatar'
import { Notice } from '@players/notice'
import { useSession } from '@players/session-context'

// A player by the name people know them by: avatar, nickname, and the login name when that
// is how they were found. The id is in the link and nowhere on the page.
function Player({ member }: { member: GroupMember }) {
  const { t } = useTranslation()
  const nickname = member.nickname || t('manage.unnamed')
  return (
    <span className="flex items-center gap-3 min-w-0">
      <UserAvatar nickname={nickname} avatarUrl={member.avatarUrl} className="size-9 shrink-0" />
      <span className="min-w-0">
        <Link className="block font-semibold underline truncate" to={`/players/${member.id}`}>
          {nickname}
        </Link>
        {member.username && (
          <span className="block text-muted-foreground text-xs truncate">@{member.username}</span>
        )}
      </span>
    </span>
  )
}

function GroupNames({ names }: { names: string[] }) {
  return (
    <span className="flex flex-wrap gap-1.5">
      {names.map((name) => (
        <span key={name} className="px-2 py-0.5 border rounded-full font-mono text-xs">
          {name}
        </span>
      ))}
    </span>
  )
}

// The manage panel: which user groups each player is in. Only the site's administrator (the
// account center's administrator of this site) sees it. A group lets its members use what is
// not for everyone, such as uploading official charts; the administrator is in no group by
// being administrator.
export function ManagePage() {
  const { t } = useTranslation()
  const session = useSession()
  const allowed = !!session.user?.siteAdmin
  const [groups, setGroups] = useState<Group[]>([])
  const [members, setMembers] = useState<GroupMember[]>([])
  // The player whose groups are being set, and the boxes ticked for them.
  const [editing, setEditing] = useState<{ member: GroupMember; groups: string[] } | null>(null)
  // What is typed in the search box, and the players the last finished search of it found.
  const [lookup, setLookup] = useState('')
  const [found, setFound] = useState<{ text: string; users: GroupMember[] } | null>(null)
  const [message, setMessage] = useState<{ kind: 'error' | 'info'; text: string } | null>(null)
  const [busy, setBusy] = useState(false)

  const reload = useCallback(async () => {
    const [catalog, listed] = await Promise.all([
      api<{ groups: Group[] }>(endpoints.groups),
      api<{ users: GroupMember[] }>(endpoints.groupMembers),
    ])
    setGroups(catalog.groups)
    setMembers(listed.users)
  }, [])
  useEffect(() => {
    if (!allowed) return
    reload().catch((e: Error) => setMessage({ kind: 'error', text: e.message }))
  }, [allowed, reload])

  // Searching as the administrator types, a moment after they stop; an answer to an earlier
  // text is dropped.
  const text = lookup.trim()
  useEffect(() => {
    if (!allowed || !text) return
    let dropped = false
    const timer = window.setTimeout(() => {
      api<{ users: GroupMember[] }>(endpoints.findPlayers(text))
        .then((answer) => {
          if (!dropped) setFound({ text, users: answer.users })
        })
        .catch((e: Error) => {
          if (!dropped) setMessage({ kind: 'error', text: e.message })
        })
    }, 250)
    return () => {
      dropped = true
      window.clearTimeout(timer)
    }
  }, [allowed, text])
  const results = found && found.text === text ? found.users : null

  if (session.loading) return null
  if (!allowed) return <Navigate to="/" replace />

  const save = async () => {
    if (!editing) return
    setBusy(true)
    setMessage(null)
    try {
      await api(
        endpoints.userGroups(editing.member.id),
        jsonRequest('PUT', { groups: editing.groups }, session.csrfToken),
      )
      await reload()
      setMessage({ kind: 'info', text: t('manage.saved') })
      setEditing(null)
      setLookup('')
      setFound(null)
    } catch (e) {
      setMessage({ kind: 'error', text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }
  const toggle = (name: string, on: boolean) =>
    setEditing(
      (current) =>
        current && {
          ...current,
          groups: on ? [...current.groups, name] : current.groups.filter((g) => g !== name),
        },
    )

  return (
    <div className="space-y-6" data-testid="manage-panel">
      <div>
        <h1 className="font-semibold text-2xl tracking-tight">{t('manage.heading')}</h1>
      </div>
      {message && <Notice kind={message.kind}>{message.text}</Notice>}

      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>{t('manage.setHeading')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="space-y-1.5">
            <Label htmlFor="manage-player">{t('manage.search')}</Label>
            <Input
              id="manage-player"
              type="search"
              value={lookup}
              onChange={(e) => setLookup(e.target.value)}
              placeholder={t('manage.searchPlaceholder')}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          {text && results && results.length === 0 && (
            <p className="text-muted-foreground text-sm" data-testid="manage-no-match">
              {t('manage.noMatch')}
            </p>
          )}
          {text && results && results.length > 0 && (
            <ul className="px-4 border rounded-2xl divide-y" data-testid="manage-found">
              {results.map((member) => (
                <li key={member.id} className="flex flex-wrap items-center gap-3 py-3">
                  <span className="flex-1 min-w-0">
                    <Player member={member} />
                  </span>
                  <GroupNames names={member.groups} />
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setMessage(null)
                      setEditing({ member, groups: member.groups })
                    }}
                  >
                    {t('manage.choose')}
                  </Button>
                </li>
              ))}
            </ul>
          )}
          {editing && (
            <div className="space-y-4 p-4 border rounded-2xl" data-testid="manage-editing">
              <Player member={editing.member} />
              <div className="space-y-3">
                {groups.map((group) => (
                  <div key={group.name} className="flex items-start gap-3">
                    <Checkbox
                      id={`group-${group.name}`}
                      checked={editing.groups.includes(group.name)}
                      onCheckedChange={(on) => toggle(group.name, on === true)}
                    />
                    <div className="space-y-0.5">
                      <Label htmlFor={`group-${group.name}`} className="font-mono">
                        {group.name}
                      </Label>
                      <p className="text-muted-foreground text-sm">{group.about}</p>
                    </div>
                  </div>
                ))}
              </div>
              <div className="flex gap-2">
                <Button onClick={() => void save()} disabled={busy}>
                  {t('manage.save')}
                </Button>
                <Button variant="ghost" onClick={() => setEditing(null)} disabled={busy}>
                  {t('manage.cancel')}
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>{t('manage.membersHeading')}</CardTitle>
        </CardHeader>
        <CardContent>
          {members.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('manage.nobody')}</p>
          ) : (
            <ul className="divide-y" data-testid="manage-members">
              {members.map((member) => (
                <li key={member.id} className="flex flex-wrap items-center gap-3 py-3">
                  <span className="flex-1 min-w-0">
                    <Player member={member} />
                  </span>
                  <GroupNames names={member.groups} />
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setMessage(null)
                      setEditing({ member, groups: member.groups })
                    }}
                  >
                    {t('manage.change')}
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
