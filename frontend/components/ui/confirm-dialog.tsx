"use client"

import { useRef, useSyncExternalStore, type ReactNode } from "react"
import { TriangleAlertIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  AlertDialog,
  AlertDialogClose,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { cn } from "@/lib/utils"

export interface ConfirmOptions {
  title: string
  description?: ReactNode
  confirmText?: string
  cancelText?: string
  /** Red confirm button for deleting or discarding work. */
  destructive?: boolean
}

type Request = ConfirmOptions & { resolve: (ok: boolean) => void }

let current: Request | null = null
const listeners = new Set<() => void>()

function emit() {
  listeners.forEach((l) => l())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function settle(ok: boolean) {
  const request = current
  if (!request) return
  current = null
  request.resolve(ok)
  emit()
}

/**
 * Promise-based replacement for window.confirm, rendered by
 * <ConfirmDialogHost /> (mounted once in Providers). Resolves false when the
 * dialog is dismissed, or when no host is mounted, so destructive actions
 * never run without an explicit confirmation.
 */
export function confirm(options: ConfirmOptions): Promise<boolean> {
  if (!listeners.size) return Promise.resolve(false)
  settle(false)
  return new Promise<boolean>((resolve) => {
    current = { ...options, resolve }
    emit()
  })
}

export function ConfirmDialogHost() {
  const request = useSyncExternalStore(
    subscribe,
    () => current,
    () => null
  )
  // Keep the last content rendered while the close animation runs.
  const last = useRef<Request | null>(null)
  if (request) last.current = request
  const shown = request ?? last.current

  return (
    <AlertDialog
      open={request !== null}
      onOpenChange={(open) => {
        if (!open) settle(false)
      }}
    >
      <AlertDialogContent>
        {shown && (
          <>
            <AlertDialogHeader className="flex-row items-start gap-3">
              {shown.destructive && (
                <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-destructive/10 text-destructive">
                  <TriangleAlertIcon className="size-4" />
                </span>
              )}
              <div className="space-y-1.5 pt-1">
                <AlertDialogTitle>{shown.title}</AlertDialogTitle>
                {shown.description && (
                  <AlertDialogDescription>
                    {shown.description}
                  </AlertDialogDescription>
                )}
              </div>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogClose render={<Button variant="outline" size="lg" />}>
                {shown.cancelText ?? "取消"}
              </AlertDialogClose>
              <Button
                size="lg"
                className={cn(
                  "px-4",
                  shown.destructive &&
                    "bg-destructive text-white hover:bg-destructive/90"
                )}
                onClick={() => settle(true)}
              >
                {shown.confirmText ?? "确定"}
              </Button>
            </AlertDialogFooter>
          </>
        )}
      </AlertDialogContent>
    </AlertDialog>
  )
}
