// The site's one frame, the same on every page of every area and drawn once above them all
// (src/root.tsx), so that it stays put when a link crosses from one area to another: a top bar
// fixed to the window, a menu that slides in at the left (a drawer over the page on a phone,
// with a bar of the main places at the bottom), and the settings sheet at the right. The layout
// follows Moesekai's (see the README's License).
//
// The site has three areas, each with a router of its own: the two song sources, under
// /fanmade and /official, and everything that belongs to no source (home, players, titles), at
// the top. The area shown tells the frame where the page is and who is signed in (./frame).
// The frame's links are plain links to whole addresses, which the root follows in place: by the
// area's router when they stay in it, by swapping the area when they leave it.
import { useEffect, useState } from 'react'
import type { ComponentType, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  ChartBarIcon,
  CompassIcon,
  FolderIcon,
  GameControllerIcon,
  GearSixIcon,
  HouseIcon,
  ChatsCircleIcon,
  SealCheckIcon,
  HashIcon,
  IdentificationCardIcon,
  ListIcon,
  MagnifyingGlassIcon,
  MedalIcon,
  MegaphoneIcon,
  LockSimpleIcon,
  MusicNotesIcon,
  PencilRulerIcon,
  PlaylistIcon,
  ShieldStarIcon,
  StarIcon,
  SignInIcon,
  SignOutIcon,
  TagIcon,
  UserIcon,
  UsersThreeIcon,
  UsersIcon,
  XIcon,
} from '@phosphor-icons/react'
import { cn } from '../lib/utils'
import { visit } from '../navigation'
import { sources, type SourceId } from '../source'
import { UserAvatar } from '../components/user-avatar'
import { Button } from '../components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '../components/ui/dropdown-menu'
import { useTellBuddy } from './buddy'
import { FrameFooter } from './footer'
import { setBellSlot, signOut, useTold } from './frame'
import { SettingsSheet } from './settings'
import { useSessionRefresh } from '../session'
import { rememberSource, useRememberedSource, useSiteRoles } from './site'
import { twinPath } from './twin'
import { useCountVisit } from './visits'
import { RewardsDialog } from '../rewards/rewards-dialog'

type Item = {
  key: string
  href: string
  label: string
  icon: ComponentType<{ className?: string; 'aria-hidden'?: boolean | 'true' | 'false' }>
  /** Addresses that light the item besides its own. */
  also?: string[]
  /** An address under href that is another item's. */
  except?: string
  /** Lit only at exactly its address. */
  exact?: boolean
  external?: boolean
}

type GroupId = 'songs' | 'players' | 'you' | 'manage' | 'more'

const menuKey = 'ourtaiko-net-menu'
const groupsKey = 'ourtaiko-net-menu-groups'
// Wide enough to keep the menu beside the page (Tailwind's lg).
const wideQuery = '(min-width: 1024px)'

const signInAddress = () =>
  `/api/v2/auth/sso/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`

function readGroups(): Partial<Record<GroupId, boolean>> {
  try {
    const kept = JSON.parse(localStorage.getItem(groupsKey) ?? '{}')
    return kept && typeof kept === 'object' ? kept : {}
  } catch {
    return {}
  }
}

