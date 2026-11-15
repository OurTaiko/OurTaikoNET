import { useEffect, useState } from 'react'
import { SITE_API_ROOT } from '../api/config'
import { PLAYERS_BASE } from '../source'
import { useTranslation } from 'react-i18next'
import { cn } from '../lib/utils'
import { DanEmblem, TitleBandArt } from './nameplate-art'

// The title and 段位 a player wears (the players' pages keep them), drawn next to
// their name. Lookups for the names on a page are gathered into one request.
// Players' pages and their API are this site's own.
export const PROFILE_URL = PLAYERS_BASE
export const PLAYERS_API = SITE_API_ROOT
export const profilePage = (id: string) => `${PROFILE_URL}/${encodeURIComponent(id)}`

type Style = 'wood' | 'teal' | 'wood2' | 'purple' | 'gold' | 'rainbow'
type Plate = {
  title: { name: string; nameTranslations?: Record<string, string>; style: Style } | null
  dan: { index: number; gold: boolean; combo?: 0 | 1 | 2 } | null
}

const danNames = [
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
]
const plates = new Map<string, Promise<Plate | null>>()
let waiting: { id: string; resolve: (plate: Plate | null) => void }[] = []

async function flush() {
  const batch = waiting
  waiting = []
  for (let i = 0; i < batch.length; i += 100) {
    const part = batch.slice(i, i + 100)
    let found: Record<string, Plate> = {}
    try {
      const response = await fetch(
        `${PLAYERS_API}/nameplates?ids=${part.map((p) => p.id).join(',')}`,
      )
      if (response.ok) found = (await response.json()).nameplates ?? {}
    } catch {
      // Profile unreachable: the names show without plates.
    }
    for (const p of part) p.resolve(found[p.id] ?? null)
  }
}

function plateOf(id: string): Promise<Plate | null> {
  let plate = plates.get(id)
  if (!plate) {
    plate = new Promise((resolve) => {
      if (waiting.length === 0) queueMicrotask(() => void flush())
      waiting.push({ id, resolve })
    })
    plates.set(id, plate)
  }
  return plate
}

export function ProfilePlate({ userId, className }: { userId: string; className?: string }) {
  const { i18n } = useTranslation()
  const [plate, setPlate] = useState<Plate | null>(null)
  useEffect(() => {
    let current = true
    void plateOf(userId).then((p) => current && setPlate(p))
    return () => {
      current = false
    }
  }, [userId])
  if (!plate || (!plate.title && !plate.dan)) return null
  const language = i18n.resolvedLanguage === 'zh-Hans' ? 'zh-Hans' : (i18n.resolvedLanguage ?? '')
  return (
    <span className={cn('inline-flex max-w-full items-center gap-1 align-middle', className)}>
      {plate.dan && (
        <DanEmblem
          index={plate.dan.index}
          gold={plate.dan.gold}
          combo={plate.dan.combo ?? 0}
          label={danNames[plate.dan.index] ?? ''}
          className="h-5"
        />
      )}
      {plate.title && (
        <TitleBandArt
          style={plate.title.style}
          name={plate.title.nameTranslations?.[language] || plate.title.name}
          className="h-5"
        />
      )}
    </span>
  )
}
