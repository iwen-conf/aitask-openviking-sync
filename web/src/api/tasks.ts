import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import { projectKeys } from './projects'
import type {
  AgentType,
  CreateTaskInput,
  CursorList,
  FailTaskInput,
  ReviewTaskInput,
  Task,
  TaskEvent,
  TaskStatus,
} from './types'

export interface TaskFilters {
  status?: TaskStatus
  assigneeAgentType?: AgentType
  skill?: string
  q?: string
}

export const taskKeys = {
  all: ['tasks'] as const,
  byProject: (projectId: string) => [...taskKeys.all, 'project', projectId] as const,
  list: (projectId: string, filters: TaskFilters) =>
    [...taskKeys.byProject(projectId), filters] as const,
  detail: (projectId: string, taskId: string) =>
    [...taskKeys.byProject(projectId), 'detail', taskId] as const,
  events: (projectId: string, taskId: string) =>
    [...taskKeys.byProject(projectId), 'events', taskId] as const,
}

function buildTaskQuery(filters: TaskFilters): string {
  const params = new URLSearchParams()
  if (filters.status) params.set('status', filters.status)
  if (filters.assigneeAgentType) params.set('assigneeAgentType', filters.assigneeAgentType)
  if (filters.skill) params.set('skill', filters.skill)
  if (filters.q) params.set('q', filters.q)
  const qs = params.toString()
  return qs ? `?${qs}` : ''
}

export function useTasksQuery(projectId: string | undefined, filters: TaskFilters = {}) {
  return useQuery({
    queryKey: projectId ? taskKeys.list(projectId, filters) : ['tasks', 'unknown'],
    queryFn: () =>
      request<CursorList<Task>>(`/api/projects/${projectId}/tasks${buildTaskQuery(filters)}`),
    enabled: Boolean(projectId),
  })
}

export function useTaskDetailQuery(projectId: string | undefined, taskId: string | undefined) {
  return useQuery({
    queryKey:
      projectId && taskId ? taskKeys.detail(projectId, taskId) : ['tasks', 'detail', 'unknown'],
    queryFn: () => request<Task>(`/api/projects/${projectId}/tasks/${taskId}`),
    enabled: Boolean(projectId && taskId),
  })
}

export function useTaskEventsQuery(projectId: string | undefined, taskId: string | undefined) {
  return useQuery({
    queryKey:
      projectId && taskId ? taskKeys.events(projectId, taskId) : ['tasks', 'events', 'unknown'],
    queryFn: () =>
      request<CursorList<TaskEvent>>(`/api/projects/${projectId}/tasks/${taskId}/events`, {
        query: { limit: 20 },
      }),
    enabled: Boolean(projectId && taskId),
  })
}

export function useCreateTaskMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateTaskInput) =>
      request<Task>(`/api/projects/${projectId}/tasks`, {
        method: 'POST',
        body: input,
      }),
    onSuccess: () => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: taskKeys.byProject(projectId) })
      queryClient.invalidateQueries({ queryKey: projectKeys.list() })
    },
  })
}

export function useCancelTaskMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (taskId: string) =>
      request<{ taskId: string; status: 'cancelled' }>(
        `/api/projects/${projectId}/tasks/${taskId}/cancel`,
        { method: 'POST', body: { reason: 'cancelled by operator' } },
      ),
    onSuccess: () => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: taskKeys.byProject(projectId) })
    },
  })
}

export function useFailTaskMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, input }: { taskId: string; input: FailTaskInput }) =>
      request<Task>(`/api/projects/${projectId}/tasks/${taskId}/fail`, {
        method: 'POST',
        body: input,
      }),
    onSuccess: (_data, vars) => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: taskKeys.byProject(projectId) })
      queryClient.invalidateQueries({ queryKey: taskKeys.events(projectId, vars.taskId) })
    },
  })
}

export function useReviewTaskMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, input }: { taskId: string; input: ReviewTaskInput }) =>
      request<Task>(`/api/projects/${projectId}/tasks/${taskId}/review`, {
        method: 'POST',
        body: input,
      }),
    onSuccess: (_data, vars) => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: taskKeys.byProject(projectId) })
      queryClient.invalidateQueries({ queryKey: taskKeys.events(projectId, vars.taskId) })
    },
  })
}

export interface DelegateTaskInput {
  taskId: string
  agentId: string
  agentType: AgentType
  reason?: string
}

interface DelegateContext {
  previous: Array<[readonly unknown[], CursorList<Task> | undefined]>
}

export function useDelegateTaskMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation<
    { taskId: string; status: TaskStatus; assigneeAgentId: string; assigneeAgentType: AgentType },
    Error,
    DelegateTaskInput,
    DelegateContext
  >({
    mutationFn: ({ taskId, agentId, agentType, reason }) =>
      request(`/api/projects/${projectId}/tasks/${taskId}/delegate`, {
        method: 'POST',
        body: { agentId, agentType, reason: reason ?? 'reassigned via board' },
      }),
    onMutate: async (vars) => {
      if (!projectId) return { previous: [] }
      await queryClient.cancelQueries({ queryKey: taskKeys.byProject(projectId) })
      const previous = queryClient
        .getQueriesData<CursorList<Task>>({ queryKey: taskKeys.byProject(projectId) })
        .filter((entry): entry is [readonly unknown[], CursorList<Task> | undefined] =>
          Array.isArray(entry[0]),
        )
      queryClient.setQueriesData<CursorList<Task>>(
        { queryKey: taskKeys.byProject(projectId) },
        (data) => {
          if (!data || typeof data !== 'object' || !('items' in data)) return data
          return {
            ...data,
            items: data.items.map((t) =>
              t.taskId === vars.taskId
                ? {
                    ...t,
                    assigneeAgentId: vars.agentId,
                    assigneeAgentType: vars.agentType,
                  }
                : t,
            ),
          }
        },
      )
      return { previous }
    },
    onError: (_err, _vars, context) => {
      if (!context) return
      for (const [key, data] of context.previous) {
        queryClient.setQueryData(key, data)
      }
    },
    onSettled: () => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: taskKeys.byProject(projectId) })
    },
  })
}
