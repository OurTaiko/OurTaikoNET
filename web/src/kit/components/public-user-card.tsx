import { CalendarBlankIcon, ClockIcon } from '@phosphor-icons/react'
import { useTranslation } from 'react-i18next'
import { cn } from 'cn'
import type { PublicUser } from '../api/types'
import { RelativeTime } from '../comments'
import { formatLocale } from '../i18n'
import type { DirectorySort } from '../queries/directory'
import { PlayerLink } from './player-link'
import { ProfilePlate } from './profile-plate'
import { ScrollingText } from './scrolling-text'
import { UserAvatar } from './user-avatar'
import { compactNumber } from './vote-control'

/**
 * One player of the directory, small enough that a screen holds many: who they are, what they
 * wear, when they were seen, and their numbers. Every card has the same height: each part has
 * one line, kept when it is empty, and a long name slides sideways. The whole card opens their
 * page. What the list is sorted by stands out.
 */
export function PublicUserCard({ user, sort }: { user: PublicUser; sort: DirectorySort }) {
  const { t, i18n } = useTranslation()
  const locale = formatLocale(i18n.language)
  const nickname = user.nickname ?? t('messages.unknownMember')
  const stats = [
    { key: 'charts', value: user.chartCount, label: t('messages.memberCharts') },
    { key: 'plays', value: user.scoreCount, label: t('messages.memberPlays') },
    { key: 'playlists', value: user.playlistCount, label: t('messages.memberPlaylists') },
    { key: 'comments', value: user.commentCount, label: t('interactions.publicCommentCount') },
    {
      key: 'karma',
      value: user.karma,
      label: t('interactions.karma'),
      title: t('interactions.karmaExplanation'),
    },
  ]
  return (
    <div className="group relative flex min-w-0 flex-col gap-3 rounded-[20px] bg-card p-4 shadow-elev-1 transition-[translate,box-shadow] duration-200 ease-emphasized focus-within:ring-3 focus-within:ring-ring focus-within:ring-offset-2 hover:-translate-y-0.5 hover:shadow-elev-2 motion-reduce:transform-none motion-reduce:transition-none">
      <div className="flex min-w-0 items-center gap-3">
        <UserAvatar
          nickname={nickname}
          avatarPath={user.avatarPath}
          className="size-12 shrink-0"
          fallbackClassName="text-lg"
        />
        <div className="min-w-0 flex-1 space-y-1">
          <h2 className="leading-5 font-bold" title={nickname}>
            {/* The link covers the card, so the numbers below stay readable text. */}
            <PlayerLink id={user.id} className="outline-none after:absolute after:inset-0">
              <ScrollingText>{nickname}</ScrollingText>
            </PlayerLink>
          </h2>
          {/* What the player wears has its line whether they wear anything or not. */}
          <div className="flex h-5 min-w-0 items-center overflow-hidden">
            <ProfilePlate userId={user.id} />
          </div>
          <dl className="flex h-4 min-w-0 gap-x-3 overflow-hidden text-xs whitespace-nowrap text-muted-foreground">
            <div
              className={cn(
                'flex shrink-0 items-center gap-1',
                sort === 'active' && 'text-foreground',
              )}
            >
              <dt title={t('messages.lastActive')}>
                <ClockIcon className="size-3.5" aria-hidden="true" />
                <span className="sr-only">{t('messages.lastActive')}</span>
              </dt>
              <dd>
                {user.lastActiveAt ? (
                  <RelativeTime value={user.lastActiveAt} />
                ) : (
                  t('messages.noActivityRecord')
                )}
              </dd>
            </div>
            {user.firstLoginAt && (
              <div
                className={cn(
                  'flex min-w-0 items-center gap-1',
                  sort === 'newest' && 'text-foreground',
                )}
              >
                <dt title={t('messages.firstLogin')}>
                  <CalendarBlankIcon className="size-3.5" aria-hidden="true" />
                  <span className="sr-only">{t('messages.firstLogin')}</span>
                </dt>
                <dd className="truncate">
                  <time dateTime={user.firstLoginAt}>
                    {new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(
                      new Date(user.firstLoginAt),
                    )}
                  </time>
                </dd>
              </div>
            )}
          </dl>
        </div>
      </div>
      <dl className="grid grid-cols-5 gap-1 border-t pt-3 text-center">
        {stats.map((stat) => (
          <div key={stat.key} className="flex min-w-0 flex-col-reverse" title={stat.title}>
            <dt className="truncate text-[11px] leading-4 text-muted-foreground">{stat.label}</dt>
            <dd
              className={cn(
                'text-sm font-bold tabular-nums',
                stat.value === 0 && 'font-normal text-muted-foreground',
                sort === 'karma' && stat.key === 'karma' && 'text-primary',
              )}
              title={stat.value.toLocaleString(locale)}
            >
              {compactNumber(stat.value, i18n.language)}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
