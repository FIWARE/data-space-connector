/** A modal yes/no question; focus starts on the safe choice (cancel), stays inside and returns to the opener. */
import { useEffect, useId, useRef, type KeyboardEvent, type MouseEvent, type ReactNode } from 'react'

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

interface ConfirmDialogProps {
  title: string
  /** The question's explanation; it is the dialog's description. */
  children: ReactNode
  cancelLabel: string
  confirmLabel: string
  onCancel: () => void
  onConfirm: () => void
}

/** Asks before a step with a consequence; the caller unmounts it on either answer. */
export function ConfirmDialog({ title, children, cancelLabel, confirmLabel, onCancel, onConfirm }: ConfirmDialogProps) {
  const id = useId()
  const dialog = useRef<HTMLDivElement>(null)
  const cancel = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    const opener = document.activeElement
    cancel.current?.focus()
    return () => {
      if (opener instanceof HTMLElement) opener.focus()
    }
  }, [])

  function keyDown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault()
      onCancel()
      return
    }
    if (event.key !== 'Tab' || !dialog.current) return
    const focusable = Array.from(dialog.current.querySelectorAll<HTMLElement>(FOCUSABLE))
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    const active = document.activeElement
    // Focus on the dialog itself (after a click on its text) is outside the cycle; Shift+Tab would leave it.
    const outside = !focusable.some((el) => el === active)
    const wrapTo = event.shiftKey ? (outside || active === first ? last : undefined) : outside || active === last ? first : undefined
    if (wrapTo) {
      event.preventDefault()
      wrapTo.focus()
    }
  }

  // A click on the backdrop would move focus behind the modal, out of reach of the trap and of Escape.
  function backdropMouseDown(event: MouseEvent) {
    if (!dialog.current?.contains(event.target as Node)) event.preventDefault()
  }

  return (
    <div className="overlay" onMouseDown={backdropMouseDown}>
      <div
        ref={dialog}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${id}-title`}
        aria-describedby={`${id}-body`}
        className="dialog panel"
        onKeyDown={keyDown}
      >
        <h3 id={`${id}-title`}>{title}</h3>
        <div id={`${id}-body`}>{children}</div>
        <div className="actions">
          <button ref={cancel} type="button" className="sec" onClick={onCancel}>
            {cancelLabel}
          </button>
          <button type="button" className="primary" onClick={onConfirm}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
