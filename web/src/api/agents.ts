import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type {
  Agent,
  CreateAgentInput,
  IssueTokenInput,
  IssueTokenResponse,
  RevokeTokenInput,
  RevokeTokenResponse,
} from './types'

export const agentKeys = {
  all: ['agents'] as const,
  list: () => [...agentKeys.all, 'list'] as const,
}

export function useAgentsQuery() {
  return useQuery({
    queryKey: agentKeys.list(),
    queryFn: () => request<{ items: Agent[] }>('/api/agents'),
  })
}

export function useCreateAgentMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateAgentInput) =>
      request<Agent>('/api/agents', { method: 'POST', body: input }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentKeys.list() })
    },
  })
}

export function useRevokeAgentTokenMutation(agentId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (params: { tokenId: string; input: RevokeTokenInput }) =>
      request<RevokeTokenResponse>(`/api/agents/${agentId}/tokens/${params.tokenId}/revoke`, {
        method: 'POST',
        body: params.input,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentKeys.list() })
    },
  })
}

export function useIssueAgentTokenMutation(agentId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: IssueTokenInput) =>
      request<IssueTokenResponse>(`/api/agents/${agentId}/tokens`, {
        method: 'POST',
        body: input,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentKeys.list() })
    },
  })
}
