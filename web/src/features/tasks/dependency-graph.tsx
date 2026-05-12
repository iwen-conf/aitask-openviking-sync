import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { TASK_STATUS_GROUP } from '@/lib/format'
import type { Task } from '@/api/types'
import { cn } from '@/lib/utils'

interface DependencyGraphProps {
  current: Task
  allTasks: Task[]
  onSelectTask(taskId: string): void
}

interface Node {
  taskId: string
  title: string
  status: Task['status']
  x: number
  y: number
  role: 'current' | 'parent' | 'child' | 'dependency'
}

interface Edge {
  fromX: number
  fromY: number
  toX: number
  toY: number
  type: 'child' | 'dependency'
}

const NODE_W = 168
const NODE_H = 52
const GAP_X = 28
const GAP_Y = 72

const ROLE_TONE: Record<Node['role'], string> = {
  current: 'border-indigo-500 bg-indigo-50 text-indigo-900 ring-2 ring-indigo-300',
  parent: 'border-violet-200 bg-violet-50 text-violet-900',
  child: 'border-emerald-200 bg-emerald-50 text-emerald-900',
  dependency: 'border-sky-200 bg-sky-50 text-sky-900',
}

const STATUS_DOT: Record<Task['status'], string> = {
  planned: 'bg-slate-400',
  delegated: 'bg-amber-400',
  running: 'bg-emerald-500',
  submitted: 'bg-sky-500',
  reviewing: 'bg-violet-500',
  done: 'bg-indigo-500',
  blocked: 'bg-orange-500',
  failed: 'bg-rose-500',
  cancelled: 'bg-slate-400',
}

function placeRow(
  items: Array<{ id: string }>,
  centerX: number,
  y: number,
): Map<string, [number, number]> {
  const map = new Map<string, [number, number]>()
  if (items.length === 0) return map
  const totalWidth = items.length * NODE_W + (items.length - 1) * GAP_X
  const startX = centerX - totalWidth / 2
  items.forEach((item, idx) => {
    const x = startX + idx * (NODE_W + GAP_X)
    map.set(item.id, [x, y])
  })
  return map
}

