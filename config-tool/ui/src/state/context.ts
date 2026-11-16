/** The store context and its hook, apart from StoreProvider so fast refresh keeps working. */
import { createContext, useContext, type Dispatch } from 'react'
import type { Action, AppState } from './reducer'

/** What useStore hands out. */
export interface StoreValue {
  state: AppState
  dispatch: Dispatch<Action>
}

/** Context StoreProvider fills; null outside it. */
export const StoreContext = createContext<StoreValue | null>(null)

/** Store state and dispatch; throws outside a StoreProvider. */
export function useStore(): StoreValue {
  const value = useContext(StoreContext)
  if (!value) {
    throw new Error('useStore must be used inside a StoreProvider')
  }
  return value
}
