import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StrictMode } from 'react'
import { describe, expect, it } from 'vitest'
import { App } from '../App'
import type { Profile, RecentEntry, RecentProfiles } from '../api/profile'
import { emptyAnswers, TargetType } from '../state/answers'
import { DEFAULT_DIR, HTTP_NOT_FOUND, lateLoad, mockApi, REPORT, refusedProfile, refusedTrial, refusedUpload, TRIAL } from '../test/api'
import { OPEN_INPUT_LABEL, PROFILE_PATH_LABEL } from './Start'
import { GENERATE_LABEL } from './TrialPrerequisites'

const CARDS = ['New configuration', 'Open values.yaml', 'Open profile', 'Try it locally']
const HTTP_PAYLOAD_TOO_LARGE = 413
const HTTP_UNPROCESSABLE = 422
const HTTP_INTERNAL = 500

function pick(name = 'provider.yaml') {
  return userEvent.upload(screen.getByLabelText(OPEN_INPUT_LABEL), new File(['a: 1\n'], name, { type: 'application/yaml' }))
}

function cards() {
  return CARDS.map((title) => screen.getByRole('button', { name: new RegExp(title) }))
}

describe('Start', () => {
  it('shows four enabled entries', () => {
    render(<App />)
    expect(cards().filter((card) => card.getAttribute('aria-disabled') === 'true')).toEqual([])
    expect(cards().map((card) => within(card).getByRole('heading').textContent)).toEqual(['＋ New configuration', '📂 Open values.yaml', '⟲ Open profile', '⚡ Try it locally'])
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

  it('keeps New configuration disabled while an upload runs, so a late report cannot replace it', async () => {
    const late = lateLoad()
    mockApi(undefined, undefined, late.load)
    render(<App />)
    await pick()
    const fresh = screen.getByRole('button', { name: /New configuration/ })
    expect(fresh).toHaveAttribute('aria-disabled', 'true')
    await userEvent.click(fresh)
    expect(screen.queryByRole('navigation', { name: 'Stages' })).toBeNull()
    late.resolve(REPORT)
    expect(await screen.findByRole('heading', { level: 1, name: new RegExp(`Opened ${REPORT.source}`) })).toBeInTheDocument()
  })

  it('aborts a running upload when S0 goes away', async () => {
    const late = lateLoad()
    const fetchMock = mockApi(undefined, undefined, late.load)
    const { unmount } = render(<App />)
    await pick()
    const init = fetchMock.mock.calls.find(([input]) => String(input).includes('/api/load'))?.[1]
    unmount()
    expect(init?.signal?.aborted).toBe(true)
  })
})

const RECENT: RecentEntry[] = [
  { path: `${DEFAULT_DIR}/provider.dscprofile.yaml`, name: 'provider', chartVersion: '10.16.0', savedAt: new Date().toISOString() },
  { path: `${DEFAULT_DIR}/consumer.dscprofile.yaml`, name: 'consumer', chartVersion: '10.15.0', savedAt: '2020-01-01T00:00:00Z' },
]
const OPENED: Profile = {
  format: 1,
  model: { chart: 'data-space-connector', version: '10.16.0', revision: 1 },
  target: { ...emptyAnswers().target, type: TargetType.Cluster, namespace: 'provider' },
  answers: {},
  open: [{ field: 'target.context' }],
  secrets: {},
}

function mockProfiles(open: (path: string) => Profile | number = () => OPENED) {
  return mockApi(undefined, undefined, undefined, {
    recent: () => ({ entries: RECENT, defaultDir: DEFAULT_DIR, notes: [] }),
    open: (path) => {
      const answer = open(path)
      return typeof answer === 'number' ? answer : { path, profile: answer, notes: [] }
    },
  })
}

describe('Start: profiles', () => {
  it('lists the recent profiles newest first with chart version and age', async () => {
    mockProfiles()
    render(<App />)
    const recent = await screen.findByRole('region', { name: 'Recent' })
    expect(within(recent).getAllByRole('button').map((entry) => entry.textContent)).toEqual([
      'provider · chart 10.16.0 · this minute',
      expect.stringMatching(/^consumer · chart 10\.15\.0 · \d+ years ago$/),
    ])
  })

  // main.tsx renders under StrictMode, whose dev mode runs every effect cleanup once on mount.
  it.each([
    ['plainly', <App key="plain" />],
    [
      'under StrictMode',
      <StrictMode key="strict">
        <App />
      </StrictMode>,
    ],
  ])('opens a recent profile on Review with its open question when rendered %s', async (_, app) => {
    mockProfiles()
    render(app)
    await userEvent.click(await screen.findByRole('button', { name: /^provider/ }))
    expect(await screen.findByRole('button', { name: /7 Review/ })).toHaveAttribute('aria-current', 'step')
    expect(screen.getByRole('heading', { name: /Open questions/ }).parentElement).toHaveTextContent(/Kube context.*marked/)
  })

  it('opens a typed path from the Open profile card, prefilled with the default directory', async () => {
    const fetchMock = mockProfiles()
    render(<App />)
    await screen.findByRole('region', { name: 'Recent' })
    await userEvent.click(screen.getByRole('button', { name: /Open profile/ }))
    const field = screen.getByLabelText(PROFILE_PATH_LABEL)
    expect(field).toHaveValue(`${DEFAULT_DIR}/`)
    await userEvent.type(field, 'other.dscprofile.yaml{Enter}')
    expect(await screen.findByRole('navigation', { name: 'Stages' })).toBeInTheDocument()
    const init = fetchMock.mock.calls.find(([input]) => String(input).endsWith('/api/profile/open'))?.[1]
    expect(JSON.parse(String(init?.body))).toEqual({ path: `${DEFAULT_DIR}/other.dscprofile.yaml` })
  })

  it('stays on S0 and shows why when a profile cannot be opened', async () => {
    mockProfiles(() => HTTP_NOT_FOUND)
    render(<App />)
    const { path } = RECENT[1] as RecentEntry
    await userEvent.click(await screen.findByRole('button', { name: /^consumer/ }))
    expect(await screen.findByRole('alert')).toHaveTextContent(`Cannot open ${path}: ${refusedProfile(path, HTTP_NOT_FOUND)}`)
    expect(screen.queryByRole('navigation', { name: 'Stages' })).toBeNull()
  })

  it.each([
    ['a corrupt store', 'the recent list cannot be read: yaml: line 1: did not find expected key'],
    ['no config directory', 'the recent list is not available: no user config directory (set DSC_CONFIG_HOME)'],
  ])('shows why the Recent list is missing with %s', async (_, note) => {
    mockApi(undefined, undefined, undefined, { recent: () => ({ entries: [], defaultDir: DEFAULT_DIR, notes: [note] }) })
    render(<App />)
    expect(await screen.findByText(note)).toHaveClass('hint')
    expect(screen.queryByRole('region', { name: 'Recent' })).toBeNull()
  })

  it('prefills the path field with a default directory that arrives after the card was opened', async () => {
    let resolve: (list: RecentProfiles) => void = () => {}
    const pending = new Promise<RecentProfiles>((done) => (resolve = done))
    mockApi(undefined, undefined, undefined, { recent: () => pending })
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: /Open profile/ }))
    const field = screen.getByLabelText(PROFILE_PATH_LABEL)
    expect(field).toHaveValue('')
    resolve({ entries: [], defaultDir: DEFAULT_DIR, notes: [] })
    await waitFor(() => expect(field).toHaveValue(`${DEFAULT_DIR}/`))
  })

  it('keeps a typed path when the default directory arrives late', async () => {
    let resolve: (list: RecentProfiles) => void = () => {}
    const pending = new Promise<RecentProfiles>((done) => (resolve = done))
    mockApi(undefined, undefined, undefined, { recent: () => pending })
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: /Open profile/ }))
    const field = screen.getByLabelText(PROFILE_PATH_LABEL)
    await userEvent.type(field, '/srv/x.dscprofile.yaml')
    resolve({ entries: RECENT, defaultDir: DEFAULT_DIR, notes: [] })
    await screen.findByRole('region', { name: 'Recent' })
    expect(field).toHaveValue('/srv/x.dscprofile.yaml')
  })

  it('shows no Recent list when the list cannot be read', async () => {
    mockApi(undefined, undefined, undefined, { recent: () => HTTP_NOT_FOUND })
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: /Open profile/ }))
    expect(screen.getByLabelText(PROFILE_PATH_LABEL)).toHaveValue('')
    expect(screen.queryByRole('region', { name: 'Recent' })).toBeNull()
  })
})

