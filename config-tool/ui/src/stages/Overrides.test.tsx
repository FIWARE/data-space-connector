import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { REPORT } from '../test/api'
import { Overrides } from './Overrides'

describe('Overrides', () => {
  it('lists unknown leaves by top-level key, then the skipped pairs, all collapsed', () => {
    const { container } = render(<Overrides report={REPORT} />)
    const summaries = [...container.querySelectorAll('summary')].map((summary) => summary.textContent)
    expect(summaries).toEqual(['keycloak (1)', 'marketplace (1)', 'Keys Helm ignores or cannot address (1)'])
    expect(container.querySelectorAll('details[open]')).toHaveLength(0)
  })

  it('shows path, line, value and note of each entry', () => {
    render(<Overrides report={REPORT} />)
    const row = screen.getByText('marketplace.replicas').closest('tr')
    expect(row).toHaveTextContent('20')
    expect(row).toHaveTextContent('differs from market.replicas = 1')
    expect(screen.getByText('mongodb.enabled').closest('tr')).toHaveTextContent('shadowed by line 31')
  })

  it('says so when nothing is carried over', () => {
    render(<Overrides report={{ ...REPORT, unknown: [], skipped: [] }} />)
    expect(screen.getByText(/the tool reads every key of provider.yaml/)).toBeInTheDocument()
  })
})
