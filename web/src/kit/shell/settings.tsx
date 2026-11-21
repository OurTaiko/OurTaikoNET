// The settings sheet the top bar's gear opens: a pane floating at the right on wide screens, a
// sheet from the bottom on narrow ones. The light or dark theme and the accent are kept in this
// browser, and so is the font, one of the built-in ones fetched when chosen; About says where the site's source is, as its license (the AGPL) asks,
// and which commit of it the website on show was built from.
import type { ComponentType, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import {
  ArrowSquareOutIcon,
  CheckIcon,
  CircleNotchIcon,
  MonitorIcon,
  MoonIcon,
  SunIcon,
  XIcon,
} from '@phosphor-icons/react'
import { cn } from '../lib/utils'
import { chooseTone, useChosenTone, type Tone } from '../accent'
import { builtInFonts, chooseFont, useFontState, type FontId } from '../font'
import { commitAddress, revision } from '../revision'
import { useTheme, type Theme } from '../theme-context'
import type { SourceId } from '../source'
import { Button } from '../components/ui/button'

export const sourceCodeAddress = 'https://github.com/OurTaiko/OurTaikoNET'
const licenseAddress = 'https://www.gnu.org/licenses/agpl-3.0.html'
const moesekaiAddress = 'https://github.com/StarMoe-org/Moesekai'

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3">
      <h3 className="font-semibold text-primary text-sm">{title}</h3>
      {children}
    </section>
  )
}

/** A row of choices, one of them taken. */
function Choices<T extends string>({
  label,
  value,
  onChange,
  options,
  testId,
}: {
  label: string
  value: T
  onChange: (value: T) => void
  options: { value: T; label: string; icon: ComponentType<{ className?: string }> }[]
  testId?: string
}) {
  return (
    <div
      role="radiogroup"
      aria-label={label}
      data-testid={testId}
      className="flex gap-1 bg-muted p-1 rounded-full"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="radio"
          aria-checked={option.value === value}
          onClick={() => onChange(option.value)}
          className={cn(
            'flex flex-1 justify-center items-center gap-2 rounded-full min-w-0 h-10 font-semibold text-muted-foreground text-sm outline-none transition-all duration-200 ease-emphasized focus-visible:ring-3 focus-visible:ring-ring/40',
            option.value === value && 'bg-card text-foreground shadow-elev-1',
          )}
        >
          <option.icon className="size-4 shrink-0" />
          <span className="truncate">{option.label}</span>
        </button>
      ))}
    </div>
  )
}

/** The built-in fonts, one taken; a chosen one is fetched while the page goes on in its font. */
function FontChoices() {
  const { t } = useTranslation()
  const { chosen, shown, loading, failed } = useFontState()
  const options: { id: FontId; name: string; megabytes?: number }[] = [
    { id: 'default', name: t('shell.fontDefault') },
    ...builtInFonts,
  ]
  const nameOf = (id: FontId) => options.find((option) => option.id === id)!.name
  return (
    <>
      <div
        role="radiogroup"
        aria-label={t('shell.font')}
        data-testid="font-choice"
        className="flex flex-col gap-1 bg-muted p-1 rounded-[20px]"
      >
        {options.map((option) => {
          const taken = option.id === chosen
          return (
            <button
              key={option.id}
              type="button"
              role="radio"
              aria-checked={taken}
              onClick={() => void chooseFont(option.id)}
              className={cn(
                'flex items-center gap-3 px-4 rounded-full min-w-0 h-10 font-semibold text-muted-foreground text-sm text-left outline-none transition-all duration-200 ease-emphasized focus-visible:ring-3 focus-visible:ring-ring/40',
                taken && 'bg-card text-foreground shadow-elev-1',
              )}
            >
              <span className="flex-1 truncate">{option.name}</span>
              {option.megabytes !== undefined && (
                <span className="font-normal text-muted-foreground text-xs tabular-nums">
                  {t('shell.fontSize', { size: option.megabytes })}
                </span>
              )}
              {taken && loading ? (
                <CircleNotchIcon className="size-4 animate-spin shrink-0" aria-hidden="true" />
              ) : (
                <CheckIcon
                  className={cn('size-4 shrink-0', !(taken && shown === chosen) && 'invisible')}
                  aria-hidden="true"
                />
              )}
            </button>
          )
        })}
      </div>
      <p aria-live="polite" className="text-muted-foreground text-xs leading-relaxed">
        {loading
          ? t('shell.fontLoading', { font: nameOf(chosen) })
          : failed
            ? t('shell.fontFailed', { font: nameOf(chosen), shown: nameOf(shown) })
            : t('shell.fontHint')}
      </p>
    </>
  )
}

