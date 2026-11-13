// The site has three areas, told apart by how the address starts: the two song sources,
// /fanmade/… and /official/…, and the pages that belong to no source (home, players, titles),
// at the top. Each is a single-page app of its own, loaded only when a page of it is opened;
// all three wear the same frame (@sitekit/shell).
import { createRoot } from 'react-dom/client'
import { SOURCE, sitePages, sourceOf } from '@sitekit/source'
import { rememberSource, rememberedSource } from '@sitekit/shell/site'

async function start() {
  const root = createRoot(document.getElementById('root')!)
  if (sourceOf(location.pathname)) {
    rememberSource(SOURCE)
    const { songs } = await import('./songs')
    root.render(await songs())
    return
  }
  if (sitePages.includes(location.pathname.split('/')[1])) {
    const { players } = await import('@players/app')
    root.render(players())
    return
  }
  // An address from before the sources were told apart (/charts/x) goes to the source used
  // last, Fanmade the first time, keeping the rest of the address.
  location.replace(`/${rememberedSource()}${location.pathname}${location.search}${location.hash}`)
}
void start()
