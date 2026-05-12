import * as React from 'react'
import { ChevronRight, FileText, Folder, FolderOpen } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { MemoryNode } from './memory-tree-utils'

interface TreeNodeProps {
  node: MemoryNode
  depth: number
  selectedUri?: string
  onSelect: (node: MemoryNode) => void
  defaultOpenDepth?: number
}

function TreeNode({ node, depth, selectedUri, onSelect, defaultOpenDepth = 1 }: TreeNodeProps) {
  const isFolder = node.children.length > 0
  const [open, setOpen] = React.useState<boolean>(depth < defaultOpenDepth)
  const isActive = !isFolder && node.uri === selectedUri

  const padding = { paddingInlineStart: 8 + depth * 12 }

  if (isFolder) {
    return (
      <div>
        <button
          type="button"
          onClick={() => setOpen((prev) => !prev)}
          className="flex w-full items-center gap-1.5 rounded-md px-2 py-1 text-left text-xs text-slate-600 transition-colors hover:bg-slate-100"
          style={padding}
        >
          <ChevronRight
            className={cn('h-3 w-3 shrink-0 transition-transform', open && 'rotate-90')}
          />
          {open ? (
            <FolderOpen className="h-3.5 w-3.5 text-amber-500" />
          ) : (
            <Folder className="h-3.5 w-3.5 text-amber-500" />
          )}
          <span className="truncate font-medium">{node.name}</span>
          <span className="ml-auto text-[10px] text-slate-400">{node.children.length}</span>
        </button>
        {open ? (
          <div>
            {node.children.map((child) => (
              <TreeNode
                key={child.path}
                node={child}
                depth={depth + 1}
                selectedUri={selectedUri}
                onSelect={onSelect}
                defaultOpenDepth={defaultOpenDepth}
              />
            ))}
          </div>
        ) : null}
      </div>
    )
  }

  return (
    <button
      type="button"
      onClick={() => onSelect(node)}
      className={cn(
        'flex w-full items-center gap-1.5 rounded-md px-2 py-1 text-left text-xs transition-colors',
        isActive ? 'bg-indigo-50 text-indigo-700' : 'text-slate-600 hover:bg-slate-100',
      )}
      style={padding}
    >
      <span className="h-3 w-3 shrink-0" />
      <FileText
        className={cn('h-3.5 w-3.5 shrink-0', isActive ? 'text-indigo-500' : 'text-slate-400')}
      />
      <span className="truncate">{node.name}</span>
    </button>
  )
}

interface MemoryTreeProps {
  root: MemoryNode
  selectedUri?: string
  onSelect: (node: MemoryNode) => void
  className?: string
}

export function MemoryTree({ root, selectedUri, onSelect, className }: MemoryTreeProps) {
  return (
    <div className={cn('space-y-0.5', className)}>
      {root.children.map((child) => (
        <TreeNode
          key={child.path}
          node={child}
          depth={0}
          selectedUri={selectedUri}
          onSelect={onSelect}
        />
      ))}
    </div>
  )
}
