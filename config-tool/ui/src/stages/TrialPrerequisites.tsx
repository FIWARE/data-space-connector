/** S0 prerequisite screen of the local trial; it never states a sizing figure, it links the one source that does. */
import { useEffect, useRef, useState } from 'react'
import { fetchTrial } from '../api/trial'
import { QUICK_START_URL, SIZING_URL } from '../links'
import { useStore } from '../state/context'

/** Label of the button that fetches the trial and enters Review. */
export const GENERATE_LABEL = 'Generate the three configurations'

/** What the trial needs and what it produces; Generate fetches the trial and enters Review, a failure stays here.
 *  Unmounting aborts the request, so a late trial never replaces a session started meanwhile. */
export function TrialPrerequisites({ onBack }: { onBack: () => void }) {
  const { dispatch } = useStore()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const request = useRef<AbortController>()

  useEffect(() => () => request.current?.abort(), [])

  async function generate() {
    const controller = new AbortController()
    request.current = controller
    setBusy(true)
    setError(undefined)
    try {
      const trial = await fetchTrial(controller.signal)
      if (!controller.signal.aborted) dispatch({ type: 'trialStarted', trial })
    } catch (failure) {
      if (controller.signal.aborted) return
      setError(`Cannot generate the local trial: ${failure instanceof Error ? failure.message : String(failure)}`)
      setBusy(false)
    }
  }

  return (
    <section className="trial-prerequisites" aria-labelledby="trial-title">
      <h1 id="trial-title">⚡ Try it locally</h1>
      <p className="lede">
        A whole data space on this machine: the Quick Start's trust anchor, provider and consumer. The tool writes their values; you run the install.
      </p>
      <div className="panel">
        <h3>What you need</h3>
        <ul className="notes">
          <li>A local Kubernetes cluster, as in the Quick Start (k3s).</li>
          <li>kubectl and Helm, to install the three releases yourself.</li>
          <li>
            The namespaces, operators, CA and infrastructure the Quick Start applies before the releases.{' '}
            <a href={QUICK_START_URL} target="_blank" rel="noreferrer">
              Quick Start ↗
            </a>
          </li>
          <li>
            Enough memory and disk for three roles.{' '}
            <a href={SIZING_URL} target="_blank" rel="noreferrer">
              Sizing ↗
            </a>
          </li>
        </ul>
      </div>
      <div className="panel">
        <h3>What you get</h3>
        <p>A development configuration: self-signed certificates and chart-deployed databases. Review lists every production warning.</p>
      </div>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
      <div className="actions">
        <button type="button" onClick={onBack}>
          Back
        </button>
        <button type="button" className="primary" disabled={busy} aria-busy={busy} onClick={() => void generate()}>
          {GENERATE_LABEL}
        </button>
      </div>
    </section>
  )
}
