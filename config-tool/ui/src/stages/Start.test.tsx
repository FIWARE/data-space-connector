import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from '../App'
import { mockApi, REPORT, refusedUpload } from '../test/api'
import { OPEN_INPUT_LABEL } from './Start'

const CARDS = ['New configuration', 'Open values.yaml', 'Open profile', 'Try it locally']
const HTTP_PAYLOAD_TOO_LARGE = 413
const HTTP_UNPROCESSABLE = 422

function pick(name = 'provider.yaml') {
  return userEvent.upload(screen.getByLabelText(OPEN_INPUT_LABEL), new File(['a: 1\n'], name, { type: 'application/yaml' }))
}

function cards() {
  return CARDS.map((title) => screen.getByRole('button', { name: new RegExp(title) }))
}

describe('Start', () => {
  it('shows four entries with New configuration and Open values.yaml enabled', () => {
    render(<App />)
    const enabled = cards().filter((card) => card.getAttribute('aria-disabled') !== 'true')
    expect(enabled.map((card) => within(card).getByRole('heading').textContent)).toEqual(['＋ New configuration', '📂 Open values.yaml'])
  })

  it.each([
    ['Open profile', 'CT-4'],
    ['Try it locally', 'CT-5'],
  ])('names the ticket that enables %s', (title, ticket) => {
    render(<App />)
    expect(screen.getByRole('button', { name: new RegExp(title) })).toHaveTextContent(`Coming in ${ticket}`)
  })

  it.each(['Open values.yaml', 'Open profile', 'Try it locally'])('stays on S0 when %s is clicked', async (title) => {
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: new RegExp(title) }))
    expect(screen.queryByRole('navigation', { name: 'Stages' })).toBeNull()
  })

  it('links the documentation and the release notes', () => {
    render(<App />)
    expect(screen.getByRole('link', { name: 'Documentation ↗' })).toHaveAttribute('target', '_blank')
    expect(screen.getByRole('link', { name: 'Release notes ↗' })).toHaveAttribute('href', expect.stringContaining('release-notes'))
  })

  it('reaches S1 Target from New configuration', async () => {
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: /New configuration/ }))
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Where is this going?')
  })

  it('accepts only YAML files', () => {
    render(<App />)
    expect(screen.getByLabelText(OPEN_INPUT_LABEL)).toHaveAttribute('accept', '.yaml,.yml')
  })

  it('lands on Review with the report after Open values.yaml', async () => {
    render(<App />)
    await pick()
    expect(await screen.findByRole('heading', { level: 1 })).toHaveTextContent(`Opened ${REPORT.source}: understood 2, carried over untouched 3`)
    expect(screen.getByRole('button', { name: /7 Review/ })).toHaveAttribute('aria-current', 'step')
  })

  it.each([
    [HTTP_PAYLOAD_TOO_LARGE, 'Cannot open big.yaml: it is larger than 8 MiB.'],
    [HTTP_UNPROCESSABLE, `Cannot open big.yaml: ${refusedUpload('big.yaml', HTTP_UNPROCESSABLE)}`],
  ])('stays on S0 and shows why when the upload fails with %i', async (status, message) => {
    mockApi(undefined, undefined, () => status)
    render(<App />)
    await pick('big.yaml')
    expect(await screen.findByRole('alert')).toHaveTextContent(message)
    expect(screen.queryByRole('navigation', { name: 'Stages' })).toBeNull()
    expect(screen.getByRole('button', { name: /Open values.yaml/ })).not.toHaveAttribute('aria-busy', 'true')
  })
})