export function DependencyGraph({ current, allTasks, onSelectTask }: DependencyGraphProps) {
  const { t } = useTranslation()
  const { nodes, edges, width, height } = useMemo(() => {
    const parent = current.parentTaskId
      ? (allTasks.find((t) => t.taskId === current.parentTaskId) ??
        ({ taskId: current.parentTaskId, title: current.parentTaskId, status: 'planned' } as Task))
      : null

    const dependencies = (current.dependencies ?? []).map(
      (id) =>
        allTasks.find((t) => t.taskId === id) ??
        ({ taskId: id, title: id, status: 'planned' } as Task),
    )

    const children = allTasks.filter((t) => t.parentTaskId === current.taskId)

    const childCount = children.length
    const depCount = dependencies.length
    const cols = Math.max(1, childCount, depCount, parent ? 1 : 1)
    const totalWidth = cols * NODE_W + (cols - 1) * GAP_X + 64
    const width = Math.max(420, totalWidth)
    const height =
      (parent ? 1 : 0) * (NODE_H + GAP_Y) + NODE_H + (childCount > 0 ? NODE_H + GAP_Y : 0) + 64
    const centerX = width / 2

    const parentY = 32
    const currentY = parent ? parentY + NODE_H + GAP_Y : 32
    const childrenY = currentY + NODE_H + GAP_Y

    const parentPos = parent
      ? placeRow([{ id: parent.taskId }], centerX, parentY)
      : new Map<string, [number, number]>()
    const childrenPos = placeRow(
      children.map((c) => ({ id: c.taskId })),
      centerX,
      childrenY,
    )
    const currentPos: [number, number] = [centerX - NODE_W / 2, currentY]

    const depRowY = currentY
    const depPos = new Map<string, [number, number]>()
    dependencies.forEach((dep, idx) => {
      const x = 24
      const y = depRowY + idx * (NODE_H + 8) - ((dependencies.length - 1) * (NODE_H + 8)) / 2
      depPos.set(dep.taskId, [x, y])
    })

    const nodes: Node[] = []
    if (parent) {
      const [px, py] = parentPos.get(parent.taskId)!
      nodes.push({
        taskId: parent.taskId,
        title: parent.title,
        status: parent.status,
        x: px,
        y: py,
        role: 'parent',
      })
    }
    nodes.push({
      taskId: current.taskId,
      title: current.title,
      status: current.status,
      x: currentPos[0],
      y: currentPos[1],
      role: 'current',
    })
    children.forEach((child) => {
      const [cx, cy] = childrenPos.get(child.taskId)!
      nodes.push({
        taskId: child.taskId,
        title: child.title,
        status: child.status,
        x: cx,
        y: cy,
        role: 'child',
      })
    })
    dependencies.forEach((dep) => {
      const [dx, dy] = depPos.get(dep.taskId)!
      nodes.push({
        taskId: dep.taskId,
        title: dep.title,
        status: dep.status,
        x: dx,
        y: dy,
        role: 'dependency',
      })
    })

    const edges: Edge[] = []
    if (parent) {
      const [px, py] = parentPos.get(parent.taskId)!
      edges.push({
        fromX: px + NODE_W / 2,
        fromY: py + NODE_H,
        toX: currentPos[0] + NODE_W / 2,
        toY: currentPos[1],
        type: 'child',
      })
    }
    children.forEach((child) => {
      const [cx, cy] = childrenPos.get(child.taskId)!
      edges.push({
        fromX: currentPos[0] + NODE_W / 2,
        fromY: currentPos[1] + NODE_H,
        toX: cx + NODE_W / 2,
        toY: cy,
        type: 'child',
      })
    })
    dependencies.forEach((dep) => {
      const [dx, dy] = depPos.get(dep.taskId)!
      edges.push({
        fromX: dx + NODE_W,
        fromY: dy + NODE_H / 2,
        toX: currentPos[0],
        toY: currentPos[1] + NODE_H / 2,
        type: 'dependency',
      })
    })

    return { nodes, edges, width, height: Math.max(height, 200) }
  }, [current, allTasks])

  if (nodes.length <= 1) {
    return (
      <p className="text-sm text-[hsl(var(--muted-foreground))]">
        当前任务无父任务、子任务或依赖。
      </p>
    )
  }

  return (
    <div className="overflow-x-auto rounded-lg border border-[hsl(var(--border))] bg-slate-50/40 p-2">
      <svg
        width={width}
        height={height}
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label="任务依赖关系图"
      >
        <defs>
          <marker id="arrow-child" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
            <path d="M0,0 L0,8 L7,4 z" fill="rgb(99 102 241)" />
          </marker>
          <marker id="arrow-dep" markerWidth="8" markerHeight="8" refX="6" refY="4" orient="auto">
            <path d="M0,0 L0,8 L7,4 z" fill="rgb(2 132 199)" />
          </marker>
        </defs>
        {edges.map((edge, idx) => (
          <line
            key={idx}
            x1={edge.fromX}
            y1={edge.fromY}
            x2={edge.toX}
            y2={edge.toY}
            stroke={edge.type === 'child' ? 'rgb(99 102 241)' : 'rgb(2 132 199)'}
            strokeWidth={1.5}
            strokeDasharray={edge.type === 'dependency' ? '4 3' : undefined}
            markerEnd={edge.type === 'child' ? 'url(#arrow-child)' : 'url(#arrow-dep)'}
          />
        ))}
        {nodes.map((node) => (
          <foreignObject key={node.taskId} x={node.x} y={node.y} width={NODE_W} height={NODE_H}>
            <button
              type="button"
              disabled={node.role === 'current'}
              onClick={() => onSelectTask(node.taskId)}
              title={`${node.title} · ${t(`task.status.${node.status}`)}`}
              className={cn(
                'flex h-full w-full flex-col justify-center gap-1 rounded-md border px-2 py-1 text-left text-xs shadow-sm transition-shadow',
                ROLE_TONE[node.role],
                node.role === 'current' ? 'cursor-default' : 'hover:shadow-md',
              )}
            >
              <span className="flex items-center gap-1.5">
                <span className={cn('h-1.5 w-1.5 rounded-full', STATUS_DOT[node.status])} />
                <span className="truncate font-mono text-[10px] opacity-70">
                  {node.taskId.slice(-8)}
                </span>
                <span className="ml-auto text-[10px] uppercase tracking-wide opacity-60">
                  {t(`task.dependencyGraph.role.${node.role}`)}
                </span>
              </span>
              <span className="truncate font-medium">{node.title || node.taskId}</span>
            </button>
          </foreignObject>
        ))}
      </svg>
      <div className="mt-2 flex items-center gap-3 px-2 text-[11px] text-[hsl(var(--muted-foreground))]">
        <LegendDot
          label={t('task.dependencyGraph.legendChild')}
          color="rgb(99 102 241)"
          dashed={false}
        />
        <LegendDot
          label={t('task.dependencyGraph.legendDependency')}
          color="rgb(2 132 199)"
          dashed={true}
        />
        <span className="ml-auto">
          {t('task.dependencyGraph.hint')}
          {TASK_STATUS_GROUP[current.status] === 'blocked'
            ? ` · ${t('task.inDependencyGraph.blockedHint')}`
            : ''}
        </span>
      </div>
    </div>
  )
}

function LegendDot({ label, color, dashed }: { label: string; color: string; dashed: boolean }) {
  return (
    <span className="inline-flex items-center gap-1">
      <svg width="22" height="6">
        <line
          x1={0}
          y1={3}
          x2={22}
          y2={3}
          stroke={color}
          strokeWidth={1.5}
          strokeDasharray={dashed ? '4 3' : undefined}
        />
      </svg>
      {label}
    </span>
  )
}
