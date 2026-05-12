import * as React from 'react'
import { Database, Loader2, RefreshCcw, FilePlus2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/shared/empty-state'
import { useMemoryListingQuery } from '@/api/memory'
import { ApiError, describeError } from '@/api/errors'
import { useProjectOutletContext } from './use-project-context'
import { MemoryTree } from '@/features/memory/memory-tree'
import {
  buildMemoryTree,
  findFirstLeaf,
  type MemoryNode,
} from '@/features/memory/memory-tree-utils'
import { MemoryViewer } from '@/features/memory/memory-viewer'
import { MemorySearch } from '@/features/memory/memory-search'
import { MemoryWriteDialog } from '@/features/memory/memory-write-dialog'

export function MemoryRoute() {
  const { projectId } = useProjectOutletContext()
  const listingQuery = useMemoryListingQuery(projectId)
  const [overrideUri, setOverrideUri] = React.useState<string | undefined>()
  const [writeOpen, setWriteOpen] = React.useState(false)

  const tree = React.useMemo(() => {
    if (!listingQuery.data) return null
    return buildMemoryTree(listingQuery.data.root, listingQuery.data.items)
  }, [listingQuery.data])

  // 首选用户最近选中的条目;无选择则回退到树中第一个文件 — 派生而非 setState
  const defaultUri = React.useMemo(() => (tree ? findFirstLeaf(tree)?.uri : undefined), [tree])
  const selectedUri = overrideUri ?? defaultUri

  // 点击 ov:// 链接时跳转到对应条目
  React.useEffect(() => {
    function handler(event: MouseEvent) {
      const target = event.target as HTMLElement | null
      if (!target) return
      const anchor = target.closest('a[href^="viking://"]') as HTMLAnchorElement | null
      if (!anchor) return
      event.preventDefault()
      const href = anchor.getAttribute('href')
      if (href) setOverrideUri(href)
    }
    document.addEventListener('click', handler)
    return () => document.removeEventListener('click', handler)
  }, [])

  if (listingQuery.isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-slate-500">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" /> 正在加载 OpenViking 目录…
      </div>
    )
  }

  if (listingQuery.error instanceof ApiError) {
    const description = describeError(listingQuery.error.envelope)
    return (
      <div className="h-full p-6">
        <EmptyState
          icon={<Database className="h-8 w-8" />}
          title={description.title}
          description={description.hint}
          action={
            <Button variant="outline" size="sm" onClick={() => listingQuery.refetch()}>
              <RefreshCcw className="h-3.5 w-3.5" /> 重试
            </Button>
          }
        />
      </div>
    )
  }

  const data = listingQuery.data
  if (!data || !tree) return null

  const onSelect = (node: MemoryNode) => {
    if (node.uri) setOverrideUri(node.uri)
  }

  return (
    <div className="grid h-full grid-cols-[320px_1fr] divide-x divide-slate-200 bg-white">
      <aside className="flex h-full flex-col overflow-hidden bg-slate-50/40">
        <header className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              OpenViking
            </p>
            <p className="break-all font-mono text-[10px] text-slate-400">{data.root}</p>
          </div>
          <Button size="sm" variant="outline" onClick={() => setWriteOpen(true)}>
            <FilePlus2 className="h-3.5 w-3.5" /> 写入
          </Button>
        </header>

        <div className="border-b border-slate-200 px-3 py-3">
          <MemorySearch projectId={projectId} onPick={(uri) => setOverrideUri(uri)} />
        </div>

        <div className="flex-1 overflow-y-auto px-2 py-3">
          {tree.children.length === 0 ? (
            <p className="px-2 text-xs text-slate-500">空目录,可点击右上角写入。</p>
          ) : (
            <MemoryTree root={tree} selectedUri={selectedUri} onSelect={onSelect} />
          )}
        </div>
      </aside>

      <section className="flex h-full min-h-0 flex-col">
        <MemoryViewer projectId={projectId} uri={selectedUri} />
      </section>

      <MemoryWriteDialog
        projectId={projectId}
        open={writeOpen}
        onOpenChange={setWriteOpen}
        onWritten={(uri) => setOverrideUri(uri)}
      />
    </div>
  )
}
