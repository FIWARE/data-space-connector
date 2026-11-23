/** Loads the role catalogue for a target chart and version, shared by S2 Role and S5 Features. */
import { useEffect, useState } from 'react'
import { fetchRoles, type RoleCatalogue } from '../api/roles'

/** The catalogue once GET /api/roles answers, or its error; an answer for a replaced chart or version is dropped. */
export function useRoles(chart: string, version: string): { catalogue?: RoleCatalogue; error?: string } {
  const [result, setResult] = useState<{ catalogue?: RoleCatalogue; error?: string }>({})
  useEffect(() => {
    const controller = new AbortController()
    fetchRoles(chart, version, controller.signal).then(
      (catalogue) => setResult({ catalogue }),
      (err: Error) => controller.signal.aborted || setResult({ error: err.message }),
    )
    return () => controller.abort()
  }, [chart, version])
  return result
}
