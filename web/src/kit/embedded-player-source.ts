// The web player's address: the build named by the canonical manifest, on the visitor's file
// root (distribution.ts). A mainland visitor's root is the mirror; the country alone decides.
export async function resolvePlayerUrl({
  manifestUrl,
  baseUrl,
  parentOrigin,
  signal,
}: {
  manifestUrl: string
  /** The visitor's file root, ending in "/"; the manifest's own root when left out. */
  baseUrl?: string
  parentOrigin: string
  signal: AbortSignal
}): Promise<string> {
  const manifest = new URL(manifestUrl, parentOrigin)
  const response = await fetch(manifest, { cache: 'no-store', credentials: 'omit', signal })
  if (!response.ok) throw new Error('PLAYER_MANIFEST_UNAVAILABLE')
  const build: unknown = await response.json()
  if (
    !build ||
    typeof build !== 'object' ||
    !('path' in build) ||
    typeof build.path !== 'string' ||
    !/^\/player\/[a-f0-9]{16}\/index\.html$/.test(build.path)
  )
    throw new Error('PLAYER_MANIFEST_INVALID')
  // Paths belong to the file root, never the embedding website. The mirror root carries an OSS
  // prefix (e.g. /ourtaiko/) that the manifest's absolute path must not drop.
  const base = new URL(baseUrl ?? '.', manifest)
  if (
    base.protocol !== 'https:' ||
    base.username ||
    base.password ||
    base.search ||
    base.hash ||
    !base.pathname.endsWith('/')
  )
    throw new Error('PLAYER_URL_INVALID')
  const url = new URL(build.path.slice(1), base)
  url.searchParams.set('parentOrigin', parentOrigin)
  return url.href
}

export function canonicalPlayerUrl(selected: string): string {
  const url = new URL(selected)
  const path = url.pathname.match(/\/player\/[a-f0-9]{16}\/index\.html$/)?.[0]
  if (!path) throw new Error('PLAYER_URL_INVALID')
  return new URL(path + url.search, 'https://distrib.ourtaiko.org').href
}
