// Which commit of the source the website on show was built from. The build is told by
// VITE_REVISION (vite.config.ts takes it from GITHUB_SHA, which the deploy workflow's build has);
// a build made elsewhere, and the development server, have none.
//
// It is the website's build only. The backend is published apart from it, and only when its
// own code changed, so the two may be of different commits.

export type Revision = { full: string; short: string }

/** The commit `raw` names: a full Git hash (40 hex digits), else none. */
export function revisionOf(raw: string | undefined): Revision | null {
  const full = (raw ?? '').trim().toLowerCase()
  return /^[0-9a-f]{40}$/.test(full) ? { full, short: full.slice(0, 7) } : null
}

/** Where a commit is shown in the repository at `repository` (its GitHub address). */
export function commitAddress(repository: string, revision: Revision) {
  return `${repository}/commit/${revision.full}`
}

export const revision = revisionOf(import.meta.env.VITE_REVISION)
