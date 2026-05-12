import { useState } from 'react'
import { Check, Copy, Terminal } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { useToast } from '@/components/ui/use-toast'

interface InitCommandDialogProps {
  open: boolean
  command: string
  onContinue(): void
  onOpenChange(open: boolean): void
}

async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // ignore, fall through to textarea
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}

export function InitCommandDialog({
  open,
  command,
  onContinue,
  onOpenChange,
}: InitCommandDialogProps) {
  const { toast } = useToast()
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    const ok = await copyToClipboard(command)
    if (ok) {
      setCopied(true)
      toast({ title: '已复制 init 命令', tone: 'success', duration: 2500 })
      setTimeout(() => setCopied(false), 2000)
    } else {
      toast({
        title: '复制失败',
        description: '请手动选中命令复制',
        tone: 'destructive',
      })
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Terminal className="h-5 w-5 text-emerald-600" />
            项目已创建
          </DialogTitle>
          <DialogDescription>
            请在本地工程目录执行以下命令，完成 OpenViking 工作区初始化与 Agent Token 注入。
          </DialogDescription>
        </DialogHeader>

        <div className="rounded-md border border-[hsl(var(--border))] bg-slate-950 p-4 font-mono text-sm text-emerald-300 shadow-inner">
          <div className="flex items-start gap-2">
            <span className="select-none text-slate-500">$</span>
            <code className="flex-1 break-all whitespace-pre-wrap leading-relaxed">{command}</code>
          </div>
        </div>

        <p className="text-xs text-[hsl(var(--muted-foreground))]">
          init 后即可在终端执行 <code className="font-mono">aitask agent register</code> 注册
          Agent， 或在本控制台直接进入任务看板创建首条委托任务。
        </p>

        <DialogFooter className="gap-2 pt-1">
          <Button type="button" variant="ghost" onClick={onContinue}>
            前往任务看板
          </Button>
          <Button type="button" onClick={handleCopy} className="gap-2">
            {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            {copied ? '已复制' : '复制命令'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
