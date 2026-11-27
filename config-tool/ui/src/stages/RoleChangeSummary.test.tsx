import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Dependency } from '../api/infrastructure'
import type { ChartSwitch, RoleCatalogue, RoleChange } from '../api/roles'
import { CERT_MANAGER, ROLES } from '../test/api'
import { RoleChangeSummary } from './RoleChangeSummary'
import { DROPPED, KEPT, KEPT_REASON, TURNS_OFF, TURNS_ON } from './roleChangeText'

const CHANGE: RoleChange = {
  fromRole: 'consumer',
  toRole: 'provider',
  enabled: ['data.scorpio', 'gone.feature'],
  disabled: ['identity.issuer'],
  kept: [
    { id: 'obs.tracing', enabled: true, source: 'user' },
    { id: 'ta.registry', enabled: false, source: 'loaded' },
  ],
}

const items = (heading: string) => within(screen.getByRole('list', { name: heading })).getAllByRole('listitem').map((li) => li.textContent)

describe('RoleChangeSummary', () => {
  it.each<[string, RoleCatalogue | undefined, Record<string, string[]>]>([
    [
      'with the catalogue names features by title, an unknown id by the id',
      ROLES,
      {
        [TURNS_ON]: ['Context broker', 'gone.feature'],
        [TURNS_OFF]: ['Credential issuer'],
        [KEPT]: [`Tracing: on, ${KEPT_REASON.user}`, `Trusted issuers registry: off, ${KEPT_REASON.loaded}`],
      },
    ],
    [
      'without the catalogue names every feature by its id',
      undefined,
      {
        [TURNS_ON]: ['data.scorpio', 'gone.feature'],
        [TURNS_OFF]: ['identity.issuer'],
        [KEPT]: [`obs.tracing: on, ${KEPT_REASON.user}`, `ta.registry: off, ${KEPT_REASON.loaded}`],
      },
    ],
  ])('%s', (_name, catalogue, sections) => {
    render(<RoleChangeSummary change={CHANGE} chartSwitch={null} catalogue={catalogue} />)
    for (const [heading, rows] of Object.entries(sections)) expect(items(heading)).toEqual(rows)
  })

  const SWITCH: ChartSwitch = {
    fromChart: 'data-space-connector',
    fromVersion: '10.16.0',
    toChart: 'trust-anchor',
    droppedChoices: [],
    carriedAnswers: [],
    droppedAnswers: ['org.displayName'],
    droppedInfrastructure: ['infrastructure.certManager'],
  }

  it.each<[string, Dependency[] | undefined, string]>([
    ['names a dropped infrastructure choice by its dependency’s title', [CERT_MANAGER], CERT_MANAGER.title],
    ['falls back to the dependency id without the old chart’s dependencies', undefined, 'certManager'],
  ])('%s', (_name, dependencies, label) => {
    render(<RoleChangeSummary change={CHANGE} chartSwitch={SWITCH} dependencies={dependencies} />)
    expect(items(DROPPED).slice(-2)).toEqual(['org.displayName', label])
  })
})
