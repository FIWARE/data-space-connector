/** S7 Review's "Set by hand" section: answers given in place of their derived default, pinned, not updated. */
import type { HandSetAnswer } from '../api/render'
import { useStore } from '../state/context'
import { answerFieldId, fieldLabel } from '../state/openQuestions'
import { HAND_SET_HEADING, HAND_SET_HINT, PINNED_LABEL, wouldDerive } from './handSetText'

/** The section, rendered only when an answer is set by hand or the render failed. */
export function HandSet({ entries, error }: { entries: readonly HandSetAnswer[]; error?: string }) {
  const { state, dispatch } = useStore()
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
                  <span className="deliberate">{PINNED_LABEL}</span>
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
