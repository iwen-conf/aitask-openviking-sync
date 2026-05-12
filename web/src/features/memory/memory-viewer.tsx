import { Loader2 } from 'lucide-react'
import { Markdown } from '@/components/shared/markdown'
import { useMemoryReadQuery } from '@/api/memory'
import { describeError, ApiError } from '@/api/errors'
import { EmptyState } from '@/components/shared/empty-state'
import { formatRelativeTime } from '@/lib/format'

interface MemoryViewerProps {
  projectId: string
  uri?: string
}

export function MemoryViewer({ projectId, uri }: MemoryViewerProps) {
  const query = useMemoryReadQuery(projectId, uri)

  if (!uri) {
    return (
      <EmptyState
        title="选择左侧任意条目查看详情"
        description="支持 Markdown / 文本 / JSON 内容,点击 ov:// 链接可在树内跳转。"
      />
    )
  }

  if (query.isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-slate-500">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" /> 正在读取 OpenViking…
      </div>
    )
  }

  if (query.error instanceof ApiError) {
    const description = describeError(query.error.envelope)
    return <EmptyState title={description.title} description={description.hint} />
  }

  const data = query.data
  if (!data) return null

  const isMarkdown = data.contentType.includes('markdown') || /\.md$/i.test(data.uri)
  const isJson = data.contentType.includes('json') || /\.json$/i.test(data.uri)

  return (
    <article className="flex h-full flex-col">
      <header className="border-b border-slate-200 px-6 py-4">
        <h2 className="text-base font-semibold text-slate-900">{data.title}</h2>
        <p className="mt-1 break-all font-mono text-[11px] text-slate-400">{data.uri}</p>
        {data.updatedAt ? (
          <p className="mt-1 text-xs text-slate-500">更新于 {formatRelativeTime(data.updatedAt)}</p>
        ) : null}
      </header>
      <div className="flex-1 overflow-y-auto px-6 py-4">
        {isMarkdown ? (
          <Markdown>{data.content}</Markdown>
        ) : isJson ? (
          <pre className="whitespace-pre-wrap rounded-lg bg-slate-950 p-4 font-mono text-xs text-slate-100">
            {(() => {
              try {
                return JSON.stringify(JSON.parse(data.content), null, 2)
              } catch {
                return data.content
              }
            })()}
          </pre>
        ) : (
          <pre className="whitespace-pre-wrap font-mono text-xs leading-6 text-slate-700">
            {data.content}
          </pre>
        )}
      </div>
    </article>
  )
}
