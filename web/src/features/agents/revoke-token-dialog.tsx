import * as React from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Loader2, AlertTriangle } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useToast } from '@/components/ui/use-toast'
import { useRevokeAgentTokenMutation } from '@/api/agents'
import { ApiError, describeError } from '@/api/errors'

const SCHEMA = z.object({
  reason: z.string().min(2, '请填写撤销原因(至少 2 个字)'),
  confirmation: z.string(),
})

type FormValues = z.infer<typeof SCHEMA>

interface RevokeTokenDialogProps {
  agentId: string
  agentName: string
  tokenId?: string
  onClose: () => void
}

export function RevokeTokenDialog({
  agentId,
  agentName,
  tokenId,
  onClose,
}: RevokeTokenDialogProps) {
  const open = Boolean(tokenId)
  const { toast } = useToast()
  const mutation = useRevokeAgentTokenMutation(agentId)
  const expectedConfirmation = tokenId ?? ''

  const form = useForm<FormValues>({
    resolver: zodResolver(SCHEMA),
    defaultValues: { reason: 'rotated', confirmation: '' },
  })

  React.useEffect(() => {
    if (!open) form.reset({ reason: 'rotated', confirmation: '' })
  }, [open, form])

  const handleSubmit = form.handleSubmit(async (values) => {
    if (!tokenId) return
    if (values.confirmation.trim() !== expectedConfirmation) {
      form.setError('confirmation', {
        type: 'manual',
        message: `二次确认需输入 ${expectedConfirmation}`,
      })
      return
    }
    try {
      await mutation.mutateAsync({
        tokenId,
        input: { reason: values.reason.trim() },
      })
      toast({
        title: 'Token 已撤销',
        description: `${agentName} · ${expectedConfirmation}`,
        tone: 'success',
      })
      onClose()
    } catch (error) {
      if (error instanceof ApiError) {
        const description = describeError(error.envelope)
        toast({ title: description.title, description: description.hint, tone: 'destructive' })
      } else {
        toast({ title: '撤销失败', tone: 'destructive' })
      }
    }
  })

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-rose-700">
            <AlertTriangle className="h-4 w-4" /> 撤销 Agent Token
          </DialogTitle>
          <DialogDescription>
            撤销后该 Token 立即失效,持有此 Token 的 Agent 进程会在下一次请求时被拒绝。操作不可撤回。
          </DialogDescription>
        </DialogHeader>

        {tokenId ? (
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="rounded-md border border-rose-200 bg-rose-50/60 px-3 py-2 text-xs text-rose-700">
              对象 Agent: <span className="font-semibold">{agentName}</span>
              <br />
              Token ID: <span className="font-mono break-all">{tokenId}</span>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="revoke-reason">撤销原因</Label>
              <Input
                id="revoke-reason"
                {...form.register('reason')}
                placeholder="例如:rotated / leaked / decommissioned"
              />
              {form.formState.errors.reason ? (
                <p className="text-[11px] text-rose-600">{form.formState.errors.reason.message}</p>
              ) : null}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="revoke-confirmation">
                二次确认 — 输入 <span className="font-mono break-all">{expectedConfirmation}</span>
              </Label>
              <Input
                id="revoke-confirmation"
                {...form.register('confirmation')}
                placeholder={expectedConfirmation}
                autoComplete="off"
              />
              {form.formState.errors.confirmation ? (
                <p className="text-[11px] text-rose-600">
                  {form.formState.errors.confirmation.message}
                </p>
              ) : null}
            </div>
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={onClose}>
                取消
              </Button>
              <Button type="submit" variant="destructive" disabled={mutation.isPending}>
                {mutation.isPending ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" /> 撤销中
                  </>
                ) : (
                  '确认撤销'
                )}
              </Button>
            </DialogFooter>
          </form>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
