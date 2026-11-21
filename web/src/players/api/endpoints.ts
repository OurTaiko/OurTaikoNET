// Browser requests stay on the website origin, which also serves the API.
const API_ROOT = '/api/v2'
export const endpoints = {
  me: `${API_ROOT}/me`,
  logout: `${API_ROOT}/auth/logout`,
  login: (returnTo: string) =>
    `${API_ROOT}/auth/sso/login?returnTo=${encodeURIComponent(returnTo)}`,
  account: (page: 'profile' | 'register') => `${API_ROOT}/auth/account/${page}`,
  nameplate: `${API_ROOT}/me/nameplate`,
  // The 3D Don: the costume catalog, one's own look (GET/PUT), anyone's.
  donCatalog: `${API_ROOT}/don/catalog`,
  // Which parts of the Don are locked and the rules that give them; what the player owns.
  donItems: `${API_ROOT}/don/items`,
  myDonItems: `${API_ROOT}/me/don/items`,
  adminDonLock: (part: string, id: number) => `${API_ROOT}/admin/don/locks/${part}/${id}`,
  adminDonGrants: `${API_ROOT}/admin/don/grants`,
  adminDonRules: (part: string, id: number) => `${API_ROOT}/admin/don/items/${part}/${id}/rules`,
  myDon: `${API_ROOT}/me/don`,
  userDon: (id: string) => `${API_ROOT}/users/${id}/don`,
  titles: `${API_ROOT}/titles`,
  // What players have just done on either source, for the home page.
  activity: `${API_ROOT}/activity`,
  // The playlists the home page recommends, and where its administrator sets them (PUT).
  featuredPlaylists: `${API_ROOT}/featured-playlists`,
  adminFeaturedPlaylists: `${API_ROOT}/admin/featured-playlists`,
  // The site's announcements, a page at a time, and where its administrator writes them (POST
  // a new one; PUT or DELETE one by its id).
  announcements: (page: number, pageSize: number) =>
    `${API_ROOT}/announcements?page=${page}&pageSize=${pageSize}`,
  announcement: (id: string) => `${API_ROOT}/announcements/${encodeURIComponent(id)}`,
  // The comments under one, and one of the site's comments by its id: itself (GET with an
  // order for its thread, PATCH, DELETE) and the viewer's vote on it.
  announcementComments: (id: string) =>
    `${API_ROOT}/announcements/${encodeURIComponent(id)}/comments`,
  comment: (id: string) => `${API_ROOT}/comments/${encodeURIComponent(id)}`,
  commentVote: (id: string) => `${API_ROOT}/comments/${encodeURIComponent(id)}/vote`,
  adminAnnouncements: `${API_ROOT}/admin/announcements`,
  adminAnnouncement: (id: string) => `${API_ROOT}/admin/announcements/${encodeURIComponent(id)}`,
  myTitles: `${API_ROOT}/me/titles`,
  myTitle: (id: string) => `${API_ROOT}/me/titles/${encodeURIComponent(id)}`,
  user: (id: string) => `${API_ROOT}/users/${id}`,
  userScores: (id: string, query: URLSearchParams) => `${API_ROOT}/users/${id}/scores?${query}`,
  userTitles: (id: string) => `${API_ROOT}/users/${id}/titles`,
  userDans: (id: string) => `${API_ROOT}/users/${id}/dans`,
  adminTitle: (id: string) => `${API_ROOT}/admin/titles/${encodeURIComponent(id)}`,
  adminGrants: `${API_ROOT}/admin/grants`,
  // The rules that give a title by themselves.
  adminTitleRules: (id: string) => `${API_ROOT}/admin/titles/${encodeURIComponent(id)}/rules`,
  adminRule: (id: number) => `${API_ROOT}/admin/rules/${id}`,
  // The manage panel: user groups.
  groups: `${API_ROOT}/admin/groups`,
  groupMembers: `${API_ROOT}/admin/users`,
  findPlayers: (text: string) => `${API_ROOT}/admin/users?q=${encodeURIComponent(text)}`,
  // How much the site is visited, for its administrator: the last days of the time zone `tz`
  // (UTC when empty), their hours and the pages seen most.
  visitReport: (days: number, tz: string) =>
    `${API_ROOT}/admin/visits?days=${days}${tz ? `&tz=${encodeURIComponent(tz)}` : ''}`,
  userGroups: (id: string) => `${API_ROOT}/admin/users/${encodeURIComponent(id)}/groups`,
}
