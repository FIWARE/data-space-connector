/** The footer: validation counters and Back/Next through the rail order. */
import { neighbour, type StageDef } from '../stages'
import { countIssues } from '../state/issues'
import { useStore } from '../state/context'

const PREVIOUS = -1
const NEXT = 1

/** Footer of the chrome. */
export function Footer({ stages }: { stages: readonly StageDef[] }) {
  const { state, dispatch } = useStore()
  const { warnings, errors, open } = countIssues(state, stages)
  const back = neighbour(stages, state.stage, PREVIOUS)
  const next = neighbour(stages, state.stage, NEXT)
  return (
    <footer className="bar">
      <span className="counts" aria-label="Validation">
        <span className={warnings > 0 ? 'has-warnings' : undefined}>⚠ {warnings} warnings</span>
        <span className={errors > 0 ? 'has-errors' : undefined}>✖ {errors} errors</span>
        <span>○ {open} open questions</span>
      </span>
      <span className="spacer" />
      <button type="button" className="sec" disabled={!back} onClick={() => back && dispatch({ type: 'goTo', stage: back.id })}>
        Back
      </button>
      <button type="button" className="primary" disabled={!next} onClick={() => next && dispatch({ type: 'goTo', stage: next.id })}>
        {next ? `Next: ${next.label}` : 'Next'}
      </button>
    </footer>
  )
}
