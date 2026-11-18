import { useFileUrl } from '../distribution'

// The cover a card shows, from its path under the distribution root: the visitor's root first
// (the mirror for mainland visitors), the canonical root once that copy fails, and none (the
// card's own placeholder) once both failed or when there is no cover.
export const useCardCover = (path?: string) => useFileUrl(path)
