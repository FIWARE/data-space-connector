/** The save action the rail, Review and ⌘S share, apart from SaveProvider so fast refresh keeps working. */
import { createContext, useContext } from 'react'

/** What useSave hands out; `saved` names the profile last written, until answers, markers or the load change. */
export interface SaveValue {
  save: () => void
  busy: boolean
  saved?: string
  /** Why saving is off, e.g. in the local trial; `save` is then a no-op. */
  disabledReason?: string
}

/** Context SaveProvider fills; null outside the chrome. */
export const SaveContext = createContext<SaveValue | null>(null)

/** The shared save action; throws outside a SaveProvider. */
export function useSave(): SaveValue {
  const value = useContext(SaveContext)
  if (!value) {
    throw new Error('useSave must be used inside a SaveProvider')
  }
  return value
}
