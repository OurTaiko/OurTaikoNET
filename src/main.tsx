// The site has three areas, told apart by how the address starts: the two song sources,
// /fanmade/… and /official/…, and the pages about players, /players/…. Each is a single-page
// app of its own, loaded only when a page of it is opened.
import { createRoot } from 'react-dom/client'
import { PLAYERS_BASE, SOURCE, sourceOf, type SourceId } from '@sitekit/source'

const lastSource = 'ourtaiko-net-source'

async function start() {
  const root = createRoot(document.getElementById('root')!)
  if (location.pathname === PLAYERS_BASE || location.pathname.startsWith(`${PLAYERS_BASE}/`)) {
    const { players } = await import('@players/app')
    root.render(players())
    return
  }
  // An address outside every area goes to the source used last (Fanmade the first time),
  // keeping the rest of the address: /charts/x becomes /fanmade/charts/x.
  if (!sourceOf(location.pathname)) {
    let source: SourceId = 'fanmade'
    try {
      source = sourceOf(`/${localStorage.getItem(lastSource)}`) ?? source
    } catch {
      // No storage: Fanmade.
    }
    location.replace(`/${source}${location.pathname}${location.search}${location.hash}`)
    return
  }
  try {
    localStorage.setItem(lastSource, SOURCE)
  } catch {
    // The choice is simply not remembered.
  }
  const { songs } = await import('./songs')
  root.render(await songs())
}
void start()
