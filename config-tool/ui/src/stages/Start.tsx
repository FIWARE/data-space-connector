/** S0 Start: the entry point shown before the chrome. */
import { DOCUMENTATION_URL, RELEASE_NOTES_URL } from '../links'
import { useStore } from '../state/context'

/** A start card that a later ticket enables. */
interface ComingCard {
  icon: string
  title: string
  text: string
  comingIn: string
}

const COMING: readonly ComingCard[] = [
  { icon: '📂', title: 'Open values.yaml', text: 'Read an existing file, change it, keep everything it already contains.', comingIn: 'CT-2/CT-3' },
  { icon: '⟲', title: 'Open profile', text: 'Continue a configuration saved earlier.', comingIn: 'CT-4' },
  { icon: '⚡', title: 'Try it locally', text: 'A whole data space on this machine: trust anchor, provider and consumer.', comingIn: 'CT-5' },
]

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
        {COMING.map((card) => (
          <button key={card.title} type="button" className="card" aria-disabled="true">
            <h3>
              {card.icon} {card.title}
            </h3>
            <p>{card.text}</p>
            <p className="hint">Coming in {card.comingIn}</p>
          </button>
        ))}
      </div>
      <p className="links">
        <a href={DOCUMENTATION_URL} target="_blank" rel="noreferrer">
          Documentation ↗
        </a>
        <a href={RELEASE_NOTES_URL} target="_blank" rel="noreferrer">
          Release notes ↗
        </a>
      </p>
    </main>
  )
}
