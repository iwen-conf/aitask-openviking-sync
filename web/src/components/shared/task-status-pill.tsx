import { useTranslation } from 'react-i18next'
import { TASK_STATUS_TONE } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import type { TaskStatus } from '@/api/types'
import { cn } from '@/lib/utils'

interface TaskStatusPillProps {
  status: TaskStatus
  className?: string
}

export function TaskStatusPill({ status, className }: TaskStatusPillProps) {
  const { t } = useTranslation()
  return (
    <Badge tone="muted" className={cn(TASK_STATUS_TONE[status], 'border-transparent', className)}>
      {t(`task.status.${status}`)}
    </Badge>
  )
}
