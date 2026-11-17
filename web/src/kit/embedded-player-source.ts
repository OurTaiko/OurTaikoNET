export async function resolvePlayerUrl({
  manifestUrl,
  parentOrigin,
  signal,
  fallbackManifestUrl,
}: {
  manifestUrl: string
  parentOrigin: string
  signal: AbortSignal
  fallbackManifestUrl?: string
}): Promise<string> {
  let manifest = new URL(manifestUrl, parentOrigin)
  let build: unknown
  try {
    const response = await fetch(manifest, { cache: 'no-store', credentials: 'omit', signal })
    if (!response.ok) throw new Error('PLAYER_MANIFEST_UNAVAILABLE')
    build = await response.json()
  } catch (error) {
    if (!fallbackManifestUrl || signal.aborted) throw error
    manifest = new URL(fallbackManifestUrl, parentOrigin)
    const response = await fetch(manifest, { cache: 'no-store', credentials: 'omit', signal })
    if (!response.ok) throw new Error('PLAYER_MANIFEST_UNAVAILABLE', { cause: error })
    build = await response.json()
  }
  if (
    !build ||
    typeof build !== 'object' ||
    !('path' in build) ||
    typeof build.path !== 'string' ||
    !/^\/player\/[a-f0-9]{16}\/index\.html$/.test(build.path)
  )
    throw new Error('PLAYER_MANIFEST_INVALID')
  // Paths belong to the manifest's CDN, never the embedding website.
  // The regional API supplies the CDN root, including an OSS prefix. Absolute
  // manifest paths must not drop that prefix (e.g. /ourtaiko/).
  let base = new URL('.', manifest)
  if ('baseUrl' in build && typeof build.baseUrl === 'string') {
    base = new URL(build.baseUrl)
    if (
      base.protocol !== 'https:' ||
      base.username ||
      base.password ||
      base.search ||
      base.hash ||
      !base.pathname.endsWith('/')
    )
      throw new Error('PLAYER_URL_INVALID')
  }
  const url = new URL(build.path.slice(1), base)
  if (!['https:', 'http:'].includes(url.protocol)) throw new Error('PLAYER_URL_INVALID')
  url.searchParams.set('parentOrigin', parentOrigin)
  return url.href
}

export function canonicalPlayerUrl(selected: string): string {
  const url = new URL(selected)
  const path = url.pathname.match(/\/player\/[a-f0-9]{16}\/index\.html$/)?.[0]
  if (!path) throw new Error('PLAYER_URL_INVALID')
  return new URL(path + url.search, 'https://distrib.ourtaiko.org').href
}
