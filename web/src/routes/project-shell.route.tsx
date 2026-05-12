import { useEffect } from 'react'
import { Outlet, useNavigate, useParams } from 'react-router-dom'
import { FolderOpen, Loader2 } from 'lucide-react'
import { useProjectQuery } from '@/api/projects'
import { useRoomSocket } from '@/api/room'
import { useToast } from '@/components/ui/use-toast'
import { ApiError } from '@/api/errors'
import { describeError } from '@/api/errors'
import { EmptyState } from '@/components/shared/empty-state'
import { Button } from '@/components/ui/button'

const PROJECT_REDIRECT_ERROR_CODES = new Set(['PROJECT_NOT_FOUND', 'PROJECT_ACCESS_DENIED'])

export function ProjectShellRoute() {
  const params = useParams<{ projectId: string }>()
  const navigate = useNavigate()
  const { toast } = useToast()
  const projectQuery = useProjectQuery(params.projectId)
  const apiError = projectQuery.error instanceof ApiError ? projectQuery.error : null
  const shouldRedirectToProjects =
    apiError !== null && PROJECT_REDIRECT_ERROR_CODES.has(apiError.envelope.code)

  useRoomSocket(params.projectId)

  useEffect(() => {
    if (apiError) {
      const description = describeError(apiError.envelope)
      toast({
        title: description.title,
        description: description.hint,
        tone: 'destructive',
      })
      if (shouldRedirectToProjects) {
        navigate('/projects', { replace: true })
      }
    }
  }, [apiError, navigate, shouldRedirectToProjects, toast])

  if (projectQuery.error) {
    const title = apiError ? describeError(apiError.envelope).title : '项目加载失败'
    const description = apiError
      ? describeError(apiError.envelope).hint
      : projectQuery.error instanceof Error
        ? projectQuery.error.message
        : '请检查网络后重试。'
    return (
      <div className="flex h-full p-8">
        <EmptyState
          icon={<FolderOpen className="h-8 w-8" />}
          title={title}
          description={description}
          action={
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => projectQuery.refetch()}>
                重试
              </Button>
              <Button onClick={() => navigate('/projects')}>返回项目列表</Button>
            </div>
          }
        />
      </div>
    )
  }

  if (projectQuery.isLoading || !projectQuery.data) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-[hsl(var(--muted-foreground))]">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" /> 正在加载项目…
      </div>
    )
  }

  const project = projectQuery.data

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-hidden">
        <Outlet context={{ projectId: project.projectId }} />
      </div>
    </div>
  )
}
