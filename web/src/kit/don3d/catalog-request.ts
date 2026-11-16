import { api } from '../api/client'
import { SITE_API_ROOT } from '../api/config'
import type { DonCatalog } from './catalog'

// The 3D Don's catalog (GET /api/v1/don/catalog): what can be worn, and where the bucket's files are.
export type ServedDonCatalog = DonCatalog & { baseUrl: string }

// The catalog is the same for every page: read once.
let catalogRequest: Promise<ServedDonCatalog> | null = null
export function readDonCatalog(): Promise<ServedDonCatalog> {
  catalogRequest ??= api<ServedDonCatalog>(`${SITE_API_ROOT}/don/catalog`).catch((e: unknown) => {
    catalogRequest = null
    throw e
  })
  return catalogRequest
}
