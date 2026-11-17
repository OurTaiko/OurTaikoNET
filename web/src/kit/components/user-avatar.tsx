import { Avatar, AvatarFallback, AvatarImage } from './ui/avatar'
import { cn } from 'cn'
import { useState } from 'react'
import { avatarSource } from '../avatar-source'

// Shows the SSO avatar when present; Base UI keeps the initial visible until the
// image loads and restores it if loading fails.
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
  const [failed, setFailed] = useState('')
  return (
    <Avatar className={className}>
      {avatarUrl && (
        <AvatarImage
          src={failed === avatarUrl ? avatarUrl : avatarSource(avatarUrl)}
          alt=""
          onError={() => setFailed(avatarUrl)}
        />
      )}
      <AvatarFallback className={cn(fallbackClassName)}>{Array.from(nickname)[0]}</AvatarFallback>
    </Avatar>
  )
}
