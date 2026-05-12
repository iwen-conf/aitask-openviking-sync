import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { PROJECT_STATUS_TONE } from '@/lib/format'
import type { ProjectStatus } from '@/api/types'
import { cn } from '@/lib/utils'

interface ProjectStatusBadgeProps {
  status: ProjectStatus
  className?: string
}

export function ProjectStatusBadge({ status, className }: ProjectStatusBadgeProps) {
  const { t } = useTranslation()
  return (
    <Badge tone="outline" className={cn(PROJECT_STATUS_TONE[status], className)}>
      {t(`project.status.${status}`)}
    </Badge>
  )
}
