import * as React from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Loader2 } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/use-toast'
import { ApiError, describeError } from '@/api/errors'
import { useMemoryWriteMutation } from '@/api/memory'
import { Markdown } from '@/components/shared/markdown'
import type { MemoryWriteTarget } from '@/api/types'

const SCHEMA = z.object({
  target: z.enum(['decisions', 'summary', 'note', 'handoff']),
  title: z.string().min(2, '标题至少 2 个字'),
  content: z.string().min(10, '正文不少于 10 个字'),
  relatedTaskId: z.string().optional(),
})

type FormValues = z.infer<typeof SCHEMA>

const TARGET_OPTIONS: { value: MemoryWriteTarget; label: string; hint: string }[] = [
  { value: 'decisions', label: '决策', hint: '写入 memory/decisions/,作为后续协作的依据' },
  { value: 'summary', label: '总结', hint: '写入 memory/summary/,沉淀阶段性成果' },
  { value: 'note', label: '便签', hint: '写入 memory/notes/,临时观察或待办' },
  { value: 'handoff', label: 'Handoff', hint: '生成 handoff 文档,供新对话承接' },
]

interface MemoryWriteDialogProps {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onWritten?: (uri: string) => void
}

export function MemoryWriteDialog({
  projectId,
  open,
  onOpenChange,
  onWritten,
}: MemoryWriteDialogProps) {
  const { toast } = useToast()
  const mutation = useMemoryWriteMutation(projectId)
  const form = useForm<FormValues>({
    resolver: zodResolver(SCHEMA),
    defaultValues: { target: 'decisions', title: '', content: '', relatedTaskId: '' },
  })

  const target = useWatch({ control: form.control, name: 'target' })
  const content = useWatch({ control: form.control, name: 'content' })
  const [showPreview, setShowPreview] = React.useState(false)

  // Dialog 关闭时重置表单与预览状态。直接对话框 close 触发,避免 setState in effect。
  const handleOpenChange = (next: boolean) => {
    if (!next) {
      form.reset()
      setShowPreview(false)
    }
    onOpenChange(next)
  }

  const handleSubmit = form.handleSubmit(async (values) => {
    try {
      const response = await mutation.mutateAsync({
        target: values.target,
        title: values.title.trim(),
        content: values.content,
        relatedTaskId: values.relatedTaskId?.trim() || undefined,
      })
      toast({
        title: '已写入 OpenViking',
        description: response.uri,
        tone: 'success',
      })
      onWritten?.(response.uri)
      handleOpenChange(false)
    } catch (error) {
      if (error instanceof ApiError) {
        const description = describeError(error.envelope)
        toast({ title: description.title, description: description.hint, tone: 'destructive' })
      } else {
        toast({ title: '写入失败', tone: 'destructive' })
      }
    }
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>写入 OpenViking</DialogTitle>
          <DialogDescription>
            提交前会预览渲染结果;写入受后端 scope 限制,任务权威状态/Token 不可在此写入。
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="memory-target">目录</Label>
              <Select
                value={target}
                onValueChange={(value) => form.setValue('target', value as MemoryWriteTarget)}
              >
                <SelectTrigger id="memory-target">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TARGET_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-[11px] text-slate-500">
                {TARGET_OPTIONS.find((opt) => opt.value === target)?.hint}
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="memory-title">标题</Label>
              <Input id="memory-title" {...form.register('title')} placeholder="简短说明本次写入" />
              {form.formState.errors.title ? (
                <p className="text-[11px] text-rose-600">{form.formState.errors.title.message}</p>
              ) : null}
            </div>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="memory-related">关联任务 ID (可选)</Label>
            <Input id="memory-related" {...form.register('relatedTaskId')} placeholder="task_..." />
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="memory-content">Markdown 正文</Label>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setShowPreview((prev) => !prev)}
              >
                {showPreview ? '编辑' : '预览'}
              </Button>
            </div>
            {showPreview ? (
              <div className="max-h-64 overflow-y-auto rounded-md border border-slate-200 bg-white px-3 py-2 text-sm">
                <Markdown>{content || ''}</Markdown>
              </div>
            ) : (
              <Textarea
                id="memory-content"
                rows={10}
                {...form.register('content')}
                placeholder="# 决策\n- 背景:\n- 决定:\n- 影响:"
              />
            )}
            {form.formState.errors.content ? (
              <p className="text-[11px] text-rose-600">{form.formState.errors.content.message}</p>
            ) : null}
          </div>

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => handleOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin" /> 写入中
                </>
              ) : (
                '提交写入'
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
