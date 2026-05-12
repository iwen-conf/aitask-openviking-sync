import { useTranslation } from 'react-i18next'
import type { RoomMember } from '@/api/types'
import { cn } from '@/lib/utils'

interface PresenceIndicatorProps {
  members: RoomMember[]
}

export function PresenceIndicator({ members }: PresenceIndicatorProps) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-wrap gap-2">
      {members.map((member) => {
        const label =
          member.memberType === 'operator'
            ? (member.operatorLabel ?? 'operator')
            : t(`agent.type.${member.agentType ?? 'system'}`)
        return (
          <span
            key={`${member.memberType}-${member.agentId ?? member.operatorLabel}`}
            className="inline-flex items-center gap-1.5 rounded-full border border-[hsl(var(--border))] bg-[hsl(var(--card))] px-2.5 py-1 text-xs text-[hsl(var(--muted-foreground))] shadow-sm"
          >
            <span
              className={cn(
                'h-1.5 w-1.5 rounded-full',
                member.online ? 'bg-emerald-500' : 'bg-slate-300',
              )}
            />
            {label}
          </span>
        )
      })}
    </div>
  )
}
