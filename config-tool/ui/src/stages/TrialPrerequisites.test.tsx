import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { QUICK_START_URL, SIZING_URL } from '../links'
import { StoreProvider } from '../state/store'
import { TrialPrerequisites } from './TrialPrerequisites'

function show() {
  render(
    <StoreProvider>
      <TrialPrerequisites onBack={vi.fn()} />
    </StoreProvider>,
  )
}

describe('TrialPrerequisites', () => {
  it.each([
    ['Quick Start ↗', QUICK_START_URL],
    ['Sizing ↗', SIZING_URL],
  ])('links %s in a new tab', (name, href) => {
    show()
    const link = screen.getByRole('link', { name })
    expect(link).toHaveAttribute('href', href)
    expect(link).toHaveAttribute('target', '_blank')
  })

  it('states no sizing figure, only links it', () => {
    show()
    expect(document.body.textContent).not.toMatch(/GiB|cores|GB/)
  })

  it('says the result is a development configuration', () => {
    show()
    expect(screen.getByText(/development configuration: self-signed certificates and chart-deployed databases/)).toBeInTheDocument()
  })
})
