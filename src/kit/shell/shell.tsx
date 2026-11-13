// The site's one frame, the same on every page of every area: a menu that floats at the left
// (a bar at the bottom on a phone), a top row of pills, and the page between them.
//
// The site has three areas, each a single-page app of its own (see main.tsx): the two song
// sources, under /fanmade and /official, and everything that belongs to no source (home,
// players, titles), at the top. The frame's links are whole addresses; one that stays in the
// area it is shown in is a router link, any other loads its area anew.
import { useEffect, useState } from 'react'
import type { ComponentType, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router-dom'
import {
  CompassIcon,
  FolderIcon,
  GameControllerIcon,
  HouseIcon,
  IdentificationCardIcon,
  ListIcon,
  MagnifyingGlassIcon,
  MedalIcon,
  PlaylistIcon,
  ShieldStarIcon,
  SidebarSimpleIcon,
  SignInIcon,
  SignOutIcon,
  TagIcon,
  UsersThreeIcon,
  UsersIcon,
  XIcon,
} from '@phosphor-icons/react'
import { cn } from '../lib/utils'
import { sources, type SourceId } from '../source'
import { ThemeToggle } from '../theme'
import { UserAvatar } from '../components/user-avatar'
import { Button } from '../components/ui/button'
import { rememberedSource, rememberSource, useSiteRoles } from './site'

export type ShellUser = { id: string; nickname: string; avatarUrl?: string }

type Item = {
  key: string
  href: string
  label: string
  icon: ComponentType<{ className?: string; 'aria-hidden'?: boolean | 'true' | 'false' }>
  /** Addresses that light the item besides its own; all start with the item's area. */
  also?: string[]
  /** An address under href that is another item's. */
  except?: string
  /** Lit only at exactly its address. */
  exact?: boolean
  external?: boolean
}

const signInAddress = () =>
  `/api/v1/auth/sso/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`

/**
 * area: where this page is ('' for the top). canUpload: whether the menu offers the chart
 * manager of the source chosen. bell: the area's notification bell, if it has one.
 */
export function Shell({
  area,
  user,
  loading,
  canUpload,
  bell,
  onSignOut,
  children,
}: {
  area: SourceId | ''
  user: ShellUser | null
  loading: boolean
  canUpload: (source: SourceId) => boolean
  bell?: ReactNode
  onSignOut: () => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const base = area ? `/${area}` : ''
  const here = base + (pathname === '/' ? (base ? '/' : '/') : pathname)
  const roles = useSiteRoles(user?.id)
  // The source the Songs group points at: this page's, or the one used last.
  const [chosen, setChosen] = useState<SourceId>(() => area || rememberedSource())
  const source = area || chosen
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem('ourtaiko-net-menu') === 'collapsed'
    } catch {
      return false
    }
  })
  const [open, setOpen] = useState(false)
  useEffect(() => setOpen(false), [pathname])
  useEffect(() => {
    try {
      localStorage.setItem('ourtaiko-net-menu', collapsed ? 'collapsed' : 'open')
    } catch {
      /* The choice is simply not remembered. */
    }
  }, [collapsed])

  const songs: Item[] = [
    {
      key: 'browse',
      href: `/${source}/`,
      label: t('shell.browse'),
      icon: CompassIcon,
      exact: true,
      also: [`/${source}/charts`],
    },
    {
      key: 'playlists',
      href: `/${source}/playlists`,
      label: t('shell.playlists'),
      icon: PlaylistIcon,
    },
    { key: 'dans', href: `/${source}/dans`, label: t('shell.dans'), icon: MedalIcon },
    ...(canUpload(source)
      ? [
          {
            key: 'charts',
            href: `/${source}/me/charts`,
            label: t('shell.charts'),
            icon: FolderIcon,
          },
        ]
      : []),
  ]
  const players: Item[] = [
    {
      key: 'players',
      href: '/players',
      label: t('shell.directory'),
      icon: UsersIcon,
      // Your own page is under You.
      except: user ? `/players/${user.id}` : undefined,
    },
    { key: 'titles', href: '/titles', label: t('shell.titles'), icon: TagIcon },
  ]
  const you: Item[] = user
    ? [
        {
          key: 'me',
          href: `/players/${user.id}`,
          label: t('shell.myPage'),
          icon: IdentificationCardIcon,
        },
        {
          key: 'myPlaylists',
          href: `/${source}/me/playlists`,
          label: t('shell.myPlaylists'),
          icon: PlaylistIcon,
        },
        ...(roles.titleManager
          ? [{ key: 'admin', href: '/admin', label: t('shell.titleAdmin'), icon: ShieldStarIcon }]
          : []),
        ...(roles.siteAdmin
          ? [{ key: 'manage', href: '/manage', label: t('shell.manage'), icon: UsersThreeIcon }]
          : []),
      ]
    : []
  const home: Item = {
    key: 'home',
    href: '/',
    label: t('shell.home'),
    icon: HouseIcon,
    exact: true,
  }
  const player: Item = {
    key: 'player',
    href: 'https://play.ourtaiko.org/',
    label: t('shell.player'),
    icon: GameControllerIcon,
    external: true,
  }
  const lit = (item: Item) =>
    !item.external &&
    here !== item.except &&
    (here === item.href ||
      here === item.href.replace(/\/$/, '') ||
      (!item.exact && here.startsWith(item.href.replace(/\/$/, '') + '/')) ||
      !!item.also?.some((prefix) => here.startsWith(prefix)))

  // Choosing the other source on one of a source's pages goes to that page's twin there;
  // elsewhere it only turns the Songs group to it.
  const choose = (next: SourceId) => {
    rememberSource(next)
    if (!area) return setChosen(next)
    if (next === area) return
    const section = pathname.split('/')[1]
    const twin = ['playlists', 'dans'].includes(section) ? `/${section}` : '/'
    location.assign(`/${next}${twin}`)
  }

  const entry = (item: Item, rail: boolean) => (
    <ShellLink
      key={item.key}
      href={item.href}
      base={base}
      external={item.external}
      aria-current={lit(item) ? 'page' : undefined}
      title={rail ? item.label : undefined}
      className={cn(
        'flex items-center rounded-full font-medium text-muted-foreground text-sm outline-none transition-colors duration-200 ease-emphasized focus-visible:ring-3 focus-visible:ring-ring/40 hover:bg-accent hover:text-foreground',
        rail ? 'size-11 justify-center' : 'gap-3.5 px-4 h-11',
        lit(item) &&
          'bg-secondary font-semibold text-secondary-foreground hover:bg-secondary hover:text-secondary-foreground',
      )}
    >
      <item.icon className="size-5 shrink-0" aria-hidden="true" />
      {rail ? (
        <span className="sr-only">{item.label}</span>
      ) : (
        <span className="truncate">{item.label}</span>
      )}
    </ShellLink>
  )
  const group = (label: string, rail: boolean) =>
    rail ? (
      <div className="mx-auto my-1.5 bg-border w-6 h-px" aria-hidden="true" />
    ) : (
      <div className="px-4 pt-3.5 pb-1.5 font-semibold text-muted-foreground text-xs">{label}</div>
    )
  const sourceSwitch = (rail: boolean) => (
    <div
      role="group"
      aria-label={t('shell.source')}
      data-testid="source-switch"
      className={cn('flex gap-1 bg-muted p-1 rounded-full', rail ? 'flex-col' : 'mx-1 mb-1.5')}
    >
      {sources.map(({ id }) => (
        <button
          key={id}
          type="button"
          aria-pressed={id === source}
          onClick={() => choose(id)}
          title={rail ? t(`shell.${id}`) : undefined}
          className={cn(
            'rounded-full font-semibold text-muted-foreground text-sm outline-none transition-all duration-200 ease-emphasized focus-visible:ring-3 focus-visible:ring-ring/40',
            rail ? 'size-9' : 'flex-1 h-9',
            id === source && 'bg-card text-foreground shadow-elev-1',
          )}
        >
          <span
            aria-hidden="true"
            className={cn(
              'inline-block rounded-full size-2',
              rail ? '' : 'mr-2',
              id === 'official' ? 'bg-don' : 'bg-ka',
            )}
          />
          {rail ? <span className="sr-only">{t(`shell.${id}`)}</span> : t(`shell.${id}`)}
        </button>
      ))}
    </div>
  )
  const account = (rail: boolean) =>
    loading ? null : user ? (
      <div
        className={cn(
          'flex items-center bg-surface-low shadow-elev-1 mt-2.5',
          rail ? 'flex-col gap-1 p-1 rounded-full' : 'gap-3 p-2.5 pl-3 rounded-[20px]',
        )}
      >
        <ShellLink
          href={`/players/${user.id}`}
          base={base}
          className="flex flex-1 items-center gap-3 rounded-full outline-none min-w-0 focus-visible:ring-3 focus-visible:ring-ring/40"
        >
          <UserAvatar nickname={user.nickname} avatarUrl={user.avatarUrl} className="size-9" />
          {!rail && (
            <span data-testid="account-nickname" className="font-semibold text-sm truncate">
              {user.nickname}
            </span>
          )}
        </ShellLink>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('shell.signOut')}
          title={t('shell.signOut')}
          onClick={onSignOut}
        >
          <SignOutIcon aria-hidden="true" />
        </Button>
      </div>
    ) : (
      <a
        href={signInAddress()}
        title={rail ? t('shell.signIn') : undefined}
        className={cn(
          'flex justify-center items-center gap-2 bg-primary mt-2.5 rounded-full font-semibold text-primary-foreground text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/40',
          rail ? 'size-11' : 'h-11',
        )}
      >
        <SignInIcon className="size-5" aria-hidden="true" />
        {rail ? <span className="sr-only">{t('shell.signIn')}</span> : t('shell.signIn')}
      </a>
    )
  const menu = (rail: boolean) => (
    <nav
      aria-label={t('messages.mainNavigation')}
      className={cn(
        'flex flex-col gap-0.5 bg-card shadow-elev-1',
        rail ? 'items-center p-2 rounded-[28px]' : 'p-3 rounded-[28px]',
      )}
    >
      {entry(home, rail)}
      {group(t('shell.songs'), rail)}
      {sourceSwitch(rail)}
      {songs.map((item) => entry(item, rail))}
      {group(t('shell.players'), rail)}
      {players.map((item) => entry(item, rail))}
      {you.length > 0 && group(t('shell.you'), rail)}
      {you.map((item) => entry(item, rail))}
      {group(t('shell.more'), rail)}
      {entry(player, rail)}
      {account(rail)}
    </nav>
  )
  const brand = (
    <ShellLink
      href="/"
      base={base}
      className="rounded-full outline-none font-extrabold text-primary text-xl tracking-tight focus-visible:ring-3 focus-visible:ring-ring/40"
    >
      OurTaiko
    </ShellLink>
  )
  const bar: Item[] = [
    home,
    { ...songs[0], label: t('shell.songs'), exact: false, also: [`/${source}/`] },
    songs[1],
    players[0],
    ...(user ? [{ ...you[0], label: t('shell.you') }] : []),
  ]

  return (
    <div className="isolate relative bg-background min-h-svh text-foreground">
      <Backdrop />
      <div className="flex items-start gap-6 mx-auto p-3 sm:p-4 max-w-[92rem]">
        {/* The menu, on screens wide enough to keep it beside the page. */}
        <aside
          className={cn(
            'hidden lg:flex top-4 sticky flex-col gap-3 max-h-[calc(100svh-2rem)] shrink-0',
            collapsed ? 'w-[3.75rem]' : 'w-64',
          )}
        >
          <div
            className={cn(
              'flex items-center bg-card shadow-elev-1 rounded-full h-14 shrink-0',
              collapsed ? 'justify-center' : 'gap-1 pr-5 pl-1.5 self-start',
            )}
          >
            <Button
              variant="ghost"
              size="icon"
              aria-label={collapsed ? t('shell.expandMenu') : t('shell.collapseMenu')}
              aria-expanded={!collapsed}
              onClick={() => setCollapsed(!collapsed)}
            >
              <SidebarSimpleIcon className="size-5" aria-hidden="true" />
            </Button>
            {!collapsed && brand}
          </div>
          <div className="overflow-y-auto -m-1 p-1 rounded-[28px] min-h-0">{menu(collapsed)}</div>
        </aside>

        <div className="flex flex-col flex-1 gap-6 min-w-0">
          <div className="flex items-center gap-2 sm:gap-3">
            <div className="lg:hidden flex items-center gap-1 bg-card shadow-elev-1 pr-4 pl-1.5 rounded-full h-14 shrink-0">
              <Button
                variant="ghost"
                size="icon"
                aria-label={t('shell.openMenu')}
                aria-expanded={open}
                onClick={() => setOpen(true)}
              >
                <ListIcon className="size-5" aria-hidden="true" />
              </Button>
              {brand}
            </div>
            <form
              action={`/${source}/`}
              role="search"
              className="hidden sm:flex flex-1 items-center gap-2.5 bg-card shadow-elev-1 ml-auto px-5 rounded-full max-w-sm h-14 text-muted-foreground focus-within:ring-3 focus-within:ring-ring/30"
            >
              <MagnifyingGlassIcon className="size-5 shrink-0" aria-hidden="true" />
              <input
                type="search"
                name="q"
                aria-label={t('shell.search')}
                placeholder={t('shell.search')}
                className="flex-1 bg-transparent outline-none min-w-0 text-foreground text-sm placeholder:text-muted-foreground"
              />
            </form>
            <div className="flex items-center bg-card shadow-elev-1 ml-auto sm:ml-0 px-1.5 rounded-full h-14 shrink-0">
              {bell}
              <ThemeToggle />
            </div>
          </div>
          <main id="main" className="space-y-6 pb-24 lg:pb-8 w-full min-w-0">
            {children}
          </main>
        </div>
      </div>

      {/* On a phone the menu opens over the page. */}
      {open && (
        <div className="lg:hidden z-50 fixed inset-0 flex">
          <button
            type="button"
            aria-label={t('shell.closeMenu')}
            className="absolute inset-0 bg-foreground/40 backdrop-blur-[2px]"
            onClick={() => setOpen(false)}
          />
          <div className="relative flex flex-col gap-3 p-3 w-72 max-w-[85vw] overflow-y-auto animate-in slide-in-from-left duration-300 ease-emphasized">
            <div className="flex items-center gap-1 bg-card shadow-elev-1 pr-5 pl-1.5 rounded-full h-14 self-start shrink-0">
              <Button
                variant="ghost"
                size="icon"
                aria-label={t('shell.closeMenu')}
                onClick={() => setOpen(false)}
              >
                <XIcon className="size-5" aria-hidden="true" />
              </Button>
              {brand}
            </div>
            {menu(false)}
          </div>
        </div>
      )}

      {/* … and its main places stay one tap away at the bottom. */}
      <nav
        aria-label={t('messages.mainNavigation')}
        className="lg:hidden right-3 bottom-3 left-3 z-40 fixed flex items-center bg-card shadow-elev-3 px-2 py-1.5 rounded-[28px]"
      >
        {bar.map((item) => (
          <ShellLink
            key={item.key}
            href={item.href}
            base={base}
            aria-current={lit(item) ? 'page' : undefined}
            className={cn(
              'flex flex-col flex-1 justify-center items-center gap-0.5 rounded-2xl outline-none min-w-0 min-h-14 font-semibold text-[0.6875rem] text-muted-foreground focus-visible:ring-3 focus-visible:ring-ring/40',
              lit(item) && 'text-secondary-foreground',
            )}
          >
            <span
              className={cn(
                'flex justify-center items-center rounded-full w-14 h-8 transition-colors duration-200 ease-emphasized',
                lit(item) && 'bg-secondary',
              )}
            >
              <item.icon className="size-5" aria-hidden="true" />
            </span>
            <span className="px-1 max-w-full truncate">{item.label}</span>
          </ShellLink>
        ))}
      </nav>
    </div>
  )
}

