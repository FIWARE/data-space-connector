/** S7 Review's record of the role changes applied this session, each reopening its summary read-only (ADR 0019). */
import { useId, useState } from 'react'
import type { RoleCatalogue } from '../api/roles'
import { useStore } from '../state/context'
import type { AppliedRoleChange } from '../state/reducer'
import { RoleChangeSummary } from './RoleChangeSummary'
import { ROLE_CHANGES_HEADING, roleChanged, SUMMARY_TOGGLE, summaryToggleLabel } from './roleChangeText'
import { useRoles } from './useRoles'

function Row({ applied, catalogue }: { applied: AppliedRoleChange; catalogue?: RoleCatalogue }) {
  const [expanded, setExpanded] = useState(false)
  const id = useId()
  const { change, chartSwitch, dependencies } = applied
  const title = (role: string) => catalogue?.roles.find((r) => r.id === role)?.title ?? role
  const [from, to] = [title(change.fromRole), title(change.toRole)]
  return (
    <li>
      <span>{roleChanged(from, to)}</span>{' '}
      <button type="button" className="link" aria-label={summaryToggleLabel(from, to)} aria-expanded={expanded} aria-controls={id} onClick={() => setExpanded(!expanded)}>
        {SUMMARY_TOGGLE}
      </button>
      <div id={id} hidden={!expanded}>
        {expanded && <RoleChangeSummary change={change} chartSwitch={chartSwitch} catalogue={catalogue} dependencies={dependencies} />}
      </div>
    </li>
  )
}

// Split from RoleChanges so the catalogue is fetched only when there is a change to name.
function Panel({ changes }: { changes: readonly AppliedRoleChange[] }) {
  const { chart, chartVersion } = useStore().state.answers.target
  // A failed catalogue request is shown by Deliberate choices; here the ids stand in for the titles.
  const { catalogue } = useRoles(chart, chartVersion)
  return (
    <div className="panel">
      <h3>{ROLE_CHANGES_HEADING}</h3>
      <ul className="notes">
        {changes.map((applied, index) => (
          // The record only grows, so the index is a stable key.
          <Row key={index} applied={applied} catalogue={catalogue} />
        ))}
      </ul>
    </div>
  )
}

/** The panel, rendered only once a role change was applied since the configuration was started or opened. */
export function RoleChanges() {
  const { roleChanges } = useStore().state
  return roleChanges.length === 0 ? null : <Panel changes={roleChanges} />
}
