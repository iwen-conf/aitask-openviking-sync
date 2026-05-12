import * as React from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Loader2, Save, AlertTriangle, Archive, CheckCircle2 } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useToast } from '@/components/ui/use-toast'
import { EmptyState } from '@/components/shared/empty-state'
import { Badge } from '@/components/ui/badge'
import { ApiError, describeError } from '@/api/errors'
import {
  useArchiveProjectMutation,
  useCompleteProjectMutation,
  useProjectQuery,
  useUpdateProjectMutation,
} from '@/api/projects'
import { useProjectOutletContext } from './use-project-context'
import type { CompletionPolicy, CompletionPolicyResultItem } from '@/api/types'
import { formatRelativeTime } from '@/lib/format'

const POLICY_SCHEMA = z.object({
  requiredTasks: z.enum(['all_required_done', 'optional']),
  blockedTasks: z.enum(['none', 'allow']),
  failedTasks: z.enum(['none', 'allow']),
  reviewPolicy: z.enum(['all_submitted_reviewed', 'optional']),
})

const SCHEMA = z.object({
  name: z.string().min(2, '至少 2 个字符').max(80, '最长 80 个字符'),
  goal: z.string().min(4, '请描述项目目标').max(200, '最长 200 个字符'),
  description: z.string().max(500, '最长 500 个字符').optional(),
  completionPolicy: POLICY_SCHEMA,
  openvikingNamespace: z.string().max(64, '最长 64 个字符').optional(),
  openvikingWorkspaceId: z.string().max(64, '最长 64 个字符').optional(),
})

type FormValues = z.infer<typeof SCHEMA>

const POLICY_FIELDS: {
  key: keyof CompletionPolicy
  label: string
  hint: string
  options: { value: string; label: string }[]
}[] = [
  {
    key: 'requiredTasks',
    label: '必需任务',
    hint: '决定 required 任务未完成时是否阻止 complete',
    options: [
      { value: 'all_required_done', label: '全部完成才能验收' },
      { value: 'optional', label: '允许有未完成项' },
    ],
  },
  {
    key: 'blockedTasks',
    label: '阻塞任务',
    hint: '决定 blocked 任务存在时是否阻止 complete',
    options: [
      { value: 'none', label: '不允许有阻塞任务' },
      { value: 'allow', label: '允许有阻塞任务' },
    ],
  },
  {
    key: 'failedTasks',
    label: '失败任务',
    hint: '决定 failed 任务存在时是否阻止 complete',
    options: [
      { value: 'none', label: '不允许有失败任务' },
      { value: 'allow', label: '允许有失败任务' },
    ],
  },
  {
    key: 'reviewPolicy',
    label: 'Review 策略',
    hint: 'submit/reviewing 状态是否必须全部通过 review',
    options: [
      { value: 'all_submitted_reviewed', label: '全部提交均需审查通过' },
      { value: 'optional', label: '不强制 review' },
    ],
  },
]

