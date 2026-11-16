// The foot of the home page, after Moesekai's: sites of friends, each a card, and the people
// the site thanks. To add one, add it to the list.
import { useTranslation } from 'react-i18next'
import { ArrowRightIcon } from '@phosphor-icons/react'

const friends = [
  {
    name: 'Sakura Taiko Viewer',
    note: 'viewer.sakura-bot.cn',
    href: 'https://viewer.sakura-bot.cn/',
  },
]
const thanked = [{ name: 'Sakura', href: 'https://github.com/sigaer' }]

export function Friends() {
  const { t } = useTranslation()
  return (
    <div className="space-y-8 pt-4" data-testid="home-friends">
      <section aria-labelledby="friends-heading" className="space-y-4">
        <h2 id="friends-heading" className="font-extrabold text-xl tracking-tight">
          {t('home.friends')}
        </h2>
        <ul className="gap-3 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4">
          {friends.map((friend) => (
            <li key={friend.href}>
              <a
                href={friend.href}
                target="_blank"
                rel="noopener noreferrer"
                className="group flex justify-between items-center gap-3 bg-card shadow-elev-1 hover:shadow-elev-2 px-5 rounded-[20px] outline-none h-16 transition-shadow focus-visible:ring-3 focus-visible:ring-ring/40"
              >
                <span className="min-w-0">
                  <span className="block font-semibold group-hover:text-primary truncate transition-colors">
                    {friend.name}
                  </span>
                  <span className="block font-bold text-[0.625rem] text-muted-foreground uppercase tracking-wider truncate">
                    {friend.note}
                  </span>
                </span>
                <ArrowRightIcon
                  className="size-[18px] text-muted-foreground group-hover:text-primary transition-colors shrink-0"
                  aria-hidden="true"
                />
              </a>
            </li>
          ))}
        </ul>
      </section>
      <section
        aria-labelledby="thanks-heading"
        className="flex md:flex-row flex-col justify-between items-center gap-4 pt-6 border-border border-t"
      >
        <h2 id="thanks-heading" className="font-semibold text-muted-foreground">
          {t('home.thanks')}
        </h2>
        <p className="flex flex-wrap justify-center gap-x-2 gap-y-1">
          <span className="text-muted-foreground">{t('home.thanksPrefix')}</span>
          {thanked.map((person, index) => (
            <span key={person.href} className="flex gap-x-2">
              {index > 0 && <span className="text-border">|</span>}
              <a
                href={person.href}
                target="_blank"
                rel="noopener noreferrer"
                className="rounded-sm outline-none font-bold text-muted-foreground hover:text-primary transition-colors focus-visible:ring-3 focus-visible:ring-ring/40"
              >
                {person.name}
              </a>
            </span>
          ))}
        </p>
      </section>
    </div>
  )
}
