import * as React from 'react'
import { cn } from '@/lib/utils'

export interface EmptyStateProps {
  icon?: React.ReactNode
  title: string
  description?: string
  action?: React.ReactNode
  className?: string
}

export function EmptyState({ icon, title, description, action, className }: EmptyStateProps) {
  return (
    <div
      className={cn(
        'flex h-full min-h-[200px] flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-[hsl(var(--border))] bg-[hsl(var(--muted)/0.5)] px-6 py-10 text-center',
        className,
      )}
    >
      {icon ? <div className="text-[hsl(var(--muted-foreground))]">{icon}</div> : null}
      <div className="space-y-1">
        <p className="text-base font-semibold text-[hsl(var(--foreground))]">{title}</p>
        {description ? (
          <p className="text-sm text-[hsl(var(--muted-foreground))]">{description}</p>
        ) : null}
      </div>
      {action}
    </div>
  )
}
