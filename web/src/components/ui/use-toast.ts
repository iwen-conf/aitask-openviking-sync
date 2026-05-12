import { useUiStore, type ToastEntry, type ToastTone } from '@/stores/ui-store'

export interface ToastInput {
  title: string
  description?: string
  tone?: ToastTone
  duration?: number
}

export function useToast() {
  const push = useUiStore((state) => state.pushToast)
  const dismiss = useUiStore((state) => state.dismissToast)
  const toast = (input: ToastInput): string =>
    push({
      title: input.title,
      description: input.description,
      tone: input.tone ?? 'default',
      duration: input.duration,
    })
  return { toast, dismiss }
}

export type { ToastEntry, ToastTone }
