import * as React from "react"
import { Dialog as DialogPrimitive } from "radix-ui"
import { X } from "lucide-react"

import { cn } from "@/lib/utils"

function Dialog({ ...props }: React.ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />
}

function DialogPortal({ ...props }: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />
}

function DialogOverlay({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        // Barely-there on purpose -- see the DialogContent comment below.
        // At bg-black/70 the overlay contradicted its own "stays out of
        // the way, doesn't occlude" design intent: it darkened the ENTIRE
        // page 70%, diagram included, every time the drawer opened.
        "fixed inset-0 z-50 bg-black/10 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
        className
      )}
      {...props}
    />
  )
}

// A side drawer, not a centered modal: this hosts atom-detail prose the
// user browses alongside the grid/list it was opened from (compare,
// re-open a sibling, chain through cross-referenced lex-ids) rather than
// a one-shot action — so it stays out of the way of the content behind
// it instead of occluding it, and closing it doesn't lose your place.
//
// title is required, not optional: every real caller has one (an atom's
// name, a gap cell's row/col label) and a screen reader announces this as
// a bare, nameless "dialog" without it (axe: aria-dialog-name). It's
// rendered sr-only rather than as a visible heading -- the drawer's own
// content already opens with its own heading treatment, and doubling it
// would just repeat the same text twice for sighted users.
function DialogContent({
  className,
  children,
  title,
  description,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Content> & { title: string; description?: string }) {
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        data-slot="dialog-content"
        className={cn(
          "fixed inset-y-0 right-0 z-50 flex h-full w-full max-w-md flex-col border-l border-rule-light bg-card font-mono text-[11px] text-ink shadow-lg data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right",
          className
        )}
        {...props}
      >
        <DialogPrimitive.Title className="sr-only">{title}</DialogPrimitive.Title>
        {description && <DialogPrimitive.Description className="sr-only">{description}</DialogPrimitive.Description>}
        {children}
        {/* The icon stays 16px (a bigger glyph would compete with the drawer's
            own content), but the hit area is a real 44x44 touch target --
            inset via padding/position rather than by growing the icon. */}
        <DialogPrimitive.Close className="absolute top-1 right-1 flex h-11 w-11 items-center justify-center rounded-sm text-ink-faint hover:bg-bg-well hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-primary/60 focus-visible:ring-inset">
          <X className="size-4" />
          <span className="sr-only">Close</span>
        </DialogPrimitive.Close>
      </DialogPrimitive.Content>
    </DialogPortal>
  )
}

function DialogBody({ className, ...props }: React.ComponentProps<"div">) {
  return <div data-slot="dialog-body" className={cn("min-h-0 flex-1 overflow-y-auto p-4", className)} {...props} />
}

export { Dialog, DialogContent, DialogBody }
