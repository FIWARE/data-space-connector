/** S0 Start: the entry point shown before the chrome. */
import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { ApiError } from '../api/client'
import { loadValues } from '../api/load'
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
  { icon: '⟲', title: 'Open profile', text: 'Continue a configuration saved earlier.', comingIn: 'CT-4' },
  { icon: '⚡', title: 'Try it locally', text: 'A whole data space on this machine: trust anchor, provider and consumer.', comingIn: 'CT-5' },
]

const HTTP_PAYLOAD_TOO_LARGE = 413
/** The server's upload cap (`maxBodyBytes`), in MiB. */
const MAX_UPLOAD_MIB = 8
/** Accessible name of the hidden file input the Open card triggers. */
export const OPEN_INPUT_LABEL = 'values.yaml to open'

function loadFailure(name: string, error: unknown): string {
  if (error instanceof ApiError && error.status === HTTP_PAYLOAD_TOO_LARGE) {
    return `Cannot open ${name}: it is larger than ${MAX_UPLOAD_MIB} MiB.`
  }
  return `Cannot open ${name}: ${error instanceof Error ? error.message : String(error)}`
}

/** The Open values.yaml card: uploads the picked file and enters Review with its report; a failure stays on S0.
 *  Unmounting aborts the upload, so a late report never replaces a session started meanwhile. */
function OpenValuesCard({ busy, setBusy, setError }: { busy: boolean; setBusy: (busy: boolean) => void; setError: (error?: string) => void }) {
  const { dispatch } = useStore()
  const input = useRef<HTMLInputElement>(null)
  const upload = useRef<AbortController>()

  useEffect(() => () => upload.current?.abort(), [])

  async function open(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    // Cleared so picking the same file again after a failure fires change again.
    event.target.value = ''
    if (!file) return
    const controller = new AbortController()
    upload.current = controller
    setBusy(true)
    setError(undefined)
    try {
      const report = await loadValues(file, controller.signal)
      if (!controller.signal.aborted) dispatch({ type: 'fileLoaded', report })
    } catch (failure) {
      if (controller.signal.aborted) return
      setError(loadFailure(file.name, failure))
      setBusy(false)
    }
  }

  return (
    <>
      <button type="button" className="card" aria-busy={busy} aria-disabled={busy} onClick={() => !busy && input.current?.click()}>
        <h3>📂 Open values.yaml</h3>
        <p>Read an existing file, change it, keep everything it already contains.</p>
        {busy && <p className="hint">Reading…</p>}
      </button>
      <input ref={input} type="file" accept=".yaml,.yml" aria-label={OPEN_INPUT_LABEL} hidden onChange={(event) => void open(event)} />
    </>
  )
}

/** S0 Start screen. */
export function Start() {
  const { dispatch } = useStore()
  const [loadError, setLoadError] = useState<string>()
  // A running upload would override a new configuration when it lands, so New waits for it.
  const [loading, setLoading] = useState(false)
  return (
    <main className="start">
      <h1>FIWARE Data Space Connector — Configurator</h1>
      <p className="lede">Configure and install a connector for your organization.</p>
      <div className="cards">
        <button type="button" className="card" aria-disabled={loading} onClick={() => !loading && dispatch({ type: 'newConfiguration' })}>
          <h3>＋ New configuration</h3>
          <p>Start from a role and go through the wizard.</p>
        </button>
        <OpenValuesCard busy={loading} setBusy={setLoading} setError={setLoadError} />
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
      {loadError && (
        <p className="field-error" role="alert">
          {loadError}
        </p>
      )}
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
