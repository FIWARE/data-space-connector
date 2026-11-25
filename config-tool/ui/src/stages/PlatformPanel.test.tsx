import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { Finding } from '../api/render'
import { Chrome } from '../chrome/Chrome'
import { repositoryDocUrl } from '../links'
import { STAGES } from '../stages'
import { UNANSWERED, type AnswerValue, type Environment } from '../state/answers'
import { initialState, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { mockApi, PLATFORM_FOUNDATIONS, RESOURCE_PROFILE, RESOURCES, renderWithFindings, type FoundationFakes } from '../test/api'
import { CUSTOM_PROFILE, measuredCaption, PLATFORM_PANEL, PROFILE_DERIVED, PROFILE_PENDING, SIZING_LINK } from './platformText'

const JAVA_MEMORY_LIMIT: Finding = {
  rule: 'java-memory-limit',
  severity: 'warning',
  stage: 'foundations',
  field: `answers.${RESOURCE_PROFILE.id}`,
  message: 'Without a memory limit, the JVM sizes its heap from the node: Keycloak, Scorpio.',
  doc: 'doc/deployment-integration/roles/PREREQUISITES.md#set-memory-limits-on-the-java-components',
}

function onFoundations(environment: Environment | typeof UNANSWERED, values: Record<string, AnswerValue> = {}): AppState {
  return {
    ...initialState,
    entered: true,
    stage: StageId.Foundations,
    visited: [StageId.Target, StageId.Role, StageId.Foundations],
    answers: {
      ...initialState.answers,
      role: 'provider',
      target: { ...initialState.answers.target, environment },
      values,
    },
  }
}

function renderPanel(state: AppState, fakes: FoundationFakes = {}) {
  mockApi(undefined, undefined, undefined, {}, {}, {}, { foundations: () => PLATFORM_FOUNDATIONS, ...fakes })
  render(
    <StoreProvider initial={state}>
      <Chrome stages={STAGES} />
    </StoreProvider>,
  )
}

const panel = () => screen.findByRole('group', { name: PLATFORM_PANEL })
const option = (value: string) => RESOURCE_PROFILE.options.find((o) => o.value === value)?.label ?? value

/** The table's rows as [component, requests, limits]. */
function rowsOf(table: HTMLElement): string[][] {
  return within(table)
    .getAllByRole('row')
    .slice(1)
    .map((row) => [...row.querySelectorAll('th, td')].map((c) => c.textContent ?? ''))
}

function expectedRows(profile: string): string[][] {
  return RESOURCES.components.map((c) => {
    const spec = RESOURCES.profiles[profile]?.[c.id]
    if (!spec) throw new Error(`fixture lacks ${profile} ${c.id}`)
    return [c.label, `${spec.requests.cpu} CPU, ${spec.requests.memory}`, `${spec.limits.cpu} CPU, ${spec.limits.memory}`]
  })
}

describe('PlatformPanel', () => {
  it.each([
    {
      name: 'derived development',
      environment: 'development',
      values: {},
      profile: 'development',
      derived: true,
    },
    {
      name: 'derived production',
      environment: 'production',
      values: {},
      profile: 'production',
      derived: true,
    },
    {
      name: 'given production',
      environment: 'development',
      values: { [RESOURCE_PROFILE.id]: 'production' },
      profile: 'production',
      derived: false,
    },
    {
      name: 'given custom',
      environment: 'production',
      values: { [RESOURCE_PROFILE.id]: 'custom' },
      profile: 'custom',
      derived: false,
    },
    {
      name: 'unresolved',
      environment: UNANSWERED,
      values: {},
      profile: undefined,
      derived: false,
    },
  ] as const)('previews the $name profile', async ({ environment, values, profile, derived }) => {
    renderPanel(onFoundations(environment, values))
    const group = await panel()
    const checked = within(group).queryByRole('radio', { checked: true })
    const table = within(group).queryByRole('table')
    if (profile === undefined) {
      expect(checked).toBeNull()
      expect(within(group).getByText(PROFILE_PENDING)).toBeInTheDocument()
      expect(table).toBeNull()
      return
    }
    expect(checked).toHaveAccessibleName(option(profile))
    expect(within(group).queryByText(PROFILE_DERIVED) !== null).toBe(derived)
    if (profile === 'custom') {
      expect(within(group).getByText(CUSTOM_PROFILE)).toBeInTheDocument()
      expect(table).toBeNull()
      return
    }
    expect(table).not.toBeNull()
    expect(rowsOf(table as HTMLElement)).toEqual(expectedRows(profile))
    expect(within(table as HTMLElement).getByText(new RegExp(measuredCaption(RESOURCES.measuredOn)))).toBeInTheDocument()
    expect(within(table as HTMLElement).getByRole('link', { name: SIZING_LINK })).toHaveAttribute('href', repositoryDocUrl(RESOURCES.doc))
  })

  it('switches the preview when a profile is chosen', async () => {
    renderPanel(onFoundations('development'))
    const group = await panel()
    await userEvent.click(within(group).getByRole('radio', { name: option('custom') }))
    expect(within(group).getByText(CUSTOM_PROFILE)).toBeInTheDocument()
    expect(within(group).queryByText(PROFILE_DERIVED)).toBeNull()
    await userEvent.click(within(group).getByRole('radio', { name: option('production') }))
    expect(rowsOf(within(group).getByRole('table'))).toEqual(expectedRows('production'))
  })

  it('shows the java-memory-limit finding on the field with its guidance', async () => {
    // The fake reports findings once the base domain is given.
    renderPanel(
      onFoundations('production', {
        'domain.base': 'example.org',
        [RESOURCE_PROFILE.id]: 'custom',
      }),
      { render: renderWithFindings([JAVA_MEMORY_LIMIT]) },
    )
    const note = (await screen.findByText(new RegExp(JAVA_MEMORY_LIMIT.message))).closest('p') as HTMLElement
    expect(note).toHaveTextContent(`Warning: ${JAVA_MEMORY_LIMIT.message}`)
    expect(within(note).getByRole('link')).toHaveAttribute('href', repositoryDocUrl(JAVA_MEMORY_LIMIT.doc))
    expect(screen.getByRole('group', { name: RESOURCE_PROFILE.label })).toHaveAccessibleDescription(expect.stringContaining(JAVA_MEMORY_LIMIT.message))
  })

  it('leaves the panel out for a bundle without platform foundations', async () => {
    renderPanel(onFoundations('production'), {
      foundations: () => ({
        ...PLATFORM_FOUNDATIONS,
        foundations: PLATFORM_FOUNDATIONS.foundations.filter((f) => f.group !== 'platform'),
        resources: null,
      }),
    })
    await screen.findByRole('heading', { name: /Foundations/ })
    expect(screen.queryByRole('group', { name: PLATFORM_PANEL })).toBeNull()
  })
})
