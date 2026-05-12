import * as React from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils'

export const Sheet = DialogPrimitive.Root
export const SheetTrigger = DialogPrimitive.Trigger
export const SheetPortal = DialogPrimitive.Portal
export const SheetClose = DialogPrimitive.Close

export const SheetOverlay = React.forwardRef<
  React.ComponentRef<typeof DialogPrimitive.Overlay>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Overlay
    ref={ref}
    className={cn(
      'fixed inset-0 z-50 bg-slate-900/40 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
      className,
    )}
    {...props}
  />
))
SheetOverlay.displayName = 'SheetOverlay'

interface SheetContentProps extends React.ComponentPropsWithoutRef<typeof DialogPrimitive.Content> {
  side?: 'right' | 'left'
  hideClose?: boolean
}

export const SheetContent = React.forwardRef<
  React.ComponentRef<typeof DialogPrimitive.Content>,
  SheetContentProps
>(({ className, children, side = 'right', hideClose = false, ...props }, ref) => (
  <SheetPortal>
    <SheetOverlay />
    <DialogPrimitive.Content
      ref={ref}
      className={cn(
        'fixed top-0 z-50 flex h-full w-full max-w-2xl flex-col gap-0 border-l border-[hsl(var(--border))] bg-[hsl(var(--card))] shadow-2xl outline-none',
        'data-[state=open]:animate-in data-[state=closed]:animate-out',
        side === 'right'
          ? 'right-0 data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right'
          : 'left-0 border-l-0 border-r data-[state=closed]:slide-out-to-left data-[state=open]:slide-in-from-left',
        className,
      )}
      {...props}
    >
      {children}
      {hideClose ? null : (
        <DialogPrimitive.Close className="absolute right-4 top-4 rounded-sm p-1 text-[hsl(var(--muted-foreground))] opacity-70 transition-opacity hover:bg-[hsl(var(--muted))] hover:opacity-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring))]">
          <X className="h-4 w-4" />
          <span className="sr-only">关闭</span>
        </DialogPrimitive.Close>
      )}
    </DialogPrimitive.Content>
  </SheetPortal>
))
SheetContent.displayName = 'SheetContent'

export function SheetHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'flex shrink-0 flex-col gap-1.5 border-b border-[hsl(var(--border))] px-6 py-5',
        className,
      )}
      {...props}
    />
  )
}

export function SheetBody({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('flex-1 overflow-y-auto px-6 py-4', className)} {...props} />
}

export function SheetFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'flex shrink-0 items-center justify-end gap-2 border-t border-[hsl(var(--border))] px-6 py-4',
        className,
      )}
      {...props}
    />
  )
}

export const SheetTitle = React.forwardRef<
  React.ComponentRef<typeof DialogPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Title>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Title
    ref={ref}
    className={cn('text-lg font-semibold text-[hsl(var(--foreground))]', className)}
    {...props}
  />
))
SheetTitle.displayName = 'SheetTitle'

export const SheetDescription = React.forwardRef<
  React.ComponentRef<typeof DialogPrimitive.Description>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Description>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Description
    ref={ref}
    className={cn('text-sm text-[hsl(var(--muted-foreground))]', className)}
    {...props}
  />
))
SheetDescription.displayName = 'SheetDescription'
