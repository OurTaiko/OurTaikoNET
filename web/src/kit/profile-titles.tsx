import { queryOptions, useQuery } from '@tanstack/react-query'
import { PLAYERS_API } from './components/profile-plate'
import { normalizeLanguage } from './i18n'
import { TitleBandArt } from './components/nameplate-art'
import { layersOf, type TitleLayers } from './nameplate-layers'

// The title catalog: the public titles (a private one is its maker's alone), and the rules that
// give titles and parts of the Don by themselves, among them those for passing a fixed 段位 course
// (dan_clear).

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

/** A rule of the catalog, as far as a song source's pages read it: what passing a 段位 course
 * gives (kind dan_clear). The players' pages have the whole rule (RewardRule). */
export type CatalogRule = {
  id: number
  item: 'title' | 'don_item'
  titleId: string | null
  donPart: string | null
  donItem: number | null
  kind: string
  source: string | null
  danId: string | null
  /** For dan_clear, the verdict it takes: 1–3 red, 4–6 gold (pass, full combo, all good). */
  clear: number | null
}

export const profileTitlesPage = '/titles'

type Catalog = { titles: ProfileTitle[]; rules: CatalogRule[] }

async function readCatalog(): Promise<Catalog> {
  const response = await fetch(`${PLAYERS_API}/titles`)
  if (!response.ok) throw new Error(String(response.status))
  const catalog = (await response.json()) as Partial<Catalog>
  return { titles: catalog.titles ?? [], rules: catalog.rules ?? [] }
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
    titles: catalog.data?.titles ?? null,
    rules: catalog.data?.rules ?? null,
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
