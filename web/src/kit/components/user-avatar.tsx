import { Avatar, AvatarFallback, AvatarImage } from './ui/avatar'
import { cn } from 'cn'
import { useState } from 'react'
import { avatarSource } from '../avatar-source'
import { useDistributionSource } from '../distribution'

// Shows the SSO avatar when present; Base UI keeps the initial visible until the
// image loads and restores it if loading fails. Mainland visitors get the mirror's copy
// (one source answer per page load); if that copy fails, the original address is tried.
export function UserAvatar({
  nickname,
  avatarUrl,
  className,
  fallbackClassName,
}: {
  nickname: string
  avatarUrl?: string
  className?: string
  fallbackClassName?: string
}) {
  const source = useDistributionSource()
  const [failed, setFailed] = useState('')
  const src =
    avatarUrl && source ? (failed === avatarUrl ? avatarUrl : avatarSource(avatarUrl, source)) : ''
  return (
    <Avatar className={className}>
      {src && (
        <AvatarImage
          src={src}
          alt=""
          // Base UI loads the picture off-screen and mounts the <img> only once it has
          // loaded, so a failure is reported here rather than as the element's error event.
          onLoadingStatusChange={(status) => {
            if (status === 'error' && avatarUrl && src !== avatarUrl) setFailed(avatarUrl)
          }}
        />
      )}
      <AvatarFallback className={cn(fallbackClassName)}>{Array.from(nickname)[0]}</AvatarFallback>
    </Avatar>
  )
}
