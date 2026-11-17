export type User = {
  id: string
  username: string
  nickname: string
  emailVerified: boolean
  avatarUrl?: string
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
  avatarUrl: string
  groups: string[]
  bits: number
}
export type Session = { user: User | null; csrfToken: string }

export type TitleStyle = 'wood' | 'teal' | 'wood2' | 'purple' | 'gold' | 'rainbow'
export type Title = {
  id: string
  name: string
  nameTranslations: Record<string, string>
  description: string
  style: TitleStyle
  official: boolean
  // The player who made it; null for the titles administrators keep.
  ownerId: string | null
}
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
  avatarUrl: string
  title: Title | null
  dan: DanBadge | null
  showDan: boolean
}
export type Source = { id: string; name: string; site: string }
export type PublicProfile = {
  user: { id: string; nickname: string; avatarUrl: string }
  nameplate: Nameplate
  sources: Source[]
  profilesAvailable: boolean
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
  /** nickname and avatarUrl are empty while the site has none for the player. */
  user: { id: string; nickname: string; avatarUrl: string }
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
// A rule that gives a title by itself. Which fields are set goes by kind; a category rule
// without a count asks for every song of the category.
export type TitleRule = {
  id: number
  // The item it gives: a title, or a part of the Don and its number (one or the other).
  titleId: string | null
  donPart: 'cos' | 'head' | 'body' | 'puchi' | 'paint' | null
  donItem: number | null
  kind: 'chart_clear' | 'category_clear' | 'site_login' | 'game_login' | 'charts_uploaded'
  source: 'fanmade' | 'official' | null
  chartId: string | null
  chart: {
    id: string
    title: string
    titleTranslations: Record<string, string>
    gone: boolean
  } | null
  category: string | null
  course: Course | null
  // The lowest crown that counts: 1 cleared, 2 full combo, 3 all good.
  clear: number | null
  count: number | null
}
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

// How much the site is visited (the backend's internal/visits). Over more than one day,
// visitors is the sum of each day's.
export type VisitCount = { views: number; visitors: number }
export type VisitDay = VisitCount & { day: string }
export type VisitReport = {
  total: VisitCount
  today: VisitCount
  /** The first day counted (YYYY-MM-DD); empty when nothing has been. */
  since: string
  /** Oldest first, today last, with the days nobody came. */
  days: VisitDay[]
  pages: { page: string; views: number }[]
}
