import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Search, X } from 'lucide-react'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import type { TaskFilters } from '@/api/tasks'
import type { AgentType, TaskStatus } from '@/api/types'

const STATUS_OPTIONS: TaskStatus[] = [
  'planned',
  'delegated',
  'running',
  'submitted',
  'reviewing',
  'done',
  'blocked',
  'failed',
  'cancelled',
]
const AGENT_OPTIONS: AgentType[] = ['claude-code', 'codex', 'gemini']
const ALL = '__all__'

interface TaskFilterBarProps {
  filters: TaskFilters
  skillOptions: string[]
  draftQ: string
  onDraftQChange(value: string): void
  onChange(next: TaskFilters): void
}

export function TaskFilterBar({
  filters,
  skillOptions,
  draftQ,
  onDraftQChange,
  onChange,
}: TaskFilterBarProps) {
  const { t } = useTranslation()
  const isActive = useMemo(
    () => Boolean(filters.status || filters.assigneeAgentType || filters.skill || filters.q),
    [filters],
  )

  const setStatus = (value: string) =>
    onChange({ ...filters, status: value === ALL ? undefined : (value as TaskStatus) })
  const setAssignee = (value: string) =>
    onChange({ ...filters, assigneeAgentType: value === ALL ? undefined : (value as AgentType) })
  const setSkill = (value: string) =>
    onChange({ ...filters, skill: value === ALL ? undefined : value })

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative">
        <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[hsl(var(--muted-foreground))]" />
        <Input
          value={draftQ}
          onChange={(event) => onDraftQChange(event.target.value)}
          placeholder={t('task.filter.searchPlaceholder')}
          className="h-9 w-56 pl-7"
        />
      </div>

      <Select value={filters.status ?? ALL} onValueChange={setStatus}>
        <SelectTrigger className="h-9 w-32">
          <SelectValue placeholder={t('task.filter.statusPlaceholder')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>{t('task.filter.allStatus')}</SelectItem>
          {STATUS_OPTIONS.map((status) => (
            <SelectItem key={status} value={status}>
              {t(`task.status.${status}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select value={filters.assigneeAgentType ?? ALL} onValueChange={setAssignee}>
        <SelectTrigger className="h-9 w-32">
          <SelectValue placeholder={t('task.filter.assigneePlaceholder')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>{t('task.filter.allAssignee')}</SelectItem>
          {AGENT_OPTIONS.map((agent) => (
            <SelectItem key={agent} value={agent}>
              {t(`agent.type.${agent}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={filters.skill ?? ALL}
        onValueChange={setSkill}
        disabled={skillOptions.length === 0}
      >
        <SelectTrigger className="h-9 w-36">
          <SelectValue placeholder={t('task.filter.skillPlaceholder')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>{t('task.filter.allSkill')}</SelectItem>
          {skillOptions.map((skill) => (
            <SelectItem key={skill} value={skill}>
              {skill}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {isActive ? (
        <Button
          variant="ghost"
          size="sm"
          className="h-9 gap-1 text-[hsl(var(--muted-foreground))]"
          onClick={() => {
            onDraftQChange('')
            onChange({})
          }}
        >
          <X className="h-3.5 w-3.5" /> {t('task.filter.clear')}
        </Button>
      ) : null}
    </div>
  )
}
