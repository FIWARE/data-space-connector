/** S3's *Data space* panel: import from a descriptor, the TIR address with its check, credential types and registration. */
import { useEffect, useRef, useState } from 'react'
import { checkTIR, importDescriptor, type ImportResult } from '../api/dataspace'
import type { CredentialType, Foundation } from '../api/foundations'
import { toProfile } from '../api/profile'
import type { Finding } from '../api/render'
import { AskLater } from '../chrome/AskLater'
import { useStore } from '../state/context'
import { AUTO_REGISTRATION_ID, CREDENTIAL_TYPES_ID, TIR_ADDRESS_ID, tirCheckOf } from '../state/dataspace'
import { answerErrors, reservedRefs } from '../state/foundations'
import { answerFieldId, fieldElementId } from '../state/openQuestions'
import { AnswerField, FieldNotes } from './AnswerField'
import {
  ADD_TYPE,
  APPLY_LABEL,
  CHECK_LABEL,
  CHECKING,
  CREDENTIAL_TYPES_HINT,
  DATASPACE_PANEL,
  DESCRIPTION_COLUMN,
  DISCARD_LABEL,
  IMPORT_BUTTON,
  IMPORT_HINT,
  IMPORT_LABEL,
  IMPORTING,
  KNOWN_LABEL,
  NO_CHANGES,
  PREVIEW_LABEL,
  previewValue,
  PROBLEMS_HEADING,
  rowError,
  rowLabels,
  TYPE_COLUMN,
  UNUSED_LABEL,
} from './dataSpaceText'
import { describedBy } from './foundationText'
import { TirOutcome } from './TirCheck'

const IMPORT_URL_ID = 'dataspace-import-url'

function message(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure)
}

/** What an import would change; only Apply sets the answers, Discard drops the preview. */
function Preview({ result, foundations, onApply, onDiscard }: { result: ImportResult; foundations: readonly Foundation[]; onApply: () => void; onDiscard: () => void }) {
  const { changes, problems, known, unused, message: info } = result.preview
  const label = (id: string) => foundations.find((f) => f.id === id)?.label ?? id
  const knownKeys = Object.keys(known ?? {}).sort()
  return (
    <div className="import-preview" role="group" aria-label={PREVIEW_LABEL}>
      <p className="hint">
        Read <code>{result.url}</code>
      </p>
      {info && <p className="info">ⓘ {info}</p>}
      {changes.length > 0 ? (
        <ul className="notes">
          {changes.map((c) => (
            <li key={c.id}>
              {label(c.id)}: {previewValue(c.current)} → {previewValue(c.imported)}
            </li>
          ))}
        </ul>
      ) : (
        !info && <p className="hint">{NO_CHANGES}</p>
      )}
      {problems.length > 0 && (
        <>
          <h4>{PROBLEMS_HEADING}</h4>
          <ul className="notes">
            {problems.map((p) => (
              <li key={p.key} className="field-error">
                {label(p.id)}: <code>{p.key}</code> {p.message}
              </li>
            ))}
          </ul>
        </>
      )}
      {knownKeys.length > 0 && (
        <p className="hint">
          {KNOWN_LABEL} {knownKeys.join(', ')}
        </p>
      )}
      {unused.length > 0 && (
        <p className="hint">
          {UNUSED_LABEL} {unused.join(', ')}
        </p>
      )}
      <div className="actions">
        <button type="button" disabled={changes.length === 0} onClick={onApply}>
          {APPLY_LABEL}
        </button>
        <button type="button" className="sec" onClick={onDiscard}>
          {DISCARD_LABEL}
        </button>
      </div>
    </div>
  )
}

