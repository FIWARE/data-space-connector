/** App root: the store, then S0 until an entry is chosen, then the chrome. */
import { Chrome } from './chrome/Chrome'
import { STAGES } from './stages'
import { Start } from './stages/Start'
import { useStore } from './state/context'
import { StoreProvider } from './state/store'

function Screen() {
  const { state } = useStore()
  return state.entered ? <Chrome stages={STAGES} /> : <Start />
}

/** The whole UI; routing is state-driven, not URL-driven. */
export function App() {
  return (
    <StoreProvider>
      <Screen />
    </StoreProvider>
  )
}
