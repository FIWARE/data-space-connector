import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { VersionSource, type LoadReport } from '../api/load'
import { Chrome } from '../chrome/Chrome'
import { STAGES } from '../stages'
import { initialState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { REPORT } from '../test/api'

function renderReview(loaded?: LoadReport) {
  return render(
    <StoreProvider initial={{ ...initialState, entered: true, stage: StageId.Review, visited: [StageId.Review], loaded }}>
      <Chrome stages={STAGES} />
    </StoreProvider>,
  )
}

describe('Review', () => {
  it('keeps the Epic 6 placeholder without a loaded file', () => {
    renderReview()
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Review — not available yet')
    expect(screen.getByText(/Epic 6/)).toBeInTheDocument()
  })

  it('heads the report with what was understood and carried over', () => {
    renderReview(REPORT)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Opened provider.yaml: understood 2, carried over untouched 3')
    expect(screen.getByText(/Chart data-space-connector 10.16.0/)).toHaveTextContent('from the file’s dsc-config header')
  })

  it('says why the latest bundle was used', () => {
    renderReview({ ...REPORT, chartVersionFrom: VersionSource.Latest, chartVersionNote: 'the file has no dsc-config header' })
    expect(screen.getByText(/Chart data-space-connector/)).toHaveTextContent('latest bundle, because the file has no dsc-config header')
  })

  it('shows the counts per class', () => {
    renderReview(REPORT)
    const counts = within(screen.getByLabelText('Counts'))
    expect(counts.getAllByRole('definition').map((dd) => dd.textContent)).toEqual(['1', '1', '2', '1'])
  })

  it('groups unknown entries by top-level key and lists skipped pairs with their reason', () => {
    renderReview(REPORT)
    expect(screen.getByText('keycloak').closest('summary')).toHaveTextContent('keycloak (1)')
    expect(screen.getByText('marketplace').closest('summary')).toHaveTextContent('marketplace (1)')
    const skipped = screen.getByText(/Keys Helm ignores or cannot address/).closest('details')
    expect(skipped).not.toHaveAttribute('open')
    expect(skipped).toHaveTextContent('shadowed by line 31')
    expect(skipped).toHaveTextContent('30')
  })

  it('lists derived answers by stage and the marker answers not derived', () => {
    renderReview(REPORT)
    expect(screen.getByRole('heading', { level: 4, name: /Features/ })).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 4, name: /Foundations/ })).toBeInTheDocument()
    expect(screen.getByText(/the profile has to supply them: did.value/)).toBeInTheDocument()
  })

  it('jumps to the stage of a derived answer', async () => {
    renderReview(REPORT)
    await userEvent.click(within(screen.getByRole('heading', { level: 4, name: /Foundations/ })).getByRole('button', { name: /go/ }))
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Foundations — not available yet')
    expect(screen.getByText('domain.base')).toBeInTheDocument()
  })
})
