import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect } from 'react'
import { describe, expect, it } from 'vitest'
import { App } from '../App'
import { Chrome } from '../chrome/Chrome'
import type { FeatureState, Profile } from '../api/profile'
import type { PresetResult } from '../api/roles'
import { SIZING_URL } from '../links'
import { STAGES } from '../stages'
import { Environment, TargetType, type Target } from '../state/answers'
import { useStore } from '../state/context'
import { initialState, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { DEFAULT_DIR, MODEL, mockApi, REPORT, refusedRole, ROLES, type PresetBody, type RoleFakes, type SaveBody } from '../test/api'
import { DEPENDS_ON_SELECTION, NOTHING_LOCKED } from './Role'

const HTTP_UNPROCESSABLE = 422
const DSC = 'data-space-connector'
const TRUST_ANCHOR = 'trust-anchor'
const TWO_CHARTS = { charts: [...MODEL.charts, { name: TRUST_ANCHOR, versions: [{ version: '1.3.6', latest: true }] }] }
const DSC_MODEL = { chart: DSC, version: '10.16.0', revision: 2 }
const TA_MODEL = { chart: TRUST_ANCHOR, version: '1.3.6', revision: 1 }
const TITLES = ['Consumer', 'Provider', 'Consumer + Provider', 'Operator', 'Custom']
const PINNED: Target = {
  ...initialState.answers.target,
  type: TargetType.File,
  namespace: 'provider',
  releaseName: 'provider',
  environment: Environment.Production,
  chartVersion: '10.16.0',
}
const ON_ROLE: AppState = {
  ...initialState,
  entered: true,
  stage: StageId.Role,
  visited: [StageId.Target, StageId.Role],
  answers: { features: {}, target: PINNED },
  profileModel: DSC_MODEL,
}

const probe: { state: AppState } = { state: initialState }

function Probe() {
  const { state } = useStore()
  useEffect(() => {
    probe.state = state
  }, [state])
  return null
}

/** POST /api/preset as the server answers it: recommended features on, a switch for a role of another chart. */
function serverPreset(loaded: Record<string, boolean> = {}) {
  const bodies: PresetBody[] = []
  const preset = (body: PresetBody): PresetResult => {
    bodies.push(body)
    const role = ROLES.roles.find((r) => r.id === body.role)
    if (!role) throw new Error(`unknown role ${body.role}`)
    const from = body.profile.target
    const switched = role.chart !== from.chart
    const features: Record<string, FeatureState> = {}
    for (const [id, offer] of Object.entries(role.features)) {
      const fromFile = body.sourceSha256 ? loaded[id] : undefined
      if (fromFile !== undefined) features[id] = { enabled: fromFile, source: 'loaded' }
      else if (offer === 'recommended') features[id] = { enabled: true, source: 'preset' }
    }
    const operator = role.chart === TRUST_ANCHOR
    const profile: Profile = {
      ...body.profile,
      model: operator ? TA_MODEL : DSC_MODEL,
      catalogue: operator ? DSC_MODEL : undefined,
      target: { ...from, chart: role.chart, chartVersion: switched ? '' : from.chartVersion },
      role: role.id,
      features,
    }
    return { profile, chartSwitch: switched ? { fromChart: from.chart, fromVersion: from.chartVersion, toChart: role.chart } : null }
  }
  return { bodies, preset }
}

function renderRole(state: AppState = ON_ROLE, roles: RoleFakes = {}, saves: SaveBody[] = []) {
  const fetchMock = mockApi(undefined, TWO_CHARTS, undefined, { save: (body) => (saves.push(body), { path: body.path, profile: body.profile, notes: [] }) }, {}, roles)
  render(
    <StoreProvider initial={state}>
      <Chrome stages={STAGES} />
      <Probe />
    </StoreProvider>,
  )
  return fetchMock
}

const radios = () => screen.getAllByRole('radio')
const titleOf = (radio: HTMLElement) => within(radio).getByRole('heading').textContent?.replace('✓', '').trim()
function card(title: string): HTMLElement {
  const found = radios().find((radio) => titleOf(radio) === title)
  if (!found) throw new Error(`no card ${title}`)
  return found
}
const findCards = () => screen.findAllByRole('radio')
const notice = () => screen.getByRole('status')
const presetCalls = (fetchMock: ReturnType<typeof mockApi>) => fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/api/preset'))

describe('Role', () => {
  it('renders the five roles with chart, footprint and the sizing link', async () => {
    renderRole()
    expect((await findCards()).map(titleOf)).toEqual(TITLES)
    expect(screen.getByRole('radiogroup', { name: 'Role' })).toBeInTheDocument()
    expect(card('Consumer')).toHaveTextContent('1 feature · ~0.8Gi memory · 50m CPU')
    expect(card('Provider')).toHaveTextContent('2 features · ~2.3Gi memory · 200m CPU')
    expect(card('Operator')).toHaveTextContent('chart: trust-anchor')
    expect(card('Operator')).toHaveTextContent('1 feature · ~0.3Gi memory · 20m CPU')
    expect(card('Provider')).not.toHaveTextContent('chart:')
    expect(card('Custom')).toHaveTextContent(DEPENDS_ON_SELECTION)
    expect(radios().every((radio) => radio.getAttribute('aria-checked') === 'false')).toBe(true)
    expect(screen.getByRole('link', { name: 'Sizing ↗' })).toHaveAttribute('href', SIZING_URL)
  })

  it('moves with the arrow keys, keeps one tab stop and selects with Enter and Space', async () => {
    const { preset } = serverPreset()
    renderRole(ON_ROLE, { preset })
    await findCards()
    expect(radios().map((radio) => radio.tabIndex)).toEqual([0, -1, -1, -1, -1])
    card('Consumer').focus()
    await userEvent.keyboard('{ArrowRight}')
    expect(card('Provider')).toHaveFocus()
    await userEvent.keyboard('{Enter}')
    await waitFor(() => expect(card('Provider')).toHaveAttribute('aria-checked', 'true'))
    expect(radios().map((radio) => radio.tabIndex)).toEqual([-1, 0, -1, -1, -1])
    await userEvent.keyboard('{ArrowLeft}{ArrowLeft}')
    expect(card('Custom')).toHaveFocus()
    await userEvent.keyboard(' ')
    await waitFor(() => expect(card('Custom')).toHaveAttribute('aria-checked', 'true'))
    expect(card('Provider')).toHaveAttribute('aria-checked', 'false')
  })

  it.each(ROLES.roles.map((role) => [role.title, role.id]))('picks %s, sending the role and marking its card', async (title, id) => {
    const { bodies, preset } = serverPreset()
    renderRole(ON_ROLE, { preset })
    await findCards()
    await userEvent.click(card(title))
    await waitFor(() => expect(card(title)).toHaveAttribute('aria-checked', 'true'))
    expect(bodies).toMatchObject([{ role: id, profile: { target: { chart: DSC, chartVersion: '10.16.0' } } }])
    expect(probe.state.answers.role).toBe(id)
    expect(radios().filter((radio) => radio.getAttribute('aria-checked') === 'true')).toHaveLength(1)
    expect(screen.getByRole('radio', { checked: true })).toBe(card(title))
  })

  it('lists the features the role turns on and says nothing is locked', async () => {
    renderRole(ON_ROLE, { preset: serverPreset().preset })
    await findCards()
    await userEvent.click(card('Provider'))
    const selection = await screen.findByLabelText('Selected role')
    expect(within(selection).getByRole('heading')).toHaveTextContent('Provider turns on')
    expect(selection).toHaveTextContent('Credential issuer · Context broker')
    expect(selection).toHaveTextContent(NOTHING_LOCKED)
    expect(within(selection).queryByLabelText('From your values.yaml')).toBeNull()
  })

  it('names the dropped version when Operator switches the chart, and again when Custom switches back', async () => {
    renderRole(ON_ROLE, { preset: serverPreset().preset })
    await findCards()
    await userEvent.click(card('Operator'))
    await waitFor(() => expect(notice()).toHaveTextContent('Target chart switched from data-space-connector 10.16.0 to trust-anchor — pick its version on step 1.'))
    await userEvent.click(within(notice()).getByRole('button', { name: 'step 1' }))
    const version = screen.getByLabelText('Version')
    await within(version).findByRole('option', { name: '1.3.6 (latest)' })
    expect(version).toHaveValue('')
    await userEvent.selectOptions(version, '1.3.6')
    await userEvent.click(screen.getByRole('button', { name: /2 Role/ }))
    await findCards()
    expect(card('Operator')).toHaveAttribute('aria-checked', 'true')
    await userEvent.click(card('Custom'))
    await waitFor(() => expect(notice()).toHaveTextContent('Target chart switched from trust-anchor 1.3.6 to data-space-connector — pick its version on step 1.'))
    expect(probe.state.answers.target).toMatchObject({ chart: DSC, chartVersion: '' })
  })

  it('disables a role of another chart while a values file is loaded, without asking the server', async () => {
    const fetchMock = renderRole({ ...ON_ROLE, loaded: REPORT })
    await findCards()
    const operator = card('Operator')
    expect(operator).toHaveAttribute('aria-disabled', 'true')
    expect(operator).toHaveAccessibleDescription(
      'Operator deploys the trust-anchor chart; the opened values.yaml is a data-space-connector file. Start a new configuration to set it up.',
    )
    expect(card('Provider')).toHaveAttribute('aria-disabled', 'false')
    await userEvent.click(operator)
    expect(presetCalls(fetchMock)).toHaveLength(0)
    expect(operator).toHaveAttribute('aria-checked', 'false')
  })

  it('lists a toggle of the loaded file as from your values.yaml, and keeps the pick a 422 refuses', async () => {
    const { bodies, preset } = serverPreset({ 'data.scorpio': false })
    renderRole({ ...ON_ROLE, loaded: REPORT }, { preset: (body) => (body.role === 'consumer' ? HTTP_UNPROCESSABLE : preset(body)) })
    await findCards()
    await userEvent.click(card('Provider'))
    const fromFile = await screen.findByLabelText('From your values.yaml')
    expect(fromFile).toHaveTextContent('Context broker: off, from your values.yaml')
    expect(screen.getByLabelText('Selected role')).toHaveTextContent('Provider turns on')
    expect(bodies).toMatchObject([{ role: 'provider', sourceSha256: REPORT.sha256 }])
    await userEvent.click(card('Consumer'))
    expect(await screen.findByRole('alert')).toHaveTextContent(`Consumer: ${refusedRole(HTTP_UNPROCESSABLE)}`)
    expect(card('Provider')).toHaveAttribute('aria-checked', 'true')
    expect(card('Consumer')).toHaveAttribute('aria-checked', 'false')
    expect(probe.state.answers.role).toBe('provider')
  })

  it('counts a missing role as an open question until a role is picked', async () => {
    renderRole(ON_ROLE, { preset: serverPreset().preset })
    await findCards()
    expect(screen.getByLabelText('Validation')).toHaveTextContent('○ 1 open questions')
    await userEvent.click(screen.getByRole('button', { name: /1 Target/ }))
    expect(within(screen.getByRole('button', { name: /2 Role/ })).getByLabelText('has open questions')).toHaveTextContent('○')
    await userEvent.click(screen.getByRole('button', { name: /2 Role/ }))
    await userEvent.click(card('Provider'))
    await waitFor(() => expect(screen.getByLabelText('Validation')).toHaveTextContent('○ 0 open questions'))
    await userEvent.click(screen.getByRole('button', { name: /1 Target/ }))
    expect(within(screen.getByRole('button', { name: /2 Role/ })).getByLabelText('complete')).toHaveTextContent('✓')
  })
})

describe('Role: profiles', () => {
  it('saves an Operator pick with role, features and catalogue, and reopens it with the same pick', async () => {
    const saves: SaveBody[] = []
    renderRole(ON_ROLE, { preset: serverPreset().preset }, saves)
    await findCards()
    await userEvent.click(card('Operator'))
    await waitFor(() => expect(card('Operator')).toHaveAttribute('aria-checked', 'true'))
    await userEvent.keyboard('{Control>}s{/Control}')
    await userEvent.click(within(await screen.findByRole('dialog', { name: 'Save profile' })).getByRole('button', { name: 'Save' }))
    const [{ path, profile: saved }] = saves as [SaveBody]
    expect(saves).toHaveLength(1)
    expect(saved).toMatchObject({
      role: 'operator',
      features: { 'ta.registry': { enabled: true, source: 'preset' } },
      catalogue: DSC_MODEL,
      target: { chart: TRUST_ANCHOR, chartVersion: '' },
    })

    cleanup()
    mockApi(undefined, TWO_CHARTS, undefined, {
      recent: () => ({ entries: [{ path, name: 'provider', chartVersion: '', savedAt: new Date().toISOString() }], defaultDir: DEFAULT_DIR, notes: [] }),
      open: (opened) => ({ path: opened, profile: saved, notes: [] }),
    })
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: /^provider/ }))
    await userEvent.click(await screen.findByRole('button', { name: /2 Role/ }))
    await findCards()
    expect(card('Operator')).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByLabelText('Selected role')).toHaveTextContent('Operator turns on')
    expect(screen.getByLabelText('Selected role')).toHaveTextContent('Trusted issuers registry')
  })
})
