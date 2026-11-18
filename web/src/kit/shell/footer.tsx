// The card at the foot of every page, part of the frame (./shell) rather than of any area, so
// that it is the same everywhere and moves aside with the menu like the page does. After
// Moesekai's footer: the site and where to find its people, the main places, the community,
// feedback, and the small print.
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowSquareOutIcon,
  BugIcon,
  CloudArrowDownIcon,
  EyeIcon,
  GithubLogoIcon,
} from '@phosphor-icons/react'
import { cn } from '../lib/utils'
import type { SourceId } from '../source'
import {
  BilibiliMark,
  QQMark,
  bilibiliAddress,
  qqGroupAddress,
  sourceCodeAddress,
} from './community'
import { useVisitSummary } from './visits'
import { refreshSource, useDistributionSource } from '../distribution'

const playerAddress = 'https://play.ourtaiko.org/'
const licenseAddress = 'https://www.gnu.org/licenses/agpl-3.0.html'
// The country database that routes mainland visitors' 3D Don files (CC BY 4.0, which asks for
// this credit in DB-IP's own words).
const dbipAddress = 'https://db-ip.com'

const linkClass =
  'inline-flex items-center gap-2 rounded-full outline-none min-h-8 text-muted-foreground text-sm transition-colors hover:text-primary focus-visible:ring-3 focus-visible:ring-ring/40'
const badgeClass =
  'inline-flex justify-center items-center gap-2 border border-border hover:bg-accent px-3 rounded-[12px] outline-none h-9 font-semibold text-sm transition-colors focus-visible:ring-3 focus-visible:ring-ring/40'

function Away({
  href,
  className,
  children,
}: {
  href: string
  className?: string
  children: ReactNode
}) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer" className={className}>
      {children}
    </a>
  )
}

function Column({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-3">
      <h2 className="font-semibold text-sm">{title}</h2>
      <ul className="space-y-1">{children}</ul>
    </div>
  )
}

