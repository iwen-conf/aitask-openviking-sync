import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type {
  MemoryListing,
  MemoryReadResponse,
  MemorySearchHit,
  MemoryWriteInput,
  MemoryWriteResponse,
} from './types'

export const memoryKeys = {
  all: ['memory'] as const,
  listing: (projectId: string) => [...memoryKeys.all, 'listing', projectId] as const,
  read: (projectId: string, uri: string) => [...memoryKeys.all, 'read', projectId, uri] as const,
  search: (projectId: string, q: string) => [...memoryKeys.all, 'search', projectId, q] as const,
}

export function useMemoryListingQuery(projectId: string | undefined) {
  return useQuery({
    queryKey: projectId ? memoryKeys.listing(projectId) : ['memory', 'listing', 'unknown'],
    queryFn: () => request<MemoryListing>(`/api/projects/${projectId}/memory`),
    enabled: Boolean(projectId),
  })
}

export function useMemoryReadQuery(projectId: string | undefined, uri: string | undefined) {
  return useQuery({
    queryKey: projectId && uri ? memoryKeys.read(projectId, uri) : ['memory', 'read', 'unknown'],
    queryFn: () =>
      request<MemoryReadResponse>(`/api/projects/${projectId}/memory/read`, {
        query: { uri },
      }),
    enabled: Boolean(projectId && uri),
  })
}

export function useMemorySearchQuery(projectId: string | undefined, q: string) {
  return useQuery({
    queryKey: projectId && q ? memoryKeys.search(projectId, q) : ['memory', 'search', 'unknown'],
    queryFn: () =>
      request<{ items: MemorySearchHit[] }>(`/api/projects/${projectId}/memory/search`, {
        query: { q },
      }),
    enabled: Boolean(projectId && q.trim().length > 0),
  })
}

export function useMemoryWriteMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: MemoryWriteInput) =>
      request<MemoryWriteResponse>(`/api/projects/${projectId}/memory/write`, {
        method: 'POST',
        body: input,
      }),
    onSuccess: () => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: memoryKeys.listing(projectId) })
    },
  })
}
