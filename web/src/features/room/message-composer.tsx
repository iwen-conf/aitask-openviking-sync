import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Loader2, Send } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { useToast } from '@/components/ui/use-toast'
import { useSendMessageMutation } from '@/api/room'
import type { AgentType, RoomMember } from '@/api/types'

const schema = z.object({
  content: z.string().min(1, '消息不能为空').max(4000, '最长 4000 字符'),
})

type FormValues = z.infer<typeof schema>

interface MessageComposerProps {
  projectId: string
  disabled?: boolean
  members?: RoomMember[]
}

const DEFAULT_MENTION_TARGETS: AgentType[] = ['claude-code', 'codex', 'gemini']

export function MessageComposer({ projectId, disabled, members = [] }: MessageComposerProps) {
  const sendMessage = useSendMessageMutation(projectId)
  const { toast } = useToast()
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { content: '' },
  })
  const watchedContent = useWatch({ control: form.control, name: 'content' })
  const mentionTargets = buildMentionTargets(members)
  const activeMention = extractActiveMention(watchedContent)
  const suggestions = activeMention
    ? mentionTargets.filter((target) => target.label.includes(activeMention.query.toLowerCase()))
    : []
  const selectedMentions = extractMentions(watchedContent, mentionTargets)

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      await sendMessage.mutateAsync({
        messageType: 'text',
        content: values.content,
        payload: {
          mentions: selectedMentions.map((mention) => ({
            type: mention.type,
            value: mention.value,
          })),
        },
      })
      form.reset({ content: '' })
    } catch (error) {
      toast({
        title: '消息发送失败',
        description: error instanceof Error ? error.message : '请检查网络后重试',
        tone: 'destructive',
      })
    }
  })

  const handleKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      void onSubmit()
    }
  }

  const insertMention = (target: MentionTarget) => {
    if (!activeMention) return
    const next =
      watchedContent.slice(0, activeMention.start) +
      `@${target.label} ` +
      watchedContent.slice(activeMention.end)
    form.setValue('content', next, { shouldDirty: true, shouldValidate: true })
  }

  return (
    <form
      onSubmit={onSubmit}
      className="border-t border-[hsl(var(--border))] bg-[hsl(var(--card))] p-4"
    >
      <div className="mx-auto flex max-w-4xl items-end gap-3">
        <div className="relative flex-1">
          <Textarea
            rows={2}
            placeholder="介入干预或发送系统指令；输入 @ 选择 Agent"
            {...form.register('content')}
            onKeyDown={handleKeyDown}
            className="min-h-[60px] resize-none"
            disabled={disabled || sendMessage.isPending}
          />
          {suggestions.length > 0 ? (
            <div className="absolute bottom-full left-0 mb-2 w-64 overflow-hidden rounded-lg border border-[hsl(var(--border))] bg-[hsl(var(--card))] shadow-xl">
              {suggestions.map((target) => (
                <button
                  key={`${target.type}:${target.value}`}
                  type="button"
                  onClick={() => insertMention(target)}
                  className="flex w-full items-center justify-between px-3 py-2 text-left text-xs hover:bg-[hsl(var(--muted))]"
                >
                  <span className="font-semibold">@{target.label}</span>
                  <span className="text-[hsl(var(--muted-foreground))]">{target.type}</span>
                </button>
              ))}
            </div>
          ) : null}
        </div>
        <Button
          type="submit"
          disabled={disabled || sendMessage.isPending || !watchedContent.trim()}
          className="gap-2 self-end"
        >
          {sendMessage.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Send className="h-4 w-4" />
          )}
          发送
        </Button>
      </div>
      {selectedMentions.length > 0 ? (
        <div className="mx-auto mt-2 flex max-w-4xl flex-wrap gap-1.5 text-xs">
          {selectedMentions.map((mention) => (
            <span
              key={`${mention.type}:${mention.value}`}
              className="rounded-full bg-indigo-50 px-2 py-0.5 font-mono text-indigo-700"
            >
              @{mention.label}
            </span>
          ))}
        </div>
      ) : null}
      {form.formState.errors.content ? (
        <p className="mx-auto mt-1 max-w-4xl text-xs text-rose-600">
          {form.formState.errors.content.message}
        </p>
      ) : null}
    </form>
  )
}

interface MentionTarget {
  label: string
  type: 'agent' | 'operator'
  value: string
}

function buildMentionTargets(members: RoomMember[]): MentionTarget[] {
  const byLabel = new Map<string, MentionTarget>()
  for (const type of DEFAULT_MENTION_TARGETS) {
    byLabel.set(type, { label: type, type: 'agent', value: type })
  }
  for (const member of members) {
    if (member.agentType) {
      byLabel.set(member.agentType, {
        label: member.agentType,
        type: 'agent',
        value: member.agentType,
      })
    }
    if (member.operatorLabel) {
      byLabel.set(member.operatorLabel, {
        label: member.operatorLabel,
        type: 'operator',
        value: member.operatorLabel,
      })
    }
  }
  return Array.from(byLabel.values()).sort((a, b) => a.label.localeCompare(b.label))
}

function extractActiveMention(
  content: string,
): { query: string; start: number; end: number } | null {
  const cursor = content.length
  const prefix = content.slice(0, cursor)
  const match = /(?:^|\s)@([a-zA-Z0-9_-]*)$/.exec(prefix)
  if (!match || match.index < 0) return null
  return {
    query: match[1] ?? '',
    start: match.index + (prefix[match.index] === '@' ? 0 : 1),
    end: cursor,
  }
}

function extractMentions(content: string, targets: MentionTarget[]): MentionTarget[] {
  const targetByLabel = new Map(targets.map((target) => [target.label.toLowerCase(), target]))
  const found = new Map<string, MentionTarget>()
  for (const match of content.matchAll(/@([a-zA-Z0-9_-]{2,80})/g)) {
    const target = targetByLabel.get((match[1] ?? '').toLowerCase())
    if (target) found.set(`${target.type}:${target.value}`, target)
  }
  return Array.from(found.values())
}
