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
