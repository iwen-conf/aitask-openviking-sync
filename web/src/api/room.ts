import { useEffect } from 'react'
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import { createRoomSocket } from './ws'
import { taskKeys } from './tasks'
import { useRoomStore } from '@/stores/room-store'
import { env } from '@/lib/env'
import type { CursorList, Room, RoomMention, RoomMessage, SendMessageInput } from './types'

export const roomKeys = {
  all: ['room'] as const,
  byProject: (projectId: string) => [...roomKeys.all, projectId] as const,
  messages: (projectId: string) => [...roomKeys.byProject(projectId), 'messages'] as const,
  mentions: (projectId: string) => [...roomKeys.byProject(projectId), 'mentions'] as const,
  unreadMentions: (projectId: string) =>
    [...roomKeys.byProject(projectId), 'mentions', 'unread'] as const,
}

export function useRoomQuery(projectId: string | undefined) {
  return useQuery({
    queryKey: projectId ? roomKeys.byProject(projectId) : ['room', 'unknown'],
    queryFn: () => request<Room>(`/api/projects/${projectId}/room`),
    enabled: Boolean(projectId),
  })
}

export function useRoomMessagesQuery(projectId: string | undefined) {
  return useQuery({
    queryKey: projectId ? roomKeys.messages(projectId) : ['room', 'unknown', 'messages'],
    queryFn: () => request<CursorList<RoomMessage>>(`/api/projects/${projectId}/room/messages`),
    enabled: Boolean(projectId),
  })
}

export function useRoomMessagesInfiniteQuery(projectId: string | undefined) {
  return useInfiniteQuery({
    queryKey: projectId ? roomKeys.messages(projectId) : ['room', 'unknown', 'messages'],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      request<CursorList<RoomMessage>>(`/api/projects/${projectId}/room/messages`, {
        query: { limit: 50, before: pageParam },
      }),
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    enabled: Boolean(projectId),
  })
}

export function useSendMessageMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SendMessageInput) =>
      request<RoomMessage>(`/api/projects/${projectId}/room/messages`, {
        method: 'POST',
        body: input,
      }),
    onSuccess: () => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: roomKeys.messages(projectId) })
    },
  })
}

export function usePinMessageMutation(projectId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ messageId, as }: { messageId: string; as: 'decision' | 'note' }) =>
      request<RoomMessage>(`/api/projects/${projectId}/room/messages/${messageId}/pin`, {
        method: 'POST',
        body: { as },
      }),
    onSuccess: () => {
      if (!projectId) return
      queryClient.invalidateQueries({ queryKey: roomKeys.messages(projectId) })
    },
  })
}

export function useUnreadMentionsQuery(projectId: string | undefined) {
  const setUnreadMentions = useRoomStore((state) => state.setUnreadMentions)
  return useQuery({
    queryKey: projectId ? roomKeys.unreadMentions(projectId) : ['room', 'unknown', 'mentions'],
    queryFn: () => request<{ count: number }>(`/api/projects/${projectId}/room/mentions/unread`),
    enabled: Boolean(projectId),
    refetchInterval: 30_000,
    select: (data) => data.count,
    meta: {
      onSuccess: (count: number) => setUnreadMentions(count),
    },
  })
}

export function useRoomMentionsQuery(projectId: string | undefined) {
  return useQuery({
    queryKey: projectId ? roomKeys.mentions(projectId) : ['room', 'unknown', 'mentions'],
    queryFn: () => request<{ items: RoomMention[] }>(`/api/projects/${projectId}/room/mentions`),
    enabled: Boolean(projectId),
  })
}

/**
 * 接入真实 WebSocket：复用 §10.3 envelope schema，将事件落到 React Query cache。
 * 当当前没有项目上下文时，连接保持 closed 状态。
 */
export function useRoomSocket(projectId: string | undefined): void {
  const setStatus = useRoomStore((state) => state.setStatus)
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!projectId || !env.wsBaseUrl) {
      setStatus('closed')
      return
    }

    const url = `${env.wsBaseUrl.replace(/\/$/, '')}/ws/projects/${projectId}/agent-room`
    const socket = createRoomSocket(url, {
      onStatus: setStatus,
      onEnvelope: (envelope) => {
        switch (envelope.eventType) {
          case 'room.message':
          case 'context.handoff_created':
            queryClient.invalidateQueries({ queryKey: roomKeys.messages(projectId) })
            break
          case 'task.updated':
            queryClient.invalidateQueries({ queryKey: taskKeys.byProject(projectId) })
            break
          case 'room.member_online':
          case 'room.member_offline':
          case 'room.connected':
            queryClient.invalidateQueries({ queryKey: roomKeys.byProject(projectId) })
            break
          default:
            break
        }
      },
    })

    return () => {
      socket.close()
    }
  }, [projectId, setStatus, queryClient])
}
