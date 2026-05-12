import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors',
  {
    variants: {
      tone: {
        default: 'border-transparent bg-[hsl(var(--accent))] text-[hsl(var(--accent-foreground))]',
        outline: 'border-[hsl(var(--border))] text-[hsl(var(--foreground))]',
        muted: 'border-transparent bg-[hsl(var(--muted))] text-[hsl(var(--muted-foreground))]',
        success: 'border-emerald-200 bg-emerald-50 text-emerald-700',
        warning: 'border-amber-200 bg-amber-50 text-amber-700',
        info: 'border-sky-200 bg-sky-50 text-sky-700',
        destructive: 'border-rose-200 bg-rose-50 text-rose-700',
      },
    },
    defaultVariants: {
      tone: 'default',
    },
  },
)

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>, VariantProps<typeof badgeVariants> {}

export function Badge({ className, tone, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ tone }), className)} {...props} />
}
