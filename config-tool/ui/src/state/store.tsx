/** The app-wide store: current screen, stage and answer set, alive for the whole session. */
import { useReducer, type ReactNode } from 'react'
import { StoreContext } from './context'
import { initialState, reducer, type AppState } from './reducer'

/** Holds the store above every screen, so switching stages never loses answers. */
export function StoreProvider({ children, initial = initialState }: { children: ReactNode; initial?: AppState }) {
  const [state, dispatch] = useReducer(reducer, initial)
  return <StoreContext.Provider value={{ state, dispatch }}>{children}</StoreContext.Provider>
}
