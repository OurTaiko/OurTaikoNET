import { useState } from 'react'
import type { SyntheticEvent } from 'react'
import { retryResourceOrigin } from '../resource-download'
import {
  canonicalBaseUrl,
  onSource,
  useDistributionSource,
  type DistributionSource,
} from '../distribution'

/**
 * The address a cover is shown from. A cover in the public bucket (named by its hash) comes from
 * the visitor's root, the mirror for mainland visitors; nothing until that root is known, and its
 * original address once the mirror's copy has failed. Any other address (the API's cover route)
 * is used as it is.
 */
export function cardCoverAddress(
  source: string,
  files: DistributionSource | null,
  mirrorFailed: boolean,
): string | undefined {
  if (!source.startsWith(canonicalBaseUrl) || mirrorFailed) return source
  return files ? onSource(source, files) : undefined
}

// The cover a card shows: `source`, unless there is none or it failed to load.
export function useCardCover(source?: string) {
  const files = useDistributionSource()
  const [mirrorFailed, setMirrorFailed] = useState('')
  const [failedSource, setFailedSource] = useState('')
  const shown =
    source && failedSource !== source
      ? cardCoverAddress(source, files, mirrorFailed === source)
      : undefined
  return {
    source: shown,
    onError: (event?: SyntheticEvent<HTMLImageElement>) => {
      if (!source) return
      if (shown && shown !== source) setMirrorFailed(source)
      else if (!event || !retryResourceOrigin(event.currentTarget)) setFailedSource(source)
    },
  }
}
