import { useQuery } from '@tanstack/react-query'
import { request } from './client'
import type { ArtifactDetail, ArtifactSummary, ArtifactType } from './types'

export const artifactKeys = {
  all: ['artifacts'] as const,
  byProject: (projectId: string) => [...artifactKeys.all, 'project', projectId] as const,
  list: (projectId: string, filters: { taskId?: string; type?: ArtifactType }) =>
    [...artifactKeys.byProject(projectId), 'list', filters] as const,
  detail: (projectId: string, artifactId: string) =>
    [...artifactKeys.byProject(projectId), 'detail', artifactId] as const,
}

export interface ArtifactListFilters {
  taskId?: string
  type?: ArtifactType
}

export function useArtifactsQuery(
  projectId: string | undefined,
  filters: ArtifactListFilters = {},
) {
  return useQuery({
    queryKey: projectId ? artifactKeys.list(projectId, filters) : ['artifacts', 'unknown'],
    queryFn: () =>
      request<{ items: ArtifactSummary[] }>(`/api/projects/${projectId}/artifacts`, {
        query: { taskId: filters.taskId, type: filters.type },
      }),
    enabled: Boolean(projectId),
  })
}

export function useArtifactDetailQuery(
  projectId: string | undefined,
  artifactId: string | undefined,
) {
  return useQuery({
    queryKey:
      projectId && artifactId
        ? artifactKeys.detail(projectId, artifactId)
        : ['artifacts', 'detail', 'unknown'],
    queryFn: () => request<ArtifactDetail>(`/api/projects/${projectId}/artifacts/${artifactId}`),
    enabled: Boolean(projectId && artifactId),
  })
}
