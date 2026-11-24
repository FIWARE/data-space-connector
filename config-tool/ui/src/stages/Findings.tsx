/** L2 findings: S7 Review's "Findings" section and the note S3 Foundations shows under a field. Neither blocks a step. */
import type { Finding } from '../api/render'
import { repositoryDocUrl } from '../links'
import { useStore } from '../state/context'
import { fieldLabel, isFieldId } from '../state/openQuestions'
import { FINDINGS_HEADING, FINDINGS_HINT, READ_MORE, SEVERITY_LABELS, stageName } from './findingsText'

function ReadMore({ finding }: { finding: Finding }) {
  return (
    <a href={repositoryDocUrl(finding.doc)} target="_blank" rel="noreferrer">
      {READ_MORE}
    </a>
  )
}

/** A finding under the field it points at. */
export function FindingNote({ finding, id }: { finding: Finding; id?: string }) {
  return (
    <p className={`finding ${finding.severity}`} id={id}>
      {SEVERITY_LABELS[finding.severity]}: {finding.message} <ReadMore finding={finding} />
    </p>
  )
}

/** Review's section, rendered only when a rule holds. */
export function Findings({ findings }: { findings: readonly Finding[] }) {
  const { state, dispatch } = useStore()
  if (findings.length === 0) return null
  return (
    <div className="panel">
      <h3>{FINDINGS_HEADING}</h3>
      <p className="hint">{FINDINGS_HINT}</p>
      <ul className="open-questions">
        {findings.map((f) => {
          const field = isFieldId(f.field) ? f.field : undefined
          const label = field ? fieldLabel(field, state.foundations) : f.field
          return (
            <li key={`${f.rule} ${f.field}`} className={`finding ${f.severity}`}>
              <span className="severity">{SEVERITY_LABELS[f.severity]}</span>
              <span className="stage">{stageName(f.stage)}</span>
              <span>{label}</span>
              <span>{f.message}</span>
              <ReadMore finding={f} />
              {field && (
                <button type="button" className="link" aria-label={`Go to ${label}`} onClick={() => dispatch({ type: 'focusField', field })}>
                  go ↗
                </button>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