/** The footer; `source` is the one the song links are of (this page's, or the one used last). */
export function FrameFooter({ source }: { source: SourceId }) {
  const { t, i18n } = useTranslation()
  // How much the site is visited; the line is left out until that is known.
  const visits = useVisitSummary()
  // Where this visitor's public files (avatars, art, the web player) come from.
  const files = useDistributionSource()
  const number = new Intl.NumberFormat(i18n.language)
  const places = [
    { href: '/', label: t('shell.home') },
    { href: `/${source}/`, label: t('shell.browse') },
    { href: `/${source}/playlists`, label: t('shell.playlists') },
    { href: `/${source}/dans`, label: t('shell.dans') },
    { href: '/players', label: t('shell.directory') },
    { href: '/titles', label: t('shell.titles') },
  ]
  return (
    <footer
      data-testid="site-footer"
      className="bg-card shadow-elev-1 px-6 sm:px-8 py-10 rounded-[28px] text-foreground"
    >
      <div className="gap-8 lg:gap-12 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-12">
        <div className="space-y-3 lg:col-span-4">
          <a
            href="/"
            className="inline-block rounded-full outline-none font-extrabold text-primary text-2xl tracking-tight focus-visible:ring-3 focus-visible:ring-ring/40"
          >
            OurTaikoNET
          </a>
          <p className="font-semibold text-muted-foreground text-xs uppercase tracking-widest">
            OurTaiko Community
          </p>
          <p className="max-w-sm text-muted-foreground text-sm">{t('shell.footer.tagline')}</p>
          <div className="inline-grid grid-cols-2 gap-2 pt-1">
            <Away href={bilibiliAddress} className={cn(badgeClass, 'text-[#fb7299]')}>
              <BilibiliMark className="size-[18px]" />
              Bilibili
            </Away>
            <Away href={sourceCodeAddress} className={cn(badgeClass, 'text-muted-foreground')}>
              <GithubLogoIcon className="size-[18px]" weight="fill" aria-hidden="true" />
              GitHub
            </Away>
            <Away href={qqGroupAddress} className={cn(badgeClass, 'text-[#0099ff]')}>
              <QQMark className="size-[18px]" />
              {t('shell.footer.qqGroup')}
            </Away>
          </div>
        </div>

        <div className="lg:col-span-2">
          <Column title={t('shell.footer.explore')}>
            {places.map((place) => (
              <li key={place.href}>
                <a href={place.href} className={linkClass}>
                  {place.label}
                </a>
              </li>
            ))}
          </Column>
        </div>

        <div className="lg:col-span-3">
          <Column title={t('shell.footer.community')}>
            <li>
              <Away href={bilibiliAddress} className={linkClass}>
                {t('shell.footer.bilibili')}
              </Away>
            </li>
            <li>
              <Away href={qqGroupAddress} className={linkClass}>
                {t('shell.footer.qqGroup')}
              </Away>
            </li>
            <li>
              <Away href={playerAddress} className={linkClass}>
                {t('shell.player')}
                <span className="text-muted-foreground/70 text-xs">(play.ourtaiko.org)</span>
              </Away>
            </li>
          </Column>
        </div>

        <div className="lg:col-span-3">
          <Column title={t('shell.footer.contact')}>
            <li>
              <Away href={`${sourceCodeAddress}/issues`} className={linkClass}>
                <BugIcon className="size-5" aria-hidden="true" />
                {t('shell.footer.feedback')}
              </Away>
            </li>
            <li>
              <Away href={sourceCodeAddress} className={linkClass}>
                <ArrowSquareOutIcon className="size-5" aria-hidden="true" />
                {t('shell.sourceCode')}
              </Away>
            </li>
          </Column>
        </div>
      </div>

      <div className="bg-border my-8 w-full h-px" />

      <div className="flex md:flex-row flex-col justify-between items-start md:items-center gap-4 text-muted-foreground text-xs">
        <div className="space-y-1">
          <p>
            © {new Date().getFullYear()} OurTaiko ·{' '}
            <Away
              href={licenseAddress}
              className="rounded-sm outline-none hover:text-primary focus-visible:ring-3 focus-visible:ring-ring/40"
            >
              AGPL-3.0-or-later
            </Away>{' '}
            ·{' '}
            <Away
              href={dbipAddress}
              className="rounded-sm outline-none hover:text-primary focus-visible:ring-3 focus-visible:ring-ring/40"
            >
              IP Geolocation by DB-IP
            </Away>
          </p>
          <p className="text-muted-foreground/70 uppercase tracking-wider">
            {t('shell.footer.nonProfit')}
          </p>
          {visits && (
            <p className="flex items-center gap-1.5 pt-1" data-testid="site-visits">
              <EyeIcon className="size-4 shrink-0" aria-hidden="true" />
              <span>
                {t('shell.footer.visits', {
                  total: number.format(visits.total.views),
                  today: number.format(visits.today.views),
                  visitors: number.format(visits.today.visitors),
                })}
              </span>
            </p>
          )}
          {files && (
            <p className="flex items-center gap-1.5" data-testid="site-file-source">
              <CloudArrowDownIcon className="size-4 shrink-0" aria-hidden="true" />
              <span>
                {t('shell.footer.source', {
                  name: t(
                    files.mirror ? 'shell.footer.sourceMirror' : 'shell.footer.sourceCanonical',
                  ),
                  host: new URL(files.baseUrl).host,
                })}
              </span>
              <a
                href="#"
                className="underline underline-offset-2 hover:text-foreground"
                onClick={(event) => {
                  event.preventDefault()
                  void refreshSource()
                }}
              >
                {t('shell.footer.sourceRefresh')}
              </a>
            </p>
          )}
        </div>
        <p className="max-w-md md:text-right">{t('shell.footer.notice')}</p>
      </div>
    </footer>
  )
}
