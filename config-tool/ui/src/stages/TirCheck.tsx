/** The TIR reachability outcome as icon and text, and the information line Review shows for it (ADR 0022, Decision 5). */
import { useStore } from '../state/context'
import { tirCheckOf, type TirCheck } from '../state/dataspace'
import { OUTCOME_ICONS, outcomeText, TIR_REVIEW_HEADING, TIR_REVIEW_HINT } from './dataSpaceText'

/** One outcome; the icon is decoration, the text is what assistive technology reads. */
export function TirOutcome({ check }: { check: TirCheck }) {
  return (
    <span className={`reach ${check.outcome}`}>
      <span aria-hidden="true">{OUTCOME_ICONS[check.outcome]}</span> {outcomeText(check)}
    </span>
  )
}

/** Review's line for the current TIR address; rendered only while one is given, never a finding. */
export function TirCheckSummary() {
  const check = tirCheckOf(useStore().state)
  if (!check) return null
  return (
    <div className="panel" role="group" aria-labelledby="review-tir-check">
      <h3 id="review-tir-check">{TIR_REVIEW_HEADING}</h3>
      <p>
        <code>{check.address}</code> <TirOutcome check={check} />
      </p>
      <p className="hint">{TIR_REVIEW_HINT}</p>
    </div>
  )
}
