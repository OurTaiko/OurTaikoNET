import { queryOptions, useQuery } from '@tanstack/react-query'
import { PLAYERS_API } from './components/profile-plate'
import { normalizeLanguage } from './i18n'
import { TitleBandArt } from './components/nameplate-art'
import { layersOf, type TitleLayers } from './nameplate-layers'

// The title catalog, which 段位 courses give titles from: the public titles (a private one is
// its maker's alone). A course names any of them, and gives them once the site's managers have
// made it official.

export type ProfileTitle = {
  id: string
  name: string
  nameTranslations: Record<string, string>
  description: string
  style: string
  // Absent in a reward given before titles had layers: its style stands for them.
  layers?: TitleLayers
  // Absent in a reward given before titles were private or public.
  visibility?: 'private' | 'public'
  ownerId: string | null
}

export const profileTitlesPage = '/titles'

async function readCatalog(): Promise<ProfileTitle[]> {
  const response = await fetch(`${PLAYERS_API}/titles`)
  if (!response.ok) throw new Error(String(response.status))
  return ((await response.json()) as { titles: ProfileTitle[] }).titles
}

// The catalog is the same whichever song source is shown.
const catalogQuery = queryOptions({
  queryKey: ['profile-titles'],
  queryFn: readCatalog,
  staleTime: Infinity,
  gcTime: Infinity,
  retry: false,
})

// The catalog, read once per page load; reload reads it again (after making a title on Profile).
export function useProfileTitles() {
  const catalog = useQuery(catalogQuery)
  return {
    titles: catalog.data ?? null,
    failed: catalog.isError,
    reload: () => void catalog.refetch(),
  }
}

// The title's name in the page's language, else its own.
export function titleText(title: ProfileTitle, language?: string) {
  return title.nameTranslations?.[normalizeLanguage(language)] || title.name
}

// The title on its band, its layers drawn as the game draws them (the public bucket's nameplate/).
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
    <TitleBandArt
      layers={layersOf(title)}
      name={titleText(title, language)}
      className={className}
    />
  )
}
