import type { TitleLayers } from '@sitekit/nameplate-layers'
import type { PlayerTotals } from '@sitekit/api/types'

export type { PlayerTotals }

export type User = {
  id: string
  username: string
  nickname: string
  emailVerified: boolean
  avatarPath?: string
  preferredLanguage?: string
  // Keeps the title catalog (the TITLE_MANAGER group).
  isAdmin: boolean
  // The account center's administrator of the site: manages it, players' groups included.
  siteAdmin: boolean
  // The user groups the player is in.
  groups: string[]
}
export type Group = { name: string; bit: number; about: string }
// A player as the manage panel lists them. username is the account center's login name and
// is there only when the player was found by typing it.
export type GroupMember = {
  id: string
  username?: string
  nickname: string
  avatarPath: string
  groups: string[]
  bits: number
}
export type Session = { user: User | null; csrfToken: string }

export type Title = {
  id: string
  name: string
  nameTranslations: Record<string, string>
  description: string
  // The band of six that game versions released before the layers draw.
  style: string
  // How a nameplate wearing it looks: items of the public nameplate set.
  layers: TitleLayers
  // private: its maker's alone, changed as they like. public: in the catalog, given by what the
  // title managers set, and changed only by a review.
  visibility: 'private' | 'public'
  // How it is obtained, in the words of the title manager who made it public.
  obtain: string
  // The player who made it; null for the titles the title managers make themselves.
  ownerId: string | null
}
/** A title in a list: with how many hold it, its maker's nickname and whether a review waits. */
export type ListedTitle = Title & { holders: number; ownerName: string; pending: boolean }
/**
 * A maker's request that the title managers review a title: to make a private one public
 * (publish) or to change a public one (change). It holds the title as asked for.
 */
export type Review = {
  id: string
  titleId: string
  userId: string
  kind: 'publish' | 'change'
  name: string
  nameTranslations: Record<string, string>
  description: string
  layers: TitleLayers
  /** What the maker tells the managers: how the title should be obtained, what a change is for. */
  request: string
  status: 'pending' | 'approved' | 'returned' | 'withdrawn'
  /** What the manager who decided answered. */
  reply: string
  createdAt: string
  decidedAt: string | null
}
/** A title the player made, with the latest request for its review. */
export type OwnTitle = ListedTitle & { review: Review | null }
export type MyTitles = { titles: OwnTitle[]; limit: number; used: number }
/** A review as the title managers see it: who asked, who decided, and the title as it is now. */
export type QueuedReview = Review & { userName: string; reviewerName: string; title: Title }
export type ReviewPage = { items: QueuedReview[]; total: number; page: number; pageSize: number }
// gold colours the emblem's text (verdict 4–6); combo picks its ring like a song's crown:
// 0 silver, 1 gold (full combo), 2 rainbow (all good).
export type DanBadge = {
  index: number
  verdict: number
  gold: boolean
  combo: 0 | 1 | 2
  achievedAt: string
}
export type Nameplate = {
  userId: string
  nickname: string
  avatarPath: string
  title: Title | null
  dan: DanBadge | null
  showDan: boolean
}
export type Source = { id: string; name: string; site: string }
export type PublicProfile = {
  user: PlayerTotals & { id: string; nickname: string; avatarPath: string }
  nameplate: Nameplate
  sources: Source[]
  profilesAvailable: boolean
  /** False when the player's numbers could not be added up: their zeros mean nothing. */
  totalsAvailable: boolean
}

