import type { ApiErrorEnvelope } from './types'

export type { ApiErrorEnvelope }

/** §22 中列出的 21 个错误码到中文文案的映射。 */
const ERROR_DICT: Record<string, { title: string; hint: string }> = {
  PROJECT_NOT_FOUND: { title: '项目不存在', hint: '请返回项目列表确认 ID。' },
  PROJECT_ACCESS_DENIED: { title: '无权访问此项目', hint: '当前 operator 未被授权。' },
  AGENT_NOT_BOUND_TO_PROJECT: {
    title: 'Agent 未绑定该项目',
    hint: '在项目设置 / Agents 中先绑定再操作。',
  },
  AGENT_TOKEN_INVALID: { title: 'Agent Token 无效', hint: '请重新颁发 Token。' },
  AGENT_TOKEN_EXPIRED: { title: 'Agent Token 已过期', hint: '请前往 Agents 页面 rotate。' },
  TASK_NOT_FOUND: { title: '任务不存在', hint: '可能已被取消或归档。' },
  TASK_NOT_ELIGIBLE_FOR_AGENT: {
    title: '该任务不允许此 Agent 执行',
    hint: '检查任务委托对象与所需 skill。',
  },
  TASK_ALREADY_DELEGATED: {
    title: '任务已被委托',
    hint: '请先取消现有委托后再重新分配。',
  },
  TASK_NOT_ASSIGNED_TO_CURRENT_AGENT: {
    title: '任务未委托给当前 Agent',
    hint: '只有受托 Agent 可以执行该任务。',
  },
  TASK_ACTIVE_RUN_MISMATCH: {
    title: 'Active Run 不匹配',
    hint: '该任务可能在另一对话中已被 fork，请刷新后重试。',
  },
  TASK_DEPENDENCY_NOT_DONE: {
    title: '依赖任务尚未完成',
    hint: '请先推进依赖任务。',
  },
  TASK_STATUS_INVALID: {
    title: '任务状态非法转换',
    hint: '当前状态不允许此操作。',
  },
  ROOM_NOT_FOUND: { title: '聊天室不存在', hint: '请确认项目是否已正确初始化。' },
  ROOM_ACCESS_DENIED: { title: '无权进入该聊天室', hint: '请联系管理员开放权限。' },
  ROOM_MESSAGE_TOO_LARGE: {
    title: '消息超出长度限制',
    hint: '请拆分内容或附在 artifact 中。',
  },
  CONTEXT_BUDGET_EXCEEDED: {
    title: 'Context 预算已用尽',
    hint: '请生成 handoff 后由新对话继续。',
  },
  CONTEXT_HANDOFF_REQUIRED: {
    title: '需要先生成 handoff',
    hint: '请运行 aitask context handoff。',
  },
  HANDOFF_NOT_FOUND: { title: 'Handoff 不存在', hint: '可能已被消费或过期。' },
  HANDOFF_ALREADY_CONSUMED: {
    title: 'Handoff 已被消费',
    hint: '请生成新的 handoff 再继续。',
  },
  OPENVIKING_WRITE_FAILED: {
    title: 'OpenViking 写入失败',
    hint: '请稍后重试，或联系运维。',
  },
  OPENVIKING_READ_FAILED: {
    title: 'OpenViking 读取失败',
    hint: '请稍后重试，或检查记忆库可达性。',
  },
}

export function describeError(error: ApiErrorEnvelope): { title: string; hint: string } {
  const known = ERROR_DICT[error.code]
  if (known) return known
  return {
    title: error.message || '未知错误',
    hint: error.retriable ? '请稍后重试。' : '请联系开发者排查。',
  }
}

export class ApiError extends Error {
  readonly envelope: ApiErrorEnvelope

  constructor(envelope: ApiErrorEnvelope) {
    super(envelope.message || envelope.code)
    this.envelope = envelope
    this.name = 'ApiError'
  }
}
