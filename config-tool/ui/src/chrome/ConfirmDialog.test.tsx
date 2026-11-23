import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { ConfirmDialog } from './ConfirmDialog'

const TITLE = 'Turn off Credential issuer?'
const CONSEQUENCE = 'Participants cannot get credentials from you.'
const CANCEL = 'Keep it on'
const CONFIRM = 'Turn it off'

/** An opener button that shows the dialog and closes it on either answer. */
function Harness({ onCancel = () => {}, onConfirm = () => {} }: { onCancel?: () => void; onConfirm?: () => void }) {
  const [open, setOpen] = useState(false)
  const close = (answer: () => void) => () => {
    answer()
    setOpen(false)
  }
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Open
      </button>
      {open && (
        <ConfirmDialog title={TITLE} cancelLabel={CANCEL} confirmLabel={CONFIRM} onCancel={close(onCancel)} onConfirm={close(onConfirm)}>
          <p>{CONSEQUENCE}</p>
        </ConfirmDialog>
      )}
    </>
  )
}

async function openDialog(props: Parameters<typeof Harness>[0] = {}) {
  render(<Harness {...props} />)
  await userEvent.click(screen.getByRole('button', { name: 'Open' }))
  return screen.getByRole('dialog')
}

describe('ConfirmDialog', () => {
  it('is a modal named by its title and described by its body, with focus on the cancel button', async () => {
    const dialog = await openDialog()
    expect(dialog).toHaveAttribute('aria-modal', 'true')
    expect(dialog).toHaveAccessibleName(TITLE)
    expect(dialog).toHaveAccessibleDescription(CONSEQUENCE)
    expect(screen.getByRole('button', { name: CANCEL })).toHaveFocus()
  })

  it('keeps Tab and Shift+Tab inside, wrapping at both ends', async () => {
    await openDialog()
    const cancel = screen.getByRole('button', { name: CANCEL })
    const confirm = screen.getByRole('button', { name: CONFIRM })
    await userEvent.tab()
    expect(confirm).toHaveFocus()
    await userEvent.tab()
    expect(cancel).toHaveFocus()
    await userEvent.tab({ shift: true })
    expect(confirm).toHaveFocus()
  })

  it.each<[string, () => Promise<unknown>, 'onCancel' | 'onConfirm']>([
    ['Escape', () => userEvent.keyboard('{Escape}'), 'onCancel'],
    ['the cancel button', () => userEvent.click(screen.getByRole('button', { name: CANCEL })), 'onCancel'],
    ['the confirm button', () => userEvent.click(screen.getByRole('button', { name: CONFIRM })), 'onConfirm'],
  ])('answers once on %s and returns focus to the opener', async (_, act, called) => {
    const handlers = { onCancel: vi.fn(), onConfirm: vi.fn() }
    await openDialog(handlers)
    await act()
    expect(handlers[called]).toHaveBeenCalledTimes(1)
    expect(handlers[called === 'onCancel' ? 'onConfirm' : 'onCancel']).not.toHaveBeenCalled()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open' })).toHaveFocus()
  })
})
