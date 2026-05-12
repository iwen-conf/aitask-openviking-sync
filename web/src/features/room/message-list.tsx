import { useEffect, useRef } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { RoomMessage } from '@/api/types'
import { fadeUp } from '@/components/shared/motion-presets'
import { MessageBubble } from './message-bubble'

interface MessageListProps {
  messages: RoomMessage[]
  isLoading: boolean
  searchTerm?: string
  hasMore?: boolean
  isFetchingMore?: boolean
  onLoadMore?(): void
  onPin?(messageId: string): void
}

export function MessageList({
  messages,
  isLoading,
  searchTerm = '',
  hasMore = false,
  isFetchingMore = false,
  onLoadMore,
  onPin,
}: MessageListProps) {
  const endRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!searchTerm.trim()) endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages.length, searchTerm])

  if (isLoading && messages.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-[hsl(var(--muted-foreground))]">
        正在加载历史消息…
      </div>
    )
  }

  if (messages.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-[hsl(var(--muted-foreground))]">
        暂无消息，发送指令开启协作。
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4 px-6 py-5">
      {hasMore ? (
        <div className="flex justify-center">
          <Button variant="outline" size="sm" onClick={onLoadMore} disabled={isFetchingMore}>
            {isFetchingMore ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
            加载更早消息
          </Button>
        </div>
      ) : null}
      <AnimatePresence initial={false}>
        {messages.map((message) => (
          <motion.div
            key={message.messageId}
            variants={fadeUp}
            initial="hidden"
            animate="visible"
            exit="exit"
          >
            <MessageBubble message={message} searchTerm={searchTerm} onPin={onPin} />
          </motion.div>
        ))}
      </AnimatePresence>
      <div ref={endRef} />
    </div>
  )
}
