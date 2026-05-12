import { Loader2, X, Download } from 'lucide-react'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { useArtifactDetailQuery } from '@/api/artifacts'
import { ApiError, describeError } from '@/api/errors'
import { Markdown } from '@/components/shared/markdown'
import { formatRelativeTime } from '@/lib/format'
import { CodeDiffViewer } from './code-diff-viewer'
import { ArtifactTypeBadge } from './artifact-type-badge'
import type { ArtifactDetail } from '@/api/types'

interface ArtifactPreviewDialogProps {
  projectId: string
  artifactId?: string
  onClose: () => void
}

export function ArtifactPreviewDialog({
  projectId,
  artifactId,
  onClose,
}: ArtifactPreviewDialogProps) {
  const open = Boolean(artifactId)
  const detailQuery = useArtifactDetailQuery(projectId, artifactId)

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-5xl overflow-hidden p-0">
        <DialogHeader className="space-y-1 border-b border-slate-200 px-6 py-4">
          <div className="flex items-center justify-between gap-3">
            <DialogTitle className="text-base">
              {detailQuery.data?.name ?? 'Artifact 预览'}
            </DialogTitle>
            <Button variant="ghost" size="icon" onClick={onClose}>
              <X className="h-4 w-4" />
            </Button>
          </div>
          {detailQuery.data ? (
            <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500">
              <ArtifactTypeBadge type={detailQuery.data.artifactType} />
              <span className="font-mono text-[11px]">{detailQuery.data.path}</span>
              <span>· {formatRelativeTime(detailQuery.data.createdAt)}</span>
              {detailQuery.data.taskId ? <span>· 任务 {detailQuery.data.taskId}</span> : null}
            </div>
          ) : null}
        </DialogHeader>

        <div className="max-h-[70vh] overflow-y-auto px-6 py-4">
          {detailQuery.isLoading ? (
            <div className="flex h-40 items-center justify-center text-sm text-slate-500">
              <Loader2 className="mr-2 h-4 w-4 animate-spin" /> 正在加载…
            </div>
          ) : detailQuery.error instanceof ApiError ? (
            <div className="rounded-md border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700">
              {describeError(detailQuery.error.envelope).title}
            </div>
          ) : detailQuery.data ? (
            <ArtifactBody artifact={detailQuery.data} />
          ) : null}
        </div>

        {detailQuery.data ? (
          <footer className="flex items-center justify-end gap-2 border-t border-slate-200 bg-slate-50 px-6 py-3 text-xs text-slate-500">
            <Button variant="outline" size="sm" asChild>
              <a
                href={`data:text/plain;charset=utf-8,${encodeURIComponent(detailQuery.data.content)}`}
                download={detailQuery.data.name}
              >
                <Download className="h-3.5 w-3.5" /> 下载文本
              </a>
            </Button>
          </footer>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function ArtifactBody({ artifact }: { artifact: ArtifactDetail }) {
  switch (artifact.artifactType) {
    case 'diff':
    case 'code_diff':
      return <CodeDiffViewer source={artifact.content} />
    case 'markdown':
      return <Markdown>{artifact.content}</Markdown>
    case 'pdf':
      return (
        <div className="rounded-lg border border-slate-200 bg-slate-50 p-4 text-sm text-slate-600">
          <p>
            PDF 文件由浏览器原生预览,可
            <a
              className="text-indigo-600 hover:underline"
              href={artifact.path}
              target="_blank"
              rel="noreferrer"
            >
              在新窗口打开
            </a>
            。
          </p>
          <p className="mt-2 font-mono text-xs text-slate-500">{artifact.path}</p>
        </div>
      )
    case 'image':
      return (
        <div className="flex items-center justify-center rounded-lg bg-slate-900/95 p-3">
          <img
            src={artifact.path}
            alt={artifact.name}
            loading="lazy"
            className="max-h-[60vh] max-w-full object-contain"
          />
        </div>
      )
    case 'json':
      return (
        <pre className="whitespace-pre-wrap rounded-lg bg-slate-950 p-4 font-mono text-xs leading-relaxed text-slate-100">
          {tryFormatJson(artifact.content)}
        </pre>
      )
    default:
      return (
        <pre className="whitespace-pre-wrap rounded-lg border border-slate-200 bg-white p-4 font-mono text-xs text-slate-700">
          {artifact.content}
        </pre>
      )
  }
}

function tryFormatJson(content: string): string {
  try {
    return JSON.stringify(JSON.parse(content), null, 2)
  } catch {
    return content
  }
}