/** The descriptor URL, the Import button and, in a live region, the preview or why the import failed. */
function Import({ foundations }: { foundations: readonly Foundation[] }) {
  const { state, dispatch } = useStore()
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const [result, setResult] = useState<ImportResult>()

  async function run() {
    setBusy(true)
    setError(undefined)
    setResult(undefined)
    try {
      setResult(await importDescriptor(url.trim(), toProfile(state)))
    } catch (failure) {
      setError(message(failure))
    } finally {
      setBusy(false)
    }
  }

  function apply() {
    for (const c of result?.preview.changes ?? []) dispatch({ type: 'setAnswer', id: c.id, value: c.imported })
    setResult(undefined)
  }

  return (
    <>
      <div className="row">
        <label htmlFor={IMPORT_URL_ID}>{IMPORT_LABEL}</label>
        <input id={IMPORT_URL_ID} type="url" value={url} aria-describedby={`${IMPORT_URL_ID}-help`} onChange={(e) => setUrl(e.target.value)} />
        <button type="button" className="sec" disabled={busy || url.trim() === ''} onClick={() => void run()}>
          {busy ? IMPORTING : IMPORT_BUTTON}
        </button>
        <p className="hint" id={`${IMPORT_URL_ID}-help`}>
          {IMPORT_HINT}
        </p>
      </div>
      <div aria-live="polite">
        {error && <p className="field-error">{error}</p>}
        {result && <Preview result={result} foundations={foundations} onApply={apply} onDiscard={() => setResult(undefined)} />}
      </div>
    </>
  )
}

