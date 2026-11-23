import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { LoadReport } from '../api/load'
import { initialState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { REPORT } from '../test/api'
import { DerivedAnswers, READ_ONLY_HINT } from './DerivedAnswers'

const CUSTOM_HINT = 'Pick a role first.'

function renderFor(stage: StageId, loaded?: LoadReport, hint?: string) {
  return render(
    <StoreProvider initial={{ ...initialState, entered: true, loaded }}>
      <DerivedAnswers stage={stage} hint={hint} />
    </StoreProvider>,
  )
}

describe('DerivedAnswers', () => {
  it.each([
    [StageId.Features, 'identity.did.enabled', 'on'],
    [StageId.Foundations, 'domain.base', 'example.org'],
  ])('lists on %s only its own answers', (stage, id, value) => {
    renderFor(stage, REPORT)
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(1)
    expect(items[0]).toHaveTextContent(`${id} = ${value}`)
  })

  it.each([
    ['the default hint', undefined, READ_ONLY_HINT],
    ['a given hint', CUSTOM_HINT, CUSTOM_HINT],
  ])('shows %s under the answers', (_name, hint, shown) => {
    renderFor(StageId.Features, REPORT, hint)
    expect(screen.getByText(shown)).toBeInTheDocument()
  })

  it.each([
    ['no file is loaded', undefined],
    ['the stage has no answers', REPORT],
  ])('renders nothing when %s', (_name, loaded) => {
    const { container } = renderFor(StageId.Role, loaded)
    expect(container).toBeEmptyDOMElement()
  })
})
