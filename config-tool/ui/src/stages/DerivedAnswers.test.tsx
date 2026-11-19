import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { LoadReport } from '../api/load'
import { initialState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { REPORT } from '../test/api'
import { DerivedAnswers } from './DerivedAnswers'

function renderFor(stage: StageId, loaded?: LoadReport) {
  return render(
    <StoreProvider initial={{ ...initialState, entered: true, loaded }}>
      <DerivedAnswers stage={stage} />
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
    ['no file is loaded', undefined],
    ['the stage has no answers', REPORT],
  ])('renders nothing when %s', (_name, loaded) => {
    const { container } = renderFor(StageId.Role, loaded)
    expect(container).toBeEmptyDOMElement()
  })
})
