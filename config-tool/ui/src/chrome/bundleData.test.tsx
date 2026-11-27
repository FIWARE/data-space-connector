import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useStore } from '../state/context'
import { initialState, type AppState } from '../state/reducer'
import { StoreProvider } from '../state/store'
import { FOUNDATIONS, HTTP_NOT_FOUND, INFRASTRUCTURE, mockApi, refusedFoundations, refusedInfrastructure } from '../test/api'
import { useBundleData } from './bundleData'

const OLD_VERSION = '10.15.0'
const NEW_VERSION = '10.16.0'

const STATE: AppState = { ...initialState, answers: { ...initialState.answers, target: { ...initialState.answers.target, chartVersion: OLD_VERSION } } }

function Probe() {
  const { state, dispatch } = useStore()
  const error = useBundleData()
  return (
    <>
      {[OLD_VERSION, NEW_VERSION].map((chartVersion) => (
        <button key={chartVersion} type="button" onClick={() => dispatch({ type: 'setTarget', target: { chartVersion } })}>
          {chartVersion}
        </button>
      ))}
      <output aria-label="error">{error ?? ''}</output>
      <output aria-label="dependencies">{state.dependencies?.length ?? 'none'}</output>
      <output aria-label="foundations">{state.foundations?.length ?? 'none'}</output>
    </>
  )
}

const shown = (name: string) => screen.getByRole('status', { name }).textContent
const setVersion = (version: string) => fireEvent.click(screen.getByRole('button', { name: version }))

/** Fails each route for the versions it lists. */
function renderProbe(failInfrastructure: string[], failFoundations: string[]) {
  const fetchMock = mockApi(
    undefined,
    undefined,
    undefined,
    {},
    {},
    {},
    { foundations: (_chart, version) => (failFoundations.includes(version) ? HTTP_NOT_FOUND : FOUNDATIONS) },
    {},
    (_chart, version) => (failInfrastructure.includes(version) ? HTTP_NOT_FOUND : INFRASTRUCTURE),
  )
  render(
    <StoreProvider initial={STATE}>
      <Probe />
    </StoreProvider>,
  )
  return fetchMock
}

const DEPENDENCY_COUNT = String(INFRASTRUCTURE.dependencies.length)
const FOUNDATION_COUNT = String(FOUNDATIONS.foundations.length)

describe('useBundleData', () => {
  it.each<[string, string[], string[], { error: string; dependencies: string; foundations: string }]>([
    ['both load', [], [], { error: '', dependencies: DEPENDENCY_COUNT, foundations: FOUNDATION_COUNT }],
    [
      'a foundations error survives the dependencies loading',
      [],
      [OLD_VERSION],
      { error: refusedFoundations(HTTP_NOT_FOUND), dependencies: DEPENDENCY_COUNT, foundations: 'none' },
    ],
    [
      'a dependencies error shows while the foundations load',
      [OLD_VERSION],
      [],
      { error: refusedInfrastructure(HTTP_NOT_FOUND), dependencies: 'none', foundations: FOUNDATION_COUNT },
    ],
  ])('%s', async (_name, failInfrastructure, failFoundations, want) => {
    renderProbe(failInfrastructure, failFoundations)
    await waitFor(() => expect({ error: shown('error'), dependencies: shown('dependencies'), foundations: shown('foundations') }).toEqual(want))
  })

  it('drops the previous version’s dependencies when the new version’s fail, and clears the error on a good version', async () => {
    const fetchMock = renderProbe([NEW_VERSION], [])
    await waitFor(() => expect(shown('dependencies')).toBe(DEPENDENCY_COUNT))

    setVersion(NEW_VERSION)
    await waitFor(() => expect(shown('error')).toBe(refusedInfrastructure(HTTP_NOT_FOUND)))
    expect(shown('dependencies')).toBe('none')
    const calls = (path: string) => fetchMock.mock.calls.filter(([input]) => String(input).includes(path)).length
    // Clearing the stale list does not retry the version that failed.
    expect(calls('/api/infrastructure')).toBe(2)
    // The foundations are refetched for the new version although the store still held the old ones.
    expect(calls('/api/foundations')).toBe(2)

    setVersion(OLD_VERSION)
    await waitFor(() => expect(shown('dependencies')).toBe(DEPENDENCY_COUNT))
    expect(shown('error')).toBe('')
  })
})
