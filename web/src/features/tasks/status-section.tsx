import { motion } from 'framer-motion'
import { useTranslation } from 'react-i18next'
import { TASK_GROUP_TONE, type TaskGroupKey } from '@/lib/format'
import { stagger } from '@/components/shared/motion-presets'
import { cn } from '@/lib/utils'
import type { Task } from '@/api/types'
import { TaskCard } from './task-card'

interface StatusSectionProps {
  groupKey: TaskGroupKey
  tasks: Task[]
  onCancel(taskId: string): void
  onSelect(taskId: string): void
}

export function StatusSection({ groupKey, tasks, onCancel, onSelect }: StatusSectionProps) {
  const { t } = useTranslation()
  if (tasks.length === 0) return null
  const meta = TASK_GROUP_TONE[groupKey]
  return (
    <div className="space-y-2">
      <div
        className={cn(
          'flex items-center gap-1.5 px-1 text-xs font-semibold uppercase tracking-wider',
          meta.tone,
        )}
      >
        <span className={cn('h-1.5 w-1.5 rounded-full', meta.dot)} />
        {t(`task.group.${groupKey}`)} ({tasks.length})
      </div>
      <motion.div variants={stagger} initial="hidden" animate="visible" className="space-y-2">
        {tasks.map((task) => (
          <TaskCard key={task.taskId} task={task} onCancel={onCancel} onSelect={onSelect} />
        ))}
      </motion.div>
    </div>
  )
}
