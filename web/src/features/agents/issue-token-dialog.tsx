import * as React from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Loader2, KeyRound, Copy } from 'lucide-react'
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
import { useToast } from '@/components/ui/use-toast'
import { useIssueAgentTokenMutation } from '@/api/agents'
import { ApiError, describeError } from '@/api/errors'
import { formatRelativeTime, shortenUlid } from '@/lib/format'
import type { Agent, IssueTokenResponse } from '@/api/types'

const KNOWN_SCOPES = [
  'task:read:own',
  'task:read:tree',
  'task:read:delegated',
  'task:create',
  'task:start:delegated',
  'task:update:delegated',
  'task:submit:delegated',
  'task:delegate:codex',
  'task:delegate:gemini',
  'task:review',
  'room:read',
  'room:write',
  'room:mention',
  'room:pin',
  'room:summarize',
  'room:history',
  'memory:read',
  'memory:search',
  'memory:write:summary',
  'memory:write:decision',
] as const

type ScopeGroup = { name: 'task' | 'room' | 'memory' | 'other'; label: string; scopes: string[] }

function buildScopeGroups(currentScopes: string[]): ScopeGroup[] {
  const universe = Array.from(new Set<string>([...KNOWN_SCOPES, ...currentScopes])).sort()
  const groups: ScopeGroup[] = [
    { name: 'task', label: 'Task', scopes: [] },
    { name: 'room', label: 'Room', scopes: [] },
    { name: 'memory', label: 'Memory', scopes: [] },
    { name: 'other', label: '其他', scopes: [] },
  ]
  for (const scope of universe) {
    const prefix = scope.split(':')[0]
    const group = groups.find((g) => g.name === prefix) ?? groups[groups.length - 1]
    group.scopes.push(scope)
  }
  return groups.filter((g) => g.scopes.length > 0)
}

function dateNDaysFromNow(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() + days)
  return d.toISOString().slice(0, 10)
}

interface IssueTokenDialogProps {
  open: boolean
  agent: Agent
  onClose: () => void
}

export function IssueTokenDialog({ open, agent, onClose }: IssueTokenDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-lg">
        {open ? <IssueTokenDialogBody key={agent.agentId} agent={agent} onClose={onClose} /> : null}
      </DialogContent>
    </Dialog>
  )
}

interface IssueTokenDialogBodyProps {
  agent: Agent
  onClose: () => void
}

