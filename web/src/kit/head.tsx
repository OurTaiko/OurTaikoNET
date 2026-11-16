// What a page says about itself to search engines and to the cards other sites draw for a link:
// the title, the description, the picture, whether it may be indexed. Like react-helmet, a page
// renders <PageMeta …/> (or calls usePageMeta) and the tags in <head> follow it; when it is gone
// the site's own description comes back. index.html carries the same tags with the site's words,
// for readers that do not run the page's script.
//
// One page entry wins: the one put up last. Pages put up one each, never a layout, so the page
// on screen is the one that speaks (React removes the old page's entry before it adds the new
// one's).
import { useEffect, useId } from 'react'
import { useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { normalizeLanguage, type Language } from './i18n'

export type PageMetaProps = {
  /** The page's own name; the site's name is put after it. Absent: the site's name alone. */
  title?: string
  /** One or two sentences; longer text is cut. Absent: the site's description. */
  description?: string
  /** A picture for link cards: an address on this site or elsewhere. Absent: the site icon. */
  image?: string | null
  /** Open Graph type. */
  type?: 'website' | 'article' | 'profile' | 'music.song' | 'music.playlist'
  /**
   * A page about the visitor alone (their messages, their charts, an editor, sign-in): kept
   * out of search results.
   */
  noindex?: boolean
}

type Site = { name: string; description: string; language: string }

/** The longest description sent; search results show about this much. */
export const DESCRIPTION_LIMIT = 160
const DEFAULT_IMAGE = '/icons/icon-512.png'
const LOCALES: Record<Language, string> = {
  'zh-Hans': 'zh_CN',
  en: 'en_US',
  ja: 'ja_JP',
  ko: 'ko_KR',
}

const entries: { id: string; meta: PageMetaProps }[] = []
let site: Site | null = null

/** Text on one line, at most `limit` characters, cut at a word where it can be. */
export function summarize(text: string, limit = DESCRIPTION_LIMIT) {
  const flat = text.replace(/\s+/g, ' ').trim()
  if (flat.length <= limit) return flat
  const cut = flat.slice(0, limit - 1)
  const space = cut.lastIndexOf(' ')
  return `${space > limit * 0.6 ? cut.slice(0, space) : cut}…`
}

function tag(kind: 'meta' | 'link', attribute: string, key: string) {
  const found = document.head.querySelector<HTMLElement>(`${kind}[${attribute}="${key}"]`)
  if (found) return found
  const made = document.createElement(kind)
  made.setAttribute(attribute, key)
  document.head.append(made)
  return made
}
function setMeta(attribute: 'name' | 'property', key: string, content: string | null) {
  if (content === null) {
    document.head.querySelector(`meta[${attribute}="${key}"]`)?.remove()
    return
  }
  tag('meta', attribute, key).setAttribute('content', content)
}

/** The tags for what is on screen now. */
export function headFor(meta: PageMetaProps, site: Site, address: URL) {
  const title = meta.title?.trim() ? `${meta.title.trim()} | ${site.name}` : site.name
  const description = summarize(meta.description?.trim() || site.description)
  const image = new URL(meta.image || DEFAULT_IMAGE, address.origin).href
  // The page itself, without what a visitor chose on it (?tab, ?page, a search).
  const canonical = new URL(address.pathname, address.origin).href
  return {
    title,
    description,
    image,
    canonical,
    type: meta.type ?? 'website',
    robots: meta.noindex ? 'noindex, nofollow' : 'index, follow',
    locale: LOCALES[normalizeLanguage(site.language)],
    // A wide card for a cover; a player's square avatar sits better small.
    card: meta.image && meta.type !== 'profile' ? 'summary_large_image' : 'summary',
  }
}

function apply() {
  if (!site) return
  const meta = entries.at(-1)?.meta ?? {}
  const head = headFor(meta, site, new URL(location.href))
  document.title = head.title
  setMeta('name', 'description', head.description)
  setMeta('name', 'robots', head.robots)
  tag('link', 'rel', 'canonical').setAttribute('href', head.canonical)
  setMeta('property', 'og:site_name', site.name)
  setMeta('property', 'og:title', head.title)
  setMeta('property', 'og:description', head.description)
  setMeta('property', 'og:type', head.type)
  setMeta('property', 'og:url', head.canonical)
  setMeta('property', 'og:image', head.image)
  setMeta('property', 'og:locale', head.locale)
  setMeta('name', 'twitter:card', head.card)
  setMeta('name', 'twitter:title', head.title)
  setMeta('name', 'twitter:description', head.description)
  setMeta('name', 'twitter:image', head.image)
}

/** The site's name and description in the visitor's language; put up once per area. */
export function SiteHead() {
  const { t, i18n } = useTranslation()
  const name = t('common.siteTitle')
  const description = t('meta.siteDescription')
  const language = i18n.resolvedLanguage ?? 'zh-Hans'
  useEffect(() => {
    site = { name, description, language }
    apply()
  }, [name, description, language])
  return null
}

/** Says what this page is; see the top of this file. */
export function usePageMeta(meta: PageMetaProps) {
  const id = useId()
  // The address is read when the tags are written; a move within the page writes them again.
  const { pathname } = useLocation()
  const { title, description, image, type, noindex } = meta
  useEffect(() => {
    const entry = { title, description, image, type, noindex }
    const at = entries.findIndex((e) => e.id === id)
    if (at === -1) entries.push({ id, meta: entry })
    else entries[at].meta = entry
    apply()
  }, [id, title, description, image, type, noindex, pathname])
  useEffect(
    () => () => {
      const at = entries.findIndex((e) => e.id === id)
      if (at !== -1) entries.splice(at, 1)
      apply()
    },
    [id],
  )
}

/** usePageMeta as an element, for pages that would rather render it. */
export function PageMeta(props: PageMetaProps) {
  usePageMeta(props)
  return null
}
