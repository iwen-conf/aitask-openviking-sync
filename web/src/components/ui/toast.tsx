import * as React from 'react'
import * as ToastPrimitive from '@radix-ui/react-toast'
import { cva, type VariantProps } from 'class-variance-authority'
import { X, CheckCircle2, AlertCircle, Info } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useUiStore, type ToastTone } from '@/stores/ui-store'

const TONE_ICON: Record<ToastTone, React.ReactNode> = {
  default: <Info className="h-4 w-4 text-[hsl(var(--primary))]" />,
  info: <Info className="h-4 w-4 text-sky-500" />,
  success: <CheckCircle2 className="h-4 w-4 text-emerald-500" />,
  destructive: <AlertCircle className="h-4 w-4 text-rose-500" />,
}

const toastVariants = cva(
  'pointer-events-auto relative flex w-full items-start gap-3 rounded-lg border p-4 shadow-md transition-all',
  {
    variants: {
      tone: {
        default: 'border-[hsl(var(--border))] bg-slate-900 text-white',
        info: 'border-sky-200 bg-white text-slate-800',
        success: 'border-emerald-200 bg-white text-slate-800',
        destructive: 'border-rose-200 bg-white text-slate-800',
      },
    },
    defaultVariants: {
      tone: 'default',
    },
  },
)

interface ToastItemProps extends VariantProps<typeof toastVariants> {
  title: string
  description?: string
  onDismiss: () => void
}

function ToastItem({ tone = 'default', title, description, onDismiss }: ToastItemProps) {
  return (
    <ToastPrimitive.Root
      className={cn(
        toastVariants({ tone }),
        'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-80 data-[state=closed]:slide-out-to-right-full data-[state=open]:slide-in-from-right-full',
      )}
      onOpenChange={(open) => {
        if (!open) onDismiss()
      }}
    >
      <span className="mt-0.5 shrink-0">{TONE_ICON[tone ?? 'default']}</span>
      <div className="flex-1 space-y-1">
        <ToastPrimitive.Title
          className={cn(
            'text-sm font-semibold',
            tone === 'default' ? 'text-white' : 'text-slate-900',
          )}
        >
          {title}
        </ToastPrimitive.Title>
        {description ? (
          <ToastPrimitive.Description
            className={cn('text-xs', tone === 'default' ? 'text-slate-200' : 'text-slate-500')}
          >
            {description}
          </ToastPrimitive.Description>
        ) : null}
      </div>
      <ToastPrimitive.Close
        className={cn(
          'rounded-md p-1 transition-colors',
          tone === 'default'
            ? 'text-slate-400 hover:text-white'
            : 'text-slate-400 hover:text-slate-700',
        )}
      >
        <X className="h-3.5 w-3.5" />
      </ToastPrimitive.Close>
    </ToastPrimitive.Root>
  )
}

export function Toaster() {
  const toasts = useUiStore((state) => state.toasts)
  const dismiss = useUiStore((state) => state.dismissToast)
  return (
    <ToastPrimitive.Provider duration={3500} swipeDirection="right">
      {toasts.map((toast) => (
        <ToastItem
          key={toast.id}
          tone={toast.tone}
          title={toast.title}
          description={toast.description}
          onDismiss={() => dismiss(toast.id)}
        />
      ))}
      <ToastPrimitive.Viewport className="fixed bottom-6 right-6 z-50 flex max-h-screen w-full max-w-sm flex-col gap-2 outline-none" />
    </ToastPrimitive.Provider>
  )
}
