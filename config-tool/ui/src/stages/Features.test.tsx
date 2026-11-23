import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useState } from 'react'
import { describe, expect, it } from 'vitest'
import type { FeaturesResult } from '../api/features'
import type { FeatureState } from '../api/profile'
import type { Feature } from '../api/roles'
import { repositoryDocUrl } from '../links'
import { Environment, TargetType, type Target } from '../state/answers'
import { useStore } from '../state/context'
import { initialState, type AppState } from '../state/reducer'
import { StageId } from '../state/stageIds'
import { StoreProvider } from '../state/store'
import { MODEL, mockApi, recordToggle, refusedRole, ROLES, type FeaturesBody, type RoleFakes } from '../test/api'
import { AVAILABLE_SECTION, Features, KEEP_ON, NO_ROLE_LINK, NOT_RECOMMENDED_SECTION, TURN_OFF } from './Features'

const HTTP_UNPROCESSABLE = 422
const DSC_MODEL = { chart: 'data-space-connector', version: '10.16.0', revision: 3 }
function feature(id: string): Feature {
  const found = ROLES.features.find((f) => f.id === id)
  if (!found) throw new Error(`no feature ${id}`)
  return found
}
const ISSUER = feature('identity.issuer')
const BROKER = feature('data.scorpio')
const TRACING = feature('obs.tracing')
const REGISTRY = feature('ta.registry')
const PRESET_ON: FeatureState = { enabled: true, source: 'preset' }
const PINNED: Target = {
  ...initialState.answers.target,
  type: TargetType.File,
  namespace: 'provider',
  releaseName: 'provider',
  environment: Environment.Production,
  chartVersion: '10.16.0',
}
const PRESETS: Record<string, Record<string, FeatureState>> = {
  consumer: { [ISSUER.id]: PRESET_ON },
  provider: { [ISSUER.id]: PRESET_ON, [BROKER.id]: PRESET_ON },
}

/** The store on S5 after `role` was picked, with `features` laid over its preset. */
function onFeatures(role?: string, features: Record<string, FeatureState> = {}): AppState {
  return {
    ...initialState,
    entered: true,
    stage: StageId.Features,
    visited: [StageId.Target, StageId.Role, StageId.Features],
    answers: { target: PINNED, role, features: { ...(role ? PRESETS[role] : {}), ...features } },
    profileModel: DSC_MODEL,
  }
}

const probe: { state: AppState } = { state: initialState }

function Probe() {
  const { state } = useStore()
  useEffect(() => {
    probe.state = state
  }, [state])
  return null
}

/** S5 plus a button that leaves it while the store lives on. */
function Harness() {
  const [shown, setShown] = useState(true)
  return (
    <>
      {shown && <Features />}
      <button type="button" onClick={() => setShown(false)}>
        leave
      </button>
      <Probe />
    </>
  )
}

/** POST /api/features recording every body; toggles answer only once released, in order, when `deferred`. */
function serverFeatures({ deferred = false, refuse = false } = {}) {
  const bodies: FeaturesBody[] = []
  const held: (() => void)[] = []
  const features = (body: FeaturesBody): FeaturesResult | number | Promise<FeaturesResult> => {
    bodies.push(body)
    if (!body.toggle) return recordToggle(body)
    if (refuse) return HTTP_UNPROCESSABLE
    if (!deferred) return recordToggle(body)
    return new Promise((resolve) => held.push(() => resolve(recordToggle(body))))
  }
  const toggles = () => bodies.filter((body) => body.toggle)
  const release = () => held.shift()?.()
  return { features, toggles, release }
}

function renderFeatures(state: AppState, roles: RoleFakes = {}) {
  mockApi(undefined, MODEL, undefined, {}, {}, roles)
  return render(
    <StoreProvider initial={state}>
      <Harness />
    </StoreProvider>,
  )
}

const box = (title: string) => screen.getByRole('checkbox', { name: title })
const findBox = (title: string) => screen.findByRole('checkbox', { name: title })
const rowOf = (title: string) => box(title).closest('div') as HTMLElement
const titlesIn = (section: string) => Array.from(screen.getByRole('group', { name: section }).querySelectorAll('strong'), (el) => el.textContent)

