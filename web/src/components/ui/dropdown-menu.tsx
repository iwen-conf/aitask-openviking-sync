import * as React from 'react'
import * as DropdownPrimitive from '@radix-ui/react-dropdown-menu'
import { Check } from 'lucide-react'
import { cn } from '@/lib/utils'

export const DropdownMenu = DropdownPrimitive.Root
export const DropdownMenuTrigger = DropdownPrimitive.Trigger
export const DropdownMenuPortal = DropdownPrimitive.Portal

export const DropdownMenuContent = React.forwardRef<
  React.ComponentRef<typeof DropdownPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof DropdownPrimitive.Content>
>(({ className, sideOffset = 6, ...props }, ref) => (
  <DropdownPrimitive.Portal>
    <DropdownPrimitive.Content
      ref={ref}
      sideOffset={sideOffset}
      className={cn(
        'z-50 min-w-[12rem] overflow-hidden rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--popover))] p-1 text-sm shadow-lg',
        'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
        className,
      )}
      {...props}
    />
  </DropdownPrimitive.Portal>
))
DropdownMenuContent.displayName = DropdownPrimitive.Content.displayName

export const DropdownMenuItem = React.forwardRef<
  React.ComponentRef<typeof DropdownPrimitive.Item>,
  React.ComponentPropsWithoutRef<typeof DropdownPrimitive.Item> & { selected?: boolean }
>(({ className, selected, children, ...props }, ref) => (
  <DropdownPrimitive.Item
    ref={ref}
    className={cn(
      'relative flex cursor-pointer select-none items-center gap-2 rounded-sm px-2 py-1.5 outline-none transition-colors',
      'data-[highlighted]:bg-[hsl(var(--accent))] data-[highlighted]:text-[hsl(var(--accent-foreground))]',
      'data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
      className,
    )}
    {...props}
  >
    <span className="flex-1">{children}</span>
    {selected ? <Check className="h-4 w-4 text-[hsl(var(--primary))]" /> : null}
  </DropdownPrimitive.Item>
))
DropdownMenuItem.displayName = DropdownPrimitive.Item.displayName

export const DropdownMenuLabel = React.forwardRef<
  React.ComponentRef<typeof DropdownPrimitive.Label>,
  React.ComponentPropsWithoutRef<typeof DropdownPrimitive.Label>
>(({ className, ...props }, ref) => (
  <DropdownPrimitive.Label
    ref={ref}
    className={cn(
      'px-2 py-1 text-xs font-semibold uppercase tracking-wider text-[hsl(var(--muted-foreground))]',
      className,
    )}
    {...props}
  />
))
DropdownMenuLabel.displayName = DropdownPrimitive.Label.displayName

export const DropdownMenuSeparator = React.forwardRef<
  React.ComponentRef<typeof DropdownPrimitive.Separator>,
  React.ComponentPropsWithoutRef<typeof DropdownPrimitive.Separator>
>(({ className, ...props }, ref) => (
  <DropdownPrimitive.Separator
    ref={ref}
    className={cn('mx-1 my-1 h-px bg-[hsl(var(--border))]', className)}
    {...props}
  />
))
DropdownMenuSeparator.displayName = DropdownPrimitive.Separator.displayName
