import { useParams } from 'react-router-dom'
import { useProjectQuery } from '@/api/projects'

/**
 * 当前操作者标签。
 * MVP 阶段从「当前激活项目」的 operatorLabel 派生（docs/API/projects.md）。
 * 如未来改为全局来源,必须先更新 docs/API 契约。
 */
export function useOperatorLabel(): string | undefined {
  const params = useParams<{ projectId?: string }>()
  const projectQuery = useProjectQuery(params.projectId)
  return projectQuery.data?.operatorLabel
}
