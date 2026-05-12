import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { AlertTriangle, GitMerge, Loader2, PaintBucket, Server } from 'lucide-react'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/use-toast'
import { useCreateTaskMutation } from '@/api/tasks'
import type { Agent, AgentType } from '@/api/types'
import { cn } from '@/lib/utils'

// The three lanes that can receive delegated work, ordered as the §1.1 matrix
// presents them: BE owns the data and contract, FE consumes the contract,
// orchestrator owns the wiring across both. `system` agents are never
// delegatable and are filtered out before they reach the picker.
const DELEGATABLE_LANES: ReadonlyArray<DelegatableLane> = ['codex', 'gemini', 'claude-code']

type DelegatableLane = Extract<AgentType, 'codex' | 'gemini' | 'claude-code'>

const LANE_ICON: Record<DelegatableLane, typeof Server> = {
  codex: Server,
  gemini: PaintBucket,
  'claude-code': GitMerge,
}

// Loose keyword fences for cross-lane heuristics. We deliberately keep these
// fuzzy: hits trigger a non-blocking nudge, never a hard block.
const LANE_KEYWORDS: Record<DelegatableLane, RegExp> = {
  codex: /(api\b|backend|后端|migration|数据库|\bsql\b|handler|接口|service|\bgo\b|grpc|schema)/i,
  gemini:
    /(\bui\b|frontend|前端|\breact\b|\btsx\b|页面|组件|component|样式|\bstyle\b|\bcss\b|tailwind|路由|route)/i,
  'claude-code': /(联调|integrat|orchestrat|\be2e\b|playwright|跨\s*lane|fullstack|端到端)/i,
}

function laneOf(agent: Agent): DelegatableLane {
  return DELEGATABLE_LANES.includes(agent.agentType as DelegatableLane)
    ? (agent.agentType as DelegatableLane)
    : 'claude-code'
}

function detectCrossLane(text: string): DelegatableLane[] {
  const trimmed = text.trim()
  if (!trimmed) return []
  return DELEGATABLE_LANES.filter((lane) => LANE_KEYWORDS[lane].test(trimmed))
}

const schema = z.object({
  title: z.string().min(2, '至少 2 个字符').max(120, '最长 120 个字符'),
  goal: z
    .string()
    .min(4, '目标至少 4 个字符')
    .max(200, '目标最长 200 个字符'),
  description: z.string().max(1000, '最长 1000 个字符').optional(),
  inputs: z.string().max(800, '最长 800 个字符').optional(),
  constraints: z.string().max(400, '最长 400 个字符').optional(),
  outputContract: z
    .string()
    .min(4, '验收标准至少 4 个字符')
    .max(300, '最长 300 个字符'),
  delegateToAgentId: z.string().min(1, '请选择受托 Agent'),
  requiredSkills: z.string().optional(),
  requiredModel: z.string().optional(),
  priority: z
    .number({ message: '请输入优先级' })
    .int()
    .min(0, '不能小于 0')
    .max(200, '最大 200')
    .optional(),
})

type FormValues = z.infer<typeof schema>

interface CreateTaskDialogProps {
  open: boolean
  projectId: string
  agents: Agent[]
  onOpenChange(open: boolean): void
}