/** The Check button and its outcome; sends one request per press and never on its own. */
function CheckTir() {
  const { state, dispatch } = useStore()
  const check = tirCheckOf(state)
  const invalid = answerErrors(state.answers.values, state.foundations, reservedRefs(state.answers.target))[TIR_ADDRESS_ID] !== undefined
  const [busy, setBusy] = useState(false)
  // A refused check belongs to the address it ran for; once the address changes it reads as not checked.
  const [error, setError] = useState<{ address: string; message: string }>()
  const shownError = error?.address === check?.address ? error?.message : undefined

  async function run(address: string) {
    setBusy(true)
    setError(undefined)
    try {
      const r = await checkTIR(address)
      const detail = r.status === 'reachable' ? r.httpStatus && `HTTP ${r.httpStatus}` : r.error
      dispatch({ type: 'tirChecked', check: { address, outcome: r.status, detail: detail || undefined } })
    } catch (failure) {
      setError({ address, message: message(failure) })
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <button type="button" className="sec" disabled={busy || !check || invalid} onClick={() => check && void run(check.address)}>
        {busy ? CHECKING : CHECK_LABEL}
      </button>
      <span role="status">{shownError ? <span className="field-error">{shownError}</span> : check && !invalid && <TirOutcome check={check} />}</span>
    </>
  )
}

/** The ordered credential-type list as a table: type and description per row; add, remove and move up or down. */
function CredentialTypesField({ foundation, findings }: { foundation: Foundation; findings: readonly Finding[] }) {
  const { state, dispatch } = useStore()
  const field = answerFieldId(foundation.id)
  const id = fieldElementId(field)
  const given = state.answers.values[foundation.id]
  const rows: CredentialType[] = Array.isArray(given) ? given : []
  const raw = answerErrors(state.answers.values, state.foundations, reservedRefs(state.answers.target))[foundation.id]
  const located = raw === undefined ? undefined : rowError(raw)
  const error = located?.text
  // Marks the input the error points at; an error without a part covers both inputs of its row.
  const invalidProps = (i: number, part: 'type' | 'description') =>
    located?.row === i && (located.part ?? part) === part ? { 'aria-invalid': true, 'aria-describedby': `${id}-error` } : {}
  const own = findings.filter((f) => f.field === field)
  // The row whose type input takes focus once rendered: the one just added.
  const focusRow = useRef<number>(undefined)

  useEffect(() => {
    if (focusRow.current === undefined) return
    document.getElementById(`${id}-type-${focusRow.current}`)?.focus()
    focusRow.current = undefined
  })

  const set = (next: CredentialType[]) => dispatch({ type: 'setAnswer', id: foundation.id, value: next })
  // An empty description is none, which the answer's check would refuse as a value.
  const edit = (i: number, key: keyof CredentialType, text: string) =>
    set(
      rows.map((r, j) => {
        if (j !== i) return r
        if (key === 'type') return { ...r, type: text }
        const { type } = r
        return text === '' ? { type } : { type, description: text }
      }),
    )
  const move = (i: number, to: number) => {
    const next = [...rows]
    next.splice(to, 0, ...next.splice(i, 1))
    set(next)
  }
  const add = () => {
    focusRow.current = rows.length
    set([...rows, { type: '' }])
  }

  return (
    // tabIndex lets a Review link focus the list.
    <fieldset className="row" id={id} tabIndex={-1} aria-describedby={describedBy(id, error, own)}>
      <legend>{foundation.label}</legend>
      {rows.length > 0 && (
        <table className="entries credential-types">
          <thead>
            <tr>
              <th scope="col">{TYPE_COLUMN}</th>
              <th scope="col">{DESCRIPTION_COLUMN}</th>
              <td />
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => {
              const labels = rowLabels(i + 1)
              return (
                // Rows have no identity of their own while their type is being typed, so the position is the key.
                <tr key={i}>
                  <td>
                    <input id={`${id}-type-${i}`} aria-label={labels.type} {...invalidProps(i, 'type')} value={r.type} onChange={(e) => edit(i, 'type', e.target.value)} />
                  </td>
                  <td>
                    <input aria-label={labels.description} {...invalidProps(i, 'description')} value={r.description ?? ''} onChange={(e) => edit(i, 'description', e.target.value)} />
                  </td>
                  <td>
                    <button type="button" className="link" aria-label={labels.up} disabled={i === 0} onClick={() => move(i, i - 1)}>
                      ↑
                    </button>{' '}
                    <button type="button" className="link" aria-label={labels.down} disabled={i === rows.length - 1} onClick={() => move(i, i + 1)}>
                      ↓
                    </button>{' '}
                    <button type="button" className="link" aria-label={labels.remove} onClick={() => set(rows.filter((_, j) => j !== i))}>
                      ✕
                    </button>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
      <button type="button" className="link" onClick={add}>
        {ADD_TYPE}
      </button>
      <AskLater field={field} />
      <FieldNotes foundation={foundation} id={id} error={error} own={own} />
      <p className="hint">{CREDENTIAL_TYPES_HINT}</p>
    </fieldset>
  )
}

/** A yes/no answer as a checkbox; unanswered shows unchecked. */
function BoolField({ foundation, findings }: { foundation: Foundation; findings: readonly Finding[] }) {
  const { state, dispatch } = useStore()
  const field = answerFieldId(foundation.id)
  const id = fieldElementId(field)
  const own = findings.filter((f) => f.field === field)
  return (
    <div className="row">
      <input
        type="checkbox"
        id={id}
        checked={state.answers.values[foundation.id] === true}
        aria-describedby={describedBy(id, undefined, own)}
        onChange={(e) => dispatch({ type: 'setAnswer', id: foundation.id, value: e.target.checked })}
      />
      <label htmlFor={id}>{foundation.label}</label>
      <AskLater field={field} />
      <FieldNotes foundation={foundation} id={id} own={own} />
    </div>
  )
}

/** The panel; a foundation the bundle does not declare is left out. */
export function DataSpacePanel({ foundations, findings }: { foundations: readonly Foundation[]; findings: readonly Finding[] }) {
  const byId = (id: string) => foundations.find((f) => f.id === id)
  const tir = byId(TIR_ADDRESS_ID)
  const types = byId(CREDENTIAL_TYPES_ID)
  const registration = byId(AUTO_REGISTRATION_ID)
  return (
    <div className="panel" role="group" aria-labelledby="foundations-dataspace">
      <h3 id="foundations-dataspace">{DATASPACE_PANEL}</h3>
      <Import foundations={foundations} />
      {tir && (
        <AnswerField foundation={tir} findings={findings}>
          <CheckTir />
        </AnswerField>
      )}
      {types && <CredentialTypesField foundation={types} findings={findings} />}
      {registration && <BoolField foundation={registration} findings={findings} />}
    </div>
  )
}
