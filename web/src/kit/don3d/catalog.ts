// The 3dchara folder's catalog.json (written by tools/3dchara/catalog.py): every file with its
// path, size and SHA-256, so a file is asked for as "<path>?v=<first 12 of the hash>".

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
  // Added by the API for a mainland visitor: the files to fetch from the mirror, at
  // mirrorBaseUrl + path + "?v=..." like the canonical ones.
  mirrorBaseUrl?: string
  mirrorPaths?: string[]
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

export type DonMirror = Pick<DonCatalog, 'mirrorBaseUrl' | 'mirrorPaths'>

const mirrored = new WeakMap<string[], Set<string>>()

// A file's address: on the mirror when the API listed it there, else under baseUrl.
export function catalogUrl(baseUrl: string, file: DonCatalogFile, mirror?: DonMirror): string {
  const paths = mirror?.mirrorPaths
  if (mirror?.mirrorBaseUrl && paths) {
    let set = mirrored.get(paths)
    if (!set) {
      set = new Set(paths)
      mirrored.set(paths, set)
    }
    if (set.has(file.path)) baseUrl = mirror.mirrorBaseUrl
  }
  const base = baseUrl.endsWith('/') || baseUrl === '' ? baseUrl : baseUrl + '/'
  return base + file.path + '?v=' + file.sha256.slice(0, 12)
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
