import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect } from 'react'
import { describe, expect, it } from 'vitest'
import { HTTP_NOT_FOUND } from '../api/client'
import { Chrome } from '../chrome/Chrome'
import { STAGES } from '../stages'
import { Environment, TargetType, type Session, type Target } from '../state/answers'
import { useStore } from '../state/context'
import { initialState, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { MODEL, mockApi, type ContextsAnswer } from '../test/api'

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

function renderTarget(target: Partial<Target> = {}, session: Session = {}) {
  const initial: AppState = {
    ...initialState,
    entered: true,
    visited: [StageId.Target],
    answers: { target: { ...initialState.answers.target, ...target } },
    session,
  }
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

const kubeconfigField = () => screen.getByLabelText('Kubeconfig')
const contextSelect = () => screen.getByLabelText('Context')

describe('Target', () => {
  it.each<[string, ContextsAnswer, string, string, boolean]>([
    ['one reported path', ONE_FILE, KUBECONFIG, 'dev', false],
    ['two reported paths', TWO_FILES, '', 'prod', true],
  ])('fills the kubeconfig field for %s', async (_name, answer, field, context, listsFiles) => {
    mockApi(() => answer)
    renderTarget({ type: TargetType.Cluster })
    await waitFor(() => expect(contextSelect()).toHaveValue(context))
    expect(kubeconfigField()).toHaveValue(field)
    const note = screen.queryByText(/Read from KUBECONFIG/)
    if (listsFiles) expect(note).toHaveTextContent('/a/config, /b/config')
    else expect(note).toBeNull()
  })

  it('shows a not-found message under the kubeconfig field for a 404', async () => {
    mockApi((path) => (path === '' ? NO_FILE : HTTP_NOT_FOUND))
    renderTarget({ type: TargetType.Cluster })
    await userEvent.type(kubeconfigField(), '/nope')
    expect(await screen.findByText('No kubeconfig found at /nope.')).toHaveAttribute('id', 'kubeconfig-error')
  })

  it('hides the not-found message until the edited path is checked', async () => {
    mockApi((path) => (path === '' ? NO_FILE : HTTP_NOT_FOUND))
    renderTarget({ type: TargetType.Cluster })
    await userEvent.type(kubeconfigField(), '/nope')
    await screen.findByText('No kubeconfig found at /nope.')
    await userEvent.type(kubeconfigField(), 'x')
    expect(screen.queryByText(/No kubeconfig found/)).toBeNull()
    expect(await screen.findByText('No kubeconfig found at /nopex.')).toBeInTheDocument()
  })

  it('clears the context when the kubeconfig in the field cannot be read', async () => {
    mockApi((path) => (path === KUBECONFIG ? ONE_FILE : HTTP_NOT_FOUND))
    renderTarget({ type: TargetType.Cluster }, { kubeconfig: KUBECONFIG })
    await waitFor(() => expect(contextSelect()).toHaveValue('dev'))
    await userEvent.type(kubeconfigField(), '.bak')
    await screen.findByText(`No kubeconfig found at ${KUBECONFIG}.bak.`)
    await waitFor(() => expect(probe.state.answers.target.context).toBeUndefined())
    expect(screen.queryByRole('option', { name: 'dev' })).toBeNull()
  })

  it('keeps the empty context once the user chose it', async () => {
    mockApi(() => ONE_FILE)
    renderTarget({ type: TargetType.Cluster }, { kubeconfig: KUBECONFIG })
    await waitFor(() => expect(contextSelect()).toHaveValue('dev'))
    await userEvent.selectOptions(contextSelect(), '')
    expect(contextSelect()).toHaveValue('')
    expect(probe.state.answers.target.context).toBeUndefined()
  })

  it('restores the chosen context after the path is edited and reverted', async () => {
    mockApi((path) => (path === KUBECONFIG ? ONE_FILE : HTTP_NOT_FOUND))
    renderTarget({ type: TargetType.Cluster }, { kubeconfig: KUBECONFIG })
    await waitFor(() => expect(contextSelect()).toHaveValue('dev'))
    await userEvent.selectOptions(contextSelect(), 'prod')
    await userEvent.type(kubeconfigField(), 'x')
    await screen.findByText(`No kubeconfig found at ${KUBECONFIG}x.`)
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
})
