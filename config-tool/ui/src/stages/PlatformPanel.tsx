/** S3's Platform panel: the resource profile and what it sets on each component (ADR 0024). */
import { resourceRows, type Foundation, type ResourceQuantities, type ResourceTable } from '../api/foundations'
import type { Finding } from '../api/render'
import { repositoryDocUrl } from '../links'
import { useStore } from '../state/context'
import { reservedRefs, resolvedAnswer, validAnswers } from '../state/foundations'
import { AnswerField, ChoiceField } from './AnswerField'
import {
  COMPONENT_HEADER,
  CUSTOM_PROFILE,
  LIMITS_HEADER,
  measuredCaption,
  PLATFORM_PANEL,
  PROFILE_DERIVED,
  PROFILE_PENDING,
  REQUESTS_HEADER,
  SIZING_LINK,
} from './platformText'

function quantities(q: ResourceQuantities): string {
  return `${q.cpu} CPU, ${q.memory}`
}

/** What the resolved profile sets: the per-component figures, the `custom` line, or the pending text. */
function Preview({ table }: { table: ResourceTable }) {
  const { state } = useStore()
  const foundations = state.foundations ?? []
  const reserved = reservedRefs(state.answers.target)
  const values = validAnswers(state.answers.values, foundations, reserved)
  const profile = resolvedAnswer(table.answer, values, foundations, reserved)
  if (profile === undefined) return <p className="hint">{PROFILE_PENDING}</p>
  const rows = resourceRows(table, profile)
  const derived = values[table.answer] === undefined
  return (
    <>
      {derived && <p className="hint">{PROFILE_DERIVED}</p>}
      {rows === undefined ? (
        <p className="hint">{CUSTOM_PROFILE}</p>
      ) : (
        <table>
          <caption>
            {measuredCaption(table.measuredOn)}{' '}
            <a href={repositoryDocUrl(table.doc)} target="_blank" rel="noreferrer">
              {SIZING_LINK}
            </a>
          </caption>
          <thead>
            <tr>
              <th scope="col">{COMPONENT_HEADER}</th>
              <th scope="col">{REQUESTS_HEADER}</th>
              <th scope="col">{LIMITS_HEADER}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({ component, spec }) => (
              <tr key={component.id}>
                <th scope="row">{component.label}</th>
                <td>{quantities(spec.requests)}</td>
                <td>{quantities(spec.limits)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}

/** The Platform panel; asks the table's profile choice and previews its figures before they are applied. */
export function PlatformPanel({
  foundations,
  table,
  findings,
}: {
  foundations: readonly Foundation[]
  table: ResourceTable | null
  findings: readonly Finding[]
}) {
  return (
    <div className="panel" role="group" aria-labelledby="foundations-platform">
      <h3 id="foundations-platform">{PLATFORM_PANEL}</h3>
      {foundations.map((f) => (
        <div key={f.id}>
          {f.type === 'choice' ? <ChoiceField foundation={f} findings={findings} /> : <AnswerField foundation={f} findings={findings} />}
          {table?.answer === f.id && <Preview table={table} />}
        </div>
      ))}
    </div>
  )
}
