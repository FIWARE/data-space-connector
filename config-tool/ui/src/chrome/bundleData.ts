/** Loads the target chart's foundations and Stage 4 dependencies for every stage: the rail, footer and Review count
 *  Stage 4's open questions whether or not it was visited. */
import { createContext, useContext, useEffect, useRef, useState } from 'react'
import { fetchFoundations } from '../api/foundations'
import { fetchInfrastructure } from '../api/infrastructure'
import { useStore } from '../state/context'

/** Why the last load failed; undefined while it has not. */
export const BundleDataError = createContext<string | undefined>(undefined)

/** The load error the chrome provides, for the stage that shows it. */
export function useBundleDataError(): string | undefined {
  return useContext(BundleDataError)
}

/** Fetches the dependencies on every chart or version change and the foundations while unknown; returns the error. */
export function useBundleData(): string | undefined {
  const { state, dispatch } = useStore()
  const { chart, chartVersion } = state.answers.target
  const [error, setError] = useState<string>()
  const dependenciesKnown = state.dependencies !== undefined
  const foundationsKnown = state.foundations !== undefined
  // A version change keeps the same chart's dependencies in the store, so the key tells a stale list apart.
  const loadedFor = useRef('')

  useEffect(() => {
    const key = `${chart}@${chartVersion}`
    if (chart === '' || (dependenciesKnown && loadedFor.current === key)) return
    const controller = new AbortController()
    fetchInfrastructure(chart, chartVersion, controller.signal).then(
      (list) => {
        loadedFor.current = key
        setError(undefined)
        dispatch({ type: 'infrastructureLoaded', dependencies: list.dependencies })
      },
      (err: Error) => controller.signal.aborted || setError(err.message),
    )
    return () => controller.abort()
  }, [chart, chartVersion, dependenciesKnown, dispatch])

  // The per-host defaults derive from Stage 3's hosts, so their foundations are needed even if Stage 3 was skipped.
  useEffect(() => {
    if (chart === '' || foundationsKnown) return
    const controller = new AbortController()
    fetchFoundations(chart, chartVersion, controller.signal).then(
      (list) => dispatch({ type: 'foundationsLoaded', foundations: list.foundations }),
      (err: Error) => controller.signal.aborted || setError(err.message),
    )
    return () => controller.abort()
  }, [chart, chartVersion, foundationsKnown, dispatch])

  return error
}