/** A link of the frame: a router link inside this area, a plain one out of it. */
function ShellLink({
  href,
  base,
  external,
  children,
  ...props
}: {
  href: string
  base: string
  external?: boolean
  children: ReactNode
  className?: string
  title?: string
  'aria-current'?: 'page'
}) {
  if (external)
    return (
      <a href={href} target="_blank" rel="noopener noreferrer" {...props}>
        {children}
      </a>
    )
  const first = href.split('/')[1]
  const area = sources.some((source) => source.id === first) ? `/${first}` : ''
  if (area !== base)
    return (
      <a href={href} {...props}>
        {children}
      </a>
    )
  return (
    <Link to={href.slice(base.length) || '/'} {...props}>
      {children}
    </Link>
  )
}

// A few faint shapes in the accent behind the page's top, as on Moesekai.
function Backdrop() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 1440 720"
      preserveAspectRatio="xMidYMin slice"
      className="-z-10 absolute inset-x-0 top-0 opacity-25 dark:opacity-20 w-full h-[45rem] pointer-events-none"
    >
      <circle cx="360" cy="76" r="14" fill="none" stroke="var(--primary)" strokeWidth="3" />
      <circle cx="1290" cy="330" r="46" fill="none" stroke="var(--primary)" strokeWidth="3" />
      <path d="M1060 150l26 10-20 20z" fill="none" stroke="var(--primary)" strokeWidth="3" />
      <path d="M430 560l-30 16 28 18z" fill="var(--primary-container)" />
      <circle cx="800" cy="96" r="8" fill="var(--primary-container)" />
    </svg>
  )
}
