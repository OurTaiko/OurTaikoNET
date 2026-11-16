import { useEffect, useState } from 'react'
import { PLAYERS_API } from './components/profile-plate'
import { normalizeLanguage } from './i18n'
import { TitleBandArt, type TitleStyle } from './components/nameplate-art'

// The title catalog of OurTaikoProfile (profile.ourtaiko.org), which 段位 courses give titles
// from. A course may give official titles (once it is official itself), its maker's own
// community titles, and the community titles Profile's administrators keep (no maker).

export type { TitleStyle }
export type ProfileTitle = {
  id: string
  name: string
  nameTranslations: Record<string, string>
  description: string
  style: TitleStyle
  official: boolean
  ownerId: string | null
}

export const profileTitlesPage = '/titles'

let catalog: Promise<ProfileTitle[]> | null = null

async function readCatalog(): Promise<ProfileTitle[]> {
  const response = await fetch(`${PLAYERS_API}/titles`)
  if (!response.ok) throw new Error(String(response.status))
  return ((await response.json()) as { titles: ProfileTitle[] }).titles
}

// The catalog, read once per page load; fresh reads it again (after making a title on Profile).
export function useProfileTitles() {
  const [titles, setTitles] = useState<ProfileTitle[] | null>(null)
  const [failed, setFailed] = useState(false)
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    let live = true
    if (!catalog || revision > 0) catalog = readCatalog()
    const reading = catalog
    reading.then(
      (found) => {
        if (!live) return
        setTitles(found)
        setFailed(false)
      },
      () => {
        if (catalog === reading) catalog = null
        if (live) setFailed(true)
      },
    )
    return () => {
      live = false
    }
  }, [revision])
  return { titles, failed, reload: () => setRevision((r) => r + 1) }
}

// Whether a course made by owner can give the title (official ones once it is official).
export function titleUsableBy(title: ProfileTitle, owner: string) {
  return title.official || title.ownerId === null || title.ownerId === owner
}

// The title's name in the page's language, else its own.
export function titleText(title: ProfileTitle, language?: string) {
  return title.nameTranslations?.[normalizeLanguage(language)] || title.name
}

// The title on its band, cut from the game's own band art (the public bucket's nameplate/).
export function TitleBand({
  title,
  language,
  className,
}: {
  title: ProfileTitle
  language?: string
  className?: string
}) {
  return (
    <TitleBandArt style={title.style} name={titleText(title, language)} className={className} />
  )
}
