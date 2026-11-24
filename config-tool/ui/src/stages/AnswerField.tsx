/** A foundation's answer field and the notes every Stage 3 field shares: error, findings, help, doc link and fan-out. */
import { useId, useState, type ReactNode } from 'react'
import type { Foundation } from '../api/foundations'
import type { Finding } from '../api/render'
import { AskLater } from '../chrome/AskLater'
import { repositoryDocUrl } from '../links'
import { useStore } from '../state/context'
import { answerErrors, textAnswer } from '../state/foundations'
import { answerFieldId, fieldElementId } from '../state/openQuestions'
import { FindingNote } from './Findings'
import { describedBy, usedByText } from './foundationText'

export const RESET_LABEL = 'Reset to derived'

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
        <FindingNote key={f.rule} finding={f} id={`${id}-${f.rule}`} />
      ))}
      <p className="hint" id={`${id}-help`}>
        {foundation.help} <DocLink foundation={foundation} /> {foundation.count > 0 && <UsedBy foundation={foundation} />}
      </p>
    </>
  )
}

/** One answer the user gives: input, error, findings, help, doc link, "ask later" and its fan-out; `reset` drops it in
 *  favour of its derived default. `value`/`onChange` replace the stored answer, for an edit not yet given; `children`
 *  sit next to the input. */
export function AnswerField({
  foundation,
  findings,
  reset,
  value,
  onChange,
  children,
}: {
  foundation: Foundation
  findings: readonly Finding[]
  reset?: () => void
  value?: string
  onChange?: (value: string) => void
  children?: ReactNode
}) {
  const { state, dispatch } = useStore()
  const field = answerFieldId(foundation.id)
  const id = fieldElementId(field)
  const error = answerErrors(state.answers.values, state.foundations)[foundation.id]
  const own = findings.filter((f) => f.field === field)
  return (
    <div className="row">
      <label htmlFor={id}>{foundation.label}</label>
      <input
        id={id}
        value={value ?? textAnswer(state.answers.values[foundation.id]) ?? ''}
        aria-invalid={error !== undefined}
        aria-describedby={describedBy(id, error, own)}
        onChange={(e) => {
          if (onChange) onChange(e.target.value)
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