export function SettingsRoute() {
  const { projectId } = useProjectOutletContext()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const projectQuery = useProjectQuery(projectId)
  const updateMutation = useUpdateProjectMutation(projectId)
  const completeMutation = useCompleteProjectMutation(projectId)
  const archiveMutation = useArchiveProjectMutation(projectId)
  const { toast } = useToast()

  const [policyResult, setPolicyResult] = React.useState<CompletionPolicyResultItem[] | null>(null)
  const [completeOpen, setCompleteOpen] = React.useState(false)
  const [archiveOpen, setArchiveOpen] = React.useState(false)
  const [archiveReason, setArchiveReason] = React.useState('release finished')

  const form = useForm<FormValues>({
    resolver: zodResolver(SCHEMA),
    values: projectQuery.data
      ? {
          name: projectQuery.data.name,
          goal: projectQuery.data.goal,
          description: projectQuery.data.description ?? '',
          completionPolicy: {
            requiredTasks: projectQuery.data.completionPolicy.requiredTasks,
            blockedTasks: projectQuery.data.completionPolicy.blockedTasks,
            failedTasks: projectQuery.data.completionPolicy.failedTasks,
            reviewPolicy: projectQuery.data.completionPolicy.reviewPolicy ?? 'optional',
          },
          openvikingNamespace: projectQuery.data.openvikingNamespace ?? '',
          openvikingWorkspaceId: projectQuery.data.openvikingWorkspaceId ?? '',
        }
      : undefined,
  })

  const project = projectQuery.data

  if (projectQuery.isLoading || !project) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-slate-500">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" /> 加载项目设置…
      </div>
    )
  }

  const archived = project.status === 'archived'
  const completed = project.status === 'completed'

  const handleSave = form.handleSubmit(async (values) => {
    try {
      await updateMutation.mutateAsync({
        name: values.name,
        goal: values.goal,
        description: values.description?.trim() || undefined,
        completionPolicy: values.completionPolicy,
        openvikingNamespace: values.openvikingNamespace?.trim() || undefined,
        openvikingWorkspaceId: values.openvikingWorkspaceId?.trim() || undefined,
      })
      toast({ title: '项目设置已保存', tone: 'success' })
    } catch (error) {
      if (error instanceof ApiError) {
        const description = describeError(error.envelope)
        toast({ title: description.title, description: description.hint, tone: 'destructive' })
      } else {
        toast({ title: '保存失败', tone: 'destructive' })
      }
    }
  })

  const handleComplete = async () => {
    try {
      const response = await completeMutation.mutateAsync()
      if (response.policyResult.passed) {
        toast({ title: '项目已标记完成', tone: 'success' })
        setCompleteOpen(false)
        setPolicyResult(null)
      } else {
        setPolicyResult(response.policyResult.failedItems)
        toast({
          title: '验收未通过',
          description: '请在下方查看未达成项',
          tone: 'destructive',
        })
      }
    } catch (error) {
      if (error instanceof ApiError) {
        const description = describeError(error.envelope)
        toast({ title: description.title, description: description.hint, tone: 'destructive' })
      } else {
        toast({ title: '完成项目失败', tone: 'destructive' })
      }
    }
  }

  const handleArchive = async () => {
    try {
      await archiveMutation.mutateAsync(archiveReason.trim() || 'release finished')
      toast({ title: '项目已归档', tone: 'success' })
      setArchiveOpen(false)
      navigate('/projects')
    } catch (error) {
      if (error instanceof ApiError) {
        const description = describeError(error.envelope)
        toast({ title: description.title, description: description.hint, tone: 'destructive' })
      } else {
        toast({ title: '归档失败', tone: 'destructive' })
      }
    }
  }

  if (archived) {
    return (
      <div className="h-full p-6">
        <EmptyState
          icon={<Archive className="h-8 w-8" />}
          title="项目已归档"
          description={`归档于 ${formatRelativeTime(project.updatedAt)},仅可只读浏览。`}
        />
      </div>
    )
  }

  return (
    <div className="h-full overflow-y-auto bg-[hsl(var(--muted)/0.4)]">
      <div className="mx-auto grid max-w-4xl gap-4 p-6">
        <form className="grid gap-4" onSubmit={handleSave}>
          <Card>
            <CardContent className="space-y-4 p-5">
              <header>
                <h3 className="text-base font-semibold text-slate-900">基本信息</h3>
                <p className="text-xs text-slate-500">
                  更新后立即生效;goal/name 同步到 Topbar 与项目列表。
                </p>
              </header>

              <div className="grid gap-3 md:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="settings-name">项目名称</Label>
                  <Input id="settings-name" {...form.register('name')} />
                  {form.formState.errors.name ? (
                    <p className="text-[11px] text-rose-600">
                      {form.formState.errors.name.message}
                    </p>
                  ) : null}
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="settings-goal">目标 (Goal)</Label>
                  <Input id="settings-goal" {...form.register('goal')} />
                  {form.formState.errors.goal ? (
                    <p className="text-[11px] text-rose-600">
                      {form.formState.errors.goal.message}
                    </p>
                  ) : null}
                </div>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="settings-desc">描述 (可选)</Label>
                <Textarea
                  id="settings-desc"
                  rows={3}
                  {...form.register('description')}
                  placeholder="补充背景、约束、关联资源链接"
                />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-4 p-5">
              <header>
                <h3 className="text-base font-semibold text-slate-900">Completion Policy</h3>
                <p className="text-xs text-slate-500">
                  决定 <code className="rounded bg-slate-100 px-1 text-[11px]">/complete</code>{' '}
                  的验收口径,任何修改都会在下次执行时生效。
                </p>
              </header>

              <div className="grid gap-3 md:grid-cols-2">
                {POLICY_FIELDS.map((field) => (
                  <PolicyField
                    key={field.key}
                    control={form.control}
                    field={field}
                    errorMessage={
                      (
                        form.formState.errors.completionPolicy as
                          | Record<string, { message?: string } | undefined>
                          | undefined
                      )?.[field.key]?.message
                    }
                    onChange={(value) =>
                      form.setValue(
                        `completionPolicy.${field.key}`,
                        value as FormValues['completionPolicy'][typeof field.key],
                        { shouldDirty: true },
                      )
                    }
                  />
                ))}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-4 p-5">
              <header>
                <h3 className="text-base font-semibold text-slate-900">
                  {t('settings.openviking.project.title', { defaultValue: 'OpenViking 索引' })}
                </h3>
                <p className="text-xs text-slate-500">
                  {t('settings.openviking.project.description', {
                    defaultValue:
                      '命名空间与工作区 ID 决定本项目在 OpenViking 中的索引坐标；服务地址与 API Key 在系统设置。',
                  })}
                </p>
              </header>

              <div className="grid gap-3 md:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="settings-ov-namespace">
                    {t('settings.openviking.project.namespace', { defaultValue: 'Namespace' })}
                  </Label>
                  <Input id="settings-ov-namespace" {...form.register('openvikingNamespace')} />
                  {form.formState.errors.openvikingNamespace ? (
                    <p className="text-[11px] text-rose-600">
                      {form.formState.errors.openvikingNamespace.message}
                    </p>
                  ) : null}
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="settings-ov-workspace">
                    {t('settings.openviking.project.workspaceId', { defaultValue: 'Workspace ID' })}
                  </Label>
                  <Input id="settings-ov-workspace" {...form.register('openvikingWorkspaceId')} />
                  {form.formState.errors.openvikingWorkspaceId ? (
                    <p className="text-[11px] text-rose-600">
                      {form.formState.errors.openvikingWorkspaceId.message}
                    </p>
                  ) : null}
                </div>
              </div>
            </CardContent>
          </Card>

          <div className="flex items-center justify-end gap-2">
            <Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Save className="h-4 w-4" />
              )}
              保存设置
            </Button>
          </div>
        </form>

        <Card>
          <CardContent className="space-y-4 p-5">
            <header className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <h3 className="text-base font-semibold text-slate-900">验收 / 归档</h3>
                <p className="text-xs text-slate-500">完成项目会按当前 policy 校验,归档后只读。</p>
              </div>
              <Badge tone="info">当前状态：{t(`project.status.${project.status}`)}</Badge>
            </header>

            {policyResult && policyResult.length > 0 ? (
              <div className="rounded-md border border-rose-200 bg-rose-50 p-3 text-xs text-rose-700">
                <p className="flex items-center gap-1 font-semibold">
                  <AlertTriangle className="h-3.5 w-3.5" /> Policy 未通过 ({policyResult.length} 项)
                </p>
                <ul className="mt-2 space-y-1">
                  {policyResult.map((item) => (
                    <li
                      key={`${item.code}-${item.taskId ?? ''}`}
                      className="flex items-start gap-2"
                    >
                      <span className="mt-1 inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-rose-500" />
                      <span>
                        <span className="font-semibold">{item.code}</span> · {item.message}
                        {item.taskId ? (
                          <button
                            type="button"
                            onClick={() => navigate(`../tasks?taskId=${item.taskId}`)}
                            className="ml-2 text-indigo-600 hover:underline"
                          >
                            查看任务
                          </button>
                        ) : null}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}

            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setCompleteOpen(true)}
                disabled={completed}
              >
                <CheckCircle2 className="h-4 w-4" />
                {completed ? '已完成' : '触发 complete 验收'}
              </Button>
              <Button type="button" variant="destructive" onClick={() => setArchiveOpen(true)}>
                <Archive className="h-4 w-4" /> 归档项目
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>

      <Dialog open={completeOpen} onOpenChange={setCompleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>完成项目验收?</DialogTitle>
            <DialogDescription>
              将按当前 Completion Policy 校验所有任务。如未通过会列出未达成项,不会强制改变状态。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setCompleteOpen(false)}>
              取消
            </Button>
            <Button onClick={handleComplete} disabled={completeMutation.isPending}>
              {completeMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              确认验收
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={archiveOpen} onOpenChange={setArchiveOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-rose-700">
              <AlertTriangle className="h-4 w-4" /> 归档项目?
            </DialogTitle>
            <DialogDescription>
              归档后项目变为只读,无法继续创建任务、发送消息或修改设置。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-1.5">
            <Label htmlFor="archive-reason">归档原因</Label>
            <Input
              id="archive-reason"
              value={archiveReason}
              onChange={(event) => setArchiveReason(event.target.value)}
            />
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setArchiveOpen(false)}>
              取消
            </Button>
            <Button
              variant="destructive"
              onClick={handleArchive}
              disabled={archiveMutation.isPending}
            >
              {archiveMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              确认归档
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

interface PolicyFieldProps {
  control: ReturnType<typeof useForm<FormValues>>['control']
  field: (typeof POLICY_FIELDS)[number]
  errorMessage?: string
  onChange: (value: string) => void
}

function PolicyField({ control, field, errorMessage, onChange }: PolicyFieldProps) {
  const value = useWatch({ control, name: `completionPolicy.${field.key}` }) ?? ''
  return (
    <div className="space-y-1.5">
      <Label>{field.label}</Label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger>
          <SelectValue placeholder="请选择" />
        </SelectTrigger>
        <SelectContent>
          {field.options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-[11px] text-slate-500">{field.hint}</p>
      {errorMessage ? <p className="text-[11px] text-rose-600">{errorMessage}</p> : null}
    </div>
  )
}
