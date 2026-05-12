import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Users, RefreshCcw, Loader2, Bot, ShieldX, Activity, Plus, UserPlus } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/shared/empty-state'
import { AgentAvatar } from '@/components/shared/agent-avatar'
import { UlidChip } from '@/components/shared/ulid-chip'
import { useAgentsQuery } from '@/api/agents'
import { ApiError, describeError } from '@/api/errors'
import { formatRelativeTime } from '@/lib/format'
import { RevokeTokenDialog } from '@/features/agents/revoke-token-dialog'
import { IssueTokenDialog } from '@/features/agents/issue-token-dialog'
import { CreateAgentDialog } from '@/features/agents/create-agent-dialog'
import type { Agent, AgentType } from '@/api/types'

interface AgentsRouteProps {
  filterType?: Extract<AgentType, 'claude-code' | 'codex' | 'gemini'>
}

export function AgentsRoute({ filterType }: AgentsRouteProps = {}) {
  const { t } = useTranslation()
  const query = useAgentsQuery()
  const items = React.useMemo(() => {
    const list = query.data?.items ?? []
    return filterType ? list.filter((agent) => agent.agentType === filterType) : list
  }, [query.data, filterType])
  const [selectedAgentId, setSelectedAgentId] = React.useState<string | undefined>()
  const [revokingTokenId, setRevokingTokenId] = React.useState<string | undefined>()
  const [issueOpen, setIssueOpen] = React.useState(false)
  const [createOpen, setCreateOpen] = React.useState(false)

  const selectedAgent = items.find((agent) => agent.agentId === selectedAgentId) ?? items[0]

  return (
    <div className="grid h-full grid-cols-[360px_1fr] divide-x divide-slate-200 bg-white">
      <aside className="flex h-full flex-col overflow-hidden bg-slate-50/40">
        <header className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
          <div className="flex items-center gap-2 text-sm font-semibold text-slate-700">
            <Users className="h-4 w-4 text-slate-500" />
            {filterType ? t(`agent.type.${filterType}`) : 'Agents'}
          </div>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setCreateOpen(true)}
              title="新建 Agent"
            >
              <UserPlus className="h-3.5 w-3.5" />
            </Button>
            <Button variant="ghost" size="sm" onClick={() => query.refetch()} title="刷新">
              <RefreshCcw className="h-3.5 w-3.5" />
            </Button>
          </div>
        </header>

        <div className="flex-1 overflow-y-auto px-2 py-3">
          {query.isLoading ? (
            <div className="flex h-40 items-center justify-center text-sm text-slate-500">
              <Loader2 className="mr-2 h-4 w-4 animate-spin" /> 加载中…
            </div>
          ) : query.error instanceof ApiError ? (
            <p className="px-3 text-xs text-rose-600">
              {describeError(query.error.envelope).title}
            </p>
          ) : items.length === 0 ? (
            <div className="space-y-2 px-3 py-2">
              <p className="text-xs text-slate-500">暂未注册 Agent。</p>
              <Button size="sm" variant="outline" onClick={() => setCreateOpen(true)}>
                <UserPlus className="h-3.5 w-3.5" /> 新建 Agent
              </Button>
            </div>
          ) : (
            items.map((agent) => {
              const active = selectedAgent?.agentId === agent.agentId
              return (
                <button
                  key={agent.agentId}
                  type="button"
                  onClick={() => setSelectedAgentId(agent.agentId)}
                  className={
                    'mb-1 flex w-full items-center gap-3 rounded-md border px-3 py-2 text-left transition-colors ' +
                    (active
                      ? 'border-indigo-200 bg-indigo-50/60'
                      : 'border-transparent hover:bg-slate-100')
                  }
                >
                  <AgentAvatar agentType={agent.agentType} size="sm" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-semibold text-slate-800">{agent.name}</p>
                    <p className="text-[11px] text-slate-500">
                      {t(`agent.type.${agent.agentType}`)} · {agent.role}
                    </p>
                  </div>
                  <StatusDot status={agent.status} />
                </button>
              )
            })
          )}
        </div>
      </aside>

      <section className="flex h-full min-h-0 flex-col">
        {selectedAgent ? (
          <AgentDetail
            agent={selectedAgent}
            onRevoke={(tokenId) => setRevokingTokenId(tokenId)}
            onIssue={() => setIssueOpen(true)}
          />
        ) : (
          <div className="flex h-full items-center justify-center p-6">
            <EmptyState
              icon={<Bot className="h-8 w-8" />}
              title="选择左侧 Agent 查看详情"
              description="详情包含 skills / models / scopes / 绑定项目 / Token 状态。"
            />
          </div>
        )}
      </section>

      {selectedAgent ? (
        <>
          <RevokeTokenDialog
            agentId={selectedAgent.agentId}
            agentName={selectedAgent.name}
            tokenId={revokingTokenId}
            onClose={() => setRevokingTokenId(undefined)}
          />
          <IssueTokenDialog
            open={issueOpen}
            agent={selectedAgent}
            onClose={() => setIssueOpen(false)}
          />
        </>
      ) : null}

      <CreateAgentDialog open={createOpen} onClose={() => setCreateOpen(false)} />
    </div>
  )
}

