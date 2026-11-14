import { useState } from 'react'

// The cover a card shows: `source`, unless there is none or it failed to load.
export function useCardCover(source?: string) {
  const [failedSource, setFailedSource] = useState('')
  return {
    source: source && failedSource !== source ? source : undefined,
    onError: () => setFailedSource(source ?? ''),
  }
}
