import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useNavigate } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useCreateProjectMutation } from '@/api/projects'
import { useToast } from '@/components/ui/use-toast'
import { InitCommandDialog } from './init-command-dialog'

const schema = z.object({
  name: z.string().min(2, '至少 2 个字符').max(80, '最长 80 个字符'),
  goal: z.string().min(4, '请描述项目目标').max(200, '最长 200 个字符'),
  description: z.string().max(500, '最长 500 个字符').optional(),
})

type FormValues = z.infer<typeof schema>

interface CreateProjectDialogProps {
  open: boolean
  onOpenChange(open: boolean): void
}

interface PendingProject {
  projectId: string
  initCommand: string
}

export function CreateProjectDialog({ open, onOpenChange }: CreateProjectDialogProps) {
  const navigate = useNavigate()
  const { toast } = useToast()
  const createProject = useCreateProjectMutation()
  const [pending, setPending] = useState<PendingProject | null>(null)
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', goal: '', description: '' },
  })

  useEffect(() => {
    if (!open) form.reset()
  }, [open, form])

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const project = await createProject.mutateAsync(values)
      onOpenChange(false)
      setPending({ projectId: project.projectId, initCommand: project.initCommand })
      toast({ title: '项目已创建', tone: 'success', duration: 2500 })
    } catch (error) {
      toast({
        title: '创建失败',
        description: error instanceof Error ? error.message : '未知错误',
        tone: 'destructive',
      })
    }
  })

  const handleContinue = () => {
    if (!pending) return
    const { projectId } = pending
    setPending(null)
    navigate(`/projects/${projectId}/tasks`)
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>新建项目</DialogTitle>
            <DialogDescription>
              创建后将在 Web Console 内生成 project / room / OpenViking 根空间， 并返回{' '}
              <span className="font-mono">aitask init</span> 命令。
            </DialogDescription>
          </DialogHeader>
          <form className="grid gap-4 py-2" onSubmit={onSubmit}>
            <div className="space-y-1">
              <Label htmlFor="project-name">项目名称</Label>
              <Input
                id="project-name"
                autoFocus
                {...form.register('name')}
                placeholder="例如：AgentTaskSystem v2"
              />
              {form.formState.errors.name ? (
                <p className="text-xs text-rose-600">{form.formState.errors.name.message}</p>
              ) : null}
            </div>
            <div className="space-y-1">
              <Label htmlFor="project-goal">目标 (Goal)</Label>
              <Input
                id="project-goal"
                {...form.register('goal')}
                placeholder="一句话描述要交付什么"
              />
              {form.formState.errors.goal ? (
                <p className="text-xs text-rose-600">{form.formState.errors.goal.message}</p>
              ) : null}
            </div>
            <div className="space-y-1">
              <Label htmlFor="project-description">描述（可选）</Label>
              <Textarea
                id="project-description"
                rows={3}
                {...form.register('description')}
                placeholder="可补充背景、关键约束、关联资源链接"
              />
            </div>
            <DialogFooter className="pt-2">
              <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
                取消
              </Button>
              <Button type="submit" disabled={createProject.isPending}>
                {createProject.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
                创建项目
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <InitCommandDialog
        open={pending !== null}
        command={pending?.initCommand ?? ''}
        onContinue={handleContinue}
        onOpenChange={(next) => {
          if (!next) setPending(null)
        }}
      />
    </>
  )
}
