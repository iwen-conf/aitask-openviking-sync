import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'

export interface OpenVikingSettings {
  serverUrl: string
  enableMemoryWrite: boolean
  enableAutoSync: boolean
  apiKeySet: boolean
  lastSyncAt?: string
  lastError?: string
}

export interface UpdateOpenVikingSettingsInput {
  serverUrl: string
  apiKey?: string
  enableMemoryWrite: boolean
  enableAutoSync: boolean
}

export interface OpenVikingStatus {
  ok: boolean
  latencyMs?: number
  error?: string
}

export const systemOpenvikingKeys = {
  all: ['system', 'openviking'] as const,
  settings: () => [...systemOpenvikingKeys.all, 'settings'] as const,
  status: () => [...systemOpenvikingKeys.all, 'status'] as const,
}

export function useSystemOpenVikingSettingsQuery() {
  return useQuery({
    queryKey: systemOpenvikingKeys.settings(),
    queryFn: () => request<OpenVikingSettings>('/api/system/openviking/settings'),
  })
}

export function useUpdateSystemOpenVikingSettingsMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: UpdateOpenVikingSettingsInput) =>
      request<OpenVikingSettings>('/api/system/openviking/settings', {
        method: 'PUT',
        body: input,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: systemOpenvikingKeys.settings() })
      queryClient.invalidateQueries({ queryKey: systemOpenvikingKeys.status() })
    },
  })
}

export function useSystemOpenVikingStatusQuery(options?: { refetchInterval?: number }) {
  return useQuery({
    queryKey: systemOpenvikingKeys.status(),
    queryFn: () => request<OpenVikingStatus>('/api/system/openviking/status'),
    refetchInterval: options?.refetchInterval,
  })
}
