import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ShieldCheck, Wifi, WifiOff, Loader2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { LanguageSwitcher } from '@/components/layout/language-switcher'
import { useRoomStore } from '@/stores/room-store'
import { useOperatorLabel } from '@/lib/use-operator-label'
import { cn } from '@/lib/utils'

const STATUS_TONE = {
  open: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  connecting: 'bg-amber-50 text-amber-700 border-amber-200',
  reconnecting: 'bg-amber-50 text-amber-700 border-amber-200',
  closed: 'bg-slate-100 text-slate-500 border-slate-200',
} as const

const STATUS_ICON = {
  open: Wifi,
  connecting: Loader2,
  reconnecting: Loader2,
  closed: WifiOff,
} as const

export function Topbar() {
  const { t } = useTranslation()
  const status = useRoomStore((state) => state.status)
  const unread = useRoomStore((state) => state.unreadMentions)
  const operatorLabel = useOperatorLabel()

  const Icon = STATUS_ICON[status]

  return (
    <header className="flex h-14 shrink-0 items-center justify-between border-b border-[hsl(var(--border))] bg-[hsl(var(--card))] px-6">
      <div className="flex items-center gap-3 text-sm text-[hsl(var(--muted-foreground))]">
        <Link to="/projects" className="font-semibold text-[hsl(var(--foreground))]">
          {t('topbar.appName')}
        </Link>
        {operatorLabel ? (
          <>
            <span className="text-[hsl(var(--border))]">/</span>
            <Badge tone="outline" className="gap-1.5 border-[hsl(var(--border))]">
              <ShieldCheck className="h-3.5 w-3.5 text-emerald-500" />
              <span className="font-mono">{operatorLabel}</span>
            </Badge>
            <span className="text-xs text-[hsl(var(--muted-foreground))]">
              {t('topbar.operatorHint')}
            </span>
          </>
        ) : null}
      </div>

      <div className="flex items-center gap-3">
        {unread > 0 ? (
          <Badge tone="warning" className="font-mono">
            {t('topbar.unreadMention', { count: unread })}
          </Badge>
        ) : null}
        <Badge tone="outline" className={cn('gap-1.5 border', STATUS_TONE[status])}>
          <Icon
            className={cn(
              'h-3.5 w-3.5',
              status === 'connecting' || status === 'reconnecting' ? 'animate-spin' : '',
            )}
          />
          <span>{t(`topbar.status.${status}`)}</span>
        </Badge>
        <LanguageSwitcher />
      </div>
    </header>
  )
}
