/** S7 Review of the local trial: the three releases, the production warnings and writing the three values files. */
import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { ApiError } from '../api/client'
import { fetchRecent, HTTP_CONFLICT } from '../api/profile'
import { writeTrial, type Trial, type TrialRelease, type TrialWarning } from '../api/trial'
import { QUICK_START_URL } from '../links'

/** Accessible name of the directory field. */
export const TRIAL_DIR_LABEL = 'Directory'
/** Label of the write button. */
export const WRITE_LABEL = 'Write files'
/** Added to a 409: the write is all-or-nothing, so a conflict leaves the directory as it was. */
export const NOTHING_WRITTEN = 'Nothing was written.'

function message(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure)
}

function Release({ release }: { release: TrialRelease }) {
  return (
    <li className="trial-release">
      <h4>{release.releaseName}</h4>
      <dl className="report-counts">
        <div>
          <dt>Namespace</dt>
          <dd>{release.namespace}</dd>
        </div>
        <div>
          <dt>Chart</dt>
          <dd>
            {release.chart}@{release.chartVersion}
          </dd>
        </div>
        <div>
          <dt>File</dt>
          <dd>
            <code>{release.file}</code>
          </dd>
        </div>
        <div>
          <dt>Same values as</dt>
          <dd>
            <code>{release.source}</code>
          </dd>
        </div>
      </dl>
      <details>
        <summary>Values of {release.releaseName}</summary>
        <pre>{release.values}</pre>
      </details>
    </li>
  )
}

/** Every production warning; deliberately without a dismiss or hide control. */
function Warnings({ warnings }: { warnings: readonly TrialWarning[] }) {
  return (
    <div className="panel">
      <h3>Production warnings ({warnings.length})</h3>
      <ul className="notes">
        {warnings.map((warning) => (
          <li key={warning.id}>
            ⚠ {warning.message} <span className="hint">({warning.releases.join(', ')})</span>
            {warning.link && (
              <>
                {' '}
                <a href={warning.link} target="_blank" rel="noreferrer">
                  more ↗
                </a>
              </>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}

/** The directory form; all three files are written or none. The default is the recent list's directory. */
function WriteFiles() {
  const [dir, setDir] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const [written, setWritten] = useState<readonly string[]>()

  useEffect(() => {
    const controller = new AbortController()
    fetchRecent(controller.signal)
      .then((recent) => setDir((typed) => typed || recent.defaultDir))
      .catch(() => {
        // Without a default the field stays empty; the user types the directory.
      })
    return () => controller.abort()
  }, [])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy) return
    setBusy(true)
    setError(undefined)
    setWritten(undefined)
    try {
      setWritten((await writeTrial(dir)).files)
    } catch (failure) {
      setError(failure instanceof ApiError && failure.status === HTTP_CONFLICT ? `${failure.message} ${NOTHING_WRITTEN}` : message(failure))
    }
    setBusy(false)
  }

  return (
    <form className="panel" onSubmit={(event) => void submit(event)}>
      <h3>Write the values files</h3>
      <div className="row">
        <label htmlFor="trial-dir">{TRIAL_DIR_LABEL}</label>
        <input id="trial-dir" type="text" value={dir} spellCheck={false} onChange={(event) => setDir(event.target.value)} />
      </div>
      <p className="hint">An existing file is never overwritten: if one of the three exists, none is written.</p>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
      {written && (
        <div role="status">
          <p>Written:</p>
          <ul className="notes">
            {written.map((path) => (
              <li key={path}>
                <code>{path}</code>
              </li>
            ))}
          </ul>
        </div>
      )}
      <div className="actions">
        <button type="submit" className="primary" disabled={busy || dir.trim() === ''} aria-busy={busy}>
          {WRITE_LABEL}
        </button>
      </div>
    </form>
  )
}

/** Review of the local trial; stands in for the load report while `state.trial` is set. `children` go last. */
export function TrialReview({ trial, children }: { trial: Trial; children?: ReactNode }) {
  return (
    <section>
      <h1>
        Local trial <span className="badge">Development</span>
      </h1>
      <p className="lede">
        Equivalent to the Quick Start; the tool emits values, you run the install.{' '}
        <a href={QUICK_START_URL} target="_blank" rel="noreferrer">
          Quick Start ↗
        </a>
      </p>
      <div className="panel">
        <h3>Releases ({trial.releases.length}), in install order</h3>
        <ol className="trial-releases">
          {trial.releases.map((release) => (
            <Release key={release.releaseName} release={release} />
          ))}
        </ol>
      </div>
      <Warnings warnings={trial.warnings} />
      <WriteFiles />
      {children}
    </section>
  )
}
