import * as React from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Loader2, UserPlus } from 'lucide-react'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/use-toast'
import { useCreateAgentMutation } from '@/api/agents'
import { ApiError, describeError } from '@/api/errors'
import type { AgentRole, AgentType } from '@/api/types'

const AGENT_TYPES: AgentType[] = ['claude-code', 'codex', 'gemini', 'system']
const AGENT_ROLES: AgentRole[] = ['coordinator', 'worker', 'reviewer', 'observer', 'system']

const schema = z.object({
  name: z.string().min(2, '名称至少 2 个字符').max(80, '名称不超过 80 个字符'),
  agentType: z.enum(['claude-code', 'codex', 'gemini', 'system']),
  role: z.enum(['coordinator', 'worker', 'reviewer', 'observer', 'system']),
  defaultModel: z.string().max(120).optional().or(z.literal('')),
  skillsRaw: z.string().optional().or(z.literal('')),
  modelsRaw: z.string().optional().or(z.literal('')),
})

type FormValues = z.infer<typeof schema>

const DEFAULTS: Record<
  AgentType,
  { model: string; skills: string; models: string; role: AgentRole }
> = {
  'claude-code': {
    model: 'claude-opus-4-7',
    skills: 'coordinator, planner',
    models: 'claude-opus-4-7, claude-sonnet-4-6',
    role: 'coordinator',
  },
  codex: { model: 'gpt-5', skills: 'worker, coder', models: 'gpt-5', role: 'worker' },
  gemini: {
    model: 'gemini-2.5-pro',
    skills: 'worker, frontend',
    models: 'gemini-2.5-pro',
    role: 'worker',
  },
  system: { model: '', skills: '', models: '', role: 'system' },
}

function csvToList(raw: string | undefined): string[] {
  if (!raw) return []
  return Array.from(
    new Set(
      raw
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
    ),
  )
}

interface CreateAgentDialogProps {
  open: boolean
  onClose: () => void
}

export function CreateAgentDialog({ open, onClose }: CreateAgentDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-lg">
        {open ? <CreateAgentDialogBody onClose={onClose} /> : null}
      </DialogContent>
    </Dialog>
  )
}

function CreateAgentDialogBody({ onClose }: { onClose: () => void }) {
  const { toast } = useToast()
  const mutation = useCreateAgentMutation()
  const [submitError, setSubmitError] = React.useState<string | null>(null)

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      agentType: 'claude-code',
      role: 'coordinator',
      defaultModel: DEFAULTS['claude-code'].model,
      skillsRaw: DEFAULTS['claude-code'].skills,
      modelsRaw: DEFAULTS['claude-code'].models,
    },
  })

  const watchedType = form.watch('agentType')

  const applyDefaults = (next: AgentType) => {
    const preset = DEFAULTS[next]
    form.setValue('role', preset.role)
    form.setValue('defaultModel', preset.model)
    form.setValue('skillsRaw', preset.skills)
    form.setValue('modelsRaw', preset.models)
  }

  const handleSubmit = form.handleSubmit(async (values) => {
    setSubmitError(null)
    try {
      const created = await mutation.mutateAsync({
        name: values.name.trim(),
        agentType: values.agentType,
        role: values.role,
        defaultModel: values.defaultModel?.trim() || undefined,
        skills: csvToList(values.skillsRaw),
        models: csvToList(values.modelsRaw),
      })
      toast({
        title: 'Agent 已创建',
        description: `${created.name} (${created.agentType}) 已注册,可在右侧颁发 Token。`,
        tone: 'success',
      })
      onClose()
    } catch (err) {
      if (err instanceof ApiError) {
        const desc = describeError(err.envelope)
        setSubmitError(desc.hint ? `${desc.title} — ${desc.hint}` : desc.title)
      } else {
        setSubmitError('创建失败,请重试。')
      }
    }
  })

  const close = () => {
    if (mutation.isPending) return
    onClose()
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2 text-indigo-700">
          <UserPlus className="h-4 w-4" />
          新建 Agent
        </DialogTitle>
        <DialogDescription>
          注册一个新的 Agent 身份。创建后可在右侧详情面板颁发 Token。
        </DialogDescription>
      </DialogHeader>

      <form onSubmit={handleSubmit} className="space-y-4">
        {submitError ? (
          <div className="rounded-md border border-rose-200 bg-rose-50/60 px-3 py-2 text-xs text-rose-700">
            {submitError}
          </div>
        ) : null}

        <div className="space-y-1.5">
          <Label htmlFor="agent-name">名称 *</Label>
          <Input id="agent-name" placeholder="例如 Claude Code" {...form.register('name')} />
          {form.formState.errors.name ? (
            <p className="text-[11px] text-rose-600">{form.formState.errors.name.message}</p>
          ) : null}
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1.5">
            <Label>类型 *</Label>
            <Select
              value={watchedType}
              onValueChange={(value) => {
                const next = value as AgentType
                form.setValue('agentType', next, { shouldValidate: true })
                applyDefaults(next)
              }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {AGENT_TYPES.map((t) => (
                  <SelectItem key={t} value={t}>
                    {t}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label>角色 *</Label>
            <Select
              value={form.watch('role')}
              onValueChange={(value) =>
                form.setValue('role', value as AgentRole, { shouldValidate: true })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {AGENT_ROLES.map((r) => (
                  <SelectItem key={r} value={r}>
                    {r}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="agent-default-model">默认 Model</Label>
          <Input
            id="agent-default-model"
            placeholder="例如 claude-opus-4-7"
            {...form.register('defaultModel')}
          />
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="agent-skills">Skills (逗号分隔)</Label>
          <Input
            id="agent-skills"
            placeholder="coordinator, planner"
            {...form.register('skillsRaw')}
          />
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="agent-models">Models (逗号分隔)</Label>
          <Input
            id="agent-models"
            placeholder="claude-opus-4-7, claude-sonnet-4-6"
            {...form.register('modelsRaw')}
          />
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={close} disabled={mutation.isPending}>
            取消
          </Button>
          <Button type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
            创建
          </Button>
        </DialogFooter>
      </form>
    </>
  )
}
