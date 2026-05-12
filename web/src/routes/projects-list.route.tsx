import { useState } from 'react'
import { Plus } from 'lucide-react'
import { useProjectsQuery } from '@/api/projects'
import { Button } from '@/components/ui/button'
import { ProjectsTable } from '@/features/projects/projects-table'
import { CreateProjectDialog } from '@/features/projects/create-project-dialog'

export function ProjectsListRoute() {
  const [createOpen, setCreateOpen] = useState(false)
  const projectsQuery = useProjectsQuery()

  return (
    <div className="flex h-full flex-col gap-6 overflow-y-auto bg-[hsl(var(--muted)/0.4)] p-8">
      <div className="flex items-start justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-bold tracking-tight text-[hsl(var(--foreground))]">
            项目列表
          </h1>
          <p className="text-sm text-[hsl(var(--muted-foreground))]">
            选择项目进入任务看板与 Agent 协作室；当前项目集合来自 GET /api/projects。
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)} className="gap-2">
          <Plus className="h-4 w-4" /> 新建项目
        </Button>
      </div>

      <ProjectsTable
        projects={projectsQuery.data?.items ?? []}
        isLoading={projectsQuery.isLoading}
        onCreate={() => setCreateOpen(true)}
      />

      <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  )
}
