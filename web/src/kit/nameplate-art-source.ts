import { verifiedResource, type ResourceFile } from './resource-download'

const canonical = 'https://distrib.ourtaiko.org/nameplate/'
type File = { path: string; size: number; sha256: string }
type Catalog = {
  version: number
  baseUrl?: string
  mirrorBaseUrl?: string
  mirrorPaths?: string[]
  files: Record<string, File>
}

export async function nameplateArtSources(): Promise<Record<string, ResourceFile>> {
  let catalog: Catalog
  try {
    const response = await fetch('/api/v1/nameplate-art/catalog')
    if (!response.ok) throw new Error('NAMEPLATE_CATALOG_UNAVAILABLE')
    catalog = (await response.json()) as Catalog
  } catch {
    const response = await fetch(canonical + 'catalog.json', { credentials: 'omit' })
    if (!response.ok) throw new Error('NAMEPLATE_CATALOG_UNAVAILABLE')
    catalog = (await response.json()) as Catalog
  }
  if (
    catalog.version !== 1 ||
    !catalog.files ||
    Object.keys(catalog.files).length === 0 ||
    Object.keys(catalog.files).length > 128
  )
    throw new Error('NAMEPLATE_CATALOG_INVALID')
  const paths = new Set(catalog.mirrorPaths)
  return Object.fromEntries(
    Object.entries(catalog.files).map(([key, file]) => {
      if (
        !/^[a-zA-Z0-9_-][a-zA-Z0-9_.-]*\.png$/.test(file.path) ||
        !/^[a-f0-9]{64}$/.test(file.sha256) ||
        !Number.isSafeInteger(file.size) ||
        file.size <= 0 ||
        file.size > 8 * 1024 * 1024
      )
        throw new Error('NAMEPLATE_CATALOG_INVALID')
      const suffix = file.path + '?v=' + file.sha256.slice(0, 12)
      const original = (catalog.baseUrl ?? canonical) + suffix
      const mirrored =
        catalog.mirrorBaseUrl && paths.has(file.path) ? catalog.mirrorBaseUrl + suffix : original
      return [
        key,
        {
          url: mirrored,
          fallbackUrl: mirrored === original ? undefined : original,
          size: file.size,
          sha256: file.sha256,
        },
      ]
    }),
  )
}

export async function loadNameplateArt(): Promise<Record<string, string>> {
  const files = await nameplateArtSources()
  const created: string[] = []
  try {
    const downloaded = await Promise.all(
      Object.entries(files).map(
        async ([key, file]) => [key, await verifiedResource(file)] as const,
      ),
    )
    return Object.fromEntries(
      downloaded.map(([key, bytes]) => {
        const url = URL.createObjectURL(new Blob([bytes], { type: 'image/png' }))
        created.push(url)
        return [key, url]
      }),
    )
  } catch (error) {
    created.forEach((url) => URL.revokeObjectURL(url))
    throw error
  }
}
