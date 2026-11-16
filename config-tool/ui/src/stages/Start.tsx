/** S0 Start: the entry point shown before the chrome. */
import { useStore } from '../state/context'

/** S0 Start screen. */
export function Start() {
  const { dispatch } = useStore()
  return (
    <main className="start">
      <h1>FIWARE Data Space Connector — Configurator</h1>
      <p className="lede">Configure and install a connector for your organization.</p>
      <div className="cards">
        <button type="button" className="card" onClick={() => dispatch({ type: 'newConfiguration' })}>
          <h3>＋ New configuration</h3>
          <p>Start from a role and go through the wizard.</p>
        </button>
      </div>
    </main>
  )
}
