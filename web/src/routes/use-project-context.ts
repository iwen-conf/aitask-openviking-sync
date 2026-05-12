import { useOutletContext } from 'react-router-dom'

export interface ProjectOutletContext {
  projectId: string
}

export function useProjectOutletContext(): ProjectOutletContext {
  return useOutletContext<ProjectOutletContext>()
}
