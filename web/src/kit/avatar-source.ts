import { onSource, type DistributionSource } from './distribution'

const publicAvatar =
  /^https:\/\/distrib\.ourtaiko\.org\/sso\/avatars\/[a-f0-9]{32}\/[a-f0-9]{32}\.webp$/

// Only the account center's public, content-addressed avatars move to the mirror; signed,
// private and custom pictures keep their own address.
export function avatarSource(original: string, source: DistributionSource): string {
  return publicAvatar.test(original) ? onSource(original, source) : original
}