function trialRequests(fetchMock: ReturnType<typeof mockApi>) {
  return fetchMock.mock.calls.filter(([input]) => String(input).startsWith('/api/trial'))
}

function lateTrial() {
  let resolve: (trial: typeof TRIAL) => void = () => {}
  const pending = new Promise<typeof TRIAL>((done) => (resolve = done))
  return { fetch: () => pending, resolve }
}

describe('Start: trial', () => {
  it('opens the prerequisite screen without fetching anything, and Back returns to the cards', async () => {
    const fetchMock = mockApi()
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: /Try it locally/ }))
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Try it locally')
    expect(screen.getByRole('button', { name: GENERATE_LABEL })).toBeEnabled()
    expect(trialRequests(fetchMock)).toEqual([])
    await userEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(cards()).toHaveLength(CARDS.length)
    expect(trialRequests(fetchMock)).toEqual([])
  })

  it('fetches the trial on Generate and lands on Review', async () => {
    const fetchMock = mockApi()
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: /Try it locally/ }))
    await userEvent.click(screen.getByRole('button', { name: GENERATE_LABEL }))
    expect(await screen.findByRole('button', { name: /7 Review/ })).toHaveAttribute('aria-current', 'step')
    expect(trialRequests(fetchMock).map(([input]) => String(input))).toEqual(['/api/trial'])
  })

  it('shows an alert and stays on the prerequisite screen when the trial cannot be fetched', async () => {
    mockApi(undefined, undefined, undefined, undefined, { fetch: () => HTTP_INTERNAL })
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: /Try it locally/ }))
    await userEvent.click(screen.getByRole('button', { name: GENERATE_LABEL }))
    expect(await screen.findByRole('alert')).toHaveTextContent(refusedTrial(HTTP_INTERNAL))
    expect(screen.queryByRole('navigation', { name: 'Stages' })).toBeNull()
    expect(screen.getByRole('button', { name: GENERATE_LABEL })).toBeEnabled()
  })

  it('aborts a running fetch on Back, so a late trial never enters Review', async () => {
    const late = lateTrial()
    const fetchMock = mockApi(undefined, undefined, undefined, undefined, { fetch: late.fetch })
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: /Try it locally/ }))
    await userEvent.click(screen.getByRole('button', { name: GENERATE_LABEL }))
    expect(screen.getByRole('button', { name: GENERATE_LABEL })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(trialRequests(fetchMock)[0]?.[1]?.signal?.aborted).toBe(true)
    late.resolve(TRIAL)
    await waitFor(() => expect(cards()).toHaveLength(CARDS.length))
    expect(screen.queryByRole('navigation', { name: 'Stages' })).toBeNull()
  })
})
