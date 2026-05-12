import type { ArtifactType } from '@/api/types'
import { cn } from '@/lib/utils'
import { ARTIFACT_TYPE_META } from './artifact-type-meta'

interface ArtifactTypeBadgeProps {
  type: ArtifactType
  className?: string
}

export function ArtifactTypeBadge({ type, className }: ArtifactTypeBadgeProps) {
  const meta = ARTIFACT_TYPE_META[type] ?? ARTIFACT_TYPE_META.other
  const Icon = meta.icon
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide',
        meta.tone,
        className,
      )}
    >
      <Icon className="h-3 w-3" /> {meta.label}
    </span>
  )
}
