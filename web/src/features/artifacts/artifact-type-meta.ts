import type { ArtifactType } from '@/api/types'
import { Diff, FileCode, FileText, Image as ImageIcon, FileType2 } from 'lucide-react'

export interface ArtifactTypeMeta {
  label: string
  tone: string
  icon: React.ComponentType<{ className?: string }>
}

export const ARTIFACT_TYPE_META: Record<ArtifactType, ArtifactTypeMeta> = {
  diff: {
    label: 'Diff',
    tone: 'bg-amber-50 text-amber-700 border-amber-100',
    icon: Diff,
  },
  code_diff: {
    label: 'Code Diff',
    tone: 'bg-amber-50 text-amber-700 border-amber-100',
    icon: Diff,
  },
  markdown: {
    label: 'Markdown',
    tone: 'bg-slate-100 text-slate-700 border-slate-200',
    icon: FileText,
  },
  pdf: {
    label: 'PDF',
    tone: 'bg-rose-50 text-rose-700 border-rose-100',
    icon: FileType2,
  },
  image: {
    label: '图片',
    tone: 'bg-indigo-50 text-indigo-700 border-indigo-100',
    icon: ImageIcon,
  },
  json: {
    label: 'JSON',
    tone: 'bg-cyan-50 text-cyan-700 border-cyan-100',
    icon: FileCode,
  },
  text: {
    label: '文本',
    tone: 'bg-slate-50 text-slate-600 border-slate-200',
    icon: FileText,
  },
  other: {
    label: '其他',
    tone: 'bg-slate-50 text-slate-600 border-slate-200',
    icon: FileText,
  },
}

export const ARTIFACT_TYPES: ArtifactType[] = [
  'diff',
  'code_diff',
  'markdown',
  'pdf',
  'image',
  'json',
  'text',
  'other',
]

export function artifactTypeLabel(type: ArtifactType): string {
  return ARTIFACT_TYPE_META[type]?.label ?? type
}
