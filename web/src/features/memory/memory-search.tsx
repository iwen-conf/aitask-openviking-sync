import * as React from 'react'
import { Search, Loader2, ArrowRight } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { useMemorySearchQuery } from '@/api/memory'
import { ApiError, describeError } from '@/api/errors'

interface MemorySearchProps {
  projectId: string
  onPick: (uri: string) => void
  className?: string
}

export function MemorySearch({ projectId, onPick, className }: MemorySearchProps) {
  const [draft, setDraft] = React.useState('')
  const [committed, setCommitted] = React.useState('')

  React.useEffect(() => {
    const handle = window.setTimeout(() => setCommitted(draft.trim()), 300)
    return () => window.clearTimeout(handle)
  }, [draft])

  const query = useMemorySearchQuery(projectId, committed)
  const items = query.data?.items ?? []

  return (
    <div className={className}>
      <div className="relative">
        <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
        <Input
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="OpenViking 搜索 (debounce 300ms)"
          className="pl-8 text-xs"
        />
      </div>

      {committed ? (
        <div className="mt-3 space-y-1">
          {query.isLoading ? (
            <p className="flex items-center gap-1 text-xs text-slate-500">
              <Loader2 className="h-3 w-3 animate-spin" /> 检索中…
            </p>
          ) : query.error instanceof ApiError ? (
            <p className="text-xs text-rose-600">{describeError(query.error.envelope).title}</p>
          ) : items.length === 0 ? (
            <p className="text-xs text-slate-500">未命中任何条目。</p>
          ) : (
            items.map((hit) => (
              <button
                key={hit.uri}
                type="button"
                onClick={() => onPick(hit.uri)}
                className="group block w-full rounded-md border border-slate-200 bg-white px-3 py-2 text-left transition-colors hover:border-indigo-200 hover:bg-indigo-50/40"
              >
                <div className="flex items-start justify-between gap-2">
                  <p className="text-xs font-medium text-slate-800">{hit.title}</p>
                  <ArrowRight className="h-3 w-3 shrink-0 text-slate-300 group-hover:text-indigo-500" />
                </div>
                <p className="mt-0.5 line-clamp-2 text-[11px] text-slate-500">{hit.snippet}</p>
                <p className="mt-1 truncate font-mono text-[10px] text-slate-400">{hit.uri}</p>
              </button>
            ))
          )}
        </div>
      ) : null}
    </div>
  )
}
