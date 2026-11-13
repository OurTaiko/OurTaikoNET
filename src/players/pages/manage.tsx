import { useCallback, useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate } from 'react-router-dom'
import { api, jsonRequest } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { Group, GroupMember } from '@players/api/types'
import { Button } from '@players/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@players/components/ui/card'
import { Checkbox } from '@players/components/ui/checkbox'
import { Input } from '@players/components/ui/input'
import { Label } from '@players/components/ui/label'
import { Notice } from '@players/notice'
import { useSession } from '@players/session-context'

const playerId = /^[0-9a-f]{32}$/

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
  const [editing, setEditing] = useState<{ id: string; groups: string[] } | null>(null)
  const [lookup, setLookup] = useState('')
  const [message, setMessage] = useState<{ kind: 'error' | 'info'; text: string } | null>(null)
  const [busy, setBusy] = useState(false)

  const reload = useCallback(async () => {
    const [catalog, listed] = await Promise.all([
      api<{ groups: Group[] }>(endpoints.groups),
      api<{ users: GroupMember[] }>(endpoints.groupMembers()),
    ])
    setGroups(catalog.groups)
    setMembers(listed.users)
  }, [])
  useEffect(() => {
    if (!allowed) return
    reload().catch((e: Error) => setMessage({ kind: 'error', text: e.message }))
  }, [allowed, reload])

  if (session.loading) return null
  if (!allowed) return <Navigate to="/" replace />

  const find = async (event: FormEvent) => {
    event.preventDefault()
    const id = lookup.trim().toLowerCase()
    setMessage(null)
    if (!playerId.test(id)) {
      setMessage({ kind: 'error', text: t('manage.idInvalid') })
      return
    }
    try {
      const found = await api<{ users: GroupMember[] }>(endpoints.groupMembers([id]))
      setEditing({ id, groups: found.users[0]?.groups ?? [] })
    } catch (e) {
      setMessage({ kind: 'error', text: (e as Error).message })
    }
  }
  const save = async () => {
    if (!editing) return
    setBusy(true)
    setMessage(null)
    try {
      await api(
        endpoints.userGroups(editing.id),
        jsonRequest('PUT', { groups: editing.groups }, session.csrfToken),
      )
      await reload()
      setMessage({ kind: 'info', text: t('manage.saved') })
      setEditing(null)
      setLookup('')
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
        <h1 className="text-2xl font-semibold tracking-tight">{t('manage.heading')}</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">{t('manage.lead')}</p>
      </div>
      {message && <Notice kind={message.kind}>{message.text}</Notice>}

      <Card className="rounded-3xl">
        <CardHeader>
          <CardTitle>{t('manage.setHeading')}</CardTitle>
          <CardDescription>{t('manage.setLead')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <form className="flex flex-wrap items-end gap-3" onSubmit={find}>
            <div className="min-w-64 flex-1 space-y-1.5">
              <Label htmlFor="manage-player">{t('manage.playerId')}</Label>
              <Input
                id="manage-player"
                value={lookup}
                onChange={(e) => setLookup(e.target.value)}
                placeholder="0123456789abcdef0123456789abcdef"
                autoComplete="off"
                spellCheck={false}
              />
            </div>
            <Button type="submit" variant="outline">
              {t('manage.find')}
            </Button>
          </form>
          {editing && (
            <div className="space-y-4 rounded-2xl border p-4" data-testid="manage-editing">
              <p className="text-sm">
                <Link className="font-mono underline" to={`/users/${editing.id}`}>
                  {editing.id}
                </Link>
              </p>
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
                      <p className="text-sm text-muted-foreground">{group.about}</p>
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
          <CardDescription>{t('manage.membersLead')}</CardDescription>
        </CardHeader>
        <CardContent>
          {members.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('manage.nobody')}</p>
          ) : (
            <ul className="divide-y" data-testid="manage-members">
              {members.map((member) => (
                <li key={member.id} className="flex flex-wrap items-center gap-3 py-3">
                  <Link className="font-mono text-sm underline" to={`/users/${member.id}`}>
                    {member.id}
                  </Link>
                  <span className="flex flex-1 flex-wrap gap-1.5">
                    {member.groups.map((name) => (
                      <span
                        key={name}
                        className="rounded-full border px-2 py-0.5 font-mono text-xs"
                      >
                        {name}
                      </span>
                    ))}
                  </span>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setMessage(null)
                      setEditing({ id: member.id, groups: member.groups })
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
