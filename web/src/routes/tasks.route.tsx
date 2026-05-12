import { TaskBoard } from '@/features/tasks/task-board'
import { useProjectOutletContext } from './use-project-context'

export function TasksRoute() {
  const { projectId } = useProjectOutletContext()
  return <TaskBoard projectId={projectId} />
}
