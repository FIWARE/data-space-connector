import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect } from 'react'
import { describe, expect, it } from 'vitest'
import { Chrome } from '../chrome/Chrome'
import { STAGES } from '../stages'
import { Environment, TargetType, type Session, type Target } from '../state/answers'
import { useStore } from '../state/context'
import type { Profile } from '../api/profile'
import type { ChartSwitch } from '../api/roles'
import { initialState, reducer, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { HTTP_NOT_FOUND, MODEL, mockApi, refusedKubeconfig, type ContextsAnswer } from '../test/api'

const KUBECONFIG = '/home/me/.kube/config'
const ONE_FILE: ContextsAnswer = { paths: [KUBECONFIG], contexts: ['dev', 'prod'], current: 'dev' }
const TWO_FILES: ContextsAnswer = { paths: ['/a/config', '/b/config'], contexts: ['dev', 'prod'], current: 'prod' }
const NO_FILE: ContextsAnswer = { paths: [], contexts: [], current: '' }

const probe: { state: AppState } = { state: initialState }

function Probe() {
  const { state } = useStore()
  useEffect(() => {
    probe.state = state
  }, [state])
  return null
}

// A picked role keeps the footer about Target; the role cases below start from their own state.
function renderTarget(target: Partial<Target> = {}, session: Session = {}) {
  return renderState({
    ...initialState,
    entered: true,
    visited: [StageId.Target],
    answers: { infrastructure: {}, values: {}, role: 'provider', features: {}, target: { ...initialState.answers.target, ...target } },
    session,
  })
}

function renderState(initial: AppState) {
  return render(
    <StoreProvider initial={initial}>
      <Chrome stages={STAGES} />
      <Probe />
    </StoreProvider>,
  )
}

function railEntry(index: number) {
  const stage = STAGES[index]
  if (!stage) throw new Error(`no stage ${index}`)
  return screen.getByRole('button', { name: new RegExp(`${index + 1} ${stage.label}`) })
}

const DSC = 'data-space-connector'
const TRUST_ANCHOR = 'trust-anchor'
const TWO_CHARTS = { charts: [...MODEL.charts, { name: TRUST_ANCHOR, versions: [{ version: '1.3.6', latest: true }] }] }
const COMPLETE: Target = {
  ...initialState.answers.target,
  type: TargetType.File,
  namespace: 'provider',
  releaseName: 'provider',
  environment: Environment.Production,
  chartVersion: '10.16.0',
}
const DSC_MODEL = { chart: DSC, version: '10.16.0', revision: 3 }
const TA_MODEL = { chart: TRUST_ANCHOR, version: '1.3.6', revision: 1 }

/** The state after the server answered POST /api/preset for `role` deploying `chart`. */
function applied(state: AppState, role: string, chart: string, chartSwitch: ChartSwitch | null): AppState {
  const operator = chart === TRUST_ANCHOR
  const profile: Profile = {
    format: 1,
    model: operator ? TA_MODEL : DSC_MODEL,
    catalogue: operator ? DSC_MODEL : undefined,
    target: { ...state.answers.target, chart, chartVersion: chartSwitch ? '' : state.answers.target.chartVersion },
    role,
    features: operator ? { 'ta.registry': { enabled: true, source: 'preset' } } : { 'data.scorpio': { enabled: true, source: 'preset' } },
    answers: {},
    open: [],
    secrets: {},
  }
  return reducer(state, { type: 'roleApplied', profile, chartSwitch, change: null })
}

const PINNED: AppState = { ...initialState, entered: true, visited: [StageId.Target, StageId.Role], answers: { infrastructure: {}, values: {}, features: {}, target: COMPLETE } }
const OPERATOR = applied(PINNED, 'operator', TRUST_ANCHOR, { fromChart: DSC, fromVersion: '10.16.0', toChart: TRUST_ANCHOR, droppedChoices: [], carriedAnswers: [], droppedAnswers: [] })
const SWITCHED_BACK = applied(OPERATOR, 'custom', DSC, { fromChart: TRUST_ANCHOR, fromVersion: '', toChart: DSC, droppedChoices: [], carriedAnswers: [], droppedAnswers: [] })
const PROVIDER = applied(PINNED, 'provider', DSC, null)

const kubeconfigField = () => screen.getByLabelText('Kubeconfig')
const contextSelect = () => screen.getByLabelText('Context')

describe('Target', () => {
  it.each<[string, ContextsAnswer, string, string, string | null]>([
    ['one reported path', ONE_FILE, KUBECONFIG, 'dev', null],
    ['two reported paths', TWO_FILES, '', 'prod', 'Read from KUBECONFIG, in this order: /a/config, /b/config'],
    ['no reported path', NO_FILE, '', '', 'No kubeconfig found (KUBECONFIG or ~/.kube/config). Type the path to one.'],
  ])('fills the kubeconfig field for %s', async (_name, answer, field, context, note) => {
    mockApi(() => answer)
    renderTarget({ type: TargetType.Cluster })
    await waitFor(() => expect(kubeconfigField()).toHaveValue(field))
    await waitFor(() => expect(contextSelect()).toHaveValue(context))
    const hint = /Read from KUBECONFIG|No kubeconfig found/
    if (note) expect(await screen.findByText(hint)).toHaveTextContent(note)
    else expect(screen.queryByText(hint)).toBeNull()
  })

  it.each<[string, string, string | null]>([
    ['a relative path', 'kube/dev.yaml', `Read from ${KUBECONFIG}`],
    ['the absolute path', KUBECONFIG, null],
  ])('names the file read for %s', async (_name, typed, note) => {
    mockApi((path) => (path === '' ? NO_FILE : ONE_FILE))
    renderTarget({ type: TargetType.Cluster })
    await userEvent.type(kubeconfigField(), typed)
    await waitFor(() => expect(contextSelect()).toHaveValue('dev'))
    const hint = screen.queryByText(/^Read from \//)
    if (note) expect(hint).toHaveTextContent(note)
    else expect(hint).toBeNull()
  })

  it('shows the server message under the kubeconfig field for a refused path', async () => {
    mockApi((path) => (path === '' ? NO_FILE : HTTP_NOT_FOUND))
    renderTarget({ type: TargetType.Cluster })
    await userEvent.type(kubeconfigField(), '/nope')
    expect(await screen.findByText(refusedKubeconfig('/nope'))).toHaveAttribute('id', 'kubeconfig-error')
  })

  it('shows the server message and no prefill when a default-rules file is refused', async () => {
    mockApi(() => HTTP_NOT_FOUND)
    renderTarget({ type: TargetType.Cluster })
    expect(await screen.findByText(refusedKubeconfig(''))).toHaveAttribute('id', 'kubeconfig-error')
    expect(kubeconfigField()).toHaveValue('')
    expect(probe.state.answers.target.context).toBeUndefined()
  })

  it('hides the refusal until the edited path is checked', async () => {
    mockApi((path) => (path === '' ? NO_FILE : HTTP_NOT_FOUND))
    renderTarget({ type: TargetType.Cluster })
    await userEvent.type(kubeconfigField(), '/nope')
    await screen.findByText(refusedKubeconfig('/nope'))
    await userEvent.type(kubeconfigField(), 'x')
    expect(screen.queryByText(/cannot read kubeconfig/)).toBeNull()
    expect(await screen.findByText(refusedKubeconfig('/nopex'))).toBeInTheDocument()
  })

  it('clears the context when the kubeconfig in the field cannot be read', async () => {
    mockApi((path) => (path === KUBECONFIG ? ONE_FILE : HTTP_NOT_FOUND))
    renderTarget({ type: TargetType.Cluster }, { kubeconfig: KUBECONFIG })
    await waitFor(() => expect(contextSelect()).toHaveValue('dev'))
    await userEvent.type(kubeconfigField(), '.bak')
    await screen.findByText(refusedKubeconfig(`${KUBECONFIG}.bak`))
    await waitFor(() => expect(probe.state.answers.target.context).toBeUndefined())
    expect(screen.queryByRole('option', { name: 'dev' })).toBeNull()
  })

  it('keeps the empty context once the user chose it, also after switching stage and back', async () => {
    mockApi(() => ONE_FILE)
    renderTarget({ type: TargetType.Cluster }, { kubeconfig: KUBECONFIG })
    await waitFor(() => expect(contextSelect()).toHaveValue('dev'))
    await userEvent.selectOptions(contextSelect(), '')
    expect(contextSelect()).toHaveValue('')
    expect(probe.state.answers.target.context).toBeUndefined()
    await userEvent.click(railEntry(1))
    await userEvent.click(railEntry(0))
    await screen.findByRole('option', { name: 'dev' })
    expect(contextSelect()).toHaveValue('')
    expect(probe.state.answers.target.context).toBeUndefined()
  })

  it('restores the chosen context after the path is edited and reverted', async () => {
    mockApi((path) => (path === KUBECONFIG ? ONE_FILE : HTTP_NOT_FOUND))
    renderTarget({ type: TargetType.Cluster }, { kubeconfig: KUBECONFIG })
    await waitFor(() => expect(contextSelect()).toHaveValue('dev'))
    await userEvent.selectOptions(contextSelect(), 'prod')
    await userEvent.type(kubeconfigField(), 'x')
    await screen.findByText(refusedKubeconfig(`${KUBECONFIG}x`))
    await waitFor(() => expect(probe.state.answers.target.context).toBeUndefined())
    await userEvent.type(kubeconfigField(), '{Backspace}')
    await waitFor(() => expect(contextSelect()).toHaveValue('prod'))
    expect(probe.state.answers.target.context).toBe('prod')
  })

  it.each<[TargetType, boolean]>([
    [TargetType.File, false],
    [TargetType.Cluster, true],
  ])('with target %s, the cluster-only controls are present: %s', async (type, present) => {
    renderTarget()
    await userEvent.click(screen.getByRole('radio', { name: type === TargetType.File ? /File only/ : /Kubernetes cluster/ }))
    for (const control of [screen.queryByLabelText('Kubeconfig'), screen.queryByLabelText('Context'), screen.queryByRole('checkbox', { name: 'create it' })]) {
      if (present) expect(control).toBeInTheDocument()
      else expect(control).toBeNull()
    }
  })

  it.each(STAGES.slice(1).map((stage, i) => [stage.label, i + 1] as const))('keeps every field after switching to %s and back', async (_label, index) => {
    mockApi(() => ONE_FILE)
    renderTarget()
    await userEvent.click(screen.getByRole('radio', { name: /Kubernetes cluster/ }))
    await waitFor(() => expect(contextSelect()).toHaveValue('dev'))
    await userEvent.selectOptions(contextSelect(), 'prod')
    await userEvent.type(screen.getByLabelText('Namespace'), 'provider')
    await userEvent.click(screen.getByRole('checkbox', { name: 'create it' }))
    await userEvent.type(screen.getByLabelText('Release name'), 'mp')
    await userEvent.selectOptions(screen.getByLabelText('Version'), '10.15.0')
    await userEvent.click(screen.getByRole('radio', { name: /Production/ }))

    await userEvent.click(railEntry(index))
    await userEvent.click(railEntry(0))

    expect(screen.getByRole('radio', { name: /Kubernetes cluster/ })).toBeChecked()
    expect(kubeconfigField()).toHaveValue(KUBECONFIG)
    expect(contextSelect()).toHaveValue('prod')
    expect(screen.getByLabelText('Namespace')).toHaveValue('provider')
    expect(screen.getByRole('checkbox', { name: 'create it' })).toBeChecked()
    expect(screen.getByLabelText('Release name')).toHaveValue('mp')
    expect(screen.getByLabelText('Chart')).toHaveValue('data-space-connector')
    expect(screen.getByLabelText('Version')).toHaveValue('10.15.0')
    expect(screen.getByRole('radio', { name: /Production/ })).toBeChecked()
  })

  it('keeps the kubeconfig path in the session, never in the answer set', async () => {
    mockApi(() => ONE_FILE)
    renderTarget({ type: TargetType.Cluster })
    await waitFor(() => expect(probe.state.session.kubeconfig).toBe(KUBECONFIG))
    await userEvent.click(screen.getByRole('radio', { name: /File only/ }))
    expect(probe.state.session.kubeconfig).toBe(KUBECONFIG)
    expect(probe.state.answers.target.context).toBe('dev')
    expect(JSON.stringify(probe.state.answers)).not.toContain('kubeconfig')
  })

  it('defaults the version to the latest one', async () => {
    renderTarget()
    await waitFor(() => expect(screen.getByLabelText('Version')).toHaveValue('10.16.0'))
    expect(screen.getByRole('option', { name: '10.16.0 (latest)' })).toBeInTheDocument()
  })

  it('offers only the versions of the selected chart, defaulting to its latest', async () => {
    mockApi(undefined, { charts: [...MODEL.charts, { name: 'trust-anchor', versions: [{ version: '0.2.0', latest: true }] }] })
    renderTarget()
    await waitFor(() => expect(screen.getByLabelText('Version')).toHaveValue('10.16.0'))
    await userEvent.selectOptions(screen.getByLabelText('Chart'), 'trust-anchor')
    await waitFor(() => expect(screen.getByLabelText('Version')).toHaveValue('0.2.0'))
    expect(screen.queryByRole('option', { name: /10\.16\.0/ })).toBeNull()
  })

  it.each<[string, Partial<Target>, number, number]>([
    ['empty', {}, 0, 4],
    ['invalid', { type: TargetType.File, namespace: 'Bad_NS', releaseName: '1abc', environment: Environment.Development }, 2, 0],
    ['complete', { type: TargetType.File, namespace: 'provider', releaseName: 'provider', environment: Environment.Production }, 0, 0],
    ['a cluster without a context', { type: TargetType.Cluster, namespace: 'provider', releaseName: 'provider', environment: Environment.Production }, 0, 1],
  ])('counts open questions and errors for %s inputs', async (_name, target, errors, open) => {
    renderTarget(target)
    await waitFor(() => expect(screen.getByLabelText('Validation')).toHaveTextContent(`✖ ${errors} errors○ ${open} open questions`))
  })

  it.each<[string, string, Partial<Target>]>([
    ['a typed field', 'Namespace', { namespace: 'provider' }],
    ['a radio group', 'Environment', { environment: Environment.Production }],
    ['a select', 'Chart version', { chartVersion: '10.15.0' }],
  ])('marks %s open without touching its value', async (_name, label, value) => {
    renderTarget({ type: TargetType.File, namespace: 'provider', releaseName: 'provider', environment: Environment.Production, chartVersion: '10.15.0', ...value })
    const ask = screen.getByRole('button', { name: `Ask ${label} later` })
    expect(ask).toHaveAttribute('aria-pressed', 'false')
    await userEvent.click(ask)
    expect(ask).toHaveAttribute('aria-pressed', 'true')
    expect(probe.state.answers.target).toMatchObject(value)
    expect(screen.getByLabelText('Validation')).toHaveTextContent('✖ 0 errors○ 1 open questions')

    await userEvent.click(railEntry(1))
    expect(within(railEntry(0)).getByLabelText('has open questions')).toHaveTextContent('○')

    await userEvent.click(railEntry(0))
    await userEvent.click(screen.getByRole('button', { name: `Ask ${label} later` }))
    expect(probe.state.open).toEqual([])
    expect(probe.state.answers.target).toMatchObject(value)
    expect(screen.getByLabelText('Validation')).toHaveTextContent('○ 0 open questions')
  })

  it('counts a marked empty field once, as open and not as an error', async () => {
    renderTarget({ type: TargetType.File, releaseName: 'provider', environment: Environment.Production })
    await userEvent.click(screen.getByRole('button', { name: 'Ask Namespace later' }))
    await waitFor(() => expect(screen.getByLabelText('Validation')).toHaveTextContent('✖ 0 errors○ 1 open questions'))
  })

  it.each<[string, AppState, string, string[]]>([
    ['an applied Operator preset', OPERATOR, TRUST_ANCHOR, ['1.3.6 (latest)']],
    ['the switch back from Operator to a pinned connector version', SWITCHED_BACK, DSC, ['10.16.0 (latest)', '10.15.0']],
  ])('asks for the version again after %s', async (_name, state, chart, versions) => {
    mockApi(undefined, TWO_CHARTS)
    renderState(state)
    const version = screen.getByLabelText('Version')
    await within(version).findByRole('option', { name: versions[0] })
    expect(screen.getByLabelText('Chart')).toHaveValue(chart)
    expect(within(version).getAllByRole('option').map((o) => o.textContent)).toEqual(['— choose a version —', ...versions])
    expect(version).toHaveValue('')
    expect(probe.state.answers.target.chartVersion).toBe('')
    expect(screen.getByLabelText('Validation')).toHaveTextContent('✖ 0 errors○ 1 open questions')
    await userEvent.click(railEntry(1))
    expect(within(railEntry(0)).getByLabelText('has open questions')).toHaveTextContent('○')
  })

  it.each<[string, AppState, string, string, string]>([
    ['an Operator preset', OPERATOR, DSC, '10.16.0', 'Role Operator was cleared because it deploys trust-anchor — pick a role on step 2.'],
    ['a Provider preset', PROVIDER, TRUST_ANCHOR, '1.3.6', 'Role Provider was cleared because it deploys data-space-connector — pick a role on step 2.'],
  ])('clears %s when the chart changes, preselecting the new chart latest version', async (_name, state, chart, latest, notice) => {
    mockApi(undefined, TWO_CHARTS)
    renderState(state)
    await screen.findByRole('option', { name: /\(latest\)/ })
    await userEvent.selectOptions(screen.getByLabelText('Chart'), chart)
    await waitFor(() => expect(screen.getByLabelText('Version')).toHaveValue(latest))
    expect(await screen.findByRole('status')).toHaveTextContent(notice)
    expect(probe.state.answers).toMatchObject({ role: undefined, features: {} })
    expect(probe.state.profileCatalogue).toBeUndefined()
    expect(screen.getByLabelText('Validation')).toHaveTextContent('✖ 0 errors○ 1 open questions')
    expect(within(railEntry(1)).getByLabelText('has open questions')).toHaveTextContent('○')
  })
})
