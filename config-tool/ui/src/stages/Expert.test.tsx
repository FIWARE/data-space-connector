import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { Chrome } from '../chrome/Chrome'
import { STAGES } from '../stages'
import { initialState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { REPORT } from '../test/api'

function renderLoaded() {
  return render(
    <StoreProvider initial={{ ...initialState, entered: true, stage: StageId.Review, visited: [StageId.Review], loaded: REPORT }}>
      <Chrome stages={STAGES} />
    </StoreProvider>,
  )
}

function expertButton() {
  return within(screen.getByRole('navigation', { name: 'Stages' })).getByRole('button', { name: /Expert/ })
}

describe('Expert', () => {
  it('opens from the rail with the overrides and a disabled YAML editor', async () => {
    renderLoaded()
    expect(expertButton()).toBeEnabled()
    await userEvent.click(expertButton())
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Expert')
    expect(screen.getByRole('heading', { level: 3, name: 'Overrides (3)' })).toBeInTheDocument()
    expect(screen.getByText(/kept verbatim and written back in place/)).toBeInTheDocument()
    expect(screen.getByText(/Keys Helm ignores or cannot address/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Edit YAML' })).toHaveAttribute('title', 'Not available yet (Epic 5)')
  })

  it('closes on a stage jump', async () => {
    renderLoaded()
    await userEvent.click(expertButton())
    await userEvent.click(screen.getByRole('button', { name: /1 Target/ }))
    expect(screen.getByRole('heading', { level: 1 })).not.toHaveTextContent('Expert')
  })
})
