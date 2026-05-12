import * as React from 'react'
import { cn } from '@/lib/utils'

interface DiffLine {
  kind: 'header' | 'meta' | 'add' | 'remove' | 'context' | 'hunk'
  content: string
}

function parseDiff(source: string): DiffLine[] {
  return source.split('\n').map<DiffLine>((line) => {
    if (line.startsWith('diff ') || line.startsWith('index ')) {
      return { kind: 'meta', content: line }
    }
    if (line.startsWith('---') || line.startsWith('+++')) {
      return { kind: 'header', content: line }
    }
    if (line.startsWith('@@')) {
      return { kind: 'hunk', content: line }
    }
    if (line.startsWith('+')) {
      return { kind: 'add', content: line }
    }
    if (line.startsWith('-')) {
      return { kind: 'remove', content: line }
    }
    return { kind: 'context', content: line }
  })
}

const TONE: Record<DiffLine['kind'], string> = {
  meta: 'text-slate-400',
  header: 'text-slate-500',
  hunk: 'text-indigo-500 bg-indigo-50/40',
  add: 'text-emerald-700 bg-emerald-50/60',
  remove: 'text-rose-700 bg-rose-50/60',
  context: 'text-slate-700',
}

interface CodeDiffViewerProps {
  source: string
  className?: string
  /** 超过此行数自动折叠多余行,默认 400 */
  maxLines?: number
}

export function CodeDiffViewer({ source, className, maxLines = 400 }: CodeDiffViewerProps) {
  const lines = React.useMemo(() => parseDiff(source), [source])
  const truncated = lines.length > maxLines
  const [expanded, setExpanded] = React.useState(!truncated)

  const visible = expanded ? lines : lines.slice(0, maxLines)

  return (
    <div className={cn('rounded-lg border border-slate-200 bg-white', className)}>
      <pre className="overflow-x-auto py-2 font-mono text-[11px] leading-5">
        {visible.map((line, index) => (
          <div
            key={`${index}-${line.content.slice(0, 16)}`}
            className={cn('px-3', TONE[line.kind])}
          >
            {line.content || ' '}
          </div>
        ))}
      </pre>
      {truncated ? (
        <button
          type="button"
          onClick={() => setExpanded((prev) => !prev)}
          className="block w-full border-t border-slate-200 bg-slate-50 py-1.5 text-center text-xs font-medium text-indigo-600 hover:bg-slate-100"
        >
          {expanded
            ? `折叠 ${lines.length - maxLines} 行`
            : `展开剩余 ${lines.length - maxLines} 行`}
        </button>
      ) : null}
    </div>
  )
}
