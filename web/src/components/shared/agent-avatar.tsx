import { ShieldCheck } from 'lucide-react'
import type { AgentType } from '@/api/types'
import { cn } from '@/lib/utils'
import claudeLogo from '@/assets/agents/claude.svg'
import codexLogo from '@/assets/agents/codex.svg'
import geminiLogo from '@/assets/agents/gemini.svg'

const LOGOS: Partial<Record<AgentType, string>> = {
  'claude-code': claudeLogo,
  codex: codexLogo,
  gemini: geminiLogo,
}

const TONES: Record<AgentType, string> = {
  'claude-code': 'bg-white border-slate-200',
  codex: 'bg-white border-slate-200',
  gemini: 'bg-white border-slate-200',
  system: 'bg-slate-100 text-slate-500 border-slate-200',
}

interface AgentAvatarProps {
  agentType: AgentType
  size?: 'sm' | 'md'
  className?: string
}

export function AgentAvatar({ agentType, size = 'md', className }: AgentAvatarProps) {
  const sizing = size === 'sm' ? 'h-7 w-7' : 'h-9 w-9'
  const iconSizing = size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'
  const logo = LOGOS[agentType]
  return (
    <div
      className={cn(
        'flex shrink-0 items-center justify-center rounded-lg border shadow-sm',
        sizing,
        TONES[agentType],
        className,
      )}
    >
      {logo ? (
        <img
          src={logo}
          alt={`${agentType} logo`}
          className={cn(iconSizing, 'object-contain')}
          draggable={false}
        />
      ) : (
        <ShieldCheck className={iconSizing} />
      )}
    </div>
  )
}
