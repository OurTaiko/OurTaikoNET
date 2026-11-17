import { SITE_API_ROOT } from './api/config'

// Preserve private/signed/custom avatars; only public SSO CDN objects are routed.
export function avatarSource(original: string): string {
  if (
    !/^https:\/\/distrib\.ourtaiko\.org\/sso\/avatars\/[a-f0-9]{32}\/[a-f0-9]{32}\.webp$/.test(
      original,
    )
  )
    return original
  return `${SITE_API_ROOT}/distribution/avatar?${new URLSearchParams({ url: original, redirect: '1' })}`
}