/** The frame around the area shown (children). */
export function Frame({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const { area, path, user, loading, uploadsHere } = useTold()
  const base = area ? `/${area}` : ''
  const here = base + (path === '/' ? '/' : path)
  const roles = useSiteRoles(user?.id)
  useSessionRefresh(here)
  // Each page shown is counted (./visits).
  useCountVisit(here)
  // The source the Songs group points at: this page's, or the one used last.
  const remembered = useRememberedSource()
  const source = area || remembered
  // The player's Don, drawn above the areas so that it keeps moving across them.
  useTellBuddy(user, loading, source)
  // Anyone uploads to Fanmade; the official source's songs are its managers'. On a source's
  // own pages, the source's session says.
  const canUpload = (of: SourceId) =>
    of === area ? uploadsHere : of === 'fanmade' || roles.groups.includes('OFFICIAL_MANAGER')

  // Beside the page (wide screens): shown or put away, as the player last left it.
  const [docked, setDocked] = useState(() => {
    try {
      return localStorage.getItem(menuKey) !== 'collapsed'
    } catch {
      return true
    }
  })
  useEffect(() => {
    try {
      localStorage.setItem(menuKey, docked ? 'open' : 'collapsed')
    } catch {
      /* The choice is simply not remembered. */
    }
  }, [docked])
  // Over the page (narrow screens).
  const [drawer, setDrawer] = useState(false)
  useEffect(() => setDrawer(false), [here])
  useEffect(() => {
    if (!drawer) return
    const close = (event: KeyboardEvent) => event.key === 'Escape' && setDrawer(false)
    document.addEventListener('keydown', close)
    return () => document.removeEventListener('keydown', close)
  }, [drawer])
  const [wide, setWide] = useState(() => window.matchMedia(wideQuery).matches)
  useEffect(() => {
    const media = window.matchMedia(wideQuery)
    const look = () => setWide(media.matches)
    media.addEventListener('change', look)
    return () => media.removeEventListener('change', look)
  }, [])
  const toggleMenu = () => (wide ? setDocked(!docked) : setDrawer(true))
  // Only once the page has been drawn do the menu's moves animate: not as it first appears.
  const [settled, setSettled] = useState(false)
  useEffect(() => {
    const frame = requestAnimationFrame(() => setSettled(true))
    return () => cancelAnimationFrame(frame)
  }, [])

  const [groups, setGroups] = useState(readGroups)
  const toggleGroup = (id: GroupId) => {
    const next = { ...groups, [id]: !(groups[id] ?? true) }
    setGroups(next)
    try {
      localStorage.setItem(groupsKey, JSON.stringify(next))
    } catch {
      /* Not remembered. */
    }
  }

  const [settings, setSettings] = useState(false)
  // The top bar thickens once the page scrolls under it.
  const [scrolled, setScrolled] = useState(false)
  useEffect(() => {
    const look = () => setScrolled(window.scrollY > 4)
    look()
    window.addEventListener('scroll', look, { passive: true })
    return () => window.removeEventListener('scroll', look)
  }, [])
  // On a narrow screen the search takes the whole top bar while it is open.
  const [searching, setSearching] = useState(false)
  useEffect(() => setSearching(false), [here])

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
        { key: 'myTitles', href: '/me/titles', label: t('shell.myTitles'), icon: SealCheckIcon },
        {
          key: 'chats',
          href: '/chats',
          label: t('shell.chats'),
          icon: ChatsCircleIcon,
        },
        // What is yours of the source the menu is turned to: your charts, then your playlists.
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
        {
          key: 'myPlaylists',
          href: `/${source}/me/playlists`,
          label: t('shell.myPlaylists'),
          icon: PlaylistIcon,
        },
      ]
    : []
  // What only those who manage the site see, under /manage/: the title catalog for its
  // managers; players' user groups, the playlists the home page recommends, the announcements
  // and the visit count for the site's administrator.
  const manage: Item[] = user
    ? [
        ...(roles.titleManager
          ? [
              {
                key: 'titleAdmin',
                href: '/manage/titles',
                label: t('shell.titleAdmin'),
                icon: ShieldStarIcon,
              },
            ]
          : []),
        // Fanmade's managers decide the categories players suggest (乐曲管理).
        ...(roles.groups.includes('FANMADE_MANAGER')
          ? [
              {
                key: 'songAdmin',
                href: '/manage/songs',
                label: t('shell.songAdmin'),
                icon: MusicNotesIcon,
              },
            ]
          : []),
        // Each source's managers decide the changes asked for its fixed 段位 courses (段位审核).
        ...(roles.groups.includes('FANMADE_MANAGER') || roles.groups.includes('OFFICIAL_MANAGER')
          ? [
              {
                key: 'danAdmin',
                href: '/manage/dans',
                label: t('shell.danAdmin'),
                icon: LockSimpleIcon,
              },
              // And the reports of the tags on its songs, with the catalog of tags (标签管理).
              {
                key: 'tagAdmin',
                href: '/manage/tags',
                label: t('shell.tagAdmin'),
                icon: HashIcon,
              },
            ]
          : []),
        ...(roles.siteAdmin
          ? [
              {
                key: 'userGroups',
                href: '/manage/groups',
                label: t('shell.manage'),
                icon: UsersThreeIcon,
              },
              {
                key: 'featured',
                href: '/manage/featured',
                label: t('shell.featured'),
                icon: StarIcon,
              },
              {
                key: 'announcements',
                href: '/manage/announcements',
                label: t('shell.announcements'),
                icon: MegaphoneIcon,
              },
              {
                key: 'visits',
                href: '/manage/visits',
                label: t('shell.visits'),
                icon: ChartBarIcon,
              },
            ]
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
  // The other OurTaiko programs: the chart editor here, the game on its own site.
  const more: Item[] = [
    { key: 'editor', href: '/edit', label: t('shell.editor'), icon: PencilRulerIcon },
    player,
  ]
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
    visit(`/${next}${twinPath(path, canUpload(next))}`)
  }

  const entry = (item: Item) => (
    <FrameLink
      key={item.key}
      href={item.href}
      external={item.external}
      aria-current={lit(item) ? 'page' : undefined}
      className={cn(
        'flex items-center gap-3.5 px-4 h-11 rounded-full font-medium text-muted-foreground text-sm outline-none transition-colors duration-200 ease-emphasized focus-visible:ring-3 focus-visible:ring-ring/40 hover:bg-accent hover:text-foreground',
        lit(item) &&
          'bg-secondary font-semibold text-secondary-foreground hover:bg-secondary hover:text-secondary-foreground',
      )}
    >
      <item.icon className="size-5 shrink-0" aria-hidden="true" />
      <span className="truncate">{item.label}</span>
    </FrameLink>
  )
  // A group of the menu: its name, which folds it away or opens it again, then its items.
  const group = (id: GroupId, label: string, content: ReactNode) => {
    const open = groups[id] ?? true
    return (
      <div
        key={id}
        className="mt-1 first:mt-0 pt-1 first:pt-0 border-t first:border-t-0 border-border/70"
      >
        <button
          type="button"
          aria-expanded={open}
          onClick={() => toggleGroup(id)}
          className="flex justify-between items-center hover:bg-accent px-4 rounded-full outline-none w-full h-9 font-semibold text-muted-foreground text-xs transition-colors focus-visible:ring-3 focus-visible:ring-ring/40"
        >
          {label}
          <CaretDownIcon
            aria-hidden="true"
            className={cn(
              'size-4',
              settled && 'transition-transform duration-300 ease-emphasized',
              open && 'rotate-180',
            )}
          />
        </button>
        <div
          className={cn(
            'grid',
            settled && 'transition-[grid-template-rows,opacity] duration-300 ease-emphasized',
            open ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
          )}
        >
          <div className="flex flex-col gap-0.5 min-h-0 overflow-hidden" inert={!open}>
            {content}
          </div>
        </div>
      </div>
    )
  }
  const sourceSwitch = (
    <div
      role="group"
      aria-label={t('shell.source')}
      data-testid="source-switch"
      className="flex gap-1 bg-muted mx-1 mt-0.5 mb-1 p-1 rounded-full"
    >
      {sources.map(({ id }) => (
        <button
          key={id}
          type="button"
          aria-pressed={id === source}
          onClick={() => choose(id)}
          className={cn(
            'flex-1 rounded-full h-9 font-semibold text-muted-foreground text-sm outline-none transition-all duration-200 ease-emphasized focus-visible:ring-3 focus-visible:ring-ring/40',
            id === source && 'bg-card text-foreground shadow-elev-1',
          )}
        >
          <span
            aria-hidden="true"
            className={cn(
              'inline-block mr-2 rounded-full size-2',
              id === 'official' ? 'bg-don' : 'bg-ka',
            )}
          />
          {t(`shell.${id}`)}
        </button>
      ))}
    </div>
  )
  // Who is signed in, kept at the foot of the menu however far it is scrolled.
  const account = (
    <div className="p-3 border-t border-border/70 shrink-0">
      {loading && !user ? (
        <div className="flex items-center gap-3 p-2" aria-hidden="true">
          <span className="bg-muted rounded-full size-10 animate-pulse" />
          <span className="flex-1 bg-muted rounded-full h-3 animate-pulse" />
        </div>
      ) : user ? (
        <div className="flex items-center gap-1">
          <FrameLink
            href={`/players/${user.id}`}
            className="flex flex-1 items-center gap-3 hover:bg-accent p-2 rounded-[20px] outline-none min-w-0 transition-colors focus-visible:ring-3 focus-visible:ring-ring/40"
          >
            <UserAvatar nickname={user.nickname} avatarPath={user.avatarPath} className="size-10" />
            <span className="flex-1 min-w-0">
              <span data-testid="account-nickname" className="block font-semibold text-sm truncate">
                {user.nickname}
              </span>
              <span className="block text-muted-foreground text-xs truncate">
                {t('shell.myPage')}
              </span>
            </span>
          </FrameLink>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('shell.signOut')}
            title={t('shell.signOut')}
            onClick={signOut}
          >
            <SignOutIcon aria-hidden="true" />
          </Button>
        </div>
      ) : (
        <a
          href={signInAddress()}
          className="flex items-center gap-3 hover:bg-accent p-2 rounded-[20px] outline-none transition-colors focus-visible:ring-3 focus-visible:ring-ring/40"
        >
          <span className="flex justify-center items-center bg-primary rounded-full size-10 text-primary-foreground shrink-0">
            <UserIcon className="size-5" aria-hidden="true" />
          </span>
          <span className="flex-1 min-w-0">
            <span className="block font-semibold text-sm truncate">{t('shell.signedOut')}</span>
            <span className="block text-muted-foreground text-xs truncate">
              {t('shell.signInHint')}
            </span>
          </span>
          <SignInIcon className="size-5 text-muted-foreground shrink-0" aria-hidden="true" />
        </a>
      )}
    </div>
  )
  // Home stays at the head of the menu and the account at its foot; only the groups between
  // them scroll (in one flex column a short window squeezed Home instead).
  const menu = (
    <>
      <nav aria-label={t('messages.mainNavigation')} className="flex flex-col flex-1 min-h-0">
        <div className="flex flex-col px-2 pt-3 pb-2 border-b border-border/70 shrink-0">
          {entry(home)}
        </div>
        <div className="flex flex-col flex-1 gap-0.5 px-2 pt-2 pb-2 min-h-0 overflow-y-auto overscroll-contain">
          {group(
            'songs',
            t('shell.songs'),
            <>
              {sourceSwitch}
              {songs.map(entry)}
            </>,
          )}
          {group('players', t('shell.players'), players.map(entry))}
          {you.length > 0 && group('you', t('shell.you'), you.map(entry))}
          {manage.length > 0 && group('manage', t('shell.manageGroup'), manage.map(entry))}
          {group('more', t('shell.more'), more.map(entry))}
        </div>
      </nav>
      {account}
    </>
  )
  const brand = (
    <FrameLink
      href="/"
      className="px-1 rounded-full outline-none font-extrabold text-primary text-xl tracking-tight focus-visible:ring-3 focus-visible:ring-ring/40 shrink-0"
    >
      OurTaikoNET
    </FrameLink>
  )
  // The route beside the name: which section this page is in and which page of it, each a
  // menu of the others. The sections are the menu's groups; which source the songs are of is
  // a setting (the switch in the menu), not a place, so it is no step of the route.
  const sections: { key: string; label: string; items: Item[] }[] = [
    { key: 'songs', label: t('shell.songs'), items: songs },
    { key: 'players', label: t('shell.players'), items: players },
    ...(you.length ? [{ key: 'you', label: t('shell.you'), items: you }] : []),
    ...(manage.length ? [{ key: 'manage', label: t('shell.manageGroup'), items: manage }] : []),
    { key: 'more', label: t('shell.more'), items: more },
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
  const search = (
    <form
      action={`/${source}/`}
      // Searched in place, like every link of the site; the action is for a page whose
      // scripts did not load.
      onSubmit={(event) => {
        event.preventDefault()
        const q = new FormData(event.currentTarget).get('q')
        visit(`/${source}/${q ? `?q=${encodeURIComponent(String(q))}` : ''}`)
      }}
      role="search"
      className={cn(
        'glass items-center gap-2.5 px-5 rounded-full h-12 text-muted-foreground focus-within:ring-3 focus-within:ring-ring/30',
        searching ? 'flex flex-1' : 'hidden lg:flex w-72 shrink',
      )}
    >
      <MagnifyingGlassIcon className="size-5 shrink-0" aria-hidden="true" />
      <input
        type="search"
        name="q"
        aria-label={t('shell.search')}
        placeholder={t('shell.search')}
        autoFocus={searching}
        className="flex-1 bg-transparent outline-none min-w-0 text-foreground text-sm placeholder:text-muted-foreground"
      />
      {searching && (
        <Button
          variant="ghost"
          size="icon-sm"
          className="-mr-3"
          aria-label={t('shell.closeSearch')}
          onClick={() => setSearching(false)}
        >
          <XIcon aria-hidden="true" />
        </Button>
      )}
    </form>
  )
  const bar: Item[] = [
    home,
    // Lit on the source's song pages, but not on its playlists, which have the next item.
    {
      ...songs[0],
      label: t('shell.songs'),
      also: [`/${source}/charts`, `/${source}/dans`],
    },
    songs[1],
    { ...players[0], label: t('shell.players') },
    // Your charts and your messages are under You in the menu, so they light You here.
    ...(user
      ? [{ ...you[0], label: t('shell.you'), also: [`/${source}/me/charts`, '/chats'] }]
      : []),
  ]

  return (
    <div className="isolate relative bg-background min-h-svh text-foreground">
      {/* What the player was given, over any page (../rewards). */}
      <RewardsDialog />
      {/* The top bar: transparent, its controls in floating capsules; what scrolls under it
          fades out at its edge. */}
      <header data-scrolled={scrolled} className="top-0 z-40 fixed inset-x-0 scroll-edge">
        <div className="flex items-center gap-2 px-2 sm:px-3 h-16">
          {searching ? (
            search
          ) : (
            <>
              <div className="flex items-center gap-1 pr-2 sm:pr-4 pl-1 rounded-full min-w-0 h-12 glass">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('shell.menu')}
                  title={t('shell.menu')}
                  aria-expanded={wide ? docked : drawer}
                  onClick={toggleMenu}
                >
                  <ListIcon className="size-5" aria-hidden="true" />
                </Button>
                {brand}
                {route}
              </div>
              <div className="flex-1 min-w-0" />
              {search}
              <div className="flex items-center gap-0.5 px-1 rounded-full h-12 glass shrink-0">
                <Button
                  variant="ghost"
                  size="icon"
                  className="lg:hidden"
                  aria-label={t('shell.search')}
                  onClick={() => setSearching(true)}
                >
                  <MagnifyingGlassIcon className="size-5" aria-hidden="true" />
                </Button>
                {/* The area's notification bell (./frame). */}
                <span ref={setBellSlot} className="contents" />
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('shell.settings')}
                  title={t('shell.settings')}
                  aria-expanded={settings}
                  className={cn(settings && 'bg-secondary text-secondary-foreground')}
                  onClick={() => setSettings(true)}
                >
                  <GearSixIcon className="size-5" aria-hidden="true" />
                </Button>
              </div>
            </>
          )}
        </div>
      </header>

      {/* Behind the menu while it is open over the page (narrow screens). */}
      <button
        type="button"
        tabIndex={-1}
        aria-hidden="true"
        className={cn(
          'lg:hidden z-50 fixed inset-0 bg-foreground/40 backdrop-blur-[2px] transition-opacity duration-300',
          drawer ? 'opacity-100' : 'pointer-events-none opacity-0',
        )}
        onClick={() => setDrawer(false)}
      />
      {/* The menu: a drawer over the page on narrow screens, a pane floating beside it on wide
          ones, which the menu button slides away and back. */}
      <aside
        aria-label={t('shell.menu')}
        inert={(wide ? !docked : !drawer) || undefined}
        className={cn(
          'z-50 lg:z-30 fixed inset-y-0 left-0 flex flex-col w-72 max-w-[85vw] overflow-hidden glass-thick rounded-r-[28px]',
          'lg:top-[4.75rem] lg:bottom-3 lg:left-3 lg:w-[17rem] lg:rounded-[28px] lg:glass',
          settled && 'transition-transform duration-300 ease-emphasized',
          // Put away a little past the edge, so that its shadow goes too.
          drawer ? 'translate-x-0' : '-translate-x-[calc(100%+1rem)]',
          docked ? 'lg:translate-x-0' : 'lg:-translate-x-[calc(100%+1.5rem)]',
        )}
      >
        <div className="lg:hidden flex items-center gap-1 px-2 h-16 shrink-0">
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('shell.closeMenu')}
            onClick={() => setDrawer(false)}
          >
            <XIcon className="size-5" aria-hidden="true" />
          </Button>
          {brand}
        </div>
        {menu}
      </aside>

      {/* The page, then the footer under it: outside every area, so it is the same on every
          page, and moved aside with the page while the menu is beside it. */}
      <div
        className={cn(
          'flex flex-col pt-[4.75rem] min-h-svh',
          settled && 'transition-[padding] duration-300 ease-emphasized',
          docked && 'lg:pl-[18.5rem]',
        )}
      >
        <main
          id="main"
          className="flex-1 space-y-6 mx-auto px-3 sm:px-4 pb-8 w-full max-w-[92rem] min-w-0"
        >
          {children}
        </main>
        <div className="mx-auto px-3 sm:px-4 pb-24 lg:pb-4 w-full max-w-[92rem] min-w-0">
          <FrameFooter source={source} />
        </div>
      </div>

      {/* On a phone the main places stay one tap away at the bottom. */}
      <nav
        aria-label={t('messages.mainNavigation')}
        className="lg:hidden right-3 bottom-3 left-3 z-40 fixed flex items-center bg-card shadow-elev-3 px-2 py-1.5 rounded-[28px]"
      >
        {bar.map((item) => (
          <FrameLink
            key={item.key}
            href={item.href}
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
          </FrameLink>
        ))}
      </nav>

      <SettingsSheet open={settings} onOpenChange={setSettings} area={area} />
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

/**
 * A link of the frame. A plain link: the root follows it in place, by the area's router or by
 * swapping the area (src/root.tsx).
 */
function FrameLink({
  href,
  external,
  children,
  ...props
}: {
  href: string
  external?: boolean
  children: ReactNode
  className?: string
  title?: string
  'aria-current'?: 'page'
}) {
  return external ? (
    <a href={href} target="_blank" rel="noopener noreferrer" {...props}>
      {children}
    </a>
  ) : (
    <a href={href} {...props}>
      {children}
    </a>
  )
}
