/** The Expert view: outside the journey, it covers the current stage until the next stage jump. */
import { useStore } from '../state/context'
import { carriedOverCount } from '../state/report'
import { Overrides } from './Overrides'

/** Overrides carried over from the loaded file; the YAML editor comes with Epic 5. */
export function Expert() {
  const { state } = useStore()
  const report = state.loaded
  return (
    <section>
      <h1>Expert</h1>
      <p className="lede">Keys the tool does not own, and the raw YAML.</p>
      {report && (
        <div className="panel">
          <h3>Overrides ({carriedOverCount(report)})</h3>
          <p className="hint">
            From {report.source}. They are kept verbatim and written back in place; no answer you give changes them.
          </p>
          <Overrides report={report} />
        </div>
      )}
      <div className="panel">
        <h3>YAML editor</h3>
        <button type="button" className="sec" disabled title="Not available yet (Epic 5)">
          Edit YAML
        </button>
      </div>
    </section>
  )
}
