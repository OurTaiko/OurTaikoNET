import { SITE_API_ROOT } from './api/config'
import { verifiedResource, type ResourceFile } from './resource-download'
import { askSource, fallbackFileUrl, fetchFile, fileUrl } from './distribution'

// The nameplate art's catalog: every file with its path under the distribution root
// ("nameplate/shadow.png"), size and SHA-256.
type File = { path: string; size: number; sha256: string }
type Catalog = { version: number; files: Record<string, File> }

export async function nameplateArtSources(): Promise<Record<string, ResourceFile>> {
  let catalog: Catalog
  try {
    const response = await fetch(`${SITE_API_ROOT}/nameplate-art/catalog`)
    if (!response.ok) throw new Error('NAMEPLATE_CATALOG_UNAVAILABLE')
    catalog = (await response.json()) as Catalog
  } catch {
    // Without the API, the catalog comes straight from the bucket, like its files. That file
    // names them from the nameplate folder, so they are given the folder.
    const response = await fetchFile('nameplate/catalog.json', { credentials: 'omit' })
    if (!response.ok) throw new Error('NAMEPLATE_CATALOG_UNAVAILABLE')
    catalog = (await response.json()) as Catalog
    if (catalog.files && typeof catalog.files === 'object')
      for (const file of Object.values(catalog.files))
        if (file && typeof file.path === 'string') file.path = 'nameplate/' + file.path
  }
  if (
    catalog.version !== 1 ||
    !catalog.files ||
    Object.keys(catalog.files).length === 0 ||
    Object.keys(catalog.files).length > 128
  )
    throw new Error('NAMEPLATE_CATALOG_INVALID')
  const source = await askSource()
  return Object.fromEntries(
    Object.entries(catalog.files).map(([key, file]) => {
      if (
        !/^nameplate\/[a-zA-Z0-9_-][a-zA-Z0-9_.-]*\.png$/.test(file.path) ||
        !/^[a-f0-9]{64}$/.test(file.sha256) ||
        !Number.isSafeInteger(file.size) ||
        file.size <= 0 ||
        file.size > 8 * 1024 * 1024
      )
        throw new Error('NAMEPLATE_CATALOG_INVALID')
      return [
        key,
        {
          url: fileUrl(file.path, source),
          fallbackUrl: fallbackFileUrl(file.path, source) || undefined,
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