const Dot = (tone: Tone) =>
  function ToneDot({ className }: { className?: string }) {
    return (
      <span
        aria-hidden="true"
        className={cn('rounded-full size-3', tone === 'don' ? 'bg-don' : 'bg-ka', className)}
      />
    )
  }
const DonDot = Dot('don')
const KaDot = Dot('ka')

export function SettingsSheet({
  open,
  onOpenChange,
  area,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  area: SourceId | ''
}) {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  const tone = useChosenTone()
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className="z-50 fixed inset-0 bg-black/30 data-closed:animate-out data-open:animate-in data-closed:fade-out-0 data-open:fade-in-0 duration-200 motion-reduce:duration-0" />
        <DialogPrimitive.Popup
          data-testid="settings"
          className={cn(
            'z-50 fixed flex flex-col bg-popover shadow-elev-3 outline-none text-popover-foreground',
            'inset-x-0 bottom-0 max-h-[85svh] rounded-t-[28px] data-open:slide-in-from-bottom data-closed:slide-out-to-bottom',
            'lg:inset-x-auto lg:top-3 lg:right-3 lg:bottom-3 lg:w-[26rem] lg:max-h-none lg:rounded-[28px] lg:data-open:slide-in-from-right lg:data-closed:slide-out-to-right',
            'data-closed:animate-out data-open:animate-in duration-300 ease-emphasized motion-reduce:duration-0',
          )}
        >
          <div className="flex items-center gap-2 py-3 pr-3 pl-6 shrink-0">
            <DialogPrimitive.Title className="flex-1 font-bold text-lg">
              {t('shell.settings')}
            </DialogPrimitive.Title>
            <DialogPrimitive.Close
              render={<Button variant="ghost" size="icon" aria-label={t('common.close')} />}
            >
              <XIcon className="size-5" aria-hidden="true" />
            </DialogPrimitive.Close>
          </div>
          <div className="flex-1 space-y-7 px-6 pt-2 pb-8 overflow-y-auto overscroll-contain">
            <Section title={t('shell.appearance')}>
              <Choices<Theme>
                label={t('shell.appearance')}
                value={theme}
                onChange={setTheme}
                options={[
                  { value: 'system', label: t('messages.system'), icon: MonitorIcon },
                  { value: 'light', label: t('messages.light'), icon: SunIcon },
                  { value: 'dark', label: t('messages.dark'), icon: MoonIcon },
                ]}
              />
            </Section>
            <Section title={t('shell.accent')}>
              <Choices<Tone>
                label={t('shell.accent')}
                value={tone}
                onChange={chooseTone}
                testId="accent-choice"
                options={[
                  { value: 'don', label: t('shell.don'), icon: DonDot },
                  { value: 'ka', label: t('shell.ka'), icon: KaDot },
                ]}
              />
              <p className="text-muted-foreground text-xs leading-relaxed">
                {area
                  ? t('shell.accentSourceHint', { source: t(`shell.${area}`) })
                  : t('shell.accentHint')}
              </p>
            </Section>
            <Section title={t('shell.font')}>
              <FontChoices />
            </Section>
            <Section title={t('shell.about')}>
              <div className="space-y-3 bg-muted/60 p-4 rounded-[20px] text-sm">
                <p className="font-extrabold text-primary text-lg tracking-tight">OurTaikoNET</p>
                <p className="text-muted-foreground leading-relaxed">{t('shell.license')}</p>
                <div className="flex flex-col gap-2">
                  <AboutLink href={sourceCodeAddress} label={t('shell.sourceCode')} />
                  <AboutLink href={licenseAddress} label="GNU AGPL-3.0" />
                  {revision && (
                    <AboutLink
                      href={commitAddress(sourceCodeAddress, revision)}
                      label={t('shell.revision')}
                      value={revision.short}
                      testId="revision"
                    />
                  )}
                </div>
                <p className="text-muted-foreground text-xs leading-relaxed">
                  {t('shell.layoutCredit')}{' '}
                  <a
                    href={moesekaiAddress}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="font-semibold text-primary underline-offset-2 hover:underline"
                  >
                    Moesekai
                  </a>
                </p>
              </div>
            </Section>
          </div>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

function AboutLink({
  href,
  label,
  value,
  testId,
}: {
  href: string
  label: string
  /** What the link leads to in a word (a commit's short hash), shown before the arrow. */
  value?: string
  testId?: string
}) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      data-testid={testId}
      className="flex items-center gap-2 bg-card hover:bg-accent px-4 rounded-full outline-none h-10 font-semibold transition-colors focus-visible:ring-3 focus-visible:ring-ring/40"
    >
      <span className="flex-1">{label}</span>
      {value && (
        <span className="font-mono font-normal text-muted-foreground text-xs">{value}</span>
      )}
      <ArrowSquareOutIcon className="size-4 text-muted-foreground" aria-hidden="true" />
    </a>
  )
}
