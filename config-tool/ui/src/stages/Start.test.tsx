import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from '../App'

const CARDS = ['New configuration', 'Open values.yaml', 'Open profile', 'Try it locally']

function cards() {
  return CARDS.map((title) => screen.getByRole('button', { name: new RegExp(title) }))
}

describe('Start', () => {
  it('shows four entries with only New configuration enabled', () => {
    render(<App />)
    const enabled = cards().filter((card) => card.getAttribute('aria-disabled') !== 'true')
    expect(enabled.map((card) => within(card).getByRole('heading').textContent)).toEqual(['＋ New configuration'])
  })

  it.each([
    ['Open values.yaml', 'CT-3'],
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
})
