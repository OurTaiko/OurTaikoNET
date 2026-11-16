import { api } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import type { DonCatalog } from '@sitekit/don3d'

// The 3D Don's catalog (GET /don/catalog): what can be worn, and where the bucket's files are.
export type Catalog = DonCatalog & { baseUrl: string }

// The catalog is the same for every page: read once.
let catalogRequest: Promise<Catalog> | null = null
export function readDonCatalog(): Promise<Catalog> {
  catalogRequest ??= api<Catalog>(endpoints.donCatalog).catch((e: unknown) => {
    catalogRequest = null
    throw e
  })
  return catalogRequest
}
