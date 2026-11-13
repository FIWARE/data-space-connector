import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from '../App'
import { STAGES, type StageDef } from '../stages'
import type { Issues } from '../state/issues'
import { initialState } from '../state/reducer'
import { StoreProvider } from '../state/store'
import { Chrome } from './Chrome'

const LABELS = ['Target', 'Role', 'Foundations', 'Infrastructure', 'Features', 'Configuration', 'Review', 'Install']
const ENTERED = { ...initialState, entered: true, visited: [initialState.stage] }

function renderChrome(stages: readonly StageDef[] = STAGES) {
  return render(
    <StoreProvider initial={ENTERED}>
      <Chrome stages={stages} />
    </StoreProvider>,
  )
}

function railButtons() {
  return within(screen.getByRole('navigation', { name: 'Stages' })).getAllByRole('button')
}

function railButton(index: number) {
  const button = railButtons()[index]
  if (!button) throw new Error(`no rail entry ${index}`)
  return button
}

function nextButton() {
  return screen.getByRole('button', { name: /^Next/ })
}

describe('App', () => {
  it('shows S0 until New configuration is chosen, then the chrome on Target', async () => {
    render(<App />)
    expect(screen.queryByRole('navigation', { name: 'Stages' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: /New configuration/ }))
    expect(screen.getByRole('button', { name: /1 Target/ })).toHaveAttribute('aria-current', 'step')
  })
})

describe('Chrome', () => {
  it('lists the 8 stages in order, then the disabled expert and save entries', () => {
    renderChrome()
    const buttons = railButtons()
    expect(buttons.slice(0, LABELS.length).map((b) => b.textContent)).toEqual(LABELS.map((label, i) => expect.stringContaining(`${i + 1} ${label}`)))
    expect(buttons.slice(LABELS.length).map((b) => [b.textContent, (b as HTMLButtonElement).disabled])).toEqual([
      ['⌘E Expert: YAML', true],
      ['⌘S Save profile', true],
    ])
  })

  it.each(STAGES.map((stage, index) => [stage.label, index, stage] as const))('selects %s from the rail', async (label, index, stage) => {
    renderChrome()
    await userEvent.click(railButton(index))
    expect(railButton(index)).toHaveAttribute('aria-current', 'step')
    const heading = screen.getByRole('heading', { level: 1 })
    if (stage.component) {
      expect(heading).not.toHaveTextContent('not available yet')
    } else {
      expect(heading).toHaveTextContent(`${label} — not available yet`)
      expect(screen.getByText(new RegExp(stage.comingIn ?? ''))).toBeInTheDocument()
    }
  })

  it.each<[string, Issues[], string]>([
    ['no selectors', [], '⚠ 0 warnings✖ 0 errors○ 0 open questions'],
    ['one selector', [{ warnings: 2, errors: 1, open: 3 }], '⚠ 2 warnings✖ 1 errors○ 3 open questions'],
    ['summed selectors', [{ warnings: 1, errors: 0, open: 1 }, { warnings: 4, errors: 2, open: 0 }], '⚠ 5 warnings✖ 2 errors○ 1 open questions'],
  ])('counts issues with %s', (_name, results, expected) => {
    const stages = STAGES.map((stage, i) => {
      const result = results[i]
      return result ? { ...stage, selector: () => result } : stage
    })
    renderChrome(stages)
    expect(screen.getByLabelText('Validation')).toHaveTextContent(expected)
  })

  it('marks a stage with errors in the rail', () => {
    const stages = STAGES.map((stage, i) => (i === 1 ? { ...stage, selector: () => ({ warnings: 0, errors: 1, open: 0 }) } : stage))
    renderChrome(stages)
    expect(within(railButton(1)).getByLabelText('has errors')).toHaveTextContent('✖')
  })

  it('walks the rail with Next and back with Back, naming the next stage', async () => {
    renderChrome()
    const back = screen.getByRole('button', { name: 'Back' })
    expect(back).toBeDisabled()
    for (const label of LABELS.slice(1)) {
      expect(nextButton()).toHaveTextContent(`Next: ${label}`)
      await userEvent.click(nextButton())
      expect(screen.getByRole('button', { name: new RegExp(`\\d ${label}`) })).toHaveAttribute('aria-current', 'step')
    }
    expect(nextButton()).toBeDisabled()
    for (const label of LABELS.slice(0, -1).reverse()) {
      await userEvent.click(back)
      expect(screen.getByRole('button', { name: new RegExp(`\\d ${label}`) })).toHaveAttribute('aria-current', 'step')
    }
    expect(back).toBeDisabled()
  })
})
