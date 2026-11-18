// The 3dchara folder's catalog.json (written by tools/3dchara/catalog.py): every file with its
// path, size and SHA-256. A published file is never replaced (a changed file gets a new path), so
// a file is asked for by its path alone. The API answers it with every path from the
// distribution root; the raw file in the folder has them from the folder (dev-page.tsx).
import { canonicalFileUrl, fileUrl, type FileRoot } from '../distribution'

export type DonCatalogFile = { path: string; size: number; sha256: string }

export type DonCatalogItem = {
  id: number
  name: Partial<Record<string, string>>
  file: DonCatalogFile
  icon?: DonCatalogFile
}

export type DonPartKind = 'cos' | 'head' | 'body' | 'puchi' | 'paint'

export type DonCatalog = {
  version: number
  base: { animations: DonCatalogFile; face: DonCatalogFile; faceAnimations: DonCatalogFile }
  defaultColors: { p1: [string, string, string]; p2: [string, string, string] }
  // paint: absent from catalogs written before makeup was added
  items: Record<Exclude<DonPartKind, 'paint'>, DonCatalogItem[]> & { paint?: DonCatalogItem[] }
}

// How a Don looks, as the API gives it: a whole costume, or a head and a body; the puchichara
// beside it (0: none); the makeup (0 or absent: none); the body, face and rim colours (null: 1P's).
export type DonLook = {
  costume: boolean
  cos: number
  head: number
  body: number
  puchi: number
  paint?: number
  colors: [string, string, string] | null
}

// Where a catalog's files are read from: a file is root.baseUrl + its path (the visitor's root,
// the mirror for mainland visitors) and root.canonicalBaseUrl + its path once that fails. The
// API's catalog gives every path from the distribution root ("3dchara/cos/0.glb").
export type DonRoot = FileRoot

/** A file's canonical address, which also names it in the caches (a copy has the same bytes). */
export function catalogUrl(root: DonRoot, file: DonCatalogFile): string {
  return canonicalFileUrl(file.path, root)
}

// Where a file is fetched: its canonical address, and the visitor's root's when that is another
// (the mirror). The mirror is someone else's bucket, so its bytes are checked against sha256
// (assets.ts fetchDonFile) and the canonical address is used when it fails.
export type DonSource = { url: string; mirror: string | null; sha256: string }

export function donSource(root: DonRoot, file: DonCatalogFile): DonSource {
  const url = catalogUrl(root, file)
  const visitor = fileUrl(file.path, root)
  return { url, mirror: visitor === url ? null : visitor, sha256: file.sha256 }
}

// An <img> of a catalog picture: the mirror's, back on the canonical one if it fails to load.
export function donImage(source: DonSource): {
  src: string
  onError?: (event: { currentTarget: HTMLImageElement }) => void
} {
  if (!source.mirror) return { src: source.url }
  return {
    src: source.mirror,
    onError: ({ currentTarget }) => {
      if (currentTarget.getAttribute('src') !== source.url) currentTarget.src = source.url
    },
  }
}

/** An item's name in the reader's language, else Japanese (the cabinet's), else its number. */
export function donItemName(
  item: DonCatalogItem | undefined,
  language: string,
  id: number,
): string {
  if (!item) return `#${id}`
  const short = language.split('-')[0]
  return item.name[language] ?? item.name[short] ?? item.name.ja ?? `#${id}`
}

export function catalogItem(
  catalog: DonCatalog,
  kind: DonPartKind,
  id: number,
): DonCatalogItem | undefined {
  return catalog.items[kind]?.find((item) => item.id === id)
}

// One model file a look wears, and whether its face plate is brought to the standard size.
export type DonLookPart = {
  kind: 'cos' | 'head' | 'body'
  item: DonCatalogItem
  normalizeFace: boolean
}

export type ResolvedDonLook = {
  parts: DonLookPart[]
  puchi: DonCatalogItem | null
  // the makeup's face sheet, drawn instead of the plain one on a face plate a third larger
  paint: DonCatalogItem | null
  colors: [string, string, string]
}

// What a look draws: the costume, or the body then the head (head with the face plate always
// normalised); an id the catalog does not have falls back to 0, puchichara and makeup 0 (or a
// missing one) are none, and no colours are 1P's.
export function resolveDonLook(catalog: DonCatalog, look: DonLook): ResolvedDonLook {
  const pick = (kind: 'cos' | 'head' | 'body', id: number) =>
    catalogItem(catalog, kind, id) ?? catalogItem(catalog, kind, 0)
  const parts: DonLookPart[] = []
  const add = (kind: 'cos' | 'head' | 'body', id: number, normalizeFace: boolean) => {
    const item = pick(kind, id)
    if (item) parts.push({ kind, item, normalizeFace })
  }
  if (look.costume) add('cos', look.cos, false)
  else {
    add('body', look.body, false)
    add('head', look.head, true)
  }
  const puchi = look.puchi > 0 ? (catalogItem(catalog, 'puchi', look.puchi) ?? null) : null
  const paintId = look.paint ?? 0
  const paint = paintId > 0 ? (catalogItem(catalog, 'paint', paintId) ?? null) : null
  return { parts, puchi, paint, colors: look.colors ?? catalog.defaultColors.p1 }
}