export type Course = 'Easy' | 'Normal' | 'Hard' | 'Oni' | 'Edit'
export type ScoreRow = {
  id: string
  songId: string | number
  course: Course
  side: 'single' | 'p1' | 'p2'
  good: number
  ok: number
  bad: number
  score: number
  drumroll: number
  max_combo: number
  ClearStatus: number
  isLatest: boolean
  submittedAt: string
  title: string
  titleTranslations: Record<string, string>
  level: number
  bestClearStatus?: number
  playCount?: number
  source: string
  chartUrl: string
  /** The play's recording on the chart's page of its service; absent when there is none to show. */
  replayUrl?: string
}
/** Something a player just did: a play they cleared, or a chart they uploaded. */
export type Activity = {
  kind: 'score' | 'upload'
  at: string
  source: string
  /** nickname and avatarPath are empty while the site has none for the player. */
  user: { id: string; nickname: string; avatarPath: string }
  chart: { id: string; title: string; titleTranslations: Record<string, string> }
  /** The play of a 'score'. */
  play?: {
    id: string
    course: Course
    side: 'single' | 'p1' | 'p2'
    level: number
    score: number
    clearStatus: number
  }
}
export type ScoreView = 'best' | 'all'
export type ScoreSort = 'level' | 'score' | 'recent'
export type ScorePage = {
  items: ScoreRow[]
  total: number
  page: number
  pageSize: number
  view: ScoreView
  sort: ScoreSort
  sources: (Source & { count: number; truncated: boolean; error?: string })[]
}
// A rule that gives an item by itself (the backend's internal/rewards/rules). Which fields are
// set goes by kind; a category rule without a count asks for every song of the category.
export type RewardRule = {
  id: number
  // The item it gives: a title (titleId), or a part of the Don and its number.
  item: 'title' | 'don_item'
  titleId: string | null
  donPart: 'cos' | 'head' | 'body' | 'puchi' | 'paint' | null
  donItem: number | null
  kind:
    'chart_clear' | 'category_clear' | 'dan_clear' | 'site_login' | 'game_login' | 'charts_uploaded'
  source: 'fanmade' | 'official' | null
  chartId: string | null
  chart: {
    id: string
    title: string
    titleTranslations: Record<string, string>
    gone: boolean
  } | null
  /** A dan_clear rule's 段位 course. It is met only while the course is there and fixed. */
  danId: string | null
  dan: {
    id: string
    title: Record<string, string>
    code: string
    fixed: boolean
    gone: boolean
  } | null
  category: string | null
  course: Course | null
  // The lowest crown that counts: 1 cleared, 2 full combo, 3 all good. For dan_clear, the lowest
  // verdict: 1–3 red, 4–6 gold, each a pass, with full combo, with all good.
  clear: number | null
  count: number | null
}
// Each source's category IDs, in the order of their bits: what a category rule may name.
export type RuleCategories = Record<string, string[]>
export type HeldTitle = { title: Title; grantedAt: string; service: string; courseTitle: string }
export type AwardRecord = {
  service: string
  courseTitle: string
  official: boolean
  verdict: number
  dan: number | null
  createdAt: string
}
export type Dans = { badges: DanBadge[]; history: AwardRecord[] }

// How much the site is visited (the backend's internal/visits), in the time zone asked for.
// Over more than one day, visitors is the sum of each day's.
export type VisitCount = { views: number; visitors: number }
export type VisitDay = VisitCount & { day: string }
export type VisitReport = {
  /** The time zone the days and hours are of (UTC when none was asked for). */
  zone: string
  total: VisitCount
  today: VisitCount
  /** The first day counted (YYYY-MM-DD); empty when nothing has been. */
  since: string
  /** Oldest first, today last, with the days nobody came. */
  days: VisitDay[]
  /** The views made in each hour of the day (0 to 23) over those days. */
  hours: number[]
  pages: { page: string; views: number }[]
  /**
   * The countries the views of those days came from, most views first: the country's two
   * letters (ISO 3166-1), its views and the different addresses they came from. The country
   * "" is the unknown one, which also holds the views made before addresses were kept.
   */
  countries: { country: string; views: number; addresses: number }[]
  /**
   * The site's addresses the views of those days were seen at (its own and those of
   * APP_OTHER_ORIGINS, such as the one in mainland China), most views first: the host name, its
   * views and the different addresses they came from. The site "" holds the views it is not
   * known of (made between the second address opening and its being kept).
   */
  sites: { site: string; views: number; addresses: number }[]
}

// The playlists the home page recommends, in the order shown. Each names a public playlist of a
// song source by its id there; what the playlist is, its source says. `note` is the
// administrator's line about it, empty for none.
export type FeaturedPlaylist = { source: string; playlistId: string; note: string }
export type FeaturedPlaylists = { items: FeaturedPlaylist[]; max: number }

// What the site's administrator has to tell every visitor: a title and a text of plain lines
// (empty when the title says it all). Pinned ones are shown first, then the newest.
export type Announcement = {
  id: string
  title: string
  body: string
  pinned: boolean
  publishedAt: string
  /** How many comments are under it, replies among them. */
  commentCount: number
}
/** What the administrator writes of one. */
export type AnnouncementDraft = Pick<Announcement, 'title' | 'body' | 'pinned'>
