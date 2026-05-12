import * as React from 'react'
import { Workflow, RefreshCcw, Loader2 } from 'lucide-react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent } from '@/components/ui/card'
import { EmptyState } from '@/components/shared/empty-state'
import { useArtifactsQuery } from '@/api/artifacts'
import { ApiError, describeError } from '@/api/errors'
import { formatRelativeTime } from '@/lib/format'
import { UlidChip } from '@/components/shared/ulid-chip'
import { useProjectOutletContext } from './use-project-context'
import { ARTIFACT_TYPES, artifactTypeLabel } from '@/features/artifacts/artifact-type-meta'
import { ArtifactTypeBadge } from '@/features/artifacts/artifact-type-badge'
import { ArtifactPreviewDialog } from '@/features/artifacts/artifact-preview-dialog'
import type { ArtifactType } from '@/api/types'

export function ArtifactsRoute() {
  const { projectId } = useProjectOutletContext()
  const [type, setType] = React.useState<ArtifactType | 'all'>('all')
  const [taskFilter, setTaskFilter] = React.useState('')
  const [previewId, setPreviewId] = React.useState<string | undefined>()

  const filters = React.useMemo(
    () => ({
      taskId: taskFilter.trim() || undefined,
      type: type === 'all' ? undefined : type,
    }),
    [type, taskFilter],
  )

  const query = useArtifactsQuery(projectId, filters)
  const items = query.data?.items ?? []

  return (
    <div className="flex h-full flex-col bg-[hsl(var(--muted)/0.4)]">
      <header className="flex flex-wrap items-center gap-3 border-b border-slate-200 bg-white px-6 py-4">
        <div className="flex items-center gap-2 text-sm text-slate-600">
          <Workflow className="h-4 w-4 text-slate-500" />
          <strong>Artifacts 资产</strong>
        </div>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <Input
            placeholder="按任务 ID 过滤 (可选)"
            value={taskFilter}
            onChange={(event) => setTaskFilter(event.target.value)}
            className="h-8 w-56 text-xs"
          />
          <div className="w-40">
            <Select value={type} onValueChange={(value) => setType(value as ArtifactType | 'all')}>
              <SelectTrigger className="h-8 text-xs">
                <SelectValue placeholder="全部类型" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部类型</SelectItem>
                {ARTIFACT_TYPES.map((value) => (
                  <SelectItem key={value} value={value}>
                    {artifactTypeLabel(value)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Button variant="outline" size="sm" onClick={() => query.refetch()}>
            <RefreshCcw className="h-3.5 w-3.5" /> 刷新
          </Button>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto p-6">
        {query.isLoading ? (
          <div className="flex h-40 items-center justify-center text-sm text-slate-500">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" /> 正在加载…
          </div>
        ) : query.error instanceof ApiError ? (
          <EmptyState
            icon={<Workflow className="h-8 w-8" />}
            title={describeError(query.error.envelope).title}
            description={describeError(query.error.envelope).hint}
          />
        ) : items.length === 0 ? (
          <EmptyState
            icon={<Workflow className="h-8 w-8" />}
            title="暂无 Artifact"
            description="任务 submit 时会自动落库;也可在任务详情中手动上传。"
          />
        ) : (
          <ul className="grid gap-3 lg:grid-cols-2">
            {items.map((item) => (
              <li key={item.artifactId}>
                <Card className="transition-shadow hover:shadow-md">
                  <CardContent className="space-y-2 p-4">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0 space-y-1">
                        <p className="truncate text-sm font-semibold text-slate-900">{item.name}</p>
                        <p className="truncate font-mono text-[11px] text-slate-400">{item.path}</p>
                      </div>
                      <ArtifactTypeBadge type={item.artifactType} />
                    </div>
                    <div className="flex flex-wrap items-center gap-2 text-[11px] text-slate-500">
                      <UlidChip id={item.artifactId} />
                      {item.taskId ? (
                        <>
                          <span>关联任务</span>
                          <UlidChip id={item.taskId} />
                        </>
                      ) : null}
                      <span className="ml-auto">{formatRelativeTime(item.createdAt)}</span>
                    </div>
                    <div className="pt-1">
                      <Button
                        variant="outline"
                        size="sm"
                        className="w-full"
                        onClick={() => setPreviewId(item.artifactId)}
                      >
                        预览
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              </li>
            ))}
          </ul>
        )}
      </div>

      <ArtifactPreviewDialog
        projectId={projectId}
        artifactId={previewId}
        onClose={() => setPreviewId(undefined)}
      />
    </div>
  )
}
