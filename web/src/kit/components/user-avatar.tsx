import { Avatar, AvatarFallback, AvatarImage } from './ui/avatar'
import { cn } from 'cn'
import { useFileUrl } from '../distribution'

// Shows the user's avatar when they have one (`avatarPath`, a path under the distribution
// root); Base UI keeps the initial visible until the image loads and restores it if loading
// fails. The picture comes from the visitor's root (the mirror for mainland visitors), and from
// the canonical root once that copy fails.
export function UserAvatar({
  nickname,
  avatarPath,
  className,
  fallbackClassName,
}: {
  nickname: string
  avatarPath?: string
  className?: string
  fallbackClassName?: string
}) {
  const picture = useFileUrl(avatarPath)
  return (
    <Avatar className={className}>
      {picture.source && (
        <AvatarImage
          src={picture.source}
          alt=""
          // Base UI loads the picture off-screen and mounts the <img> only once it has
          // loaded, so a failure is reported here rather than as the element's error event.
          onLoadingStatusChange={(status) => {
            if (status === 'error') picture.onError()
          }}
        />
      )}
      <AvatarFallback className={cn(fallbackClassName)}>{Array.from(nickname)[0]}</AvatarFallback>
    </Avatar>
  )
}
