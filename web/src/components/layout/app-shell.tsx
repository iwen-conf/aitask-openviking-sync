import { useState } from 'react'
import { Outlet } from 'react-router-dom'
import { Sidebar } from './sidebar'
import { Topbar } from './topbar'
import { CreateProjectDialog } from '@/features/projects/create-project-dialog'

export function AppShell() {
  const [createOpen, setCreateOpen] = useState(false)

  return (
    <div className="flex h-screen min-h-0 overflow-hidden bg-[hsl(var(--muted)/0.4)]">
      <Sidebar onCreateProject={() => setCreateOpen(true)} />
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar />
        <main className="relative flex-1 overflow-hidden">
          <Outlet />
        </main>
      </div>
      <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  )
}
