import { createBrowserRouter, Navigate } from 'react-router-dom'
import { AppShell } from '@/components/layout/app-shell'
import { ProjectsListRoute } from './projects-list.route'
import { ProjectShellRoute } from './project-shell.route'
import { OverviewRoute } from './overview.route'
import { TasksRoute } from './tasks.route'
import { TaskDetailRoute } from './task-detail.route'
import { RoomRoute } from './room.route'
import { MemoryRoute } from './memory.route'
import { ArtifactsRoute } from './artifacts.route'
import { AgentsRoute } from './agents.route'
import { SettingsRoute } from './settings.route'
import { SystemSettingsRoute } from './system-settings.route'
import { NotFoundRoute } from './not-found.route'

export const router = createBrowserRouter([
  {
    element: <AppShell />,
    children: [
      { index: true, element: <Navigate to="/projects" replace /> },
      { path: 'projects', element: <ProjectsListRoute /> },
      { path: 'agents', element: <AgentsRoute /> },
      { path: 'agents/claude', element: <AgentsRoute filterType="claude-code" /> },
      { path: 'agents/codex', element: <AgentsRoute filterType="codex" /> },
      { path: 'agents/gemini', element: <AgentsRoute filterType="gemini" /> },
      { path: 'settings', element: <SystemSettingsRoute /> },
      {
        path: 'projects/:projectId',
        element: <ProjectShellRoute />,
        children: [
          { index: true, element: <Navigate to="tasks" replace /> },
          { path: 'overview', element: <OverviewRoute /> },
          { path: 'tasks', element: <TasksRoute /> },
          { path: 'tasks/:taskId', element: <TaskDetailRoute /> },
          { path: 'room', element: <RoomRoute /> },
          { path: 'memory', element: <MemoryRoute /> },
          { path: 'artifacts', element: <ArtifactsRoute /> },
          { path: 'settings', element: <SettingsRoute /> },
        ],
      },
      { path: '*', element: <NotFoundRoute /> },
    ],
  },
])
