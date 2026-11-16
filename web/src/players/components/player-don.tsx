// A player's own 3D Don, small, as the game stands it beside their nameplate on the song select
// (SELECT_LOOP): the look they chose on their page, from the public bucket. Loaded lazily, so three.js
// stays out of the pages that only link here; draws nothing until the catalog and look are read.
import { useEffect, useState } from 'react'
import { api } from '@players/api/client'
import { endpoints } from '@players/api/endpoints'
import { readDonCatalog, type Catalog } from '@players/don-catalog'
import { DonPreview, type DonLook } from '@sitekit/don3d'
import { cn } from '@sitekit/lib/utils'

// A tighter frame than the dressing room's 480x560: the Don and its puchichara with their feet on
// the bottom edge, so a small box is mostly Don.
const frame = { width: 340, height: 340, pivotX: 0.56, pivotY: 0.03 }

export default function PlayerDon({ userId, className }: { userId: string; className?: string }) {
  const [don, setDon] = useState<{ catalog: Catalog; look: DonLook } | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    setDon(null)
    Promise.all([
      readDonCatalog(),
      api<{ look: DonLook }>(endpoints.userDon(userId), { signal: controller.signal }),
    ])
      .then(([catalog, answer]) => setDon({ catalog, look: answer.look }))
      .catch(() => undefined) // an extra beside the plate: without it the plate stands alone
    return () => controller.abort()
  }, [userId])
  return (
    <div className={cn('relative aspect-square', className)} aria-hidden="true">
      {don && (
        <DonPreview
          catalog={don.catalog}
          baseUrl={don.catalog.baseUrl}
          look={don.look}
          motion="SelectLoop"
          frame={frame}
          className="absolute inset-0"
        />
      )}
    </div>
  )
}
