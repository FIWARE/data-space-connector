/** The "ask later" toggle every markable field carries. */
import { useStore } from '../state/context'
import { fieldLabel, type FieldId } from '../state/openQuestions'

/** Toggles the field's "ask later" marker; the value stays either way. */
export function AskLater({ field }: { field: FieldId }) {
  const { state, dispatch } = useStore()
  const label = fieldLabel(field, state.foundations)
  return (
    <button
      type="button"
      className="ask"
      aria-pressed={state.open.includes(field)}
      aria-label={`Ask ${label} later`}
      title={`Keep ${label} as an open question`}
      onClick={() => dispatch({ type: 'toggleOpen', field })}
    >
      Ask later
    </button>
  )
}