describe('Features', () => {
  it('groups the role chart features into three sections, none disabled, the other chart left out', async () => {
    renderFeatures(onFeatures('consumer'))
    await findBox(ISSUER.title)
    expect(titlesIn('Recommended for Consumer')).toEqual([ISSUER.title])
    expect(titlesIn(AVAILABLE_SECTION)).toEqual([TRACING.title])
    expect(titlesIn(NOT_RECOMMENDED_SECTION)).toEqual([BROKER.title])
    expect(screen.getAllByRole('checkbox').every((el) => !(el as HTMLInputElement).disabled)).toBe(true)
    expect(screen.queryByRole('checkbox', { name: REGISTRY.title })).not.toBeInTheDocument()
    expect(box(ISSUER.title)).toBeChecked()
    expect(box(TRACING.title)).not.toBeChecked()
  })

  it('points to step 2 while no role is picked', async () => {
    const fetchMock = mockApi()
    render(
      <StoreProvider initial={onFeatures()}>
        <Features />
        <Probe />
      </StoreProvider>,
    )
    await userEvent.click(screen.getByRole('button', { name: NO_ROLE_LINK }))
    expect(probe.state.stage).toBe(StageId.Role)
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it.each<[string, string, Record<string, FeatureState>, string, string | undefined]>([
    ['a user turned off recommended', 'provider', { [ISSUER.id]: { enabled: false, source: 'user' } }, ISSUER.title, 'recommended on for Provider — you turned it off'],
    ['a file turned on available', 'provider', { [TRACING.id]: { enabled: true, source: 'loaded' } }, TRACING.title, 'off by default for Provider — your values file turns it on'],
    ['a file turned off recommended', 'provider', { [BROKER.id]: { enabled: false, source: 'loaded' } }, BROKER.title, 'recommended on for Provider — your values file turns it off'],
    ['a user turned on not-offered', 'consumer', { [BROKER.id]: { enabled: true, source: 'user' } }, BROKER.title, 'not recommended for Consumer — you turned it on'],
    ['a user choice equal to the preset', 'provider', { [ISSUER.id]: { enabled: true, source: 'user' } }, ISSUER.title, undefined],
  ])('marks %s', async (_name, role, features, title, marker) => {
    renderFeatures(onFeatures(role, features))
    await findBox(title)
    if (marker) {
      expect(box(title)).toHaveAccessibleDescription(expect.stringContaining(marker))
    } else {
      expect(rowOf(title).querySelector('.deliberate')).toBeNull()
    }
  })

  it('switches a not-offered feature on without asking and marks it', async () => {
    const server = serverFeatures()
    renderFeatures(onFeatures('consumer'), { features: server.features })
    await userEvent.click(await findBox(BROKER.title))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await waitFor(() => expect(box(BROKER.title)).toBeChecked())
    expect(server.toggles().map((body) => body.toggle)).toEqual([{ feature: BROKER.id, enabled: true }])
    expect(rowOf(BROKER.title)).toHaveTextContent('not recommended for Consumer — you turned it on')
    expect(probe.state.answers.features[BROKER.id]).toEqual({ enabled: true, source: 'user' })
  })

  it('sends a second toggle only after the first answered, built on its result', async () => {
    const server = serverFeatures({ deferred: true })
    renderFeatures(onFeatures('consumer'), { features: server.features })
    await userEvent.click(await findBox(TRACING.title))
    await userEvent.click(box(BROKER.title))
    expect(screen.getByRole('group', { name: 'Features' })).toHaveAttribute('aria-busy', 'true')
    expect(server.toggles()).toHaveLength(1)
    server.release()
    await waitFor(() => expect(server.toggles()).toHaveLength(2))
    expect(server.toggles()[1]?.profile.features?.[TRACING.id]).toEqual({ enabled: true, source: 'user' })
    server.release()
    await waitFor(() => expect(box(BROKER.title)).toBeChecked())
    expect(box(TRACING.title)).toBeChecked()
    expect(screen.getByRole('group', { name: 'Features' })).toHaveAttribute('aria-busy', 'false')
    expect(probe.state.answers.features).toMatchObject({ [TRACING.id]: { enabled: true }, [BROKER.id]: { enabled: true } })
  })

  it('drops a response that arrives after the stage was left', async () => {
    const server = serverFeatures({ deferred: true })
    renderFeatures(onFeatures('consumer'), { features: server.features })
    await userEvent.click(await findBox(TRACING.title))
    await userEvent.click(box(BROKER.title))
    const before = probe.state.answers.features
    await userEvent.click(screen.getByRole('button', { name: 'leave' }))
    server.release()
    await new Promise((done) => setTimeout(done))
    expect(probe.state.answers.features).toBe(before)
    expect(server.toggles()).toHaveLength(1)
  })

  it('shows a refused toggle next to its row and keeps the state', async () => {
    renderFeatures(onFeatures('consumer'), { features: serverFeatures({ refuse: true }).features })
    await userEvent.click(await findBox(TRACING.title))
    expect(await within(rowOf(TRACING.title)).findByRole('alert')).toHaveTextContent(refusedRole(HTTP_UNPROCESSABLE))
    expect(box(TRACING.title)).not.toBeChecked()
    expect(probe.state.answers.features[TRACING.id]).toBeUndefined()
  })

  it.each([
    ['its consequence', ISSUER.title, ISSUER.consequence],
    ['its summary when it names no consequence', BROKER.title, BROKER.summary],
  ])('asks before turning off a recommended feature, with %s and the role guide', async (_name, title, text) => {
    renderFeatures(onFeatures('provider'))
    await userEvent.click(await findBox(title))
    const dialog = screen.getByRole('dialog', { name: `Turn off ${title}?` })
    expect(dialog).toHaveAccessibleDescription(expect.stringContaining(text))
    expect(within(dialog).getByRole('link', { name: 'What the Provider role needs ↗' })).toHaveAttribute('href', repositoryDocUrl('roles#provider'))
    expect(within(dialog).getByRole('button', { name: KEEP_ON })).toHaveFocus()
  })

  it.each([
    ['the cancel button', () => userEvent.click(screen.getByRole('button', { name: KEEP_ON }))],
    ['Escape', () => userEvent.keyboard('{Escape}')],
  ])('keeps the feature on after %s', async (_name, cancel) => {
    const server = serverFeatures()
    renderFeatures(onFeatures('provider'), { features: server.features })
    await userEvent.click(await findBox(ISSUER.title))
    await cancel()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(box(ISSUER.title)).toBeChecked()
    expect(server.toggles()).toHaveLength(0)
  })

  it('turns the feature off on confirm and marks the choice', async () => {
    const server = serverFeatures()
    renderFeatures(onFeatures('provider'), { features: server.features })
    await userEvent.click(await findBox(ISSUER.title))
    await userEvent.click(screen.getByRole('button', { name: TURN_OFF }))
    await waitFor(() => expect(box(ISSUER.title)).not.toBeChecked())
    expect(server.toggles().map((body) => body.toggle)).toEqual([{ feature: ISSUER.id, enabled: false }])
    expect(rowOf(ISSUER.title)).toHaveTextContent('recommended on for Provider — you turned it off')
    expect(probe.state.answers.features[ISSUER.id]).toEqual({ enabled: false, source: 'user' })
  })
})
