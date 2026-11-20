/** Stands in for a stage the local trial skips. */
import { useStore } from '../state/context'

/** Label of the button that leaves the trial for a fresh configuration. */
export const START_NEW_LABEL = 'Start a new configuration'

/** Says the stage is skipped; the only way into it is to leave the trial. */
export function Skipped({ label }: { label: string }) {
  const { dispatch } = useStore()
  return (
    <section>
      <h1>{label} — skipped in the local trial</h1>
      <p className="lede">The local trial ships fixed values, so there is nothing to answer here.</p>
      <div className="actions">
        <button type="button" className="primary" onClick={() => dispatch({ type: 'newConfiguration' })}>
          {START_NEW_LABEL}
        </button>
      </div>
    </section>
  )
}