function IssueTokenDialogBody({ agent, onClose }: IssueTokenDialogBodyProps) {
  const { toast } = useToast()
  const mutation = useIssueAgentTokenMutation(agent.agentId)

  const [minDate] = React.useState(() => dateNDaysFromNow(1))
  const [maxDate] = React.useState(() => dateNDaysFromNow(180))
  const [defaultDate] = React.useState(() => dateNDaysFromNow(30))

  const schema = React.useMemo(
    () =>
      z.object({
        expiresAt: z
          .string()
          .min(1, '请选择过期时间')
          .refine((v) => v >= minDate, `过期时间必须不早于 ${minDate}`)
          .refine((v) => v <= maxDate, `过期时间必须不晚于 ${maxDate}`),
      }),
    [minDate, maxDate],
  )
  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { expiresAt: defaultDate },
  })

  const [scopes, setScopes] = React.useState<string[]>(agent.scopes ?? [])
  const [scopesError, setScopesError] = React.useState<string | null>(null)
  const [neverExpires, setNeverExpires] = React.useState(false)
  const [view, setView] = React.useState<'form' | 'success'>('form')
  const [result, setResult] = React.useState<IssueTokenResponse | null>(null)
  const [submitError, setSubmitError] = React.useState<string | null>(null)

  const scopeGroups = React.useMemo(() => buildScopeGroups(agent.scopes ?? []), [agent.scopes])

  const toggleScope = (scope: string) => {
    setScopes((prev) => {
      const set = new Set(prev)
      if (set.has(scope)) set.delete(scope)
      else set.add(scope)
      const next = Array.from(set)
      if (next.length > 0) setScopesError(null)
      return next
    })
  }

  const handleSubmit = form.handleSubmit(async (values) => {
    setSubmitError(null)
    if (scopes.length === 0) {
      setScopesError('至少选择 1 个 scope')
      return
    }
    try {
      const expiresAtIso = neverExpires
        ? null
        : new Date(`${values.expiresAt}T23:59:59Z`).toISOString()
      const res = await mutation.mutateAsync({
        expiresAt: expiresAtIso,
        scopes,
      })
      setResult(res)
      setView('success')
    } catch (err) {
      if (err instanceof ApiError) {
        const desc = describeError(err.envelope)
        setSubmitError(desc.hint ? `${desc.title} — ${desc.hint}` : desc.title)
      } else {
        setSubmitError('颁发失败,请重试。')
      }
    }
  })

  const handleCopy = async () => {
    if (!result) return
    try {
      await navigator.clipboard.writeText(result.agentToken)
      toast({
        title: '已复制',
        description: '请立即保存到 Agent 进程的 keychain 或 aitask auth token import --token。',
        tone: 'success',
      })
    } catch {
      toast({
        title: '复制失败',
        description: '请手动选中输入框文本复制。',
        tone: 'destructive',
      })
    }
  }

  const close = () => {
    if (mutation.isPending) return
    onClose()
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2 text-emerald-700">
          <KeyRound className="h-4 w-4" />
          {view === 'form' ? '颁发 Agent Token' : 'Token 已生成'}
        </DialogTitle>
        <DialogDescription>
          {view === 'form'
            ? '颁发后明文 token 仅在此弹窗内展示一次,关闭后无法再次查看。'
            : '此 token 仅展示一次,关闭后无法再次查看,请立即复制保存。'}
        </DialogDescription>
      </DialogHeader>

      {view === 'form' ? (
        <form onSubmit={handleSubmit} className="space-y-4">
          {submitError ? (
            <div className="rounded-md border border-rose-200 bg-rose-50/60 px-3 py-2 text-xs text-rose-700">
              {submitError}
            </div>
          ) : null}

          <div className="rounded-md border border-emerald-200 bg-emerald-50/60 px-3 py-2 text-xs text-emerald-800">
            对象 Agent: <span className="font-semibold">{agent.name}</span>
            <br />
            Type: <span className="font-mono">{agent.agentType}</span> · Role:{' '}
            <span className="font-mono">{agent.role}</span>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="issue-expires">过期时间</Label>
            <label className="flex cursor-pointer items-center gap-2 text-[11px] text-slate-700">
              <input
                type="checkbox"
                className="h-3.5 w-3.5 cursor-pointer rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
                checked={neverExpires}
                onChange={(e) => setNeverExpires(e.target.checked)}
              />
              <span>永不过期(谨慎使用,长期有效不会自动失效)</span>
            </label>
            <Input
              id="issue-expires"
              type="date"
              min={minDate}
              max={maxDate}
              disabled={neverExpires}
              {...form.register('expiresAt')}
            />
            <p className="text-[11px] text-slate-500">
              {neverExpires ? '已选择永不过期,日期字段将被忽略。' : '范围 [明日, +180 天]'}
            </p>
            {!neverExpires && form.formState.errors.expiresAt ? (
              <p className="text-[11px] text-rose-600">{form.formState.errors.expiresAt.message}</p>
            ) : null}
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>Scopes ({scopes.length} 项已选)</Label>
              <div className="flex items-center gap-2 text-[11px] text-slate-500">
                <button
                  type="button"
                  className="underline-offset-2 hover:underline"
                  onClick={() => {
                    setScopes(scopeGroups.flatMap((g) => g.scopes))
                    setScopesError(null)
                  }}
                >
                  全选
                </button>
                <span className="text-slate-300">·</span>
                <button
                  type="button"
                  className="underline-offset-2 hover:underline"
                  onClick={() => {
                    setScopes([])
                    setScopesError('至少选择 1 个 scope')
                  }}
                >
                  全不选
                </button>
              </div>
            </div>
            <div className="max-h-64 space-y-3 overflow-y-auto rounded-md border border-slate-200 bg-slate-50/40 p-3">
              {scopeGroups.map((group) => (
                <div key={group.name} className="space-y-1">
                  <p className="text-[11px] font-semibold uppercase tracking-wide text-slate-500">
                    {group.label}
                  </p>
                  <div className="space-y-0.5">
                    {group.scopes.map((scope) => {
                      const id = `scope-${scope.replace(/:/g, '_')}`
                      const checked = scopes.includes(scope)
                      return (
                        <label
                          key={scope}
                          htmlFor={id}
                          className="flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 text-[11px] hover:bg-slate-100"
                        >
                          <input
                            id={id}
                            type="checkbox"
                            className="h-3.5 w-3.5 cursor-pointer rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
                            checked={checked}
                            onChange={() => toggleScope(scope)}
                          />
                          <span className="font-mono text-slate-700">{scope}</span>
                        </label>
                      )
                    })}
                  </div>
                </div>
              ))}
            </div>
            {scopesError ? <p className="text-[11px] text-rose-600">{scopesError}</p> : null}
          </div>

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={close}>
              取消
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin" /> 颁发中
                </>
              ) : (
                '颁发 Token'
              )}
            </Button>
          </DialogFooter>
        </form>
      ) : result ? (
        <div className="space-y-4">
          <div className="rounded-md border border-amber-200 bg-amber-50/60 px-3 py-2 text-xs text-amber-800">
            此 token 仅展示一次,关闭后无法再次查看,请立即复制保存。
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="issue-token-plain">Agent Token (明文)</Label>
            <div className="flex items-center gap-2">
              <Input
                id="issue-token-plain"
                readOnly
                value={result.agentToken}
                className="font-mono text-[11px]"
                onFocus={(e) => e.currentTarget.select()}
              />
              <Button type="button" variant="outline" size="sm" onClick={handleCopy}>
                <Copy className="h-3.5 w-3.5" /> 复制
              </Button>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-2 text-[11px] text-slate-600">
            <div>
              <p className="text-slate-500">Token ID</p>
              <p className="font-mono">{shortenUlid(result.tokenId)}</p>
            </div>
            <div>
              <p className="text-slate-500">过期</p>
              <p>{result.expiresAt ? formatRelativeTime(result.expiresAt) : '永不过期'}</p>
            </div>
            <div>
              <p className="text-slate-500">Scope</p>
              <p>{scopes.length} 项</p>
            </div>
          </div>

          <DialogFooter>
            <Button type="button" onClick={close}>
              我已复制,关闭
            </Button>
          </DialogFooter>
        </div>
      ) : null}
    </>
  )
}
