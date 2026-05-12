import { useEffect, useMemo, useState } from 'react'
import { Database, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { UlidChip } from '@/components/shared/ulid-chip'
import { useToast } from '@/components/ui/use-toast'
import {
  usePinMessageMutation,
  useRoomMessagesInfiniteQuery,
  useRoomQuery,
  useUnreadMentionsQuery,
} from '@/api/room'
import { useProjectQuery } from '@/api/projects'
import { useRoomStore } from '@/stores/room-store'
import { MessageList } from './message-list'
import { MessageComposer } from './message-composer'
import { PresenceIndicator } from './presence-indicator'

interface AgentRoomProps {
  projectId: string
}

export function AgentRoom({ projectId }: AgentRoomProps) {
  const projectQuery = useProjectQuery(projectId)
  const roomQuery = useRoomQuery(projectId)
  const messagesQuery = useRoomMessagesInfiniteQuery(projectId)
  const unreadMentionsQuery = useUnreadMentionsQuery(projectId)
  const pinMessage = usePinMessageMutation(projectId)
  const status = useRoomStore((state) => state.status)
  const setUnreadMentions = useRoomStore((state) => state.setUnreadMentions)
  const unreadMentions = useRoomStore((state) => state.unreadMentions)
  const { toast } = useToast()
  const [searchTerm, setSearchTerm] = useState('')

  useEffect(() => {
    if (typeof unreadMentionsQuery.data === 'number') {
      setUnreadMentions(unreadMentionsQuery.data)
    }
  }, [setUnreadMentions, unreadMentionsQuery.data])

  const messages = useMemo(() => {
    const pages = messagesQuery.data?.pages ?? []
    return pages.flatMap((page) => page.items).reverse()
  }, [messagesQuery.data])
  const project = projectQuery.data
  const room = roomQuery.data

  const filteredMessages = useMemo(() => {
    const needle = searchTerm.trim().toLowerCase()
    if (!needle) return messages
    return messages.filter((message) => {
      const payload = message.payload ? JSON.stringify(message.payload).toLowerCase() : ''
      return (
        message.content.toLowerCase().includes(needle) ||
        message.messageId.toLowerCase().includes(needle) ||
        payload.includes(needle)
      )
    })
  }, [messages, searchTerm])

  const handlePin = async (messageId: string) => {
    try {
      await pinMessage.mutateAsync({ messageId, as: 'decision' })
      toast({ title: '消息已 pin 为 decision 候选', tone: 'success' })
    } catch (error) {
      toast({
        title: 'Pin 失败',
        description: error instanceof Error ? error.message : undefined,
        tone: 'destructive',
      })
    }
  }

  return (
    <div className="flex h-full flex-col overflow-hidden bg-[hsl(var(--card))]">
      <div className="z-10 shrink-0 border-b border-[hsl(var(--border))] bg-[hsl(var(--card))] px-6 py-5 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-bold tracking-tight text-[hsl(var(--foreground))]">
                Agent 协作室
              </h1>
              {project ? (
                <span className="inline-flex items-center gap-2 rounded-md border border-[hsl(var(--accent))] bg-[hsl(var(--accent)/0.4)] px-2.5 py-0.5 text-sm font-semibold text-[hsl(var(--accent-foreground))]">
                  {project.name}
                </span>
              ) : null}
              {room ? <UlidChip id={room.roomId} /> : null}
            </div>
            <p className="text-xs text-[hsl(var(--muted-foreground))]">
              §10 / §16：消息流由后端 Task Service 与 Room Service 联合驱动；前端不构造系统消息。
              当前连接状态：
              <span className="font-mono text-[hsl(var(--foreground))]">{status}</span>
            </p>
            {room ? <PresenceIndicator members={room.members} /> : null}
          </div>

          <div className="flex flex-wrap items-center justify-end gap-2">
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[hsl(var(--muted-foreground))]" />
              <Input
                value={searchTerm}
                onChange={(event) => setSearchTerm(event.target.value)}
                placeholder="搜索消息 / task id"
                className="h-8 w-56 pl-7 text-xs"
              />
            </div>
            {unreadMentions > 0 ? (
              <span className="rounded-full bg-rose-100 px-2.5 py-1 text-xs font-semibold text-rose-700">
                @{unreadMentions}
              </span>
            ) : null}
            <Button
              variant="outline"
              className="gap-2"
              onClick={() => toast({ title: '记忆库同步已交由后端任务处理', tone: 'info' })}
            >
              <Database className="h-4 w-4 text-[hsl(var(--primary))]" />
              同步至记忆库
            </Button>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto bg-[hsl(var(--muted)/0.4)] scrollbar-thin">
        <MessageList
          messages={filteredMessages}
          isLoading={messagesQuery.isLoading}
          searchTerm={searchTerm}
          hasMore={messagesQuery.hasNextPage}
          isFetchingMore={messagesQuery.isFetchingNextPage}
          onLoadMore={() => void messagesQuery.fetchNextPage()}
          onPin={handlePin}
        />
      </div>

      <MessageComposer
        projectId={projectId}
        disabled={status !== 'open' && status !== 'reconnecting'}
        members={room?.members ?? []}
      />
    </div>
  )
}
