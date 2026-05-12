import i18n from '@/i18n'
import type { ProjectStatus, TaskStatus } from '@/api/types'

function activeLocale(): string {
  return i18n.resolvedLanguage ?? i18n.language ?? 'zh-CN'
}

const relativeFormatterCache = new Map<string, Intl.RelativeTimeFormat>()
const absoluteFormatterCache = new Map<string, Intl.DateTimeFormat>()
const timeFormatterCache = new Map<string, Intl.DateTimeFormat>()

function relFormatter(locale: string): Intl.RelativeTimeFormat {
  let cached = relativeFormatterCache.get(locale)
  if (!cached) {
    cached = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
    relativeFormatterCache.set(locale, cached)
  }
  return cached
}

function absFormatter(locale: string): Intl.DateTimeFormat {
  let cached = absoluteFormatterCache.get(locale)
  if (!cached) {
    cached = new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' })
    absoluteFormatterCache.set(locale, cached)
  }
  return cached
}

function clockFormatter(locale: string): Intl.DateTimeFormat {
  let cached = timeFormatterCache.get(locale)
  if (!cached) {
    cached = new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit' })
    timeFormatterCache.set(locale, cached)
  }
  return cached
}

/**
 * ISO-8601 时间字符串相对化展示。
 * 1 分钟内显示「刚刚」/「just now」(取决于当前语言),否则按 minute/hour/day 相对化,超过 7 天回落绝对日期。
 */
export function formatRelativeTime(iso: string): string {
  const target = new Date(iso).getTime()
  if (Number.isNaN(target)) return iso
  const now = Date.now()
  const diffSec = Math.round((target - now) / 1000)
  const absSec = Math.abs(diffSec)
  const locale = activeLocale()

  if (absSec < 60) return i18n.t('common.justNow', { defaultValue: '刚刚' })
  const f = relFormatter(locale)
  if (absSec < 3600) return f.format(Math.round(diffSec / 60), 'minute')
  if (absSec < 86_400) return f.format(Math.round(diffSec / 3600), 'hour')
  if (absSec < 86_400 * 7) return f.format(Math.round(diffSec / 86_400), 'day')
  return absFormatter(locale).format(target)
}

export function formatTime(iso: string): string {
  const target = new Date(iso)
  if (Number.isNaN(target.getTime())) return iso
  return clockFormatter(activeLocale()).format(target)
}

/** ULID 截短：保留前缀 + 末 6 位，便于扫读。 */
export function shortenUlid(id: string): string {
  if (!id.includes('_')) return id
  const [prefix, rest] = id.split('_')
  if (!rest || rest.length <= 8) return id
  return `${prefix}_…${rest.slice(-6)}`
}

/**
 * Status badge 视觉颜色。语言无关，因此保留为静态字典；
 * 对应文案统一通过 `t('project.status.*')` / `t('task.status.*')` 取值。
 */
export const PROJECT_STATUS_TONE: Record<ProjectStatus, string> = {
  draft: 'bg-slate-100 text-slate-600 border-slate-200',
  active: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  paused: 'bg-amber-50 text-amber-700 border-amber-200',
  blocked: 'bg-orange-50 text-orange-700 border-orange-200',
  reviewing: 'bg-violet-50 text-violet-700 border-violet-200',
  completed: 'bg-indigo-50 text-indigo-700 border-indigo-200',
  archived: 'bg-slate-100 text-slate-500 border-slate-200',
}

export const TASK_STATUS_TONE: Record<TaskStatus, string> = {
  planned: 'bg-slate-100 text-slate-600',
  delegated: 'bg-amber-50 text-amber-700',
  running: 'bg-emerald-50 text-emerald-700',
  submitted: 'bg-sky-50 text-sky-700',
  reviewing: 'bg-violet-50 text-violet-700',
  done: 'bg-indigo-50 text-indigo-700',
  blocked: 'bg-orange-50 text-orange-700',
  failed: 'bg-rose-50 text-rose-700',
  cancelled: 'bg-slate-100 text-slate-500',
}

/** 任务状态对应的「指示灯」纯色实心 dot；与 TASK_STATUS_TONE 保持同源色调。 */
export const TASK_STATUS_DOT: Record<TaskStatus, string> = {
  planned: 'bg-slate-400',
  delegated: 'bg-amber-500',
  running: 'bg-emerald-500',
  submitted: 'bg-sky-500',
  reviewing: 'bg-violet-500',
  done: 'bg-indigo-500',
  blocked: 'bg-orange-500',
  failed: 'bg-rose-500',
  cancelled: 'bg-slate-300',
}

/** running / submitted 状态加上呼吸效果，提示「活跃任务」。 */
export const TASK_STATUS_PULSE: Partial<Record<TaskStatus, boolean>> = {
  running: true,
  submitted: true,
}

/** §21.3 三段语义在每个 Agent 列内部的分组归类，详见 `docs/前端/decisions.md` D-FE-001。 */
export type TaskGroupKey = 'queue' | 'running' | 'blocked' | 'done'

export const TASK_STATUS_GROUP: Record<TaskStatus, TaskGroupKey> = {
  planned: 'queue',
  delegated: 'queue',
  running: 'running',
  submitted: 'running',
  reviewing: 'running',
  blocked: 'blocked',
  done: 'done',
  failed: 'done',
  cancelled: 'done',
}

/** 每个 Agent 列内分组小标题的视觉 token；label 通过 `t('task.group.*')` 取值。 */
export const TASK_GROUP_TONE: Record<TaskGroupKey, { tone: string; dot: string }> = {
  queue: { tone: 'text-slate-500', dot: 'bg-amber-400' },
  running: { tone: 'text-slate-500', dot: 'bg-emerald-500' },
  blocked: { tone: 'text-orange-600', dot: 'bg-orange-500' },
  done: { tone: 'text-slate-500', dot: 'bg-slate-400' },
}
