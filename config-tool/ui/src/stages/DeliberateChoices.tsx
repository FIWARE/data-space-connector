/** S7 Review's list of features whose state differs from the role preset by a user or values-file choice. */
import { useEffect, useRef, useState } from 'react'
import { fetchFeatures, type FeatureChoice } from '../api/features'
import { toProfile } from '../api/profile'
import { useStore } from '../state/context'
import { deliberateLabel } from '../state/deliberate'
import { StageId } from '../state/stageIds'
import { useRoles } from './useRoles'

/** Heading of the section. */
export const DELIBERATE_HEADING = 'Deliberate choices'

/** The section, rendered only with a picked role and at least one deliberate choice; a failed request shows its error. */
export function DeliberateChoices() {
  const { state, dispatch } = useStore()
  const { role: roleId, features, target } = state.answers
  const { catalogue, error: rolesError } = useRoles(target.chart, target.chartVersion)
  const [choices, setChoices] = useState<FeatureChoice[]>()
  const [error, setError] = useState<string>()
  const sha256 = state.loaded?.sha256
  const latest = useRef(state)

  useEffect(() => {
    latest.current = state
  }, [state])

  // Only the role, its features and the loaded file change the choices; the rest of the profile comes from `latest`.
  useEffect(() => {
    if (!roleId) return
    let current = true
    fetchFeatures(toProfile(latest.current), sha256).then(
      (result) => current && setChoices(result.choices),
      (err: Error) => current && setError(err.message),
    )
    return () => {
      current = false
    }
  }, [roleId, features, sha256])

  const failure = error ?? rolesError
  if (!roleId) return null
  if (failure) {
    return (
      <div className="panel">
        <h3>{DELIBERATE_HEADING}</h3>
        <p className="field-error" role="alert">
          {failure}
        </p>
      </div>
    )
  }
  const role = catalogue?.roles.find((r) => r.id === roleId)
  const deliberate = choices?.filter((choice) => choice.deliberate) ?? []
  if (!role || deliberate.length === 0) return null
  const titles = new Map(catalogue?.features.map((f) => [f.id, f.title]))
  return (
    <div className="panel">
      <h3>
        {DELIBERATE_HEADING} ({deliberate.length})
      </h3>
      <ul className="open-questions">
        {deliberate.map((choice) => {
          const title = titles.get(choice.id) ?? choice.id
          return (
            <li key={choice.id}>
              <span>{title}</span>
              <span className="deliberate">{deliberateLabel(choice, role.title)}</span>
              <button type="button" className="link" aria-label={`Go to ${title}`} onClick={() => dispatch({ type: 'goTo', stage: StageId.Features })}>
                go ↗
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
