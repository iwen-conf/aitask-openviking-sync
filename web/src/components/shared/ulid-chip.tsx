import { Copy, Check } from 'lucide-react'
import { useState } from 'react'
import { cn } from '@/lib/utils'
import { shortenUlid } from '@/lib/format'
import { useToast } from '@/components/ui/use-toast'

interface UlidChipProps {
  id: string
  full?: boolean
  className?: string
}

export function UlidChip({ id, full = true, className }: UlidChipProps) {
  const [copied, setCopied] = useState(false)
  const { toast } = useToast()

  const handleCopy = async (event: React.MouseEvent) => {
    event.preventDefault()
    event.stopPropagation()
    try {
      await navigator.clipboard.writeText(id)
      setCopied(true)
      toast({ title: 'ID 已复制', tone: 'success', duration: 1800 })
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast({ title: '复制失败，请手动选择 ID', tone: 'destructive' })
    }
  }

  return (
    <button
      type="button"
      onClick={handleCopy}
      className={cn(
        'inline-flex max-w-full items-start gap-1 rounded-md border border-slate-200 bg-slate-50 px-2 py-0.5 text-left font-mono text-xs text-slate-500 transition-colors hover:border-indigo-200 hover:bg-indigo-50 hover:text-indigo-700',
        className,
      )}
      title={id}
    >
      <span className="min-w-0 break-all">{full ? id : shortenUlid(id)}</span>
      {copied ? (
        <Check className="h-3 w-3 shrink-0" />
      ) : (
        <Copy className="h-3 w-3 shrink-0 opacity-60" />
      )}
    </button>
  )
}
