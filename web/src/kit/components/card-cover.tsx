import { useState } from 'react'
import type { SyntheticEvent } from 'react'
import { retryResourceOrigin } from '../resource-download'

// The cover a card shows: `source`, unless there is none or it failed to load.
export function useCardCover(source?: string) {
  const [failedSource, setFailedSource] = useState('')
  return {
    source: source && failedSource !== source ? source : undefined,
    onError: (event?: SyntheticEvent<HTMLImageElement>) => {
      if (!event || !retryResourceOrigin(event.currentTarget)) setFailedSource(source ?? '')
    },
  }
}
