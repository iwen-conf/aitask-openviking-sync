import { AgentRoom } from '@/features/room/agent-room'
import { useProjectOutletContext } from './use-project-context'

export function RoomRoute() {
  const { projectId } = useProjectOutletContext()
  return <AgentRoom projectId={projectId} />
}