export function CreateTaskDialog({ open, onOpenChange, projectId, agents }: CreateTaskDialogProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const createTask = useCreateTaskMutation(projectId)

  // Group agents by lane once per render. `system` agents never appear here,
  // and a lane with zero agents shows up as a disabled card in the UI so
  // users see the absence and know to invite one rather than silently
  // not having the option.
  const agentsByLane = useMemo(() => {
    const buckets: Record<DelegatableLane, Agent[]> = {
      codex: [],
      gemini: [],
      'claude-code': [],
    }
    for (const agent of agents) {
      if (agent.agentType === 'system') continue
      buckets[laneOf(agent)].push(agent)
    }
    return buckets
  }, [agents])

  const initialAgent = useMemo(() => {
    for (const lane of DELEGATABLE_LANES) {
      const first = agentsByLane[lane][0]
      if (first) return first
    }
    return undefined
  }, [agentsByLane])

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      title: '',
      goal: '',
      description: '',
      inputs: '',
      constraints: '',
      outputContract: '',
      delegateToAgentId: initialAgent?.agentId ?? '',
      requiredSkills: '',
      requiredModel: initialAgent?.defaultModel ?? '',
      priority: 50,
    },
  })

  const watchedAgentId = useWatch({ control: form.control, name: 'delegateToAgentId' })
  const watchedTitle = useWatch({ control: form.control, name: 'title' })
  const watchedDescription = useWatch({ control: form.control, name: 'description' })

  const selectedAgent = useMemo(
    () => agents.find((a) => a.agentId === watchedAgentId),
    [agents, watchedAgentId],
  )
  const selectedLane: DelegatableLane | null = selectedAgent ? laneOf(selectedAgent) : null

  // Cross-lane suggestion: if the title+description text mentions things
  // owned by 2+ different lanes, hint at splitting. Stays a hint — submitting
  // is never blocked, because §1.1 explicitly allows orchestrator-owned tasks
  // that legitimately span lanes.
  const crossLaneHits = useMemo(
    () => detectCrossLane(`${watchedTitle ?? ''}\n${watchedDescription ?? ''}`),
    [watchedTitle, watchedDescription],
  )
  const showCrossLaneHint =
    crossLaneHits.length >= 2 &&
    (selectedLane ? !crossLaneHits.includes(selectedLane) || selectedLane !== 'claude-code' : true)

  useEffect(() => {
    if (open) {
      form.reset({
        title: '',
        goal: '',
        description: '',
        inputs: '',
        constraints: '',
        outputContract: '',
        delegateToAgentId: initialAgent?.agentId ?? '',
        requiredSkills: '',
        requiredModel: initialAgent?.defaultModel ?? '',
        priority: 50,
      })
    }
  }, [open, initialAgent, form])

  // When the user picks a different lane via the lane card, sync the model
  // hint to that agent's defaultModel so the form doesn't carry a stale value.
  const pickAgent = (agent: Agent) => {
    form.setValue('delegateToAgentId', agent.agentId, { shouldValidate: true })
    if (agent.defaultModel && !form.getValues('requiredModel')) {
      form.setValue('requiredModel', agent.defaultModel)
    }
  }

  const onSubmit = form.handleSubmit(async (values) => {
    const targetAgent = agents.find((agent) => agent.agentId === values.delegateToAgentId)
    if (!targetAgent) {
      toast({ title: '请选择受托 Agent', tone: 'destructive' })
      return
    }
    try {
      const task = await createTask.mutateAsync({
        title: values.title,
        goal: values.goal,
        description: values.description,
        inputs: values.inputs,
        constraints: values.constraints,
        outputContract: values.outputContract,
        delegateToAgentId: values.delegateToAgentId,
        delegateToAgentType: targetAgent.agentType as AgentType,
        requiredSkills: values.requiredSkills
          ? values.requiredSkills
              .split(/[,，]/)
              .map((skill) => skill.trim())
              .filter(Boolean)
          : undefined,
        requiredModel: values.requiredModel,
        priority: values.priority,
      })
      toast({
        title: '任务已创建',
        description: `${task.taskId} 已委托给 ${t(`agent.type.${targetAgent.agentType}`)}`,
        tone: 'success',
      })
      onOpenChange(false)
    } catch (error) {
      toast({
        title: '创建失败',
        description: error instanceof Error ? error.message : '未知错误',
        tone: 'destructive',
      })
    }
  })

  const skillPlaceholder = selectedLane
    ? t(`agent.lane.${selectedLane}.skillPlaceholder`)
    : 'backend-api, frontend-ui'
  const contractPlaceholder = selectedLane
    ? t(`agent.lane.${selectedLane}.contractPlaceholder`)
    : t('task.field.acceptance.placeholder')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>新建委托任务</DialogTitle>
          <DialogDescription>
            按 lane 选择受托方：每个 lane 拥有明确的代码边界，跨 lane
            的需求建议拆分成两条任务再编排。
          </DialogDescription>
        </DialogHeader>

        <form className="grid gap-4 py-2" onSubmit={onSubmit}>
          <div className="space-y-1">
            <Label htmlFor="task-title">{t('task.field.title.label')}</Label>
            <Input
              id="task-title"
              autoFocus
              placeholder={t('task.field.title.placeholder')}
              {...form.register('title')}
            />
            {form.formState.errors.title ? (
              <p className="text-xs text-rose-600">{form.formState.errors.title.message}</p>
            ) : null}
          </div>

          <div className="space-y-1">
            <Label htmlFor="task-goal">
              {t('task.field.goal.label')} <span className="text-rose-500">*</span>
            </Label>
            <Input
              id="task-goal"
              placeholder={t('task.field.goal.placeholder')}
              {...form.register('goal')}
            />
            <p className="text-[11px] text-[hsl(var(--muted-foreground))]">
              {t('task.field.goal.hint')}
            </p>
            {form.formState.errors.goal ? (
              <p className="text-xs text-rose-600">{form.formState.errors.goal.message}</p>
            ) : null}
          </div>

          <div className="space-y-1">
            <Label htmlFor="task-description">{t('task.field.context.label')}</Label>
            <Textarea
              id="task-description"
              rows={3}
              placeholder={t('task.field.context.placeholder')}
              {...form.register('description')}
            />
            <p className="text-[11px] text-[hsl(var(--muted-foreground))]">
              {t('task.field.context.hint')}
            </p>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="task-inputs">{t('task.field.inputs.label')}</Label>
              <Textarea
                id="task-inputs"
                rows={3}
                placeholder={t('task.field.inputs.placeholder')}
                {...form.register('inputs')}
              />
              <p className="text-[11px] text-[hsl(var(--muted-foreground))]">
                {t('task.field.inputs.hint')}
              </p>
            </div>
            <div className="space-y-1">
              <Label htmlFor="task-constraints">{t('task.field.constraints.label')}</Label>
              <Textarea
                id="task-constraints"
                rows={3}
                placeholder={t('task.field.constraints.placeholder')}
                {...form.register('constraints')}
              />
              <p className="text-[11px] text-[hsl(var(--muted-foreground))]">
                {t('task.field.constraints.hint')}
              </p>
            </div>
          </div>

          <div className="space-y-1">
            <Label htmlFor="task-output">
              {t('task.field.acceptance.label')} <span className="text-rose-500">*</span>
            </Label>
            <Textarea
              id="task-output"
              rows={3}
              placeholder={contractPlaceholder}
              {...form.register('outputContract')}
            />
            <p className="text-[11px] text-[hsl(var(--muted-foreground))]">
              {t('task.field.acceptance.hint')}
            </p>
            {form.formState.errors.outputContract ? (
              <p className="text-xs text-rose-600">
                {form.formState.errors.outputContract.message}
              </p>
            ) : null}
          </div>

          {showCrossLaneHint ? (
            <div className="flex items-start gap-2 rounded-md border border-amber-300/60 bg-amber-50 px-3 py-2 text-xs text-amber-800">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 flex-shrink-0" />
              <div className="space-y-0.5">
                <p className="font-semibold">
                  检测到跨 lane 关键词：{crossLaneHits.map((l) => t(`agent.type.${l}`)).join(' + ')}
                </p>
                <p className="text-amber-700">
                  建议把它拆成两条子任务（一条给 Codex 后端，一条给 Gemini 前端），再用 Claude Code
                  编排端到端联调。这只是提示，不阻断提交。
                </p>
              </div>
            </div>
          ) : null}

          <div className="space-y-2">
            <div className="flex items-baseline justify-between">
              <Label>委托给 Lane</Label>
              {form.formState.errors.delegateToAgentId ? (
                <p className="text-xs text-rose-600">
                  {form.formState.errors.delegateToAgentId.message}
                </p>
              ) : null}
            </div>
            <div className="grid gap-2">
              {DELEGATABLE_LANES.map((lane) => (
                <LaneCard
                  key={lane}
                  lane={lane}
                  agents={agentsByLane[lane]}
                  selectedAgentId={watchedAgentId}
                  onPickAgent={pickAgent}
                />
              ))}
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="task-priority">优先级</Label>
              <Input
                id="task-priority"
                type="number"
                {...form.register('priority', { valueAsNumber: true })}
                placeholder="0–200"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="task-model">指定模型</Label>
              <Input
                id="task-model"
                placeholder="gpt-5-codex"
                {...form.register('requiredModel')}
              />
            </div>
          </div>

          <div className="space-y-1">
            <Label htmlFor="task-skills">所需 Skill（逗号分隔）</Label>
            <Input
              id="task-skills"
              placeholder={skillPlaceholder}
              {...form.register('requiredSkills')}
            />
          </div>

          <DialogFooter className="pt-2">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={createTask.isPending}>
              {createTask.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              发布任务
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

interface LaneCardProps {
  lane: DelegatableLane
  agents: Agent[]
  selectedAgentId: string
  onPickAgent: (agent: Agent) => void
}

// LaneCard renders a single delegation lane:
//   - clickable when at least one agent is available (selecting picks the
//     first agent in the lane)
//   - shows an inline mini-select when the lane has multiple agents so users
//     can disambiguate without leaving the dialog
//   - dimmed and explained when empty, so missing-agent gaps are visible
//     instead of silently disabled
function LaneCard({ lane, agents, selectedAgentId, onPickAgent }: LaneCardProps) {
  const { t } = useTranslation()
  const Icon = LANE_ICON[lane]
  const empty = agents.length === 0
  const selectedHere = agents.find((a) => a.agentId === selectedAgentId) ?? null
  const active = Boolean(selectedHere)

  const onCardClick = () => {
    if (empty || active) return
    onPickAgent(agents[0])
  }

  return (
    <div
      role={empty ? undefined : 'button'}
      tabIndex={empty ? -1 : 0}
      onClick={onCardClick}
      onKeyDown={(event) => {
        if (empty) return
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          onCardClick()
        }
      }}
      className={cn(
        'rounded-md border bg-[hsl(var(--card))] px-3 py-2 transition-colors',
        active
          ? 'border-[hsl(var(--primary))] ring-1 ring-[hsl(var(--primary))]'
          : 'border-[hsl(var(--border))] hover:border-[hsl(var(--primary))]/60',
        empty ? 'cursor-not-allowed opacity-60' : 'cursor-pointer',
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-2">
          <span
            className={cn(
              'mt-0.5 flex h-7 w-7 items-center justify-center rounded-md',
              active
                ? 'bg-[hsl(var(--primary))] text-[hsl(var(--primary-foreground))]'
                : 'bg-[hsl(var(--muted))] text-[hsl(var(--muted-foreground))]',
            )}
          >
            <Icon className="h-3.5 w-3.5" />
          </span>
          <div className="space-y-0.5">
            <div className="flex items-center gap-2 text-sm">
              <span className="font-semibold">{t(`agent.lane.${lane}.title`)}</span>
              <span className="text-xs text-[hsl(var(--muted-foreground))]">
                · {t(`agent.type.${lane}`)}
              </span>
            </div>
            <p className="text-xs text-[hsl(var(--muted-foreground))]">
              {t(`agent.lane.${lane}.owns`)}
            </p>
            {t(`agent.lane.${lane}.avoids`) ? (
              <p className="text-[11px] text-[hsl(var(--muted-foreground))]/80">
                ⚠ {t(`agent.lane.${lane}.avoids`)}
              </p>
            ) : null}
          </div>
        </div>
        <div className="flex-shrink-0">
          {empty ? (
            <span className="text-[11px] text-[hsl(var(--muted-foreground))]">
              暂无 Agent，前往 /agents 邀请
            </span>
          ) : agents.length === 1 ? (
            <span className="font-mono text-[11px] text-[hsl(var(--muted-foreground))]">
              {agents[0].name}
            </span>
          ) : (
            <Select
              value={selectedHere?.agentId ?? ''}
              onValueChange={(value) => {
                const next = agents.find((a) => a.agentId === value)
                if (next) onPickAgent(next)
              }}
            >
              <SelectTrigger
                className="h-7 w-[160px] text-xs"
                onClick={(event) => event.stopPropagation()}
              >
                <SelectValue placeholder="选择 Agent" />
              </SelectTrigger>
              <SelectContent>
                {agents.map((agent) => (
                  <SelectItem key={agent.agentId} value={agent.agentId}>
                    {agent.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </div>
      </div>
    </div>
  )
}
