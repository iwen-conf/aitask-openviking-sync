import type { Task, TaskStatus } from '@/api/types'

const DRAGGABLE_STATUSES: ReadonlySet<TaskStatus> = new Set(['planned', 'delegated', 'blocked'])

export function isDraggableTask(task: Task): boolean {
  return DRAGGABLE_STATUSES.has(task.status)
}
