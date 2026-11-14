// The site's one frame, the same on every page of every area: a menu that floats at the left
// (a bar at the bottom on a phone), a top row of pills, and the page between them.
//
// The site has three areas, each with a router of its own (see src/root.tsx): the two song
// sources, under /fanmade and /official, and everything that belongs to no source (home,
// players, titles), at the top. The frame's links are whole addresses; one that stays in the
// area it is shown in is a router link, any other is a plain link, which the root follows by
// swapping the area in place.
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { ComponentType, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router-dom'
import {
  CaretDownIcon,
  CompassIcon,
  FolderIcon,
  GameControllerIcon,
  HouseIcon,
  BellIcon,
  IdentificationCardIcon,
  ListIcon,
  MagnifyingGlassIcon,
  MedalIcon,
  PlaylistIcon,
  ShieldStarIcon,
  SignInIcon,
  SignOutIcon,
  TagIcon,
  UsersThreeIcon,
  UsersIcon,
  XIcon,
} from '@phosphor-icons/react'
import { cn } from '../lib/utils'
import { visit } from '../navigation'
import { sources, type SourceId } from '../source'
import { ThemeToggle } from '../theme'
import { UserAvatar } from '../components/user-avatar'
import { Button } from '../components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '../components/ui/dropdown-menu'
import { rememberSource, useRememberedSource, useSiteRoles } from './site'
import { twinPath } from './twin'

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

// How far the menu is scrolled. The frame is drawn anew each time the area changes (see
// src/root.tsx); the menu of the new one starts where the last one was left.
let menuScroll = 0

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
  const remembered = useRememberedSource()
  const source = area || remembered
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem('ourtaiko-net-menu') === 'collapsed'
    } catch {
      return false
    }
  })
  const [open, setOpen] = useState(false)
  useEffect(() => setOpen(false), [pathname])
  const menuBox = useRef<HTMLElement>(null)
  // When the player last moved the menu themselves: only then is a scroll theirs to remember.
  const menuTouched = useRef(0)
  const touchMenu = () => {
    menuTouched.current = Date.now()
  }
  useLayoutEffect(() => {
    const box = menuBox.current
    if (!box) return
    const restore = () => {
      box.scrollTop = menuScroll
    }
    restore()
    // The menu grows once the session is known (the player's own items), and until then it
    // may be too short to scroll that far: put it back again as it grows.
    if (typeof ResizeObserver === 'undefined' || !box.firstElementChild) return
    const watcher = new ResizeObserver(restore)
    watcher.observe(box.firstElementChild)
    return () => watcher.disconnect()
  }, [])
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
          key: 'notifications',
          href: '/notifications',
          label: t('shell.notifications'),
          icon: BellIcon,
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

  // Choosing the other source on one of a source's pages goes to the same page there (see
  // ./twin); elsewhere it only turns the Songs group to it.
  const choose = (next: SourceId) => {
    rememberSource(next)
    if (!area || next === area) return
    // This area's session says who uploads here; for the other source the site's groups do:
    // anyone uploads to Fanmade, the official source's songs are its managers'.
    const mayUpload = next === 'fanmade' || roles.groups.includes('OFFICIAL_MANAGER')
    visit(`/${next}${twinPath(pathname, mayUpload)}`)
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
      className="rounded-full outline-none font-extrabold text-primary text-xl tracking-tight focus-visible:ring-3 focus-visible:ring-ring/40 shrink-0"
    >
      OurTaikoNET
    </ShellLink>
  )
  // The route beside the name: which section this page is in and which page of it, each a
  // menu of the others. The sections are the menu's groups; which source the songs are of is
  // a setting (the switch in the menu), not a place, so it is no step of the route.
  const sections: { key: string; label: string; items: Item[] }[] = [
    { key: 'songs', label: t('shell.songs'), items: songs },
    { key: 'players', label: t('shell.players'), items: players },
    ...(you.length ? [{ key: 'you', label: t('shell.you'), items: you }] : []),
  ]
  const found = sections.find((section) => section.items.some(lit))
  // A page of a source that is no item of the menu (signing in, the account) is still among
  // the songs.
  const section = found ?? (area ? sections[0] : undefined)
  const page = found?.items.find(lit)
  const route = section && (
    <nav
      aria-label={t('shell.route')}
      data-testid="route"
      className="hidden md:flex items-center gap-1 min-w-0 text-sm"
    >
      <span aria-hidden="true" className="px-1 text-muted-foreground/60 text-lg">
        /
      </span>
      <Crumb
        label={t('shell.section')}
        value={section.key}
        options={sections.map(({ key, label, items }) => ({ key, label, href: items[0].href }))}
        muted={!!page}
      />
      {page && (
        <>
          <span aria-hidden="true" className="px-1 text-muted-foreground/60 text-lg">
            /
          </span>
          <Crumb
            label={t('shell.page')}
            value={page.key}
            options={section.items.map(({ key, label, href }) => ({ key, label, href }))}
          />
        </>
      )}
    </nav>
  )
  const bar: Item[] = [
    home,
    // Lit on the source's song pages, but not on its playlists, which have the next item.
    {
      ...songs[0],
      label: t('shell.songs'),
      also: [`/${source}/charts`, `/${source}/dans`, `/${source}/me/charts`],
    },
    songs[1],
    { ...players[0], label: t('shell.players') },
    ...(user ? [{ ...you[0], label: t('shell.you') }] : []),
  ]

  return (
    <div className="isolate relative bg-background min-h-svh text-foreground">
      <Backdrop />
      <div className="flex flex-col gap-4 sm:gap-5 mx-auto p-3 sm:p-4 max-w-[92rem]">
        {/* The top row stays in reach while the page scrolls: the name with the route, the
            search, and the tools. */}
        <div className="top-3 sm:top-4 z-30 sticky flex items-center gap-2 sm:gap-3">
          <div className="flex items-center gap-1 bg-card shadow-elev-1 pr-4 pl-1.5 rounded-full min-w-0 h-14">
            <Button
              variant="ghost"
              size="icon"
              className="max-lg:hidden"
              aria-label={collapsed ? t('shell.expandMenu') : t('shell.collapseMenu')}
              aria-expanded={!collapsed}
              onClick={() => setCollapsed(!collapsed)}
            >
              <ListIcon className="size-5" aria-hidden="true" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="lg:hidden"
              aria-label={t('shell.openMenu')}
              aria-expanded={open}
              onClick={() => setOpen(true)}
            >
              <ListIcon className="size-5" aria-hidden="true" />
            </Button>
            {brand}
            {route}
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
        <div className="flex items-start gap-6">
          {/* The menu, on screens wide enough to keep it beside the page. */}
          <aside
            ref={menuBox}
            onWheel={touchMenu}
            onTouchMove={touchMenu}
            onPointerDown={touchMenu}
            onKeyDown={touchMenu}
            onScroll={(event) => {
              // A scroll the browser made itself (the menu was too short for a moment) is
              // not the player's, and must not replace where they left it.
              if (Date.now() - menuTouched.current < 1500)
                menuScroll = event.currentTarget.scrollTop
            }}
            className={cn(
              'hidden lg:block top-[5.5rem] sticky max-h-[calc(100svh-6.5rem)] overflow-y-auto -m-1 p-1 rounded-[28px] shrink-0',
              collapsed ? 'w-[4.25rem]' : 'w-[16.5rem]',
            )}
          >
            {menu(collapsed)}
          </aside>
          <main id="main" className="flex-1 space-y-6 pb-24 lg:pb-8 min-w-0">
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

/**
 * One step of the route: its name, and under it the other places at the same level.
 * Choosing one goes there.
 */
function Crumb({
  label,
  value,
  options,
  muted = false,
}: {
  label: string
  value: string
  options: { key: string; label: string; href: string }[]
  muted?: boolean
}) {
  const current = options.find((option) => option.key === value)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            aria-label={`${label}: ${current?.label ?? ''}`}
            className={cn(
              'group/crumb inline-flex items-center gap-1 rounded-full outline-none min-w-0 h-9 pr-1 pl-2 font-semibold focus-visible:ring-3 focus-visible:ring-ring/40 hover:bg-accent transition-colors',
              muted ? 'text-muted-foreground' : 'text-foreground',
            )}
          />
        }
      >
        <span className="truncate">{current?.label}</span>
        <CaretDownIcon
          className="size-4 shrink-0 transition-transform group-aria-expanded/crumb:rotate-180"
          aria-hidden="true"
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" sideOffset={10} className="p-1.5 rounded-[20px] min-w-48">
        <DropdownMenuRadioGroup
          value={value}
          onValueChange={(key) => {
            const next = options.find((option) => option.key === key)
            if (next && next.key !== value) visit(next.href)
          }}
        >
          {options.map((option) => (
            <DropdownMenuRadioItem
              key={option.key}
              value={option.key}
              // The one this page is under is lit, with its tick in front.
              className="data-checked:bg-secondary pr-5 pl-11 rounded-[14px] h-11 data-checked:font-semibold text-[0.9375rem] data-checked:text-secondary-foreground [&>[data-slot=dropdown-menu-radio-item-indicator]]:right-auto [&>[data-slot=dropdown-menu-radio-item-indicator]]:left-3.5"
            >
              {option.label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
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
