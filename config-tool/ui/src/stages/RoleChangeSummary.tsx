/** What a role change turns on, turns off and keeps, and what a chart switch carries over (ADR 0019). */
import { useId } from 'react'
import type { ChartSwitch, RoleCatalogue, RoleChange } from '../api/roles'
import {
  CARRIED_FIELDS,
  CARRIED_OVER,
  chartMoveSentence,
  DROPPED,
  droppedFeatureChoices,
  droppedVersion,
  KEPT,
  KEPT_REASON,
  NO_FEATURE_CHANGES,
  TURNS_OFF,
  TURNS_ON,
} from './roleChangeText'

/** A headed list, named by its heading; omitted when it has no rows. */
function Section({ heading, rows }: { heading: string; rows: readonly { key: string; text: string }[] }) {
  const id = useId()
  if (rows.length === 0) return null
  return (
    <>
      <h4 id={id}>{heading}</h4>
      <ul className="notes" aria-labelledby={id}>
        {rows.map((row) => (
          <li key={row.key}>{row.text}</li>
        ))}
      </ul>
    </>
  )
}

/** Renders a RoleChange with catalogue titles (falling back to the id); each feature id shows in one section only. */
export function RoleChangeSummary({ change, chartSwitch, catalogue }: { change: RoleChange; chartSwitch: ChartSwitch | null; catalogue?: RoleCatalogue }) {
  const title = (id: string) => catalogue?.features.find((f) => f.id === id)?.title ?? id
  const features = (ids: readonly string[]) => ids.map((id) => ({ key: id, text: title(id) }))
  const kept = change.kept.map((k) => ({ key: k.id, text: `${title(k.id)}: ${k.enabled ? 'on' : 'off'}, ${KEPT_REASON[k.source]}` }))
  const noChange = change.enabled.length === 0 && change.disabled.length === 0 && kept.length === 0
  const listed = new Set([...change.enabled, ...change.disabled, ...change.kept.map((k) => k.id)])
  return (
    <div className="role-change">
      {chartSwitch && <p>{chartMoveSentence(chartSwitch)}</p>}
      {noChange && <p>{NO_FEATURE_CHANGES}</p>}
      <Section heading={TURNS_ON} rows={features(change.enabled)} />
      <Section heading={TURNS_OFF} rows={features(change.disabled)} />
      <Section heading={KEPT} rows={kept} />
      {chartSwitch && (
        <>
          <Section
            heading={CARRIED_OVER}
            rows={[...CARRIED_FIELDS.map((field) => ({ key: field, text: field })), ...chartSwitch.carriedAnswers.map((id) => ({ key: `answer:${id}`, text: id }))]}
          />
          <Section
            heading={DROPPED}
            rows={[
              { key: 'version', text: droppedVersion(chartSwitch) },
              { key: 'choices', text: droppedFeatureChoices(chartSwitch) },
              ...features(chartSwitch.droppedChoices.filter((id) => !listed.has(id))),
              ...chartSwitch.droppedAnswers.map((id) => ({ key: `answer:${id}`, text: id })),
            ]}
          />
        </>
      )}
    </div>
  )
}
