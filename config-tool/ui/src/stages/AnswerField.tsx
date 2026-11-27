/** A foundation's answer field and the notes every Stage 3 field shares: error, findings, help, doc link and fan-out. */
import { useId, useState, type ReactNode } from 'react'
import type { Foundation } from '../api/foundations'
import type { Finding } from '../api/render'
import { AskLater } from '../chrome/AskLater'
import { repositoryDocUrl } from '../links'
import { useStore } from '../state/context'
import { answerErrors, defaultApplies, derivedDefault, reservedRefs, resolvedAnswer, textAnswer, validAnswers } from '../state/foundations'
import { answerFieldId, fieldElementId, type FieldId } from '../state/openQuestions'
import { FindingNote } from './Findings'
import { describedBy, findingNoteId, usedByText } from './foundationText'

export const RESET_LABEL = 'Reset to derived'

/** Where a field's answer lives when it is not a Stage 3 answer: its field id, value, format error and setter. */
export interface FieldBinding {
  field: FieldId
  value: string | undefined
  error?: string
  set: (value: string) => void
}

/** The "used by" button and, on demand, the paths it stands for. */
export function UsedBy({ foundation }: { foundation: Foundation }) {
  const [expanded, setExpanded] = useState(false)
  const listId = useId()
  if (foundation.count === 0) return <span className="hint">{usedByText(foundation)}</span>
  return (
    <>
      <button type="button" className="link" aria-expanded={expanded} aria-controls={listId} onClick={() => setExpanded(!expanded)}>
        {usedByText(foundation)}
      </button>
      {expanded && (
        <ul id={listId} className="targets" aria-label={`Where ${foundation.label} is used`}>
          {foundation.targets.map((t) => (
            <li key={t.path}>
              <code>{t.path}</code>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}

/** The link to a foundation's doc; nothing when it has none. */
export function DocLink({ foundation }: { foundation: Foundation }) {
  if (!foundation.doc) return null
  return (
    <a href={repositoryDocUrl(foundation.doc)} target="_blank" rel="noreferrer">
      About {foundation.label} ↗
    </a>
  )
}

/** A field's error, its findings and its help line with the doc link and, when it has targets, the fan-out. */
export function FieldNotes({ foundation, id, error, own }: { foundation: Foundation; id: string; error?: string; own: readonly Finding[] }) {
  return (
    <>
      {error && (
        <p className="field-error" id={`${id}-error`}>
          {error}
        </p>
      )}
      {own.map((f) => (
        <FindingNote key={findingNoteId(id, f)} finding={f} id={findingNoteId(id, f)} />
      ))}
      <p className="hint" id={`${id}-help`}>
        {foundation.help} <DocLink foundation={foundation} /> {foundation.count > 0 && <UsedBy foundation={foundation} />}
      </p>
    </>
  )
}

/** One answer the user gives: input, error, findings, help, doc link, "ask later" and its fan-out; `reset` drops it in
 *  favour of its derived default. `value`/`onChange` replace the stored answer, for an edit not yet given; `children`
 *  sit next to the input. `binding` stores the answer elsewhere than in the Stage 3 answers. */
export function AnswerField({
  foundation,
  findings,
  reset,
  value,
  onChange,
  children,
  binding,
}: {
  foundation: Foundation
  findings: readonly Finding[]
  reset?: () => void
  value?: string
  onChange?: (value: string) => void
  children?: ReactNode
  binding?: FieldBinding
}) {
  const { state, dispatch } = useStore()
  const field = binding?.field ?? answerFieldId(foundation.id)
  const id = fieldElementId(field)
  const error = binding ? binding.error : answerErrors(state.answers.values, state.foundations, reservedRefs(state.answers.target))[foundation.id]
  const stored = binding ? binding.value : textAnswer(state.answers.values[foundation.id])
  const own = findings.filter((f) => f.field === field)
  return (
    <div className="row">
      <label htmlFor={id}>{foundation.label}</label>
      <input
        id={id}
        value={value ?? stored ?? ''}
        aria-invalid={error !== undefined}
        aria-describedby={describedBy(id, error, own)}
        onChange={(e) => {
          if (onChange) onChange(e.target.value)
          else if (binding) binding.set(e.target.value)
          else dispatch({ type: 'setAnswer', id: foundation.id, value: e.target.value })
        }}
      />
      {children}
      {reset && (
        <button type="button" className="link" onClick={reset}>
          {RESET_LABEL}
        </button>
      )}
      <AskLater field={field} />
      <FieldNotes foundation={foundation} id={id} error={error} own={own} />
    </div>
  )
}

/** A choice as a radio group, each option with its note; the default option is checked while none is given. A given
 *  choice whose default follows other answers can be reset to it; one with a `binding` (its resolved value) cannot. */
export function ChoiceField({ foundation, findings, binding }: { foundation: Foundation; findings: readonly Finding[]; binding?: FieldBinding }) {
  const { state, dispatch } = useStore()
  const field = binding?.field ?? answerFieldId(foundation.id)
  const id = fieldElementId(field)
  const foundations = state.foundations ?? []
  const reserved = reservedRefs(state.answers.target)
  const chosen = binding ? binding.value : resolvedAnswer(foundation.id, state.answers.values, foundations, reserved)
  const choose = (value: string) => (binding ? binding.set(value) : dispatch({ type: 'setAnswer', id: foundation.id, value }))
  const resettable =
    !binding &&
    state.answers.values[foundation.id] !== undefined &&
    derivedDefault(foundation) &&
    defaultApplies(foundation, validAnswers(state.answers.values, foundations, reserved), foundations, reserved)
  const own = findings.filter((f) => f.field === field)
  return (
    // tabIndex lets a Review link focus the group.
    <fieldset className="row" id={id} tabIndex={-1} aria-describedby={describedBy(id, undefined, own)}>
      <legend>{foundation.label}</legend>
      {foundation.options.map((o, i) => (
        <div key={o.value} className="choice">
          <input
            type="radio"
            id={`${id}-option-${i}`}
            name={id}
            value={o.value}
            checked={chosen === o.value}
            aria-describedby={o.note ? `${id}-option-${i}-note` : undefined}
            onChange={() => choose(o.value)}
          />
          <label htmlFor={`${id}-option-${i}`}>{o.label}</label>
          {o.note && (
            <span className="hint" id={`${id}-option-${i}-note`}>
              {o.note}
            </span>
          )}
        </div>
      ))}
      {resettable && (
        <button type="button" className="link" onClick={() => dispatch({ type: 'setAnswer', id: foundation.id, value: '' })}>
          {RESET_LABEL}
        </button>
      )}
      <AskLater field={field} />
      <FieldNotes foundation={foundation} id={id} own={own} />
    </fieldset>
  )
}
