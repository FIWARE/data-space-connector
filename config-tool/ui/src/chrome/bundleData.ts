/** Loads the target chart's foundations and Stage 4 dependencies for every stage: the rail, footer and Review count
 *  Stage 4's open questions whether or not it was visited. */
import { createContext, useContext, useEffect, useRef, useState } from 'react'
import { fetchFoundations } from '../api/foundations'
import { fetchInfrastructure } from '../api/infrastructure'
import { useStore } from '../state/context'
import type { Action } from '../state/reducer'

/** Why the last load failed; undefined while it has not. */
export const BundleDataError = createContext<string | undefined>(undefined)

/** The load error the chrome provides, for the stage that shows it. */
export function useBundleDataError(): string | undefined {
  return useContext(BundleDataError)
}

type Load = (chart: string, version: string, signal: AbortSignal) => Promise<Action>

const loadInfrastructure: Load = (chart, version, signal) =>
  fetchInfrastructure(chart, version, signal).then((list) => ({ type: 'infrastructureLoaded', dependencies: list.dependencies }))

const loadFoundations: Load = (chart, version, signal) =>
  fetchFoundations(chart, version, signal).then((list) => ({ type: 'foundationsLoaded', foundations: list.foundations }))

const INFRASTRUCTURE_FAILED: Action = { type: 'infrastructureFailed' }

/** Fetches and dispatches `load` once per chart@version, unless the store already holds what was loaded for it;
 *  returns this key's fetch error. A failure dispatches `failed`, so a list of another version is not counted. */
function useKeyedLoad(chart: string, chartVersion: string, known: boolean, load: Load, failed?: Action): string | undefined {
  const { dispatch } = useStore()
  const [error, setError] = useState<{ key: string; message: string }>()
  // A version change keeps the old list in the store, so `known` alone cannot tell it is stale.
  const loadedFor = useRef('')
  // Without this, `failed` clearing the store would retry the same key at once.
  const failedFor = useRef('')
  const key = `${chart}@${chartVersion}`

  useEffect(() => {
    if (chart === '' || (known ? loadedFor.current : failedFor.current) === key) return
    const controller = new AbortController()
    load(chart, chartVersion, controller.signal).then(
      (action) => {
        loadedFor.current = key
        failedFor.current = ''
        setError(undefined)
        dispatch(action)
      },
      (err: Error) => {
        if (controller.signal.aborted) return
        failedFor.current = key
        loadedFor.current = ''
        setError({ key, message: err.message })
        if (failed) dispatch(failed)
      },
    )
    return () => controller.abort()
  }, [chart, chartVersion, key, known, load, failed, dispatch])

  return error?.key === key ? error.message : undefined
}

/** Loads the dependencies and the foundations (the per-host defaults derive from Stage 3's hosts, even if Stage 3
 *  was skipped) for the current chart@version; returns the first fetch error. */
export function useBundleData(): string | undefined {
  const { state } = useStore()
  const { chart, chartVersion } = state.answers.target
  const infraError = useKeyedLoad(chart, chartVersion, state.dependencies !== undefined, loadInfrastructure, INFRASTRUCTURE_FAILED)
  const foundationsError = useKeyedLoad(chart, chartVersion, state.foundations !== undefined, loadFoundations)
  return infraError ?? foundationsError
}
