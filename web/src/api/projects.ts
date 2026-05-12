import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type {
  ArchiveProjectResponse,
  CompleteProjectResponse,
  CreateProjectInput,
  CreateProjectResponse,
  CursorList,
  Project,
  ProjectSummary,
  UpdateProjectInput,
} from './types'

export const projectKeys = {
  all: ['projects'] as const,
  list: () => [...projectKeys.all, 'list'] as const,
  detail: (projectId: string) => [...projectKeys.all, 'detail', projectId] as const,
}

export function useProjectsQuery() {
  return useQuery({
    queryKey: projectKeys.list(),
    queryFn: () => request<CursorList<ProjectSummary>>('/api/projects'),
  })
}

export function useProjectQuery(projectId: string | undefined) {
  return useQuery({
    queryKey: projectId ? projectKeys.detail(projectId) : ['projects', 'detail', 'unknown'],
    queryFn: () => request<Project>(`/api/projects/${projectId}`),
    enabled: Boolean(projectId),
  })
}

export function useCreateProjectMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateProjectInput) =>
      request<CreateProjectResponse>('/api/projects', {
        method: 'POST',
        body: input,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
    },
  })
}

export function useUpdateProjectMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: UpdateProjectInput) =>
      request<Project>(`/api/projects/${projectId}`, {
        method: 'PATCH',
        body: input,
      }),
    onSuccess: () => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: projectKeys.detail(projectId) })
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
    },
  })
}

export function useCompleteProjectMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      request<CompleteProjectResponse>(`/api/projects/${projectId}/complete`, {
        method: 'POST',
        body: { confirm: true },
      }),
    onSuccess: () => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: projectKeys.detail(projectId) })
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
    },
  })
}

export function useArchiveProjectMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (reason: string) =>
      request<ArchiveProjectResponse>(`/api/projects/${projectId}/archive`, {
        method: 'POST',
        body: { confirm: true, reason },
      }),
    onSuccess: () => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: projectKeys.detail(projectId) })
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
    },
  })
}
