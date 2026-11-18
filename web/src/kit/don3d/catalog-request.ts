import { api } from '../api/client'
import { SITE_API_ROOT } from '../api/config'
import { askSource } from '../distribution'
import type { DonCatalog, DonRoot } from './catalog'

// The 3D Don's catalog (GET /api/v2/don/catalog): what can be worn, every file named by its path
// under the distribution root. `root` is where this visitor reads those files (distribution.ts).
export type ServedDonCatalog = DonCatalog & { root: DonRoot }

// The catalog is the same for every page: read once.
let catalogRequest: Promise<ServedDonCatalog> | null = null
export function readDonCatalog(): Promise<ServedDonCatalog> {
  catalogRequest ??= Promise.all([api<DonCatalog>(`${SITE_API_ROOT}/don/catalog`), askSource()])
    .then(([catalog, root]) => ({ ...catalog, root }))
    .catch((e: unknown) => {
      catalogRequest = null
      throw e
    })
  return catalogRequest
}
