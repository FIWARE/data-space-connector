import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { RootSource, VersionSource, type LoadReport } from '../api/load'
import { Chrome } from '../chrome/Chrome'
import { STAGES } from '../stages'
import { Environment, TargetType, type Target } from '../state/answers'
import type { FeatureState } from '../api/profile'
import type { FieldId } from '../state/openQuestions'
import { initialState, type AppliedRoleChange, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { catalogueFeature, lateLoad, mockApi, recordToggle, refusedRole, REPORT, refusedUpload } from '../test/api'
import { DELIBERATE_HEADING } from './DeliberateChoices'
import { FEATURES_HEADING } from './Features'
import { ROOT_LABEL, WHOLE_FILE } from './Review'
import { ROLE_CHANGES_HEADING, roleChanged, summaryToggleLabel, TURNS_OFF } from './roleChangeText'

const HTTP_UNPROCESSABLE = 422
/** A picked role, so only the Target questions are open. */
const PICKED = 'provider'
const FILE = new File(['dsc:\n  did:\n    enabled: true\n'], 'wrapped.yaml')
/** A report of values another chart wraps under `dsc:`. */
const WRAPPED: LoadReport = { ...REPORT, source: FILE.name, root: 'dsc', rootCandidates: ['dsc', ''] }

/** A Target with every question answered. */
const ANSWERED: Target = {
  ...initialState.answers.target,
  type: TargetType.File,
  namespace: 'provider',
  releaseName: 'provider',
  chartVersion: '10.16.0',
  environment: Environment.Production,
}

function renderReview(loaded?: LoadReport, loadedFile?: File, more: Partial<AppState> = {}) {
  return render(
    <StoreProvider initial={{ ...initialState, entered: true, stage: StageId.Review, visited: [StageId.Review], loaded, loadedFile, ...more }}>
      <Chrome stages={STAGES} />
    </StoreProvider>,
  )
}

const ISSUER = catalogueFeature('identity.issuer')
const BROKER = catalogueFeature('data.scorpio')
const TRACING = catalogueFeature('obs.tracing')
/** Consumer with its recommended issuer turned off and the not-offered broker turned on. */
const OVERRULED: Record<string, FeatureState> = {
  [ISSUER.id]: { enabled: false, source: 'user' },
  [BROKER.id]: { enabled: true, source: 'loaded' },
  [TRACING.id]: { enabled: false, source: 'user' },
}
const deliberateList = () => within(screen.getByRole('heading', { level: 3, name: /^Deliberate choices/ }).closest('.panel') as HTMLElement)
const deliberatePanelAlert = async () => {
  await screen.findByRole('heading', { level: 3, name: DELIBERATE_HEADING })
  return deliberateList().getByRole('alert')
}
const featureRequests = (fetchMock: ReturnType<typeof mockApi>) =>
  fetchMock.mock.calls.filter(([input]) => new URL(String(input), 'http://x').pathname === '/api/features')

const openList = () => within(screen.getByRole('heading', { level: 3, name: /^Open questions/ }).closest('.panel') as HTMLElement)

describe('Review', () => {
  it.each<[string, LoadReport | undefined]>([
    ['the placeholder', undefined],
    ['the load report', REPORT],
  ])('lists the open questions above %s, marking the marked ones', (_name, loaded) => {
    renderReview(loaded, undefined, { answers: { role: PICKED, features: {}, target: { ...ANSWERED, namespace: '' } }, open: ['target.environment', 'target.namespace'] })
    expect(screen.getByRole('heading', { level: 3, name: 'Open questions (2)' })).toBeInTheDocument()
    expect(openList().getAllByRole('listitem').map((li) => li.textContent)).toEqual(['NamespaceTargetmarkedgo ↗', 'EnvironmentTargetmarkedgo ↗'])
  })

  it('says when no question is open', () => {
    renderReview(undefined, undefined, { answers: { role: PICKED, features: {}, target: ANSWERED } })
    expect(screen.getByRole('heading', { level: 3, name: 'Open questions (0)' })).toBeInTheDocument()
    expect(openList().getByText('None: every question is answered.')).toBeInTheDocument()
    expect(screen.getByLabelText('Validation')).toHaveTextContent('○ 0 open questions')
  })

  it('lists an unpicked role with the count the footer shows', () => {
    renderReview(undefined, undefined, { answers: { features: {}, target: ANSWERED } })
    expect(screen.getByRole('heading', { level: 3, name: 'Open questions (1)' })).toBeInTheDocument()
    expect(openList().getAllByRole('listitem').map((li) => li.textContent)).toEqual(['RoleRolego ↗'])
    expect(screen.getByLabelText('Validation')).toHaveTextContent('○ 1 open questions')
  })

  it('lands on Role with the role group focused', async () => {
    mockApi()
    renderReview(undefined, undefined, { answers: { features: {}, target: ANSWERED } })
    await userEvent.click(screen.getByRole('button', { name: 'Go to Role' }))
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('What does this organization do in the data space?')
    await waitFor(() => expect(screen.getAllByRole('radio')[0]).toHaveFocus())
  })

  it.each<[string, Partial<Target>, FieldId[], string, string]>([
    ['an unanswered text field', { namespace: '' }, [], 'Namespace', 'Namespace'],
    ['a marked select', {}, ['target.chartVersion'], 'Chart version', 'Version'],
    ['an unanswered radio group', { environment: '' }, [], 'Environment', 'Development'],
  ])('lands on Target with %s focused', async (_name, change, open, label, input) => {
    renderReview(undefined, undefined, { answers: { role: PICKED, features: {}, target: { ...ANSWERED, ...change } }, open })
    await userEvent.click(screen.getByRole('button', { name: `Go to ${label}` }))
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Where is this going?')
    expect(screen.getByLabelText(new RegExp(`^${input}`))).toHaveFocus()
  })

  it('keeps the Epic 6 placeholder without a loaded file', () => {
    renderReview()
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Review — not available yet')
    expect(screen.getByText(/Epic 6/)).toBeInTheDocument()
  })

  it('heads the report with what was understood and carried over', () => {
    renderReview(REPORT)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Opened provider.yaml: understood 2, carried over untouched 3')
    expect(screen.getByText(/Chart data-space-connector 10.16.0/)).toHaveTextContent('from the file’s dsc-config header')
  })

  it('says why the latest bundle was used', () => {
    renderReview({ ...REPORT, chartVersionFrom: VersionSource.Latest, chartVersionNote: 'the file has no dsc-config header' })
    expect(screen.getByText(/Chart data-space-connector/)).toHaveTextContent('latest bundle, because the file has no dsc-config header')
  })

  it('shows the counts per class', () => {
    renderReview(REPORT)
    const counts = within(screen.getByLabelText('Counts'))
    expect(counts.getAllByRole('definition').map((dd) => dd.textContent)).toEqual(['1', '1', '2', '1'])
  })

  it('groups unknown entries by top-level key and lists skipped pairs with their reason', () => {
    renderReview(REPORT)
    expect(screen.getByText('keycloak').closest('summary')).toHaveTextContent('keycloak (1)')
    expect(screen.getByText('marketplace').closest('summary')).toHaveTextContent('marketplace (1)')
    const skipped = screen.getByText(/Keys Helm ignores or cannot address/).closest('details')
    expect(skipped).not.toHaveAttribute('open')
    expect(skipped).toHaveTextContent('shadowed by line 31')
    expect(skipped).toHaveTextContent('30')
  })

  it('lists derived answers by stage and the marker answers not derived', () => {
    renderReview(REPORT)
    expect(screen.getByRole('heading', { level: 4, name: /Features/ })).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 4, name: /Foundations/ })).toBeInTheDocument()
    expect(screen.getByText(/the profile has to supply them: did.value/)).toBeInTheDocument()
  })

  it('jumps to the stage of a derived answer', async () => {
    renderReview(REPORT)
    await userEvent.click(within(screen.getByRole('heading', { level: 4, name: /Foundations/ })).getByRole('button', { name: /go/ }))
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Foundations — not available yet')
    expect(screen.getByText('domain.base')).toBeInTheDocument()
  })

  it('says the values were read from the whole file, with nothing to pick', () => {
    renderReview(REPORT, FILE)
    expect(screen.getByText(/Read from/)).toHaveTextContent(`Read from ${WHOLE_FILE} (detected)`)
    expect(screen.queryByLabelText(ROOT_LABEL)).not.toBeInTheDocument()
  })

  it('re-reads the same file under another root', async () => {
    const fetchMock = mockApi(undefined, undefined, (name, root) => ({ ...WRAPPED, source: name, root: root ?? 'dsc', rootFrom: RootSource.Requested }))
    renderReview(WRAPPED, FILE)
    const picker = screen.getByLabelText(ROOT_LABEL)
    expect(picker).toHaveValue('dsc')
    expect(screen.getByText(/Read from/)).toHaveTextContent('Read from dsc (detected)')
    await userEvent.selectOptions(picker, WHOLE_FILE)
    expect(await screen.findByText(/Read from/)).toHaveTextContent(`Read from ${WHOLE_FILE} (as chosen)`)
    const [, init] = fetchMock.mock.calls.at(-1) ?? []
    expect(init?.body).toBe(FILE)
  })

  it('applies a re-read that answers after the user left Review, without pulling them back', async () => {
    const late = lateLoad()
    const fetchMock = mockApi(undefined, undefined, late.load)
    renderReview(WRAPPED, FILE)
    await userEvent.selectOptions(screen.getByLabelText(ROOT_LABEL), WHOLE_FILE)
    const nav = within(screen.getByRole('navigation', { name: 'Stages' }))
    await userEvent.click(nav.getByRole('button', { name: /Foundations/ }))
    const init = fetchMock.mock.calls.find(([input]) => String(input).includes('/api/load'))?.[1]
    expect(init?.signal?.aborted).toBeFalsy()
    late.resolve({ ...WRAPPED, root: '', rootFrom: RootSource.Requested })
    await act(() => Promise.resolve())
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Foundations — not available yet')
    await userEvent.click(nav.getByRole('button', { name: /Review/ }))
    expect(screen.getByText(/Read from/)).toHaveTextContent(`Read from ${WHOLE_FILE} (as chosen)`)
  })

  it('keeps the picker disabled while a re-read started before leaving Review is in flight', async () => {
    const late = lateLoad()
    mockApi(undefined, undefined, late.load)
    renderReview(WRAPPED, FILE)
    await userEvent.selectOptions(screen.getByLabelText(ROOT_LABEL), WHOLE_FILE)
    const nav = within(screen.getByRole('navigation', { name: 'Stages' }))
    await userEvent.click(nav.getByRole('button', { name: /Foundations/ }))
    await userEvent.click(nav.getByRole('button', { name: /Review/ }))
    expect(screen.getByLabelText(ROOT_LABEL)).toBeDisabled()
    late.resolve({ ...WRAPPED, root: '', rootFrom: RootSource.Requested })
    await act(() => Promise.resolve())
    expect(screen.getByLabelText(ROOT_LABEL)).toBeEnabled()
  })

  it('keeps the report and shows why a re-read was refused', async () => {
    mockApi(undefined, undefined, () => HTTP_UNPROCESSABLE)
    renderReview(WRAPPED, FILE)
    await userEvent.selectOptions(screen.getByLabelText(ROOT_LABEL), WHOLE_FILE)
    expect(await screen.findByRole('alert')).toHaveTextContent(refusedUpload(FILE.name, HTTP_UNPROCESSABLE))
    expect(screen.getByLabelText(ROOT_LABEL)).toHaveValue('dsc')
  })

  it.each<[string, LoadReport | undefined]>([
    ['the placeholder', undefined],
    ['the load report', REPORT],
  ])('lists the deliberate choices with the Stage 5 marker above %s', async (_name, loaded) => {
    renderReview(loaded, undefined, { answers: { role: 'consumer', features: OVERRULED, target: ANSWERED } })
    expect(await screen.findByRole('heading', { level: 3, name: `${DELIBERATE_HEADING} (2)` })).toBeInTheDocument()
    expect(deliberateList().getAllByRole('listitem').map((li) => li.textContent)).toEqual([
      `${ISSUER.title}recommended on for Consumer — you turned it offgo ↗`,
      `${BROKER.title}not recommended for Consumer — your values file turns it ongo ↗`,
    ])
  })

  it.each<[string, LoadReport | undefined]>([
    ['the placeholder', undefined],
    ['the load report', REPORT],
  ])('lists the role changes of this session on %s', async (_name, loaded) => {
    const roleChanges: AppliedRoleChange[] = [
      { change: { fromRole: 'consumer', toRole: 'provider', enabled: [BROKER.id], disabled: [], kept: [] }, chartSwitch: null },
      { change: { fromRole: 'provider', toRole: 'gone-role', enabled: [], disabled: [BROKER.id], kept: [] }, chartSwitch: null },
    ]
    renderReview(loaded, undefined, { answers: { role: 'gone-role', features: {}, target: ANSWERED }, roleChanges })
    const panel = within((await screen.findByRole('heading', { level: 3, name: ROLE_CHANGES_HEADING })).closest('.panel') as HTMLElement)
    expect(await panel.findByText(roleChanged('Consumer', 'Provider'))).toBeInTheDocument()
    expect(panel.getByText(roleChanged('Provider', 'gone-role'))).toBeInTheDocument()
    await userEvent.click(panel.getByRole('button', { name: summaryToggleLabel('Provider', 'gone-role') }))
    expect(within(panel.getByRole('list', { name: TURNS_OFF })).getByRole('listitem')).toHaveTextContent(BROKER.title)
    expect(panel.getAllByRole('button', { expanded: true })).toHaveLength(1)
  })

  it('leaves the role changes out while there is none', async () => {
    renderReview(undefined, undefined, { answers: { role: PICKED, features: {}, target: ANSWERED } })
    await act(async () => {})
    expect(screen.queryByRole('heading', { level: 3, name: ROLE_CHANGES_HEADING })).toBeNull()
  })

  it('jumps to Features from a deliberate choice', async () => {
    renderReview(undefined, undefined, { answers: { role: 'consumer', features: OVERRULED, target: ANSWERED } })
    await userEvent.click(await screen.findByRole('button', { name: `Go to ${ISSUER.title}` }))
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(FEATURES_HEADING)
  })

  it.each<[string, Record<string, FeatureState>]>([
    ['the preset', {}],
    ['a user choice equal to the preset', { [ISSUER.id]: { enabled: true, source: 'user' } }],
  ])('leaves the section out with %s only', async (_name, features) => {
    const fetchMock = mockApi()
    renderReview(undefined, undefined, { answers: { role: 'consumer', features, target: ANSWERED } })
    await waitFor(() => expect(featureRequests(fetchMock)).toHaveLength(1))
    await act(async () => {})
    expect(screen.queryByRole('heading', { name: new RegExp(DELIBERATE_HEADING) })).not.toBeInTheDocument()
  })

  it('shows a refused features request and lists the choices once a re-read answers', async () => {
    let refuse = true
    const reread: LoadReport = { ...WRAPPED, root: '', rootFrom: RootSource.Requested, sha256: 'b'.repeat(64) }
    mockApi(undefined, undefined, () => reread, undefined, undefined, {
      features: (body) => (refuse ? HTTP_UNPROCESSABLE : recordToggle(body)),
    })
    renderReview(WRAPPED, FILE, { answers: { role: 'consumer', features: OVERRULED, target: ANSWERED } })
    expect(await deliberatePanelAlert()).toHaveTextContent(refusedRole(HTTP_UNPROCESSABLE))
    refuse = false
    await userEvent.selectOptions(screen.getByLabelText(ROOT_LABEL), WHOLE_FILE)
    expect(await screen.findByRole('heading', { level: 3, name: `${DELIBERATE_HEADING} (2)` })).toBeInTheDocument()
    expect(deliberateList().queryByRole('alert')).not.toBeInTheDocument()
  })

  it('asks nothing about features while no role is picked', async () => {
    const fetchMock = mockApi()
    renderReview(undefined, undefined, { answers: { features: OVERRULED, target: ANSWERED } })
    await act(async () => {})
    expect(featureRequests(fetchMock)).toHaveLength(0)
    expect(screen.queryByRole('heading', { name: new RegExp(DELIBERATE_HEADING) })).not.toBeInTheDocument()
  })
})
