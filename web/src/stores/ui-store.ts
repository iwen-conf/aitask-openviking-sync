import { create } from 'zustand'

export type ToastTone = 'default' | 'success' | 'destructive' | 'info'

export interface ToastEntry {
  id: string
  title: string
  description?: string
  tone: ToastTone
  expiresAt: number
}

interface UiStore {
  toasts: ToastEntry[]
  pushToast: (entry: Omit<ToastEntry, 'id' | 'expiresAt'> & { duration?: number }) => string
  dismissToast: (id: string) => void
}

export const useUiStore = create<UiStore>((set) => ({
  toasts: [],
  pushToast: ({ duration = 3500, ...payload }) => {
    const id = `toast_${Math.random().toString(36).slice(2, 10)}`
    const entry: ToastEntry = {
      id,
      tone: payload.tone ?? 'default',
      title: payload.title,
      description: payload.description,
      expiresAt: Date.now() + duration,
    }
    set((state) => ({ toasts: [...state.toasts, entry] }))
    if (duration > 0) {
      setTimeout(() => {
        set((state) => ({ toasts: state.toasts.filter((toast) => toast.id !== id) }))
      }, duration)
    }
    return id
  },
  dismissToast: (id) =>
    set((state) => ({ toasts: state.toasts.filter((toast) => toast.id !== id) })),
}))