function StatusDot({ status }: { status: Agent['status'] }) {
  const isActive = status === 'active'
  return (
    <span
      className={
        'inline-flex h-2 w-2 shrink-0 rounded-full ' +
        (isActive ? 'bg-emerald-500 shadow-[0_0_0_3px_rgba(16,185,129,0.18)]' : 'bg-slate-300')
      }
    />
  )
}

interface AgentDetailProps {
  agent: Agent
  onRevoke: (tokenId: string) => void
  onIssue: () => void
}

function AgentDetail({ agent, onRevoke, onIssue }: AgentDetailProps) {
  const { t } = useTranslation()
  const tokens = agent.tokens ?? []
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="border-b border-slate-200 bg-white px-6 py-4">
        <div className="flex items-start gap-4">
          <AgentAvatar agentType={agent.agentType} />
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <h2 className="text-lg font-bold text-slate-900">{agent.name}</h2>
              <Badge tone={agent.status === 'active' ? 'success' : 'muted'}>
                {agent.status === 'active' ? '在线' : '离线'}
              </Badge>
            </div>
            <p className="text-sm text-slate-500">
              {t(`agent.type.${agent.agentType}`)} · {agent.role}
              {agent.defaultModel ? ` · 默认 ${agent.defaultModel}` : ''}
            </p>
            <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500">
              <UlidChip id={agent.agentId} />
              {agent.lastSeenAt ? (
                <span className="flex items-center gap-1">
                  <Activity className="h-3 w-3" /> {formatRelativeTime(agent.lastSeenAt)}
                </span>
              ) : null}
            </div>
          </div>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto bg-[hsl(var(--muted)/0.4)] p-6">
        <div className="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardContent className="space-y-2 p-5">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                Skills
              </h3>
              {agent.skills && agent.skills.length > 0 ? (
                <div className="flex flex-wrap gap-1.5">
                  {agent.skills.map((skill) => (
                    <Badge key={skill} tone="info">
                      {skill}
                    </Badge>
                  ))}
                </div>
              ) : (
                <p className="text-xs text-slate-500">未声明 skill。</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-2 p-5">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                Models
              </h3>
              {agent.models && agent.models.length > 0 ? (
                <div className="flex flex-wrap gap-1.5">
                  {agent.models.map((model) => (
                    <Badge key={model} tone="muted">
                      {model}
                    </Badge>
                  ))}
                </div>
              ) : (
                <p className="text-xs text-slate-500">未配置 model。</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-2 p-5">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                Scopes
              </h3>
              <ul className="space-y-1 font-mono text-[11px] text-slate-600">
                {agent.scopes.length === 0 ? (
                  <li className="text-slate-400">未授权任何 scope。</li>
                ) : (
                  agent.scopes.map((scope) => (
                    <li key={scope} className="rounded bg-slate-100 px-2 py-0.5">
                      {scope}
                    </li>
                  ))
                )}
              </ul>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-2 p-5">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                绑定项目
              </h3>
              {agent.boundProjects && agent.boundProjects.length > 0 ? (
                <ul className="space-y-1">
                  {agent.boundProjects.map((projectId) => (
                    <li key={projectId}>
                      <UlidChip id={projectId} />
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-xs text-slate-500">尚未绑定任何项目。</p>
              )}
            </CardContent>
          </Card>

          <Card className="lg:col-span-2">
            <CardContent className="space-y-3 p-5">
              <div className="flex items-center justify-between">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                  Token 状态 (明文不展示)
                </h3>
                <div className="flex items-center gap-2">
                  <Badge tone="muted">{tokens.length} 条</Badge>
                  <Button size="sm" variant="outline" onClick={onIssue}>
                    <Plus className="h-3.5 w-3.5" /> 颁发
                  </Button>
                </div>
              </div>
              {tokens.length === 0 ? (
                <p className="text-xs text-slate-500">该 Agent 尚未颁发 Token。</p>
              ) : (
                <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
                  {tokens.map((token) => {
                    const revoked = Boolean(token.revokedAt)
                    return (
                      <li
                        key={token.tokenId}
                        className="flex items-center justify-between gap-3 px-4 py-2 text-xs"
                      >
                        <div className="space-y-0.5">
                          <UlidChip id={token.tokenId} />
                          <p className="text-[11px] text-slate-500">
                            过期{' '}
                            {token.expiresAt ? formatRelativeTime(token.expiresAt) : '永不过期'}
                            {revoked ? ` · 撤销于 ${formatRelativeTime(token.revokedAt!)}` : ''}
                          </p>
                          <p className="font-mono text-[10px] text-slate-400">
                            {token.scopes.slice(0, 4).join(', ')}
                            {token.scopes.length > 4 ? ` …+${token.scopes.length - 4}` : ''}
                          </p>
                        </div>
                        <div>
                          {revoked ? (
                            <Badge tone="destructive">已撤销</Badge>
                          ) : (
                            <Button
                              size="sm"
                              variant="destructive"
                              onClick={() => onRevoke(token.tokenId)}
                            >
                              <ShieldX className="h-3.5 w-3.5" /> 撤销
                            </Button>
                          )}
                        </div>
                      </li>
                    )
                  })}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}
