/** S7 Review's "Set by hand" section: answers given in place of their derived default, which no longer follow it. */
import { useEffect, useRef, useState } from 'react'
import { toProfile } from '../api/profile'
import { fetchRender, type HandSetAnswer } from '../api/render'
import { useStore } from '../state/context'
import { answerErrors } from '../state/foundations'
import { answerFieldId, fieldLabel } from '../state/openQuestions'
import { HAND_SET_HEADING, HAND_SET_HINT, wouldDerive } from './handSetText'

/** The hand-set answers of the current answers; none while there is no answer or one is invalid. */
function useHandSet(): { entries: readonly HandSetAnswer[]; error?: string } {
  const { state } = useStore()
  const values = state.answers.values
  const sha = state.loaded?.sha256
  const latest = useRef(state)
  const [result, setResult] = useState<{ entries: readonly HandSetAnswer[]; error?: string }>({ entries: [] })
  const skip = Object.keys(values).length === 0 || Object.keys(answerErrors(values, state.foundations)).length > 0

  useEffect(() => {
    latest.current = state
  }, [state])

  useEffect(() => {
    if (skip) return
    const controller = new AbortController()
    fetchRender(toProfile(latest.current), sha, controller.signal).then(
      (r) => setResult({ entries: r.handSet }),
      (err: Error) => controller.signal.aborted || setResult({ entries: [], error: err.message }),
    )
    return () => controller.abort()
  }, [values, sha, skip])
  return skip ? { entries: [] } : result
}

/** The section, rendered only when an answer is set by hand or the render failed. */
export function HandSet() {
  const { state, dispatch } = useStore()
  const { entries, error } = useHandSet()
  if (entries.length === 0 && !error) return null
  return (
    <div className="panel">
      <h3>{HAND_SET_HEADING}</h3>
      {error ? (
        <p className="field-error" role="alert">
          {error}
        </p>
      ) : (
        <>
          <p className="hint">{HAND_SET_HINT}</p>
          <ul className="open-questions">
            {entries.map((entry) => {
              const field = answerFieldId(entry.id)
              const label = fieldLabel(field, state.foundations)
              return (
                <li key={entry.id}>
                  <span>{label}</span>
                  <code>{String(entry.value)}</code>
                  {entry.derived !== '' && entry.derived !== entry.value && <span className="stage">{wouldDerive(entry.derived)}</span>}
                  <button type="button" className="link" aria-label={`Go to ${label}`} onClick={() => dispatch({ type: 'focusField', field })}>
                    go ↗
                  </button>
                </li>
              )
            })}
          </ul>
        </>
      )}
    </div>
  )
}
